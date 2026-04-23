# exam_v3 — TODO for next session

workdirs that are relevant:
~/play/msf.github.io/blogpost/ 
~/play/llama

exam_v3 is a significant rework of exam_v2 (new interfaces, new prompt, new harness). Old blog-sweep results under `../results/exam_v2/` do NOT transfer; re-score everything.

## Scope for next session

1. Write `sweep-exam3.sh` (model the new script on the existing `...llama/sweep.sh` and `...llamma/sweep-qwen36.sh`, but target only `exam_v3`).
2. Run the sweep.
3. Review results.

Do not expand scope to blog writeup, prompt tweaks, or re-doing eval.sh. Those are separate phases.

## sweep-exam3.sh requirements

- Based on `../sweep.sh` (uses `exam-driver.go` at `/home/miguel/play/llama/exam-driver.go`, one model loaded at a time, results written to `../results/exam_v3/<display>/seed<N>/`).
- Only `EXAMS=(exam_v3)`.
- Use `SEEDS=(42 123 456)` (same convention as prior sweeps).
- `TIMEOUT=10m` default; `max-tokens=16384` default (all models in this sweep have thinking disabled, so the old `8192` thinkoff budget also works — pick one and document).
- Health-check llama-swap at `http://localhost:8080` before starting.
- After each model finishes all seeds: `POST /models/unload` and a short sleep.
- `result.json` existence skips the cell (resume-friendly).

## Model list for the sweep (in this order)

Display name (directory under `results/exam_v3/`) → llama-swap key:

| Order | Display | Swap key |
|---|---|---|
| 1 | `gemma4-26b-mxfp4` | `gemma4-26b-mxfp4-64k` |
| 2 | `qwen35-35b-mxfp4` | `qwen35-35b-mxfp4` |
| 3 | `qwen36-35b-q5km` | `qwen36-35b-q5km-thinkoff` |
| 4 | `gpt-oss` | `gpt-oss` |
| 5 | `qwen3-coder-30b-draft` | `qwen3-coder-draft` |
| 6 | `gemma4-e4b-q8` | `gemma4-e4b` |
| 7 | `qwen35-9b-q4km` | `qwen35-9b` |
| 8 | `gemma4-26b-q8` | `gemma4-26b-q8-32k` |
| 9 | `qwen35-35b-q6k` | `qwen35-35b-q6k` |

All are served with thinking disabled and ctx-size >= 32k (Q8 Gemma capped at 32k due to memory; rest at 64k). See `/home/miguel/play/llama/config.yaml` for current serving config.


## Hosted-model runs already complete (for comparison)

Runs already exist under `../results/exam_v3/`:

| Model | seed1 | seed42 | seed123 | seed456 |
|---|---|---|---|---|
| `gpt-54` | 9/13 | — | — | — |
| `sonnet-45` | 11/13 | — | — | — |
| reference-solution (hand-written, not shipped) | 13/13 | — | — | — |
| skeleton baseline | 1/13 | — | — | — |

These used seed1 for convenience. If you want direct comparison against the local sweep (which uses seeds 42/123/456), re-run `gpt-54` and `sonnet-45` at those seeds.

## Where to look for context

- **Harness / grader**: `./grader_test.go`, `./eval.sh`, `./scraper.go`, `./prompt.txt`, `./scraper_solution.go`.
- **Driver binary**: `/home/miguel/play/llama/exam-driver.go` (no rebuild needed; `go run` from the sweep).
- **Existing sweep scripts**: `../sweep.sh`, `../sweep-qwen36.sh`.
- **llama-swap config**: `/home/miguel/play/llama/config.yaml` (all 9 swap keys exist there).
- **Deprecation rationale**: `../exam_v2/DEPRECATED.md`.

## Grader behavior reminder

- `eval.sh` runs `go test -race -json -timeout 60s`. Requires `jq`.
- 13 scored leaf tests; parent aggregates `TestLongOutage` and `TestHangBehavior` are filtered out in scoring.
- Typical runtimes: correct submission ~3s, broken ~7s, compile-fail <1s.
- Per-submission output: `result.json` with `{"score","max","summary","observed"}`.

## Known grader gaps (from gpt-54 and sonnet-45 runs)

- Both models failed `TestLongOutage/EvictionNotContiguous` — both used FIFO. May indicate the prompt under-specifies that random eviction is what the grader rewards; flag for review after the sweep if near-universal failure.
- `TestNewScraperValidation` is a cheap gate; sonnet-45 missed it by copy-pasting the skeleton's buggy `NewScraper`.

## Git / branch state

- Branch: `exam-v3-rework` (not yet merged to main).
- Last commit: `exam_v3: new harness with in-process grader; deprecate exam_v2`.
- `results/` is gitignored — responses and scores are local-filesystem only.
- Before starting the sweep, check `git status`; don't accumulate uncommitted harness changes across the sweep run.

## Other

- `scraper_solution.go` in this directory is NOT shipped to models. It lives here for grader validation only. eval.sh pulls code from the response file; grader_test.go doesn't import the solution.
- Don't `rm -rf ../results/exam_v2/` — those are the frozen old-blog-post results. Leave them for historical reference.

## Overcoming issues
You're running unnatended, don't ask questions, solve your own problems, monitor for issues so that you can course correct.
