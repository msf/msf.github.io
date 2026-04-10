# Benchmark Session Summary — April 2026

## Hardware
Framework 13, Ryzen AI 370HX, Radeon 890M iGPU, 64GB DDR5. Vulkan backend.
llama.cpp b8708, llama-swap v199.

## What we tested
15 local LLMs on two Go coding exams, 3 seeds each (42, 123, 456), temp 1.0, 10 min timeout, 16k context.

### Exam V1: Three simple Go programs (/15)
Factorial, word frequency counter, file tree walker. Scoring: build(1) + runs(1) + correct(3) = 5pts each.

### Exam V2: Resilience modification (/10)
Modify a 208-line Go scraper to add: memory buffer during outages, random eviction when full, background flush goroutine. Scored by Go integration tests (not grep). Mock server with controllable online/offline state.

## Final Results (16k context, 3 seeds)

| Model | v1 mean | v1 range | v2 mean | v2 compiles | tok/s |
|-------|---------|----------|---------|-------------|-------|
| gemma4-26b-q4km | 14.0/15 | 14-14 | 4.0/10 | 2/3 | 18.6 |
| gemma4-26b-mxfp4 | 14.0/15 | 14-14 | 4.0/10 | 2/3 | 18.8 |
| gemma4-26b-q5km | 14.0/15 | 14-14 | 4.0/10 | 2/3 | 17.0 |
| qwen35-35b-q4km | 14.0/15 | 14-14 | 2.3/10 | 1/3 | 22.1 |
| qwen35-35b-q5km | 12.0/15 | 9-14 | 3.3/10 | 2/3 | 21.0 |
| qwen35-35b-q6k | 14.3/15 | 14-15 | 2.7/10 | 1/3 | 22.1 |
| qwen35-35b-mxfp4 | 14.0/15 | 14-14 | 2.0/10 | 1/3 | 21.9 |
| qwen3-coder-30b-draft | 14.0/15 | 14-14 | 3.7/10 | 2/3 | 25.9 |
| gpt-oss-20b | 14.0/15 | 14-14 | 3.7/10 | 2/3 | 27.0 |
| qwen35-9b-q4km | 12.3/15 | 9-14 | 2.3/10 | 1/3 | 13.6 |
| gemma4-e4b-q8 | 12.7/15 | 10-14 | 3.7/10 | 2/3 | 13.8 |

Pruned (consistently bad): deepseek-coder-v2, deepseek-r1-14b, glm-flash, glm-flash-reap, gemma3n-e4b.

## Key findings

### Best model: Gemma 4 26B-A4B
All three quants (Q4_K_M, MXFP4, Q5_K_M) score identically: 14.0/15 on exam_v1 (rock solid, never drops), 4.0/10 on exam_v2, 2/3 compile rate. **No quant matters for Gemma 4** — pick the smallest (MXFP4 at 15 GB) or fastest.

Why Gemma 4 wins over Qwen3.5-35B:
- exam_v1: 14/14/14 vs 14/14/14 (tie on Q4_K_M) but Qwen3.5 other quants are variable (9-15)
- exam_v2: 4.0 mean vs 2.0-3.3 mean, 2/3 compiles vs 1-2/3 compiles
- Gemma 4 is smaller (15-21 GB vs 20-32 GB) and more consistent across seeds

### gpt-oss-20b: best value
14.0/15 (rock solid), 3.7/10 exam_v2, 2/3 compiles, 27 tok/s, **12 GB**. Smallest model that competes with the big MoEs. Dense, no draft model needed.

### Qwen3.5-35B: fast but flaky
Fastest MoE at 21-22 tok/s, exam_v1 is solid at Q4_K_M, but exam_v2 compile rate is poor (1/3 for Q4_K_M, 2/3 for Q5_K_M). Q5_K_M is the best quant if you use Qwen3.5 (28 GB).

### Quantization conclusions
- **Gemma 4 26B**: quant doesn't matter. Q4_K_M = MXFP4 = Q5_K_M on both exams.
- **Qwen3.5-35B**: Q5_K_M is best for hard tasks (2/3 compiles vs 1/3 for others). But 28 GB.
- **MXFP4 is NOT a trap** — our initial n=1 conclusion was wrong. Multi-seed shows it's fine for Gemma 4 and no worse than Q4_K_M for Qwen3.5.

### Methodology lessons learned
- **n=1 results are noise.** Every major conclusion from our initial single-seed runs was wrong or misleading.
- **Grep-based scoring inflates results.** Old eval scored 15-18/20 by keyword matching. Real integration tests dropped scores to 0-8/10.
- **Context truncation was hiding as compile failures.** 8k context wasn't enough for exam_v2. 16k fixed some but not all compile failures.
- **Compile rate > peak score** for practical use.
- **Test the binary, not the source.** Integration tests against the compiled binary with a mock server are the right approach.

## Explorations & infra built during this session

1. **llama-swap config** — all models using `--model` with local HF cache paths (avoids HF rate limits)
2. **exam-driver.go** — generic Go driver for running prompts through llama-swap
3. **exam_v1/eval.sh** — fixed scoring (removed unreachable factorial quality point, graduated wordfreq scoring)
4. **exam_v2/harness/** — Go integration test suite replacing bash grep eval
   - 10 behavioral tests: online flow, buffering, flush, bounded, random eviction, multi-cycle, buf-size edge cases, graceful shutdown, race detector
   - Auto-detects buffer/flush flags from binary's `-h` output
   - Adapts scrape interval (10ms for DurationVar binaries, 1s for IntVar)
5. **exam_v2/scraper.go** — updated interval flag from `IntVar` to `DurationVar` for fast test execution
6. **sweep.sh** — model-first ordering (load once, run all exams × all seeds before swapping)
7. **Qwen3.5 quant sweep** — downloaded MXFP4_MOE, Q5_K_M, Q6_K (all via llama-cli with HF rate limit recovery)
8. **Gemma 4 quant sweep** — downloaded MXFP4_MOE, Q5_K_M, E4B Q8_0

## Directory layout

```
blogpost/
  local-llm-coding-harder-test.md  # Episode 2 blogpost
  sweep.sh                          # Multi-seed sweep runner
  AGENTS.md                         # This file
  exam_v1/
    prompt.txt                      # 3 Go programs prompt
    eval.sh                         # Scoring evaluator (fixed)
    test_input.txt                  # Wordfreq test input
  exam_v2/
    prompt.txt                      # Resilience modification prompt
    eval.sh                         # Compiles + runs Go test harness
    scraper.go                      # Input code (DurationVar interval)
    mock/
      main.go                       # Mock server (inverter + sink + control API)
    harness/
      harness_test.go               # Integration tests (10 tests)
      go.mod
      scraper_original.go.txt       # Original unmodified scraper for reference
  results/
    exam_v1/{model}/seed{42,123,456}/   # 11 models × 3 seeds
    exam_v2/{model}/seed{42,123,456}/   # 11 models × 3 seeds (16k context)
```

## llama-swap config
`~/play/llama/config.yaml` — 11 active models, all using `--model` with local paths, `--ctx-size 16384`, `--reasoning off`.

## Models on disk
HF cache at `/mnt/ai-models/huggingface/hub/`. Active models:
- Qwen3.5-35B-A3B: Q4_K_M, MXFP4_MOE, Q5_K_M, Q6_K
- Qwen3.5-9B: Q4_K_M
- Qwen3-Coder-30B-A3B: Q4_K_M + Qwen3-0.6B draft
- Gemma 4 26B-A4B: UD-Q4_K_M, MXFP4_MOE, UD-Q5_K_M
- Gemma 4 E4B: Q8_0
- gpt-oss-20b: MXFP4

Pruned from benchmarks (still on disk): DeepSeek-Coder-V2-Lite, DeepSeek-R1-14B, GLM-4.7-Flash, GLM-4.7-Flash-REAP, gemma-3n-E4B, Qwen3.5-4B.
