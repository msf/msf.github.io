// agent-driver.go — bounded tool-loop driver for agentic tasks.
//
// Usage:
//
//	go run ./agent-driver.go -task tasks/bug-hunt-01 -endpoint http://127.0.0.1:8090 \
//	    -out ../artifacts/results/agentic -seed 42 <model>
//
// Companion to ../exam-driver.go. Where exam-driver does one-shot generation
// and scores the text, this drives a real tool loop against a frozen repo
// snapshot and scores the *outcome* (verify.sh exit code), not the trajectory.
//
// Tool surface is deliberately four tools and no shell: a bash tool makes the
// trace unbounded and lets a model score by accident.
//
// Every attempt is capped on turns, wall time, and tokens. Exceeding any cap
// is a recorded failure, not a crash.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	fEndpoint = flag.String("endpoint", "http://localhost:8080", "OpenAI-compatible endpoint")
	fTask     = flag.String("task", "", "task directory (required)")
	fOut      = flag.String("out", "results", "output directory root")
	fSeed     = flag.Int("seed", 42, "seed (-1 for nondeterministic)")
	fMaxTurns = flag.Int("max-turns", 20, "hard cap on tool-call rounds")
	// Cap on cumulative *generated* tokens. Not context size: prompt tokens grow
	// monotonically as the transcript replays, so counting them here would fire
	// on a long-but-healthy run.
	fMaxTokens = flag.Int("max-tokens", 32768, "hard cap on cumulative completion tokens")
	// 4096 is too small: write_file must carry a whole file as a JSON string, so
	// a thinking model runs out mid-argument and the call is lost or malformed.
	fPerCall = flag.Int("per-call-tokens", 16384, "max_tokens per completion")
	fTimeout = flag.Duration("timeout", 20*time.Minute, "hard cap on wall time")
	fKeep    = flag.Bool("keep-sandbox", false, "keep the sandbox dir after the run")

	// Samplers. Negative means "omit from the request and use whatever the
	// server was started with". Comparing two endpoints requires either sending
	// identical values to both or sending none to either — the old unconditional
	// temperature=1.0 did neither, silently overriding per-model server config.
	fTemp = flag.Float64("temp", -1, "temperature (negative = omit, use server default)")
	fTopP = flag.Float64("top-p", -1, "top_p (negative = omit)")
	fTopK = flag.Int("top-k", -1, "top_k (negative = omit)")
	fMinP = flag.Float64("min-p", -1, "min_p (negative = omit)")

	// Thinking models return chain-of-thought in message.reasoning_content.
	// Echoing it back keeps llama-server's prefix cache valid across turns
	// (cf. preserve_thinking in config.yaml); dropping it forces a full
	// re-prefill every turn. Off by default: not every endpoint accepts the
	// field on input.
	fReplayReasoning = flag.Bool("replay-reasoning", false, "echo reasoning_content back to the server")

	fMaxRecover = flag.Int("max-tool-parse-recoveries", 3,
		"how many server-side tool-call parse failures to feed back before giving up")
)

// --- wire types (OpenAI chat completions subset) ---

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// ReasoningContent is where llama.cpp puts thinking when reasoning is on.
	// Verified by probe: the field is `reasoning_content`, and Content comes back
	// empty while the whole per-call token allowance is spent here.
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type chatRequest struct {
	Model       string           `json:"model"`
	Messages    []message        `json:"messages"`
	Tools       []map[string]any `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	TopK        *int             `json:"top_k,omitempty"`
	MinP        *float64         `json:"min_p,omitempty"`
	Seed        *int             `json:"seed,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// --- result record ---

type callRecord struct {
	Turn  int    `json:"turn"`
	Name  string `json:"name"`
	Args  string `json:"args"`
	Error string `json:"error,omitempty"`
	Bytes int    `json:"result_bytes"`
}

// harnessConfig is recorded in every result. Without it a result file cannot be
// interpreted later: the first six runs of this harness were invalidated by a
// per-call token cap that left no trace in the output.
type harnessConfig struct {
	Endpoint        string   `json:"endpoint"`
	PerCallTokens   int      `json:"per_call_tokens"`
	MaxTurns        int      `json:"max_turns"`
	MaxTokens       int      `json:"max_completion_tokens"`
	Timeout         string   `json:"timeout"`
	Temperature     *float64 `json:"temperature"`
	TopP            *float64 `json:"top_p"`
	TopK            *int     `json:"top_k"`
	MinP            *float64 `json:"min_p"`
	ReplayReasoning bool     `json:"replay_reasoning"`
	MaxRecoveries   int      `json:"max_tool_parse_recoveries"`
	Tools           []string `json:"tools"`
}

type result struct {
	Task       string `json:"task"`
	Model      string `json:"model"`
	Endpoint   string `json:"endpoint"`
	Seed       int    `json:"seed"`
	Passed     bool   `json:"passed"`
	StopReason string `json:"stop_reason"`
	Turns      int    `json:"turns"`
	ToolCalls  int    `json:"tool_calls"`
	ToolErrors int    `json:"tool_errors"`
	RetryCount int    `json:"retry_count"`
	// TotalTokens accumulates billed tokens across requests. It used to be
	// overwritten each turn, which reported context size and hid the real spend.
	TotalTokens      int           `json:"total_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	ContextTokens    int           `json:"context_tokens_last"`
	ReasoningChars   int           `json:"reasoning_chars"`
	Recoveries       int           `json:"tool_parse_recoveries"`
	WallSeconds      float64       `json:"wall_s"`
	Config           harnessConfig `json:"harness_config"`
	Verify           string        `json:"verify_output"`
	Calls            []callRecord  `json:"calls"`
}

func main() {
	flag.Parse()
	models := flag.Args()
	if *fTask == "" || len(models) != 1 {
		fmt.Fprintln(os.Stderr, "usage: agent-driver -task <dir> [flags] <model>")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if err := run(models[0]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(model string) error {
	taskDir, err := filepath.Abs(*fTask)
	if err != nil {
		return err
	}
	taskName := filepath.Base(taskDir)

	prompt, err := os.ReadFile(filepath.Join(taskDir, "prompt.md"))
	if err != nil {
		return fmt.Errorf("read prompt: %w", err)
	}

	sandbox, err := os.MkdirTemp("", "agentic-"+taskName+"-")
	if err != nil {
		return err
	}
	if !*fKeep {
		defer os.RemoveAll(sandbox)
	}
	if err := copyTree(filepath.Join(taskDir, "repo"), sandbox); err != nil {
		return fmt.Errorf("stage sandbox: %w", err)
	}
	fmt.Fprintf(os.Stderr, "sandbox: %s\n", sandbox)

	outDir := filepath.Join(*fOut, taskName, sanitize(model), fmt.Sprintf("seed%d", *fSeed))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	transcript, err := os.Create(filepath.Join(outDir, "transcript.log"))
	if err != nil {
		return err
	}
	defer transcript.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *fTimeout)
	defer cancel()

	res := loop(ctx, model, string(prompt), sandbox, transcript)
	res.Task, res.Model, res.Endpoint, res.Seed = taskName, model, *fEndpoint, *fSeed
	res.Config = currentConfig()

	// Outcome is the verifier's exit code, regardless of how the loop ended.
	verifyOut, passed := verify(taskDir, sandbox)
	res.Passed = passed
	res.Verify = verifyOut

	writeJSON(filepath.Join(outDir, "result.json"), res)
	writeJSON(filepath.Join(outDir, "tool_calls.json"), res.Calls)
	os.WriteFile(filepath.Join(outDir, "verify_output"), []byte(verifyOut), 0o644)

	fmt.Printf("%s\t%s\tseed%d\tpassed=%v\tstop=%s\tturns=%d\ttool_errors=%d\tretries=%d\trecov=%d\tcompletion_tok=%d\tctx_tok=%d\treasoning_chars=%d\twall=%.1fs\n",
		taskName, model, *fSeed, res.Passed, res.StopReason, res.Turns, res.ToolErrors, res.RetryCount,
		res.Recoveries, res.CompletionTokens, res.ContextTokens, res.ReasoningChars, res.WallSeconds)
	fmt.Fprintf(os.Stderr, "results: %s\n", outDir)
	return nil
}

func loop(ctx context.Context, model, prompt, sandbox string, log io.Writer) result {
	start := time.Now()
	res := result{StopReason: "model_stopped"}
	seen := map[string]bool{} // failed (name+args) signatures, for retry_count

	msgs := []message{{Role: "user", Content: prompt}}
	fmt.Fprintf(log, "=== USER ===\n%s\n", prompt)

	client := &http.Client{Timeout: *fTimeout}

	for res.Turns = 0; res.Turns < *fMaxTurns; res.Turns++ {
		if ctx.Err() != nil {
			res.StopReason = "timeout"
			break
		}
		if res.CompletionTokens > *fMaxTokens {
			res.StopReason = "token_budget"
			break
		}

		resp, err := complete(ctx, client, model, msgs)
		if err != nil {
			// A server-side tool-call parse failure is the model emitting bad
			// JSON, not the harness breaking. Feed it back and let the model
			// retry, the same as a local dispatch error.
			if isToolParseError(err) && res.Recoveries < *fMaxRecover {
				res.Recoveries++
				res.ToolErrors++
				fmt.Fprintf(log, "\n=== TOOL-CALL PARSE FAILURE (recovery %d/%d) ===\n%v\n",
					res.Recoveries, *fMaxRecover, err)
				msgs = append(msgs, message{Role: "user", Content: toolParseFeedback})
				continue
			}
			res.StopReason = "api_error: " + err.Error()
			fmt.Fprintf(log, "\n=== API ERROR ===\n%v\n", err)
			break
		}
		res.TotalTokens += resp.Usage.TotalTokens
		res.CompletionTokens += resp.Usage.CompletionTokens
		res.ContextTokens = resp.Usage.PromptTokens
		if len(resp.Choices) == 0 {
			res.StopReason = "empty_response"
			break
		}
		finish := resp.Choices[0].FinishReason
		am := resp.Choices[0].Message
		am.Role = "assistant"

		res.ReasoningChars += len(am.ReasoningContent)
		fmt.Fprintf(log, "\n=== ASSISTANT (turn %d, finish=%s) ===\n", res.Turns, finish)
		if am.ReasoningContent != "" {
			fmt.Fprintf(log, "[reasoning_content, %d chars]\n%s\n",
				len(am.ReasoningContent), truncate(am.ReasoningContent, 4000))
		}
		fmt.Fprintf(log, "%s\n", am.Content)

		if !*fReplayReasoning {
			am.ReasoningContent = ""
		}
		msgs = append(msgs, am)

		if len(am.ToolCalls) == 0 {
			// finish=length with no tool call means the completion was cut off
			// mid-emission: a harness cap, not the model deciding it was done.
			if finish == "length" {
				res.StopReason = "output_truncated"
			} else {
				res.StopReason = "model_stopped"
			}
			break
		}

		for _, tc := range am.ToolCalls {
			res.ToolCalls++
			sig := tc.Function.Name + "\x00" + tc.Function.Arguments
			out, terr := dispatch(sandbox, tc.Function.Name, tc.Function.Arguments)

			rec := callRecord{Turn: res.Turns, Name: tc.Function.Name, Args: truncate(tc.Function.Arguments, 500), Bytes: len(out)}
			if terr != nil {
				res.ToolErrors++
				rec.Error = terr.Error()
				out = "ERROR: " + terr.Error()
				if seen[sig] {
					res.RetryCount++
				}
				seen[sig] = true
			}
			res.Calls = append(res.Calls, rec)

			fmt.Fprintf(log, "\n--- TOOL %s(%s) ---\n%s\n",
				tc.Function.Name, truncate(tc.Function.Arguments, 300), truncate(out, 4000))

			msgs = append(msgs, message{Role: "tool", ToolCallID: tc.ID, Content: out})
		}
	}
	if res.Turns >= *fMaxTurns {
		res.StopReason = "turn_cap"
	}
	res.WallSeconds = time.Since(start).Seconds()
	return res
}

func complete(ctx context.Context, c *http.Client, model string, msgs []message) (*chatResponse, error) {
	req := chatRequest{
		Model:      model,
		Messages:   msgs,
		Tools:      toolSchema(),
		ToolChoice: "auto",
		MaxTokens:  *fPerCall,

		// Any sampler left negative is omitted so the server's own configuration
		// stands. Sending a value to one endpoint but not the other is what makes
		// two cells incomparable.
		Temperature: optFloat(*fTemp),
		TopP:        optFloat(*fTopP),
		TopK:        optInt(*fTopK),
		MinP:        optFloat(*fMinP),
	}
	if *fSeed >= 0 {
		s := *fSeed
		req.Seed = &s
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(*fEndpoint, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}
	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode: %w (%s)", err, truncate(string(raw), 400))
	}
	if out.Error != nil {
		return nil, fmt.Errorf("server: %s", out.Error.Message)
	}
	return &out, nil
}

// --- tools ---

func toolSchema() []map[string]any {
	fn := func(name, desc string, props map[string]any, required []string) map[string]any {
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": desc,
				"parameters": map[string]any{
					"type":       "object",
					"properties": props,
					"required":   required,
				},
			},
		}
	}
	str := func(d string) map[string]any { return map[string]any{"type": "string", "description": d} }

	return []map[string]any{
		fn("list_files", "List files in a directory of the repository, relative to the repo root.",
			map[string]any{"dir": str("Directory relative to repo root. Use \".\" for the root.")}, []string{"dir"}),
		fn("read_file", "Read the full contents of a file in the repository.",
			map[string]any{"path": str("File path relative to repo root.")}, []string{"path"}),
		fn("write_file", "Overwrite a file in the repository with new contents. Always write the complete file.",
			map[string]any{
				"path":    str("File path relative to repo root."),
				"content": str("Complete new contents of the file."),
			}, []string{"path", "content"}),
		fn("run_tests", "Run the repository's Go test suite with the race detector and return the output.",
			map[string]any{}, []string{}),
	}
}

func dispatch(sandbox, name, args string) (string, error) {
	var a map[string]any
	if args == "" {
		args = "{}"
	}
	if err := json.Unmarshal([]byte(args), &a); err != nil {
		return "", fmt.Errorf("arguments are not valid JSON: %v", err)
	}
	argStr := func(k string) (string, error) {
		v, ok := a[k]
		if !ok {
			return "", fmt.Errorf("missing required argument %q", k)
		}
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("argument %q must be a string, got %T", k, v)
		}
		return s, nil
	}

	switch name {
	case "list_files":
		dir, err := argStr("dir")
		if err != nil {
			return "", err
		}
		return listFiles(sandbox, dir)
	case "read_file":
		p, err := argStr("path")
		if err != nil {
			return "", err
		}
		return readFile(sandbox, p)
	case "write_file":
		p, err := argStr("path")
		if err != nil {
			return "", err
		}
		content, err := argStr("content")
		if err != nil {
			return "", err
		}
		return writeFile(sandbox, p, content)
	case "run_tests":
		return runTests(sandbox), nil
	default:
		return "", fmt.Errorf("unknown tool %q; available tools are list_files, read_file, write_file, run_tests", name)
	}
}

// resolve contains a model-supplied path inside the sandbox.
func resolve(sandbox, rel string) (string, error) {
	if filepath.IsAbs(rel) {
		rel = strings.TrimPrefix(rel, "/")
	}
	abs := filepath.Join(sandbox, filepath.Clean("/"+rel))
	if abs != sandbox && !strings.HasPrefix(abs, sandbox+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the repository", rel)
	}
	return abs, nil
}

func listFiles(sandbox, dir string) (string, error) {
	abs, err := resolve(sandbox, dir)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return "", fmt.Errorf("cannot list %q: %v", dir, err)
	}
	var lines []string
	for _, e := range entries {
		if e.IsDir() {
			lines = append(lines, e.Name()+"/")
			continue
		}
		info, err := e.Info()
		if err != nil {
			lines = append(lines, e.Name())
			continue
		}
		lines = append(lines, fmt.Sprintf("%s (%d bytes)", e.Name(), info.Size()))
	}
	sort.Strings(lines)
	if len(lines) == 0 {
		return "(empty directory)", nil
	}
	return strings.Join(lines, "\n"), nil
}

func readFile(sandbox, path string) (string, error) {
	abs, err := resolve(sandbox, path)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("cannot read %q: %v", path, err)
	}
	return string(b), nil
}

func writeFile(sandbox, path, content string) (string, error) {
	abs, err := resolve(sandbox, path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("cannot write %q: %v", path, err)
	}
	return fmt.Sprintf("wrote %s (%d bytes)", path, len(content)), nil
}

func runTests(sandbox string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "-race", "-timeout", "120s", "./...")
	cmd.Dir = sandbox
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOWORK=off")
	out, err := cmd.CombinedOutput()
	status := "PASS (all tests passed)"
	if err != nil {
		status = "FAIL (see output above)"
	}
	return truncate(string(out), 12000) + "\n--- result: " + status + " ---"
}

// --- verification ---

func verify(taskDir, sandbox string) (string, bool) {
	script := filepath.Join(taskDir, "verify.sh")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script, sandbox)
	cmd.Dir = taskDir
	out, err := cmd.CombinedOutput()
	return string(out), err == nil
}

// --- helpers ---

// toolParseFeedback is what the model sees after the server rejected its
// tool-call arguments. Deliberately generic: naming the offending tool would
// hand the model a hint the harness shouldn't give.
const toolParseFeedback = "Your last tool call could not be parsed: the arguments were not valid JSON " +
	"(most likely truncated mid-string). Re-issue the call. If you are writing a file, " +
	"send the complete file contents as a single properly escaped JSON string."

func isToolParseError(err error) bool {
	return strings.Contains(err.Error(), "parse tool call")
}

func optFloat(v float64) *float64 {
	if v < 0 {
		return nil
	}
	return &v
}

func optInt(v int) *int {
	if v < 0 {
		return nil
	}
	return &v
}

func currentConfig() harnessConfig {
	return harnessConfig{
		Endpoint:        *fEndpoint,
		PerCallTokens:   *fPerCall,
		MaxTurns:        *fMaxTurns,
		MaxTokens:       *fMaxTokens,
		Timeout:         fTimeout.String(),
		Temperature:     optFloat(*fTemp),
		TopP:            optFloat(*fTopP),
		TopK:            optInt(*fTopK),
		MinP:            optFloat(*fMinP),
		ReplayReasoning: *fReplayReasoning,
		MaxRecoveries:   *fMaxRecover,
		Tools:           []string{"list_files", "read_file", "write_file", "run_tests"},
	}
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal %s: %v\n", path, err)
		return
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", path, err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n... [truncated, %d bytes total]", len(s))
}

func sanitize(s string) string {
	return strings.NewReplacer("/", "_", ":", "_", " ", "_").Replace(s)
}
