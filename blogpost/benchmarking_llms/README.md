# benchmarking_llms

Canonical versioned workspace for the local-LLM benchmarking lab.

This directory is the source of truth for:
- benchmark prompts, harnesses, and evaluators
- sweep/download/install scripts
- experiment notes, reports, and plans
- preserved local artifacts and historical outputs

It is intentionally separate from the runtime workspace at `~/play/llama`, which stays machine-local and non-git because it contains binaries, configs, caches, and other host-specific setup.

## Layout

- `bench/` — benchmark definitions and harnesses (`exam_v1`, `exam_v2`, `exam_v3`)
- `scripts/` — sweep runners plus legacy bench/exam scripts and download helpers
- `docs/notes/` — durable notes and audits
- `docs/reports/` — experiment reports
- `docs/plans/` — review briefs and run plans
- `machines/` — concrete machine definitions and plans
- `exam-driver.go` — canonical Go driver used by the sweep scripts
- `artifacts/results/` — current local sweep outputs
- `artifacts/logs/` — current local sweep logs
- `artifacts/history/` — preserved historical outputs from older layouts

## Runtime split

Keep these concerns separate:

- `~/play/llama` — runtime only: llama.cpp releases, llama-swap, rendered config, local logs, HF cache references
- `blogpost/benchmarking_llms/` — versioned source, docs, and preserved artifacts
- sibling blog posts in `blogpost/*.md` — public writeups that link back to this lab

## Preservation policy

Historical artifacts are evidence. Some no longer line up perfectly with the current code or layout; keep them anyway.

- `artifacts/history/` is archival. Do not rewrite or “clean up” in place.
- New runs belong under `artifacts/results/` and `artifacts/logs/`.
- If you relocate a known artifact root, preserve provenance and document the new location clearly.

## First places to read

- `AGENTS.md`
- `docs/notes/LOCAL_LLM_CODING_BENCHMARKS.md`
- `docs/notes/LOCAL_LLM_PERFORMANCE.md`
- `docs/reports/QWEN36_PHASE1_RESULTS.md`
- `bench/exam_v3/REPORT.md`
- `machines/framework13.md`
