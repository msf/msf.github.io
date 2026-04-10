# Model Search Notes (2026-02-22)

## Hardware constraints
- Framework 13: Ryzen AI 370HX, Radeon 890M, 64GB DDR5, ~89.6 GB/s bandwidth
- GTT expanded to 58GB via `amdgpu.gttsize=58880` kernel param
- Sweet spot: MoE 30B-A3B with spec decode, or dense 8-20B Q4_K_M/MXFP4
- llama.cpp b7992 Vulkan (RADV), `--temp 1 --seed 42 --jinja --single-turn`
- Speculative decoding works: Qwen3-0.6B Q8_0 as draft for any Qwen3-* model

## Current best: Qwen3-Coder-30B-A3B Q4_K_M + draft
- Exam: 13/15, 26s wall, 54 t/s, 17.8GB RSS
- Ties gpt-oss-20b on score, 2.5x faster wall time
- Qwen3-0.6B spec decode draft, MoE with only 3B active params
- Previous champion: Qwen3-8B Q4_K_M (11/15, 27s wall)

## Tested 2026-02-22

| Model | Exam | Wall | Tok/s | RSS | Notes |
|-------|------|------|-------|-----|-------|
| **Qwen3-Coder-30B-A3B Q4_K_M + draft** | **13/15** | **0:26** | 54 | 17.8GB | **Best score+speed** |
| gpt-oss-20b MXFP4 | 13/15 | 1:07 | 24 | 11.7GB | Baseline |
| Qwen3-8B Q4_K_M + draft | 11/15 | 0:27 | 9 | 4.9GB | Baseline |
| Qwen3-14B Q4_K_M + draft | 8/15 | 1:00 | 8 | 8.7GB | Worse than 8B |
| GLM-4.7-Flash Q4_K_M (full 30B) | 5/15 | 1:52 | 70 | 17.6GB | Fast tok/s but bad code output |
| GLM-4.7-Flash REAP-23B-A3B Q4_K_M | 3/15 | 2:17 | 81 | 13.3GB | Pruned, even worse |
| Nemotron-3-Nano-30B-A3B Q4_K_M | 0/15 | 1:13 | 94 | 23.6GB | Format issues, all build fail |
| Qwen3-14B thinking mode | DNF | >5min | -- | -- | Burns all budget on thinking tokens |
| Qwen3-Coder-30B-A3B Q4_K_M (31GB GTT) | crash | -- | -- | -- | Vulkan OOM before GTT expansion |

## Flagship models to watch for distills

None of these have small versions yet. All are massive MoE:

| Model | Org | Total | Active | Status |
|-------|-----|-------|--------|--------|
| **Kimi K2.5** | Moonshot | 1T | 32B | Only full size. Moonlight-16B-A3B exists but old/weak |
| **Qwen3.5** | Alibaba | 397B | 17B | Only 397B-A17B. Hybrid GDN+MoE architecture (novel). Most likely to get distills given Qwen's track record |
| **MiniMax M2.5** | MiniMax | ~450B+ | ~45B | Only full size. No small models at all |
| **GLM-5** | Zhipu/THUDM | 754B | ? | Only full size. GLM-4.7-Flash (full 30B) scored 5/15 |

**Most promising**: Qwen3.5 distills. Same tokenizer would mean Qwen3-0.6B draft still works for spec decode. Watch https://huggingface.co/Qwen for 7B/14B releases.

## GLM-4.7-Flash: REAP vs standard -- both tested, both disappointing

- REAP (pruned 23B-A3B): 3/15 exam, 81 t/s, 13.3GB
- Standard (full 30B-A3B): 5/15 exam, 70 t/s, 17.6GB -- fast inference but poor code generation
- Despite strong published benchmarks (SWE-bench 59.2%, AIME 91.6%), our Go coding exam
  exposes weakness in structured multi-file output. 2 of 3 programs failed to build.
- Not worth further investigation unless MTP support lands in llama.cpp.

## Speculative decoding landscape

**llama.cpp draft-model pairs** (requires same tokenizer):
- **Qwen3**: 0.6B drafts for 8B/14B/30B -- working, only viable option today
- **Gemma 3n**: E2B drafts for E4B -- both too small to be interesting
- No draft models exist for GLM, DeepSeek, Kimi, or MiniMax families

**MTP (Multi-Token Prediction)** -- the real opportunity:
GLM-4.7 and DeepSeek have MTP heads baked into weights. The model predicts multiple tokens per forward pass without a separate draft model. vLLM/SGLang support it (`--speculative-config.method mtp`). **llama.cpp does NOT support MTP yet.** This is the single biggest feature gap for local inference on this hardware. MoE + self-speculation would be ideal.

**Watch for**: llama.cpp MTP support (open PRs/issues exist). Would unlock GLM-4.7-Flash and DeepSeek self-speculation.

## Abliteration opportunity

gpt-oss-20b wastes thinking tokens on safety deliberation. Could abliterate:
- Load fp16 (~40GB, fits in 64GB RAM)
- Run contrastive prompts, compute refusal direction
- Subtract from residual stream weights, re-quantize to GGUF
- ~50 lines of Python with transformers + torch

## Search URL
https://huggingface.co/models?num_parameters=min:9B,max:32B&apps=llama.cpp&sort=trending

## Partial downloads in cache (abandoned)
- `Qwen2.5-Coder-1.5B Q8_0` (potential draft model for Qwen2.5 family)
- `DeepSeek-Coder-V2-Lite Q4_K_M` (already have Q8_0)
- `Qwen3-Coder-30B-A3B Q4_K_M` (Vulkan OOM, needs GTT expansion)

## Nemotron-3-Nano-30B-A3B
Was downloading at end of session. Another 30B MoE with 3B active -- same GTT issue as Qwen3-Coder-30B. Needs `amdgpu.gttsize=61440` kernel param first.

## Note on thinking mode
- `/no_think` genuinely makes models dumber -- no hidden silent thinking. Tokens ARE the reasoning. Autoregressive models cannot "think silently" because reasoning IS the token sequence in context.
- gpt-oss-20b wastes thinking tokens on safety deliberation (copyright checks etc).
- Thinking mode unusable on this hardware for 14B+ models (too slow, >5min for exam).

## Script fixes applied
- `bench.sh` and `exam.sh` default changed: `llama-cli` -> `llama-completion` (mutation from blog cleanup)
- `N_PREDICT=-2` works with `llama-completion` (broken with `llama-cli`)
- Added auto-detection of draft models: Qwen3-* gets Qwen3-0.6B draft via `-hfrd`
