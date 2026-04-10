# exam_v3 report

_Date: 2026-04-18_

This report summarizes the `exam_v3` sweep, the failed unattended batch, the clean rerun, the final rankings, and the main lessons.

## Scope and artifacts

Primary artifacts:

- Clean rerun status: `../../artifacts/logs/exam_v3/20260418-220433-rerun/status.tsv`
- Clean rerun cell logs: `../../artifacts/logs/exam_v3/20260418-220433-rerun/cells/`
- Clean rerun results: `../../artifacts/results/exam_v3/<model>/seed{42,123,456}/`
- Archived failed local run: `../../artifacts/results/exam_v3_failed_run_20260418-211748/`
- Hosted reference runs: `../../artifacts/results/exam_v3/gpt-54/seed1/`, `../../artifacts/results/exam_v3/sonnet-45/seed1/`
- qwen3-coder draft smoke retries:
  - `../../artifacts/logs/exam_v3/qwen3-coder-smoke-20260418-211800.log`
  - `../../artifacts/logs/exam_v3/qwen3-coder-smoke-8192-20260418-212902.log`
  - `../../artifacts/logs/exam_v3/qwen3-coder-smoke-32k-20260418-214523.log`

Serving/config context verified from `/home/miguel/play/llama/config.yaml` and the rerun scripts:

- `llama.cpp` via `llama-swap`
- `--reasoning off`
- `--flash-attn on`
- `--cache-type-k q8_0 --cache-type-v q8_0`
- `--gpu-layers 99 --jinja`
- Final clean rerun used `max_tokens=8192`
- Default generation timeout was `10m`
- `qwen3-coder-draft` received dedicated smoke retries at up to `15m`

## What happened

`exam_v3` is a substantial rework of `exam_v2`, so all local results had to be rerun from scratch.

The first unattended batch was operationally bad:

- it did not actively inspect result quality while the sweep was running
- it hid driver failures by piping the driver output through `tail -1`
- it used the wrong unload endpoint (`/models/unload`), which returned `404` on this local `llama-swap` build
- `qwen3-coder-draft` silently failed startup/generation and produced empty directories / `0/0`-style non-results

That batch was not suitable as the final source of truth. Its local outputs were archived and a clean rerun was performed.

Recovery work performed before the rerun:

- fixed unloading to use `POST /api/models/unload`
- patched `exam-driver.go` to try `/api/models/unload` first
- added a global `healthCheckTimeout: 300` to `llama-swap` config
- rewrote `sweep-exam3.sh` to keep per-cell logs and stop on infrastructure tripwires
- archived the old local outputs under `../../artifacts/results/exam_v3_failed_run_20260418-211748/`
- reran the local sweep from a clean slate

## Clean rerun results

### Local models, seed-by-seed

| Model | seed42 | seed123 | seed456 | Best | Avg |
|---|---:|---:|---:|---:|---:|
| `gemma4-26b-mxfp4` | 11 | 11 | 0 | 11 | 7.33 |
| `gemma4-26b-q8` | 11 | 11 | 0 | 11 | 7.33 |
| `gpt-oss` | 5 | 0 | 11 | 11 | 5.33 |
| `qwen36-35b-q5km` | 7 | 0 | 0 | 7 | 2.33 |
| `qwen35-35b-q6k` | 0 | 6 | 0 | 6 | 2.00 |
| `gemma4-e4b-q8` | 0 | 0 | 0 | 0 | 0.00 |
| `qwen35-35b-mxfp4` | 0 | 0 | 0 | 0 | 0.00 |
| `qwen35-9b-q4km` | 0 | 0 | 0 | 0 | 0.00 |

Unranked:

- `qwen3-coder-30b-draft`: no valid scored run; excluded after repeated infra/runtime failures

### Ranking

Ranked by best seed score, then average across seeds:

| Rank | Model | Best | Avg | Notes |
|---|---|---:|---:|---|
| 1 | `gemma4-26b-mxfp4` | 11 | 7.33 | 2 strong runs, 1 compile fail |
| 1 | `gemma4-26b-q8` | 11 | 7.33 | same pattern as MXFP4 |
| 3 | `gpt-oss` | 11 | 5.33 | very volatile: 5 / 0 / 11 |
| 4 | `qwen36-35b-q5km` | 7 | 2.33 | best local Qwen, but only 1/3 viable |
| 5 | `qwen35-35b-q6k` | 6 | 2.00 | 1 partial run, 2 compile fails |
| 6 | `gemma4-e4b-q8` | 0 | 0.00 | all compile fails |
| 6 | `qwen35-35b-mxfp4` | 0 | 0.00 | all compile fails |
| 6 | `qwen35-9b-q4km` | 0 | 0.00 | all compile fails |

### Hosted reference context

These were already present and use `seed1`, so they are context, not apples-to-apples ranking entries:

| Model | Seed | Score |
|---|---:|---:|
| `sonnet-45` | 1 | 11/13 |
| `gpt-54` | 1 | 9/13 |

Interpretation:

- best local Gemma matched `sonnet-45`'s seed1 score ceiling on this harness
- local `qwen3.6` did not come close
- local `qwen3.5` was much worse than expected based on `exam_v2`

## Comparison with the failed unattended batch

### Operationally, the first batch was a failure

That is true and important. It should not have been accepted as a finished run.

### Numerically, for the Qwen models that produced artifacts, the failed batch did not distort the conclusion

This was the main surprise.

For these four Qwen models:

- `qwen35-35b-mxfp4`
- `qwen36-35b-q5km`
- `qwen35-35b-q6k`
- `qwen35-9b-q4km`

I verified:

- all `12/12` `response.txt` files from failed run vs clean rerun have the same SHA-256 hash
- compile errors match across runs
- aggregate Qwen score is unchanged across runs: `13` total points in both runs

Per-model old vs rerun summary:

| Model | Failed run | Clean rerun | Note |
|---|---|---|---|
| `qwen35-35b-mxfp4` | `0,0,0` | `0,0,0` | identical |
| `qwen36-35b-q5km` | `6,0,0` | `7,0,0` | same output; one flaky test flipped |
| `qwen35-35b-q6k` | `0,7,0` | `0,6,0` | same output; one flaky test flipped |
| `qwen35-9b-q4km` | `0,0,0` | `0,0,0` | identical |
| `qwen3-coder-30b-draft` | invalid / empty | excluded | no trustworthy score |

The 1-point deltas for `qwen36-35b-q5km` and `qwen35-35b-q6k` came from `TestLongOutage/BoundedBuffer` flipping pass/fail on byte-identical responses. That is grader/runtime nondeterminism on borderline solutions, not model drift.

## Qwen family deep dive

### Aggregate outcome

Across the four scored Qwen models in the clean rerun:

- 12 total cells
- 10 compile failures
- 2 scored cells
- best Qwen score: `7/13`
- mean score across all Qwen cells: `1.08`

By contrast:

- Gemma locals averaged `4.89` with a best of `11/13`
- `gpt-oss` averaged `5.33` with a best of `11/13`

### The main reason Qwen did badly: compile-surface failures

Most Qwen runs failed before the grader could meaningfully evaluate resilience semantics.

#### `qwen35-35b-mxfp4`

All three seeds compile-failed.

Representative failures:

- duplicate method definitions (`flushBuffer` declared twice)
- treating `*InverterData` like `Metric`
- using nonexistent fields like `m.Fields`
- negating an `error` as if it were a `bool`

Verified examples:

- `../../artifacts/results/exam_v3/qwen35-35b-mxfp4/seed42/scraper.go:139-170`
- `../../artifacts/results/exam_v3/qwen35-35b-mxfp4/seed42/scraper.go:251-262`
- `../../artifacts/results/exam_v3/qwen35-35b-mxfp4/seed123/test.log`
- `../../artifacts/results/exam_v3/qwen35-35b-mxfp4/seed456/test.log`

#### `qwen36-35b-q5km`

This was the strongest local Qwen, but still only one viable seed.

- `seed42`: compiled, scored `7/13`
- `seed123`: pointer/value confusion compile fail
- `seed456`: pointer/value confusion compile fail

Verified examples:

- `../../artifacts/results/exam_v3/qwen36-35b-q5km/seed123/test.log`
- `../../artifacts/results/exam_v3/qwen36-35b-q5km/seed456/test.log`

#### `qwen35-35b-q6k`

- `seed42`: compile fail from half-implemented timeout vars (`readCtx`, `writeCtx` unused)
- `seed123`: compiled, scored `6/13` in the rerun
- `seed456`: invented APIs and duplicated methods

Verified examples:

- `../../artifacts/results/exam_v3/qwen35-35b-q6k/seed42/test.log`
- `../../artifacts/results/exam_v3/qwen35-35b-q6k/seed456/test.log`

#### `qwen35-9b-q4km`

This one collapsed completely.

Representative failures:

- undefined `sync`
- mid-file `import "sync"`
- local-variable chaos / unused vars / `:=` misuse

Verified example:

- `../../artifacts/results/exam_v3/qwen35-9b-q4km/seed123/response.txt:344`

### When Qwen did compile, it still implemented the outage contract poorly

The two viable Qwen outputs were:

- `qwen36-35b-q5km` seed42 -> `7/13`
- `qwen35-35b-q6k` seed123 -> `6/13`

Shared pattern:

- passed: read loop, cancellation, some load/hang behavior
- failed: validation, no-loss transitions, short outages, full flush, eviction behavior

This means the compiled Qwen solutions were not random garbage. They understood the general architecture, but they did not preserve the intended data-loss semantics.

Concrete evidence:

#### qwen3.6 partial solution

`../../artifacts/results/exam_v3/qwen36-35b-q5km/seed42/scraper.go:93-147`

Behavior:

- keeps a simple in-memory buffer
- flushes the oldest buffered metric in a background goroutine
- drops oldest when full
- buffers on sink write error

This is plausible but still not enough to satisfy the grader's transition/no-loss expectations.

#### qwen3.5 q6k partial solution

`../../artifacts/results/exam_v3/qwen35-35b-q6k/seed123/scraper.go:137-180`

Behavior:

- uses a queue as the buffer
- drops oldest/current metric when the queue is under pressure
- drops metrics on failed write in the write loop

That directly explains the failures in the no-loss tests.

## What we learned

### 1. The failed unattended run was operationally bad, but not numerically misleading for the scored Qwen models

This is the most important correction.

The batch management failure was real. The Qwen underperformance was also real.

### 2. `exam_v3` is harsher on compile discipline than `exam_v2`

The Qwen family looked much better on `exam_v2`. On `exam_v3`, the dominant failure mode is not subtle semantic mistakes; it is writing invalid Go or drifting away from the skeleton/types.

### 3. `qwen3.6` underperformed expectations on this exact local stack

This was surprising. The best local Qwen was `qwen36-35b-q5km`, but its ceiling here was `7/13`, well below both Gemma 26B variants at `11/13`.

### 4. `qwen3.5` was much worse than expected

Especially the 35B MXFP4 variant: `0/13` on all three seeds, with stable, reproducible compile failures.

### 5. `qwen3-coder-draft` remains unresolved

This report should not be read as evidence that the model itself is bad.

What is verified is narrower:

- initial `llama-swap` startup handling was broken for it
- startup health timeout was fixed
- even after that, exam generation still timed out at `10m` and `15m`
- reducing the draft variant to `32k` context did not rescue it

So the current local serving configuration for `qwen3-coder-draft` is not benchmark-ready.

### 6. The harness itself still has likely spec/grader issues

Two patterns showed up repeatedly across strong models too:

- `TestNewScraperValidation`
- `TestLongOutage/EvictionNotContiguous`

That does not explain the bulk of the Qwen compile failures, but it does explain why many otherwise good runs topped out at `11/13` instead of `13/13`.

## Interpretation of the Qwen surprise

My current interpretation is intentionally narrow:

These results do **not** prove that the Qwen family is worse than Gemma in general.

They do show that, on this exact setup:

- current local `llama.cpp` / `llama-swap` stack
- current Unsloth GGUF snapshots
- current quant choices
- current prompt / harness / seeds

...the Qwen family underperformed badly, mostly because of compile-surface instability.

That makes provider / quantization / packaging a live hypothesis, not cope.

The evidence for that hypothesis is:

- all tested Qwen locals here came from Unsloth GGUF snapshots in `config.yaml`
- `qwen3.6` still lost badly despite being the strongest Qwen in the set
- `qwen3.5` failed much more catastrophically than expected
- the failures are mostly invalid-Go / wrong-interface failures, exactly the kind of thing that can move with provider quantization, tokenizer/template packaging, or runtime behavior

## Recommended next experiments

Reasonable next steps, in order:

1. **Try alternative Qwen GGUF providers / quants**
   - especially for `qwen3.5-35b` and `qwen3.6-35b`
   - current evidence supports testing whether the problem is partly the current Unsloth GGUF stack
2. **Run non-draft `qwen3-coder-30b`**
   - the draft variant is still infra-blocked, so it is not a fair datapoint
3. **Keep the same harness and seeds when comparing providers**
   - we already know the Qwen outputs are stable across reruns on the current stack; that makes provider comparison cleaner
4. **Treat hosted references as context only unless rerun on seeds 42/123/456**
   - current hosted scores are seed1 only

If alternate Qwen providers are available in GGUF form, trying them is justified. The current report is exactly the kind of evidence that says “the model family may not be the whole story; this local packaging/runtime may be part of it.”
