---
title: "Rebuilding the coding benchmark: Exam V2 → V3"
date: 2026-05-07T22:59:29+01:00
---

# Rebuilding the coding benchmark: Exam V2 → V3

*Part 4/4 — **Part 4** ← [Part 3](https://blog.mfilipe.eu/post/local-llm-coding-harder-test/) ← [Part 2](https://blog.mfilipe.eu/post/local-llm-performance-framework13/) ← [Part 1](https://blog.mfilipe.eu/post/benchmarking-local-llms-go-coding/)*

*May 2026 — co-authored with Gemma 4*

The transition from Exam V2 to V3 was necessitated by the discovery that the V2 harness was providing invalid scoring data, masking model instability and quantization failures.

## Setup
- **Hardware**: Framework 13 (Ryzen AI 370HX, 64GB DDR5).
- **Inference**: `llama-swap` via Vulkan. 
- **KV Cache**: `q8_0` (fixed across all runs).
- **Environment**: Go-based scraper resilience task (buffering, eviction, background flush).

## The Problem: V2 Harness Flaws
The Exam V2 harness relied on shell scripts and `grep`-based evaluation. This "vibecoded" approach introduced three critical failures:

1. **Denominator Inflation**: When a model's code panicked and crashed the `go test` suite, the harness only counted the tests that had successfully run. This allowed models that crashed early to achieve 100% scores (e.g., `2/2` instead of `2/10`).
2. **Masked Instability**: The loose scoring made it impossible to distinguish between a "bad" model and an "unstable" one. We saw high variance in Qwen and GPT-OSS across seeds that the harness failed to flag as high-uncertainty.
3. **Fragile Parsing**: Using `grep` to check for behavioral markers was prone to false positives/negatives, especially when models hallucinated markers or failed to follow formatting.

**Trigger for rebuild**: High-tier models (Qwen 3.6) were producing lower scores than lower-tier models, indicating the measuring stick was broken.

## The Solution: Exam V3
V3 is a complete rewrite of the evaluation logic, moving from shell-based orchestration to an in-process Go grader.

- **In-process execution**: The grader runs the tests directly, eliminating shell-parsing errors.
- **Deterministic scoring**: Uses `go test -race -json` and `jq` to parse results.
- **Fixed Denominator**: Every run is measured against the full test suite (13 tests), regardless of whether the process crashes.
- **Performance**: Evaluation time per submission dropped from **~60s to ~4s**.

## Results (Clean Rerun)
*Note: All models were re-run to ensure a clean baseline on the new harness.*

| Model | Best (Seed) | Avg (3 Seeds) | Notes |
| :--- | :---: | :---: | :--- |
| **Gemma 4 26B (MXFP4 + KV:Q8)** | **11/13** | **7.33** | Most stable; consistent across seeds. |
| **GPT-OSS 20B (MXFP4 + KV:Q8)** | **11/13** | **5.33** | High variance: `5 / 0 / 11`. |
| **Qwen 3.6 35B (Q5_K_M + KV:Q8)** | **7/13** | **2.33** | Strongest Qwen; highly seed-dependent. |
| **Qwen 3.5 35B (Q6_K + KV:Q8)** | **6/13** | **2.00** | Significant performance drop. |
| **Qwen 3.5 35B (MXFP4 + KV:Q8)** | **0/13** | **0.00** | Consistent compile failure. |

## Key Findings
- **Quantization Instability**: The `0/13` score for Qwen 3.5 MXFP4 suggests the current Unsloth GGUF stack/quantization is fundamentally broken for this task.
- **Seed Variance is Real**: The `5/0/11` split for GPT-OSS highlights that single-seed benchmarks are useless for assessing coding reliability.
- **In-process is mandatory**: The speed and accuracy gains of V3 make the shell-based V2 approach untenable for scaling.

## What we got wrong
We assumed a shell-script wrapper was sufficient for complex behavioral testing. We prioritized ease of implementation over deterministic measurement, which led to the "denominator inflation" bug.

## Next Steps
- **Provider Audit**: Test alternative Qwen GGUF providers to see if the MXFP4 failure is a model issue or a packaging/quantization issue.
- **Phase 2 (Agentic Harness)**: Move away from synthetic Go tasks toward evaluating models on real-world tasks (analyzing actual repos, logs, and manifests).

***

**Footnote:**
- Still surprised Qwen 3.6 didn't score better; Gemma 4 totally kicked butt on this test.
- *vibecoded on hermes + gemma4 models*
