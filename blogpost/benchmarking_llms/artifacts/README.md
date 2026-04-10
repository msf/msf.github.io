# Artifacts

This directory holds non-versioned local outputs and preserved historical evidence.

## Current writable roots

- `results/` — active sweep outputs
- `logs/` — active sweep logs

## Archived roots and provenance

- `history/blogpost-bench_results/` — moved from `blogpost/bench_results`
- `history/play-bench_results/` — moved from `~/play/bench_results`
- `history/exam_v1-results-legacy/` — moved from `bench/exam_v1/results`
- `history/runtime-framework13/` — copied from the runtime workspace under `~/play/llama` / `/mnt/ai-models/llama`
- `logs/exam_v3/` — moved from the old `exam_v3/logs` location
- `results/` — moved from the old `blogpost/results` location

Some archived artifacts refer to old absolute paths or old harness layouts. That is expected. Preserve them as evidence.

## Rules

- Do not rewrite or prune `history/` casually.
- Put new results in `results/`, not `history/`.
- Put new logs in `logs/`, not beside benchmark source files.
