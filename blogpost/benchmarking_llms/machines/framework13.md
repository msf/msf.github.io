# Framework 13 machine plan

This is the reference local benchmark machine for this workspace.

## Verified identity

Verified on 2026-04-23 from the local machine state:

- Hostname: `fw13`
- OS/kernel: Ubuntu 24.04.4 LTS, `Linux 6.17.0-20-generic`
- CPU: AMD Ryzen AI 9 HX 370
- GPU: Radeon 890M iGPU
- RAM: 64 GB DDR5
- Memory bandwidth ceiling: ~89.6 GB/s
- Runtime backend: Vulkan
- Active llama.cpp release: `llama-b8708`

This is the machine the current local benchmark conclusions are anchored to.

## Purpose

Use this machine for:

- the canonical local benchmark baseline in this repo
- quantized-model comparisons on weak-but-real laptop hardware
- validating whether a runtime/bootstrap change still preserves the known Framework 13 behavior

Do **not** treat it as a generic template for every host. It is the concrete plan for this host only.

## Filesystem and runtime roots

Canonical source/workspace:

- Lab root: `~/play/msf.github.io/blogpost/benchmarking_llms`
- Bench definitions: `~/play/msf.github.io/blogpost/benchmarking_llms/bench`
- Results: `~/play/msf.github.io/blogpost/benchmarking_llms/artifacts/results`
- Logs: `~/play/msf.github.io/blogpost/benchmarking_llms/artifacts/logs`

Machine-local runtime:

- Runtime root: `~/play/llama` -> `/mnt/ai-models/llama`
- Active release symlink: `~/play/llama/llama-current`
- Active release target: `/mnt/ai-models/llama/llama-b8708`
- llama-swap binary: `~/play/llama/llama-swap`
- llama-swap config: `~/play/llama/config.yaml`

Model/cache roots:

- Hugging Face cache: `/mnt/ai-models/huggingface/hub`
- Legacy GGUF cache: `/mnt/ai-models/gguf-models`
- `~/.cache/huggingface` and `~/.cache/llama.cpp` are symlinked into those mounts

These are not all the same filesystem. Do not assume hard links will work across them.

## Runtime defaults on this machine

The current serving config is built around:

- `llama-swap` on `http://localhost:8080`
- `llama-server` from `llama-b8708`
- `--flash-attn on`
- `--cache-type-k q8_0 --cache-type-v q8_0`
- `--gpu-layers 99`
- `--jinja`
- top-level `healthCheckTimeout: 300`

Default operating mode:

- use **Vulkan** as the baseline backend
- use **power-saver** when the goal is reproducible benchmarking with lower jitter
- use performance/AC mode only when explicitly chasing peak throughput numbers, and document that separately

Reason: previous measurements on this machine showed tighter variance on power-saver than on performance mode because thermals introduce more jitter when clocks float high.

## Active served model set

This machine’s current `llama-swap` config is broader than the benchmark shortlist. It currently serves:

### Main benchmark / comparison models

- `qwen36-35b-q5km-thinkoff`
- `qwen36-35b-q5km-thinkon`
- `gemma4-26b-mxfp4-64k`
- `gemma4-26b-q8-32k`
- `gemma4-e4b`
- `gpt-oss`

### Qwen3.5 family variants still kept for comparison

- `qwen35-35b`
- `qwen35-35b-think`
- `qwen35-35b-mxfp4`
- `qwen35-35b-q5km`
- `qwen35-35b-q6k`
- `qwen35-9b`

### Other local reference models

- `qwen3-coder`
- `qwen3-coder-draft`
- `gemma4-31b`

## Default choices on this machine

Use these unless the task explicitly says otherwise:

- **Default quality baseline:** `gemma4-26b-mxfp4-64k`
- **Default Qwen3.6 non-thinking comparison:** `qwen36-35b-q5km-thinkoff`
- **Default Qwen3.6 high-quality/slow comparison:** `qwen36-35b-q5km-thinkon`
- **Historical value reference:** `gpt-oss` (keep as context only; user does not want it in further sweeps)
- **Default benchmark harness:** `exam_v3`
- **Default seeds:** `42 123 456`
- **Default exam_v3 max tokens:** `8192`
- **Default sweep:** `scripts/sweep-exam3.sh`

Historical note: `scripts/sweep.sh` remains a historical exam_v1/exam_v2 runner. Do not treat it as the default path on this machine.

## Machine-specific benchmark policy

### What this machine is good at

- testing whether 12–30 GB quantized models are actually usable on weak local hardware
- comparing latency/compile-rate tradeoffs under unified memory pressure
- validating whether speculative decoding or longer context is worth it on an iGPU laptop

### What this machine is not good at

- pretending server-GPU leaderboard claims will transfer directly
- brute-forcing lots of wide quant sweeps without a shortlist
- silently soaking GPU-hours to gain a tiny confidence bump

### Working quant policy here

- prefer MXFP4 as the default 4-bit choice when quality is not measurably worse
- keep larger quants only where they buy a real compile-rate or stability improvement
- for Gemma 4 26B, current results do **not** justify keeping every quant forever
- for Qwen3.5-35B, keep the existing set until a narrower use case justifies pruning

## Current refresh state for the key Unsloth repos

Live-checked on 2026-04-23.

| Repo | Local snapshot in config | Remote repo SHA | Status |
|------|--------------------------|-----------------|--------|
| `unsloth/Qwen3.6-35B-A3B-GGUF` | `9280dd353ab587157920d5bd391ada414d84e552` | `a483e9e6cbd595906af30beda3187c2663a1118c` | **stale** |
| `unsloth/gemma-4-26B-A4B-it-GGUF` | `8bacec5c8e829a25502cdfe3c3f5b6aabee3218c` | `b68961b3c96e42475123a39fe3f8aa149163cf8b` | **stale** |
| `unsloth/gemma-4-E4B-it-GGUF` | `ce152932ac27bc40bc9c727386760424d50bb456` | `ce152932ac27bc40bc9c727386760424d50bb456` | current |

Implication:

- do **not** treat current local Qwen3.6 or Gemma4-26B results as “latest upstream weights” without saying they are from stale snapshots
- if a task is specifically about current Unsloth Gemma/Qwen3.6 behavior, refresh those repos first, then smoke test before any real sweep
- Gemma4-E4B does not currently need a refresh on this check

## What to do before running real work on this machine

1. Run `./scripts/bootstrap-framework13.sh`.
2. `source machines/framework13.env`.
3. Confirm `readlink -f ~/play/llama/llama-current` still matches the expected release or record the new one.
4. Check whether the task depends on fresh Qwen3.6 / Gemma4-26B weights.
5. Start `llama-swap` with `~/play/llama/config.yaml` if it is not already up.
6. Use `scripts/sweep-exam3.sh` unless you are explicitly reproducing old exam_v1/v2 work.

Recommended commands:

```bash
./scripts/bootstrap-framework13.sh
source ./machines/framework13.env
"$LLAMA_SWAP" --config "$LLAMA_SWAP_CONFIG" --listen localhost:8080
./scripts/sweep-exam3.sh
```

## Known constraints / caveats

- Unified memory bandwidth is the main ceiling.
- Qwen3.5 remains flaky under local quantization on this box.
- `qwen3-coder-draft` is still not benchmark-ready; treat it as experimental.
- The runtime workspace (`~/play/llama`) is not the source of truth for benchmark code or notes anymore.
- Historical artifacts under `artifacts/history/` are evidence, not a clean reproducible state.

## Machine-specific surprises and unresolved findings

These are important because they change how future Framework 13 results should be interpreted.

### 1. Qwen looked much worse on `exam_v3` than expected

This was the biggest surprise on this machine.

- On `exam_v2`, Qwen3.5 and later Qwen3.6 still looked competitive enough to keep investing in.
- On `exam_v3`, local Qwen results collapsed hard relative to expectation.
- The strongest local Qwen3.6 run only reached `7/13`, while Gemma 26B variants hit `11/13`.
- Qwen3.5 was even worse; some variants went to stable `0/13` outcomes.

This is documented in `bench/exam_v3/REPORT.md` and should be treated as a **machine-specific anomaly worth revisiting**, not a settled conclusion about the family in general.

### 2. The current hypothesis is not “Qwen bad”, but “this exact local stack may be hurting Qwen”

The current live hypothesis for Framework 13 is:

- provider packaging,
- quantization choice,
- tokenizer/template packaging,
- and/or local llama.cpp / llama-swap behavior

may be hurting Qwen more than Gemma on this machine.

That hypothesis is justified because:

- all tested Qwen locals here came from Unsloth GGUF snapshots
- `qwen3.6` underperformed badly despite being the strongest Qwen in the set
- most failures were invalid-Go / wrong-interface failures, not subtle semantic misses

So for this box, **alternate Qwen GGUF providers/quants remain an open investigation item**.

### 3. Qwen3.6 gave conflicting signals across harness generations

Framework 13 notes currently contain two real but conflicting stories:

- `docs/reports/QWEN36_PHASE1_RESULTS.md`: Qwen3.6 thinking-on looked like the new best local coding model on the older `exam_v2` harness.
- `bench/exam_v3/REPORT.md`: Qwen3.6 underperformed badly on the newer `exam_v3` harness.

That conflict is real. It means:

- do not collapse all “Qwen3.6 on Framework 13” conclusions into one sentence
- always specify **which exam/harness** the claim comes from

### 4. Quant exploration is not finished on this machine

Important unfinished Framework 13 questions still exist:

- Qwen3.6 MXFP4 vs UD-Q5_K_M
- Qwen3.5 retune at larger context and with thinking enabled
- Gemma4 draft experiments (`E4B` and maybe `E2B` if compatible/useful)
- whether Q8 variants are ever worth the memory/throughput tradeoff on this box outside narrow cases

Current known points:

- Gemma 4 26B looked largely quant-insensitive in the earlier local benchmark
- Qwen3.5 did **not** look quant-insensitive; Q5_K_M was more reliable than MXFP4 there
- `gemma4-26b-q8-32k` exists in the current runtime, but Q8 should not be assumed to be the default best choice here

### 5. `qwen3-coder-draft` remains unresolved

This machine still does not have a benchmark-ready setup for `qwen3-coder-draft`.

Verified on this box:

- startup handling needed fixes
- health-check timeout had to be raised
- even after that, generation still timed out at `10m` and `15m`
- reducing context to `32k` did not rescue it

So do not treat current Framework 13 results as a fair verdict on that model yet.

### 6. Some benchmark conclusions are still blocked on harness correctness

Framework 13-specific benchmark interpretation is still constrained by tooling debt:

- the old `exam_v2` harness is known-bad and needs redesign / re-scoring
- published blog numbers for that older harness are known suspect
- `exam_v3` is better, but still has likely spec/grader rough edges

So on this machine, “which model is best?” still depends partly on which harness generation you mean.

## Deferred Framework 13 backlog

This is the real machine backlog, distilled from the older notes. These are not generic project TODOs; they affect future work on this box specifically.

### Runtime / config review

Still deferred for this machine:

- review `~/play/llama/config.yaml` for flag consistency
- review whether `q4_0` KV cache is ever worth using here versus the current `q8_0`
- review context-size consistency across entries
- review sampling params consistency, especially for Qwen-family entries
- review `draft-max` tuning for drafted models based on actual hit rate

### Quant / draft follow-ups

Still open on this machine:

- Qwen3.6 UD-Q5_K_M with `Qwen3-0.6B Q8` draft
- Qwen3.6 MXFP4 (with and without draft)
- Qwen3.5-35B Q5_K_M at higher context, thinking on/off
- Gemma4-26B MXFP4 with Gemma4 draft variants (`E4B`, possibly `E2B`)
- non-draft `qwen3-coder-30b` as a fairer datapoint than the currently broken draft path
- keep `gpt-oss` out of further sweeps unless the user explicitly reopens that question

### Benchmark-process debt

Still open before spending more serious GPU-hours on this machine:

- redesign/review the old `exam_v2` harness
- re-score saved responses after that redesign
- publish corrected blog numbers or an errata
- preserve strict time-boxing for unattended runs so this laptop does not soak endless wall-clock time on bad measuring sticks

### Cleanup / hygiene

Low priority but still real on this machine:

- remove partial HF downloads for `gemma-3-1b-it` and `Nemotron-3-Nano-30B-A3B`
- decide the long-term default quant set instead of keeping every large variant forever

## Immediate next maintenance for this machine

1. Refresh `unsloth/Qwen3.6-35B-A3B-GGUF`.
2. Refresh `unsloth/gemma-4-26B-A4B-it-GGUF`.
3. Smoke-test the refreshed `qwen36-35b-q5km-thinkoff`, `qwen36-35b-q5km-thinkon`, and `gemma4-26b-mxfp4-64k` entries.
4. Decide whether the next Framework 13 question is:
   - provider/quant investigation for Qwen,
   - runtime/config tuning,
   - or finishing the old harness debt.
5. Only then re-run any serious Qwen3.6 vs Gemma4 comparison.
