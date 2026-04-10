# Local LLM coding benchmarks — April 2026

Canonical copy of the benchmark/session notes that previously lived in `AGENTS.md` before that file was reduced to layout/runtime guidance.

## Hardware and setup

Framework 13, Ryzen AI 370HX, Radeon 890M iGPU, 64 GB DDR5. Vulkan backend.
llama.cpp b8708, llama-swap v199.

## What we tested

15 local LLMs on two Go coding exams, 3 seeds each (42, 123, 456), temp 1.0, 10 minute timeout, 16k context.

### Exam v1: three simple Go programs (/15)

Factorial, word frequency counter, file tree walker. Scoring: build(1) + runs(1) + correct(3) = 5 points each.

### Exam v2: resilience modification (/10)

Modify a 208-line Go scraper to add in-memory buffering during outages, random eviction when full, and a background flush goroutine. Scored by Go integration tests against a controllable mock server.

## Final results

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

Pruned after the first round (`exam_v1` scores from memory; result files were deleted during reorganization):

| Model | v1 mean | v1 range | Notes |
|-------|---------|----------|-------|
| deepseek-coder-v2-16b | 10.3/15 | 9-13 | Legacy coder, outclassed |
| glm-flash-30b | 10.0/15 | 9-12 | Dense 30B, too slow for score |
| gemma3n-e4b-q8 | 8.0/15 | 5-10 | Previous gen Gemma |
| glm-flash-reap-23b | 7.0/15 | 4-10 | MoE variant, worse than dense |
| deepseek-r1-14b | 5.0/15 | 5-5 | Reasoning model with `--reasoning off` |

Qwen3-8B, the episode 1 champion at 4.7 GB, was superseded by Qwen3.5-9B for this benchmark, but is still worth keeping as a small reference baseline.

## Main conclusions

- **Best model: Gemma 4 26B-A4B.** Q4_K_M, MXFP4, and Q5_K_M all score the same here: 14.0/15 on exam v1, 4.0/10 on exam v2, 2/3 compile rate. Quant did not matter in this benchmark.
- **Best value: gpt-oss-20b.** 14.0/15, 3.7/10, 2/3 compiles, 27 tok/s, 12 GB. Smallest model that still competes with the larger MoEs.
- **Qwen3.5-35B is fast but flaky under quantization.** Q5_K_M is the best quant for hard tasks here, but still trails Gemma 4 in reliability.
- **MXFP4 was not a trap.** Initial single-seed conclusions were wrong. Multi-seed runs showed MXFP4 was fine for Gemma 4 and not worse than Q4_K_M for Qwen3.5.

### Methodology lessons

- n=1 results are noise.
- Grep-based scoring inflates results; integration tests against the compiled binary are the right evaluator.
- 8k context was too small for exam v2; 16k removed some false failures from truncation.
- Compile rate matters more than peak score for practical use.
- Test the binary, not the source.

## Infra kept in this workspace

- `~/play/llama/config.yaml` — llama-swap config, all models using `--model` with local HF cache paths
- `~/play/msf.github.io/blogpost/benchmarking_llms/exam-driver.go` — Go driver that sends prompts through llama-swap and runs evaluators
- `~/play/msf.github.io/blogpost/benchmarking_llms/scripts/sweep.sh` — current multi-seed sweep runner, model-first ordering
- `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v1/eval.sh` — scoring evaluator for the three-program exam
- `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/eval.sh` — compile + Go integration harness entrypoint
- `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/harness/harness_test.go` — 10 behavioral tests
- `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/mock/main.go` — controllable mock server

## Current model inventory and curation plan

HF cache root: `/mnt/ai-models/huggingface/hub/`.

### Keep on disk

- Qwen3-8B: Q4_K_M. Small dense reference baseline; not in the current llama-swap config, but intentionally kept.
- Qwen3.5-0.8B: Q8_0. Small reference / possible draft companion for the Qwen3.5 family.
- Qwen3.5-35B-A3B: Q4_K_M, MXFP4_MOE, Q5_K_M, Q6_K.
- Qwen3.5-9B: Q4_K_M.
- Qwen3-Coder-30B-A3B: Q4_K_M + Qwen3-0.6B draft.
- Gemma 4 26B-A4B: UD-Q4_K_M, MXFP4_MOE, UD-Q5_K_M.
- Gemma 4 E4B: Q8_0.
- gpt-oss-20b: MXFP4.
- Qwen2.5-Coder-7B: Q8_0. Separate editor/FIM/tooling use; not part of the current benchmark set.
- gemma-3-4b: Q4_K_M. Legacy reference, low priority.

### Refresh needed

Both local Gemma repos are stale versus the Apr 11 republish:

- `unsloth/gemma-4-26B-A4B-it-GGUF`: local `bd1a2329b14654bebfdf4b3346cd3b8e123fd81b` → remote `8bacec5c8e829a25502cdfe3c3f5b6aabee3218c`
- `unsloth/gemma-4-E4B-it-GGUF`: local `960a8cd001a5ec7a679e2c5d93f9916238e76d10` → remote `ce152932ac27bc40bc9c727386760424d50bb456`

Unsloth's README note for these repos: **"Re-download for Google's latest chat template and llama.cpp fixes."**

Re-download only the Gemma files worth keeping active. Do not do a full quant sweep just to refresh the cache.

### Working quant policy on this machine

- Prefer MXFP4 as the default 4-bit choice when a model offers it.
- Prefer Q5 / Q6 / Q8 when the quality gain is real and the speed hit is acceptable on this box.
- For Gemma 4 26B, current benchmark results did not separate Q4_K_M, MXFP4, and Q5_K_M, so long-term pruning should probably collapse to MXFP4 plus one larger quant instead of keeping all three forever.
- For Qwen3.5-35B, keep the current set until a narrower use case justifies pruning. Existing benchmark results favored Q5_K_M more than the theory did.

### Cleanup candidates

- `ggml-org--gemma-3-1b-it-GGUF` — partial / empty download, safe to remove.
- `unsloth--Nemotron-3-Nano-30B-A3B-GGUF` — partial / empty download, safe to remove.
- Those two removals are about hygiene, not space. The real space wins are the extra large-model quants, especially Qwen3.5-35B and Gemma 4 26B, so do not prune those blindly without first deciding the long-term default quant set.
- Defer decisions on `gemma-3-4b` and `Qwen2.5-Coder-7B`. They are not active in llama-swap, but they may still be useful as references or tooling models.

Pruned from benchmarks and deleted from disk (67 GB freed): DeepSeek-Coder-V2-Lite, DeepSeek-R1-14B, GLM-4.7-Flash, GLM-4.7-Flash-REAP, gemma-3n-E4B, Qwen3.5-4B.

### Deferred next steps

1. Re-download the stale Gemma 4 repos / files you actually want to keep active.
2. Remove the two partial / empty repos above.
3. Later, add `gemma-4-E2B-it` as the tiny draft and `gemma-4-31B-it` as the dense comparison model.
4. After those downloads, do smoke tests only: Gemma 4 26B MoE with / without draft, then Gemma 4 31B dense with / without draft. No full sweep until the load / generation path is sane.

## Open questions

- Why is Qwen3.5 so flaky under quantization?
- Terminal-Bench 2 and SWE-bench still lack quantized local-model baselines.
- Would 5 seeds materially change confidence, or just burn more iGPU hours?
