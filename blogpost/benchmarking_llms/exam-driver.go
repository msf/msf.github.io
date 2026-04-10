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
)

// API types — minimal, only what we need.
type (
	apiReq struct {
		Model       string   `json:"model"`
		Messages    []apiMsg `json:"messages"`
		MaxTokens   int      `json:"max_tokens"`
		Temperature float64  `json:"temperature"`
		Seed        int      `json:"seed,omitempty"`
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

	// Save combined result
	j, _ := json.MarshalIndent(map[string]any{
		"model": r.Model, "tps": r.TPS, "tokens": r.Tokens,
		"wall_s": r.Wall.Seconds(), "eval": r.Eval,
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
	body, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(context.Background(), *fTimeout)
	defer cancel()
	httpReq, _ := http.NewRequestWithContext(ctx, "POST",
		*fEndpoint+"/v1/chat/completions", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
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

func unload() {
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
