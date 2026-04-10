# Parking lot — deferred items from 2026-04-17 Qwen3.6 session

Items the user raised or I surfaced, explicitly NOT done in this session.
Listed here so nothing gets forgotten.

## 1. llama-server settings review across config.yaml

**What**: audit all existing entries in `~/play/llama/config.yaml` for flags
consistency, KV cache quantization, context sizes, sampling params.

**Why deferred**: bundling it with a benchmark session risks conflating
"settings change" with "model change" in the results.

**Concrete candidates to review**:
- `qwen35-35b-q5km` etc. — all at 16384 ctx. Likely want 32k for fairer agentic use.
- `qwen3-coder-draft` — draft-max 16, may want to tune per actual hit rate.
- `qwen35-35b-think` — has budget 8192, others don't; standardize.
- `--cache-type-k q8_0 --cache-type-v q8_0` — is Q4 KV ever worth it for these models? The `common-flags-q4kv` macro exists but is unused.
- Sampling params: most entries don't set them; they use llama.cpp defaults.
  Qwen3.6 docs are explicit about recommended values; the same likely helps
  Qwen3.5 / Qwen3-coder. Unknown if worth enforcing.

## 2. Qwen3.6 MXFP4 quant

**What**: download `Qwen3.6-35B-A3B-MXFP4_MOE.gguf` (21.7 GB), add config,
benchmark head-to-head vs UD-Q5_K_M.

**Why deferred**: first establish whether Q5 is competitive at all. If yes,
then ask the "is MXFP4 enough" question. Qwen3.5's own data hints MXFP4 may
not be the best default for this family (2.0/10 exam v2 vs 3.3 for Q5_K_M).

## 3. Qwen3.5 retune — more context + thinking

User suspects Qwen3.5 was under-tuned. Worth revisiting once Qwen3.6 baseline
is established. Variants to try:
- Qwen3.5-35B Q5_K_M at 32k, thinking on, budget 16k
- Qwen3.5-35B Q5_K_M at 32k, thinking off
Compare to same pair for Qwen3.6.

## 4. gpt-oss-20b

User explicitly doesn't want to use it ("legalese / self-censoring"). Not
retesting. Could consider stripping out via system prompt, but that's a
different experiment. **Intent: do not run gpt-oss in further sweeps.**

## 5. Ryzen 8500G fallback server

User floated: "a real outcome of this could be that I set up my openclaw to
fallback to local model usage on my server that has a ryzen 8500G cpu/apu"

Hardware: different GPU (RDNA 3.5 iGPU integrated in Phoenix vs Strix Point).
Different memory bandwidth. Different thermal envelope.

**Prereqs**:
- Know what fits (probably gpt-oss-20b or Qwen3.5-9B, not 35B MoE).
- Pick a candidate model from this session's winners, re-bench on that hardware.
- Set up llama-swap + llama.cpp vulkan build for that machine.
- OpenClaw (= OpenCode or similar?) fallback config with timeout-based switch.

**Not a Phase 1/2 item. Phase 3 at earliest.**

## 6. Phase 2 harness build

See `PHASE2_AGENTIC_HARNESS_DESIGN.md`. Key input needed from user:
**2-3 concrete real tasks** (repo snapshots, logs, manifests) to build
tasks from. Without that, harness is just abstract.

## 7. Re-download stale Gemma repos

From `LOCAL_LLM_CODING_BENCHMARKS.md`:
> Both local Gemma repos are stale versus the Apr 11 republish.

`unsloth/gemma-4-26B-A4B-it-GGUF` — local already at latest snapshot (8bace...). Double-check after this sweep.
`unsloth/gemma-4-E4B-it-GGUF` — local at ce1529..., which matches latest per LOCAL_LLM_CODING_BENCHMARKS.md. Probably fine.

## 8. Cleanup: partial HF downloads

From `LOCAL_LLM_CODING_BENCHMARKS.md`:
- `ggml-org--gemma-3-1b-it-GGUF` — partial/empty, safe to remove
- `unsloth--Nemotron-3-Nano-30B-A3B-GGUF` — partial/empty, safe to remove

Not space-critical, but hygiene. Not this session.
