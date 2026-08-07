// exam-driver: generic LLM exam runner for llama-swap.
//
// For each model: sends a prompt, saves the response, runs an evaluator command,
// and collects the score. The evaluator is exam-specific and follows a simple protocol:
//
//	eval.sh <response-file> <work-dir>
//	stdout: {"score":N, "max":M, "summary":"..."}
//
// Usage:
//
//	go run exam-driver.go -prompt exam/prompt.txt -eval exam/eval.sh model1 model2
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var (
	fEndpoint  = flag.String("endpoint", "http://localhost:8080", "llama-swap URL")
	fPrompt    = flag.String("prompt", "", "prompt file (required)")
	fEval      = flag.String("eval", "", "evaluator command (required)")
	fOut       = flag.String("out", "results", "output directory")
	fMaxTokens = flag.Int("max-tokens", 16384, "max tokens")
	fTemp      = flag.Float64("temp", 1.0, "temperature")
	fSeed      = flag.Int("seed", 42, "seed (-1 for nondeterministic)")
	fTimeout   = flag.Duration("timeout", 10*time.Minute, "generation timeout")
	// Hosted OpenAI-compatible endpoints (api.anthropic.com, api.openai.com)
	// need a bearer token; llama-swap does not. Empty = no auth header.
	fAPIKey = flag.String("api-key", os.Getenv("EXAM_API_KEY"), "bearer token (default $EXAM_API_KEY)")
	// Samplers the driver did not previously send, so they came from whatever
	// each llama-server was launched with. On hopper the Qwen MTP entries set
	// top_p/top_k/min_p and the Gemma entries do not (llama.cpp defaults:
	// top_k 40, min_p 0.05), which makes a cross-model ranking partly a sampler
	// comparison — the same defect that made the 2026-08-06 A-vs-B gap
	// unattributable. Send them explicitly so parity is a recorded run
	// parameter, not an implicit property of config.yaml.
	// Negative = omit from the request and let the server default apply, which
	// keeps hosted cells (Anthropic/OpenAI reject top_k/min_p) working.
	fTopP = flag.Float64("top-p", -1, "top_p (<0 = omit, use server default)")
	fTopK = flag.Int("top-k", -1, "top_k (<0 = omit, use server default)")
	fMinP = flag.Float64("min-p", -1, "min_p (<0 = omit, use server default)")
)

// API types — minimal, only what we need.
type (
	apiReq struct {
		Model       string   `json:"model"`
		Messages    []apiMsg `json:"messages"`
		MaxTokens   int      `json:"max_tokens"`
		Temperature float64  `json:"temperature"`
		Seed        int      `json:"seed,omitempty"`
		TopP        *float64 `json:"top_p,omitempty"`
		TopK        *int     `json:"top_k,omitempty"`
		MinP        *float64 `json:"min_p,omitempty"`
	}
	apiMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	apiResp struct {
		Choices []struct{ Message apiMsg } `json:"choices"`
		Usage   struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
		Timings struct {
			PredictedPerSec float64 `json:"predicted_per_second"`
		} `json:"timings"`
		Error *struct{ Message string } `json:"error,omitempty"`
	}
)

type evalScore struct {
	Score   int    `json:"score"`
	Max     int    `json:"max"`
	Summary string `json:"summary"`
}

type result struct {
	Model  string
	TPS    float64
	Tokens int
	Wall   time.Duration
	Eval   evalScore
}

func main() {
	log.SetFlags(0)
	flag.Parse()
	models := flag.Args()
	if *fPrompt == "" || *fEval == "" || len(models) == 0 {
		fmt.Fprintln(os.Stderr, "usage: exam-driver -prompt FILE -eval CMD model1 [model2 ...]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	prompt, err := os.ReadFile(*fPrompt)
	if err != nil {
		log.Fatalf("read prompt: %v", err)
	}
	os.MkdirAll(*fOut, 0o755)

	var results []result
	for i, model := range models {
		if i > 0 {
			unload()
			time.Sleep(2 * time.Second)
		}
		results = append(results, runModel(model, string(prompt)))
	}

	// Summary
	fmt.Printf("\n%-25s %7s %7s %s\n", "Model", "Tok/s", "Wall", "Score")
	fmt.Printf("%-25s %7s %7s %s\n", "-----", "-----", "----", "-----")
	for _, r := range results {
		fmt.Printf("%-25s %7.1f %7s %d/%d  %s\n",
			r.Model, r.TPS, fmtDur(r.Wall), r.Eval.Score, r.Eval.Max, r.Eval.Summary)
	}
}

func runModel(model, prompt string) result {
	dir := filepath.Join(*fOut, model)
	os.MkdirAll(dir, 0o755)
	r := result{Model: model}

	// 1. Generate
	fmt.Printf("--- %s ---\n", model)
	fmt.Print("  generating... ")
	t0 := time.Now()
	resp, err := generate(model, prompt)
	r.Wall = time.Since(t0)
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return r
	}
	content := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
	}
	r.TPS = resp.Timings.PredictedPerSec
	r.Tokens = resp.Usage.CompletionTokens
	os.WriteFile(filepath.Join(dir, "response.txt"), []byte(content), 0o644)
	fmt.Printf("%d tok, %.1f tok/s, %s\n", r.Tokens, r.TPS, fmtDur(r.Wall))

	// 2. Evaluate
	fmt.Print("  evaluating... ")
	cmd := exec.Command(*fEval, filepath.Join(dir, "response.txt"), dir)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return r
	}
	if err := json.Unmarshal(out, &r.Eval); err != nil {
		fmt.Printf("bad eval output: %v\n%s\n", err, out)
		return r
	}
	fmt.Printf("%d/%d\n", r.Eval.Score, r.Eval.Max)

	// Save combined result. `sampler` records what the client actually sent, so
	// a cross-model table can be audited for parity after the fact; a null means
	// the request omitted it and the server's launch flag applied.
	sampler := map[string]any{"temperature": *fTemp, "max_tokens": *fMaxTokens,
		"top_p": nil, "top_k": nil, "min_p": nil}
	if *fTopP >= 0 {
		sampler["top_p"] = *fTopP
	}
	if *fTopK >= 0 {
		sampler["top_k"] = *fTopK
	}
	if *fMinP >= 0 {
		sampler["min_p"] = *fMinP
	}
	j, _ := json.MarshalIndent(map[string]any{
		"model": r.Model, "tps": r.TPS, "tokens": r.Tokens,
		"wall_s": r.Wall.Seconds(), "eval": r.Eval, "sampler": sampler,
	}, "", "  ")
	os.WriteFile(filepath.Join(dir, "result.json"), j, 0o644)
	return r
}

func generate(model, prompt string) (*apiResp, error) {
	req := apiReq{
		Model:       model,
		Messages:    []apiMsg{{Role: "user", Content: prompt}},
		MaxTokens:   *fMaxTokens,
		Temperature: *fTemp,
	}
	if *fSeed >= 0 {
		req.Seed = *fSeed
	}
	if *fTopP >= 0 {
		req.TopP = fTopP
	}
	if *fTopK >= 0 {
		req.TopK = fTopK
	}
	if *fMinP >= 0 {
		req.MinP = fMinP
	}
	body, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(context.Background(), *fTimeout)
	defer cancel()
	httpReq, _ := http.NewRequestWithContext(ctx, "POST",
		*fEndpoint+"/v1/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	if *fAPIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+*fAPIKey)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	var cr apiResp
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, err
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("api: %s", cr.Error.Message)
	}
	return &cr, nil
}

// unload evicts the current model so the next one starts from a known state.
// llama-swap's admin route has moved across versions: v211 (hopper) serves
// GET /unload and 404s both POST paths; later builds accept the POST forms.
// Try each in turn so one driver works against both.
func unload() {
	r, err := http.Get(*fEndpoint + "/unload")
	if err == nil {
		r.Body.Close()
		if r.StatusCode >= 200 && r.StatusCode < 300 {
			return
		}
	}
	for _, path := range []string{"/api/models/unload", "/models/unload"} {
		r, err := http.Post(*fEndpoint+path, "", nil)
		if err != nil {
			continue
		}
		r.Body.Close()
		if r.StatusCode >= 200 && r.StatusCode < 300 {
			return
		}
	}
}

func fmtDur(d time.Duration) string {
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
