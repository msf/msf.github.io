# Local LLM benchmark lab — agent context

## Purpose

This directory is the canonical versioned workspace for local-LLM experiments, benchmark harnesses, notes, and preserved artifacts.

Keep it separate from the machine-local runtime at `~/play/llama`.

## Canonical layout

- `bench/` — benchmark prompts, harnesses, graders
- `scripts/` — sweep/download/install scripts
- `docs/notes/` — durable notes and audits
- `docs/reports/` — experiment reports
- `docs/plans/` — planning / review briefs
- `machines/` — concrete machine env files and plans
- `exam-driver.go` — canonical Go driver for llama-swap runs
- `artifacts/results/` — current run outputs
- `artifacts/logs/` — current run logs
- `artifacts/history/` — preserved historical outputs from older layouts

## Preservation rules

1. Treat `artifacts/history/` as immutable evidence unless the task is explicitly archival migration.
2. Preserve provenance when moving artifacts. If a known old path is in active use, keep a compatibility path or document the relocation clearly.
3. Do not delete ambiguous logs/results/code just because they no longer match the current harness.
4. New sweep outputs go to `artifacts/results/` and `artifacts/logs/`, not mixed into source directories.
5. Machine-specific runtime defaults belong under `machines/`, not scattered through notes or scripts.

## Runtime split

- `~/play/llama` — binaries, llama-swap, host-specific config, caches, runtime-only logs
- `~/play/msf.github.io/blogpost/benchmarking_llms` — source, docs, archival metadata, preserved artifacts

If you need the serving config, check `~/play/llama/config.yaml`.
If you need the benchmark driver, use `./exam-driver.go` here.

## Current benchmark summary

- Best quality on this box: Gemma 4 26B-A4B
- Best value: gpt-oss-20b
- Qwen3.5 is fast but flaky under local quantization
- Concrete current-machine plan: `machines/framework13.md`

Detailed tables live in:
- `docs/notes/LOCAL_LLM_CODING_BENCHMARKS.md`
- `bench/exam_v3/REPORT.md`
- `docs/reports/QWEN36_PHASE1_RESULTS.md`

## Known drift to verify before running anything heavy

- `scripts/sweep.sh` is a historical exam_v1/exam_v2 runner. Verify its swap-key mappings against the current `~/play/llama/config.yaml` before trusting it.
- `artifacts/history/` contains runs from multiple layouts and harness generations. Do not assume every referenced path maps to the current tree.

## Working conventions

- Prefer relative paths inside this repo.
- For user-facing docs, distinguish clearly between canonical source paths and archived artifact paths.
- For new notes, favor `docs/notes/` or `docs/reports/` over dumping markdown in the root.
