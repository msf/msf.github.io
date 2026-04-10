# Local LLMs got better. So I made a harder test.

*April 2026 -- co-authored with Claude Opus 4.6 via [opencode](https://opencode.ai)*

*Previous: [I benchmarked 8 local LLMs writing Go on my Framework 13](benchmarking-local-llms-go-coding.md)*

Two months ago I tested 8 local models writing simple Go programs on my Framework 13 laptop. Three models tied at 13/15 and I couldn't tell them apart. The test was too easy.

Since then, Qwen3.5, Gemma 4, and Qwen3-Coder dropped. The old test would probably give them all 13/15 and teach me nothing. So I built a harder test, fixed the methodology that was lying to me, and ran 15 models through both exams with 3 seeds each.

The methodology journey ended up being more interesting than the results.

## The setup

Same Framework 13: Ryzen AI 370HX, Radeon 890M iGPU, 64GB DDR5, Vulkan backend (still no ROCm for this GPU). llama.cpp b8708. All models served through [llama-swap](https://github.com/mostlygeek/llama-swap) for hot-swapping by API.

One runtime change mattered a lot: `--reasoning off` suppresses thinking tokens in Qwen3.5/DeepSeek-R1 models. This alone took Qwen3.5-35B from 4/15 to 14/15 on the easy exam — without it, the model wastes its entire context budget on chain-of-thought before writing any code.

## How my first attempt went wrong

I started the way most people benchmark: one seed, grep-based scoring, call it a day.

The first exam_v2 evaluator checked whether the model's code contained keywords like `sync.Mutex`, `rand.Intn`, `go func`. Models scored 15-18/20. Impressive! Except none of it meant anything. A model can write `sync.Mutex` and still have three data races. It can write `rand.Intn` inside a function that never gets called.

The first exam_v1 run (seed 42 only) ranked Qwen3.5-35B Q5_K_M at 8/15 — looked broken. The MXFP4 quant at 14/15 — looked great. I nearly wrote a whole section about how MXFP4 was surprisingly good and Q5_K_M was degraded. Both conclusions were noise.

So I rebuilt everything.

### Fix 1: Real execution, not grep

I replaced the grep-based exam_v2 evaluator with Go integration tests. The [test harness](exam_v2/harness/harness_test.go) compiles the model's code into a binary, starts it against a [mock server](exam_v2/mock/main.go) with controllable online/offline state, and runs 10 behavioral tests:

- **OnlineFlow**: metrics reach the sink when online
- **BuffersDuringOutage**: nothing leaks when offline
- **FlushOnReconnect**: buffered metrics flush on reconnect
- **BufferBounded**: flushed count stays within buffer-size
- **EvictionRandom**: evicted items are a mix of old and new (not FIFO/LIFO)
- **MultipleOutageCycles**: survives 3 offline/online transitions
- **BufferSizeZero**: doesn't panic with `-buffer-size 0`
- **BufferSizeOne**: works with `-buffer-size 1`
- **GracefulShutdown**: exits cleanly on SIGINT
- **RaceDetector**: compiled with `-race`, no data races

Score = tests passed. The harness auto-detects flag names and types from the binary's `-h` output, so it adapts to whatever CLI interface the model invents. Scraper runs at 10ms poll rate for fast execution (~30s per model).

The same models that scored 15-18/20 with grep now scored 0-8/10 with real tests. If your eval can't execute the code, you're measuring vibes.

### Fix 2: Multiple seeds

Three seeds per model (42, 123, 456), temp 1.0. Results reported as mean and range.

Single-seed benchmarks lie. Qwen3.5-35B Q5_K_M scored 9/15 on seed 42 and 14/15 on seed 456 for exam_v1. At n=1, you'd conclude it's broken. At n=3, it's fine — the 9 was a fluke.

### Fix 3: Context window

An early run with 8k context showed terrible compile rates. I spent an afternoon debugging "model quality" before noticing the responses were literally truncated mid-function. The model was writing correct code that got cut off at the context limit. Bumped to 16k and half the compile failures vanished.

Infrastructure bugs masquerading as model quality problems — the most dangerous kind, because you'll draw confident wrong conclusions.

## The models

Started with 15, pruned to 11 after the first round. DeepSeek-Coder-V2-Lite, DeepSeek-R1-14B, GLM-4.7-Flash, GLM-4.7-Flash-REAP, and gemma-3n-E4B scored consistently badly and weren't worth 3-seed sweeps.

| Model | Architecture | Quant | Size |
|-------|-------------|-------|------|
| Gemma 4 26B-A4B | 26B MoE (4B active) | UD-Q4_K_M | 16 GB |
| Gemma 4 26B-A4B | same | MXFP4_MOE | 15 GB |
| Gemma 4 26B-A4B | same | UD-Q5_K_M | 21 GB |
| Qwen3.5-35B-A3B | 35B MoE (3B active) | Q4_K_M | 21 GB |
| Qwen3.5-35B-A3B | same | Q5_K_M | 28 GB |
| Qwen3.5-35B-A3B | same | Q6_K | 32 GB |
| Qwen3.5-35B-A3B | same | MXFP4_MOE | 20 GB |
| Qwen3-Coder-30B-A3B | 30B MoE (3B active) | Q4_K_M + 0.6B draft | 18+0.6 GB |
| gpt-oss-20b | 20B dense | MXFP4 | 12 GB |
| Qwen3.5-9B | 9B dense | Q4_K_M | 5.3 GB |
| Gemma 4 E4B | ~8B E4B | Q8_0 | 7.6 GB |

## Exam 1: Three Go programs (/15)

Write factorial, word frequency counter, and file tree walker. 5 points each: build(1) + runs(1) + correct output(3). Same tasks as episode 1, with [fixed scoring](exam_v1/eval.sh) (graduated wordfreq credit, removed an unreachable quality point).

| Model | Mean | Range | Tok/s |
|-------|------|-------|-------|
| Qwen3.5-35B Q6_K | 14.3 | 14-15 | 22.1 |
| **Gemma 4 26B Q4_K_M** | **14.0** | **14-14** | **18.6** |
| **Gemma 4 26B MXFP4** | **14.0** | **14-14** | **18.8** |
| **Gemma 4 26B Q5_K_M** | **14.0** | **14-14** | **17.0** |
| **Qwen3.5-35B Q4_K_M** | **14.0** | **14-14** | **22.1** |
| **Qwen3.5-35B MXFP4** | **14.0** | **14-14** | **21.9** |
| **Qwen3-Coder-30B + draft** | **14.0** | **14-14** | **25.9** |
| **gpt-oss-20b** | **14.0** | **14-14** | **27.0** |
| Gemma 4 E4B Q8 | 12.7 | 10-14 | 13.8 |
| Qwen3.5-9B | 12.3 | 9-14 | 13.6 |
| Qwen3.5-35B Q5_K_M | 12.0 | 9-14 | 21.0 |

**Bold models scored 14/15 on all three seeds.** Exam 1 is a reliability gate — if a model is decent at Go, it passes consistently. The spread (14-14 vs 9-14) tells you which models are dependable vs. which are coin flips.

## Exam 2: The resilience test (/10)

The real test. The model gets a 208-line Go metrics scraper and must modify it to: buffer metrics in memory during network outages, randomly evict when the buffer is full, and flush via a background goroutine on reconnect.

Scored by Go integration tests (described above), not grep. Each seed runs the full 10-test suite.

| Model | Mean | Range | Compiles | Tok/s |
|-------|------|-------|----------|-------|
| **Gemma 4 26B Q4_K_M** | **4.0** | **0-6** | **2/3** | **18.6** |
| **Gemma 4 26B MXFP4** | **4.0** | **0-6** | **2/3** | **18.8** |
| **Gemma 4 26B Q5_K_M** | **4.0** | **0-6** | **2/3** | **17.0** |
| Gemma 4 E4B Q8 | 3.7 | 0-7 | 2/3 | 13.8 |
| gpt-oss-20b | 3.7 | 0-6 | 2/3 | 27.0 |
| Qwen3-Coder-30B + draft | 3.7 | 0-6 | 2/3 | 25.9 |
| Qwen3.5-35B Q5_K_M | 3.3 | 0-6 | 2/3 | 21.0 |
| Qwen3.5-35B Q6_K | 2.7 | 0-8 | 1/3 | 22.1 |
| Qwen3.5-35B Q4_K_M | 2.3 | 0-7 | 1/3 | 22.1 |
| Qwen3.5-9B | 2.3 | 0-7 | 1/3 | 13.6 |
| Qwen3.5-35B MXFP4 | 2.0 | 0-6 | 1/3 | 21.9 |

"Compiles" means the model produced code that builds AND passes at least one test. Every model fails at least one seed — either the code doesn't compile, or it compiles but all 10 tests fail. When models DO compile and pass, they typically get 5-7/10. Common failures: `rand.Intn(0)` panic with buffer-size 0, off-by-one in flush logic, concurrent buffer access without proper locking.

This exam is hard. 4/10 is the ceiling.

## Quantization

### Gemma 4 26B: quant doesn't matter

| Quant | Size | Exam 1 | Exam 2 | Compiles |
|-------|------|--------|--------|----------|
| UD-Q4_K_M | 16 GB | 14.0 (14-14) | 4.0 (0-6) | 2/3 |
| MXFP4_MOE | 15 GB | 14.0 (14-14) | 4.0 (0-6) | 2/3 |
| UD-Q5_K_M | 21 GB | 14.0 (14-14) | 4.0 (0-6) | 2/3 |

Identical scores across all three quants, both exams. Pick the smallest: MXFP4 at 15 GB.

### Qwen3.5-35B: Q5_K_M is the most reliable

| Quant | Size | Exam 1 | Exam 2 | Compiles |
|-------|------|--------|--------|----------|
| Q4_K_M | 21 GB | 14.0 (14-14) | 2.3 (0-7) | 1/3 |
| **Q5_K_M** | **28 GB** | **12.0 (9-14)** | **3.3 (0-6)** | **2/3** |
| Q6_K | 32 GB | 14.3 (14-15) | 2.7 (0-8) | 1/3 |
| MXFP4_MOE | 20 GB | 14.0 (14-14) | 2.0 (0-6) | 1/3 |

Q5_K_M has the best compile rate on the hard exam (2/3 vs 1/3) but wobbles on the easy exam (9-14 range). Q4_K_M is rock-solid on easy tasks but collapses on hard ones. An odd inversion — more bits doesn't monotonically help. Q5_K_M is the best overall if you use Qwen3.5, but at 28 GB it still can't match Gemma 4's consistency.

## External benchmarks

No published benchmarks test quantized models at this size. Every number on leaderboards is full-precision on server GPUs. Our quantized-on-iGPU results are, as far as I can find, novel.

For context, here's how these models score on published benchmarks (full precision):

| Benchmark | Qwen3.5-35B | Gemma 4 26B |
|-----------|-------------|-------------|
| Terminal-Bench 2 | 40.5% | -- |
| SWE-bench Verified | 69.2% | -- |
| TAU2 | 81.2 | 68.2 |
| LiveCodeBench | -- | 77.1% |
| Arena AI (Elo) | 1400 | 1441 |

Gemma 4 edges out Qwen3.5 on Arena AI despite being smaller and having a lower TAU2 score. Our local quantized results tell a similar story: Gemma 4 is more consistent where it counts. The question nobody else is answering is whether these scores survive quantization and constrained hardware. For Gemma 4, they clearly do.

## Conclusions

**Gemma 4 26B-A4B is the best local coding model on this hardware.** 14/15 on all seeds, all quants. Highest exam_v2 mean (4.0/10). Quant-insensitive — the 15 GB MXFP4 scores the same as the 21 GB Q5_K_M. For a 26B MoE at 18 tok/s on an iGPU, that's the one to use.

**gpt-oss-20b is the best value.** 14/15 rock solid, 3.7/10 on exam_v2, 27 tok/s, 12 GB. Smallest and fastest model that competes with the bigger MoEs. Dense architecture, no draft model needed.

**Qwen3.5-35B is fast but flaky.** 21-22 tok/s but lower compile rates than Gemma 4 on the hard exam. If you need it, Q5_K_M (28 GB) is the best quant.

**The methodology was the real discovery.** Every major conclusion from the initial single-seed grep-based run was wrong. MXFP4 isn't surprisingly good — it's the same as Q4_K_M. Q5_K_M isn't broken — seed 42 was a fluke. And models scoring 15-18/20 by keyword matching scored 0-8/10 under real execution. If you're benchmarking local models with n=1 and regex, you're fooling yourself.

## Run it yourself

```bash
# Start llama-swap
~/play/llama/llama-swap --config ~/play/llama/config.yaml --listen localhost:8080 --watch-config

# Multi-seed sweep
./sweep.sh
```

All code: [exam_v1/](exam_v1/), [exam_v2/](exam_v2/), [sweep.sh](sweep.sh), [results/](results/)
