# Gemma 4 vs Qwen3.5: benchmarking quantized local LLMs on Go coding

*April 2026*[^1]

*Part 3/3 — **Part 3** ← [Part 2](local-llm-performance-framework13.md) ← [Part 1](benchmarking-local-llms-go-coding.md)*

[^1]: Co-authored with Claude Opus 4.6.

In [episode 1](benchmarking-local-llms-go-coding.md) three models tied at 13/15. The test was too easy — it couldn't separate a good model from a mediocre one having a lucky run. Since then Qwen3.5, Gemma 4, and Qwen3-Coder dropped. They'd all tie too. We needed a harder exam and better methodology. We also suspected (correctly) that single-seed results were noise and that our grep-based scoring was garbage, so we planned for multi-seed and real test execution from the start.

Same hardware: Framework 13, Ryzen AI 370HX, Radeon 890M iGPU, 64 GB DDR5, Vulkan. llama.cpp b8708, models served through [llama-swap](https://github.com/mostlygeek/llama-swap). All runs: 3 seeds (42, 123, 456), temp 1.0, 16k context, 10 min timeout. `--reasoning off` for Qwen3.5 (without it, the model burns its entire context on chain-of-thought before writing any code). Started with 15 models, pruned to 11 after the first round — DeepSeek-Coder-V2-Lite, DeepSeek-R1-14B, GLM-4.7-Flash, GLM-4.7-Flash-REAP, and gemma-3n-E4B scored consistently badly.

## The two exams

**Exam v1** (/15): Factorial, word frequency counter, file tree walker. 5 points each: builds(1) + runs(1) + correct output(3). Easy. A reliability gate — most decent models hit 14/15 every seed.

**Exam v2** (/10): Modify a 208-line Go metrics scraper to add resilience — buffer metrics in memory during network outages, randomly evict when the buffer is full, flush via a background goroutine on reconnect. This is the one that separates models.

## How the eval works

The first exam_v2 evaluator used grep. It checked whether the code contained `sync.Mutex`, `rand.Intn`, `go func`. Models scored 15–18/20. That was broken — a model can write `sync.Mutex` and still have three data races.

We replaced it with Go integration tests. The [harness](exam_v2/harness/harness_test.go) compiles the model's code into a binary, runs it against a [mock server](exam_v2/mock/main.go) with controllable online/offline state, and executes 10 tests:

| Test | What it checks |
|------|----------------|
| OnlineFlow | Metrics reach the sink when online |
| BuffersDuringOutage | Nothing leaks when offline |
| FlushOnReconnect | Buffered metrics flush on reconnect |
| BufferBounded | Flushed count stays within buffer-size |
| EvictionRandom | Evicted items are a mix of old and new |
| MultipleOutageCycles | Survives 3 offline/online transitions |
| BufferSizeZero | Doesn't panic with `-buffer-size 0` |
| BufferSizeOne | Works with `-buffer-size 1` |
| GracefulShutdown | Exits cleanly on SIGINT |
| RaceDetector | Compiled with `-race`, no data races |

The harness auto-detects flag names from the binary's `-h` output, so it adapts to whatever CLI the model invents. The same models that scored 15–18/20 with grep scored 0–8/10 under real execution.

## Results

### Exam v1: Three Go programs (/15)

| Model | Mean | Range | Tok/s |
|-------|------|-------|-------|
| Qwen3.5-35B Q6_K | 14.3 | 14–15 | 22.1 |
| **Gemma 4 26B Q4_K_M** | **14.0** | **14–14** | **18.6** |
| **Gemma 4 26B MXFP4** | **14.0** | **14–14** | **18.8** |
| **Gemma 4 26B Q5_K_M** | **14.0** | **14–14** | **17.0** |
| **Qwen3.5-35B Q4_K_M** | **14.0** | **14–14** | **22.1** |
| **Qwen3.5-35B MXFP4** | **14.0** | **14–14** | **21.9** |
| **Qwen3-Coder-30B + draft** | **14.0** | **14–14** | **25.9** |
| **gpt-oss-20b** | **14.0** | **14–14** | **27.0** |
| Gemma 4 E4B Q8 | 12.7 | 10–14 | 13.8 |
| Qwen3.5-9B | 12.3 | 9–14 | 13.6 |
| Qwen3.5-35B Q5_K_M | 12.0 | 9–14 | 21.0 |

Bold = 14/15 on all three seeds. The 14–14 vs 9–14 spread is what matters here — it tells you which models are dependable vs. which ones roll dice.

**Previously tested (dropped after first round):**

| Model | Mean | Range | Notes |
|-------|------|-------|-------|
| DeepSeek-Coder-V2-Lite 16B Q8 | 10.3 | 9–13 | Legacy coder model, outclassed |
| GLM-4.7-Flash 30B Q4_K_M | 10.0 | 9–12 | Dense 30B, too slow for the score |
| gemma-3n-E4B Q8 | 8.0 | 5–10 | Previous gen Gemma, inconsistent |
| GLM-4.7-Flash-REAP 23B Q4_K_M | 7.0 | 4–10 | MoE variant, worse than dense |
| DeepSeek-R1-14B Q4_K_M | 5.0 | 5–5 | Reasoning model with `--reasoning off` — barely functional |

These models consistently scored below the new contenders and were dropped from exam v2 and the quantization sweep. Qwen3-8B (the episode 1 champion at 4.7 GB) was superseded by Qwen3.5-9B — same family, newer weights.

### Exam v2: Resilience modification (/10)

| Model | Mean | Compiles | Tok/s |
|-------|------|----------|-------|
| Gemma 4 26B Q4_K_M | 4.0 | 2/3 | 18.6 |
| Gemma 4 26B MXFP4 | 4.0 | 2/3 | 18.8 |
| Gemma 4 26B Q5_K_M | 4.0 | 2/3 | 17.0 |
| Qwen3-Coder-30B + draft | 3.7 | 2/3 | 25.9 |
| gpt-oss-20b | 3.7 | 2/3 | 27.0 |
| Gemma 4 E4B Q8 | 3.7 | 2/3 | 13.8 |
| Qwen3.5-35B Q5_K_M | 3.3 | 2/3 | 21.0 |
| Qwen3.5-35B Q6_K | 2.7 | 1/3 | 22.1 |
| Qwen3.5-35B Q4_K_M | 2.3 | 1/3 | 22.1 |
| Qwen3.5-9B | 2.3 | 1/3 | 13.6 |
| Qwen3.5-35B MXFP4 | 2.0 | 1/3 | 21.9 |

"Compiles" = produced code that builds and passes at least one test. Every model fails at least one seed. When they do compile and pass, they typically get 5–7/10. Common failures: `rand.Intn(0)` panic with buffer-size 0, off-by-one in flush logic, concurrent buffer access without locking. 4/10 is the ceiling so far.

## Quantization

**Gemma 4 26B: quant doesn't matter.**

| Quant | Size | Exam v1 | Exam v2 | Compiles |
|-------|------|---------|---------|----------|
| UD-Q4_K_M | 16 GB | 14.0 (14–14) | 4.0 | 2/3 |
| MXFP4_MOE | 15 GB | 14.0 (14–14) | 4.0 | 2/3 |
| UD-Q5_K_M | 21 GB | 14.0 (14–14) | 4.0 | 2/3 |

Identical across all three. Pick the smallest: MXFP4 at 15 GB.

**Qwen3.5-35B: Q5_K_M is the most reliable.**

| Quant | Size | Exam v1 | Exam v2 | Compiles |
|-------|------|---------|---------|----------|
| Q4_K_M | 21 GB | 14.0 (14–14) | 2.3 | 1/3 |
| **Q5_K_M** | **28 GB** | **12.0 (9–14)** | **3.3** | **2/3** |
| Q6_K | 32 GB | 14.3 (14–15) | 2.7 | 1/3 |
| MXFP4_MOE | 20 GB | 14.0 (14–14) | 2.0 | 1/3 |

Q5_K_M has the best compile rate on the hard exam (2/3 vs 1/3) but wobbles on the easy one (9–14). More bits doesn't monotonically help. At 28 GB it still can't match Gemma 4's consistency.

**We don't fully understand why Qwen3.5 is so flaky under quantization.** On paper it should be the stronger model — it leads on Terminal-Bench 2, SWE-bench, and TAU2 at full precision. But quantized and running locally, it compiles less often than Gemma 4 across every quant we tried. Our best guess: Qwen3.5's hybrid architecture (Gated DeltaNet + MoE with 256 tiny experts) may be more sensitive to weight precision loss than Gemma 4's 128-expert MoE. But we haven't proven that — it's speculation. If you know more about this, we'd like to hear it.

## What we got wrong and fixed

**Context truncation.** With 8k context, models were writing correct code that got cut off mid-function. Looked like model quality problems. It was infrastructure. Bumped to 16k, half the compile failures disappeared.

**Grep scoring.** The original exam_v2 eval checked for keywords in the source code. Models scored 15–18/20. Under real execution with integration tests: 0–8/10. We wasted a round of runs on this before rebuilding the eval as a proper Go test harness.

**Single-seed noise.** Every major conclusion from our initial n=1 runs was wrong or misleading. One model looked terrible on seed 42, great on seed 123. Three seeds is the minimum — we should probably use five, but GPU hours aren't free on an iGPU.

## External context

No published benchmarks test quantized models at this size class. Every leaderboard number is full-precision on server GPUs. For reference, self-reported full-precision scores from the model authors:

| Benchmark | Qwen3.5-35B | Gemma 4 26B |
|-----------|-------------|-------------|
| Terminal-Bench 2 | 40.5% | — |
| SWE-bench Verified | 69.2% | — |
| TAU2 | 81.2 | 68.2 |
| LiveCodeBench | — | 77.1% |
| Arena AI (Elo) | 1400 | 1441 |

Qwen3.5 looks stronger on paper (TAU2, SWE-bench). Gemma 4 edges it on Arena AI. Our local quantized results tell a different story: Gemma 4 is more consistent where it counts — actually compiling and running under constrained context on real hardware.

## Conclusions

**Gemma 4 26B-A4B MXFP4 is the best local coding model on this hardware.** 15 GB, 14/15 rock solid, 4.0/10 on the hard exam, quant-insensitive. 18 tok/s on an iGPU.

**gpt-oss-20b is the best value.** 12 GB, 27 tok/s, 14/15 rock solid, 3.7/10 on exam v2. Smallest and fastest model that competes with the bigger MoEs. Dense, no draft model needed.

**Qwen3.5-35B is fast but flaky.** 21–22 tok/s but lower compile rates on the hard exam. Q5_K_M (28 GB) is the best quant if you need it.

**Compile rate matters more than peak score.** A model that compiles 2/3 seeds and scores 4/10 is more useful than one that compiles 1/3 and scores 8 on a lucky run.

**Multi-seed and real execution are non-negotiable.** Grep scoring inflated results. Single-seed results were noise. Every major conclusion from the initial n=1 grep-based run was wrong.

## Reproduce it

```bash
./sweep.sh
```

All code: [exam_v1/](exam_v1/), [exam_v2/](exam_v2/), [sweep.sh](sweep.sh). Results are local-only (regenerate with `./sweep.sh`).
