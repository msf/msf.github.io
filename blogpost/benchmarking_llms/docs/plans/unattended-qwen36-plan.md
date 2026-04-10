# Unattended Qwen3.6 exam run — plan + scope
Started: 2026-04-17 20:25 WEST
User instruction: "go unattended, I trust your judgement, I need to cook dinner now"
Refinement: "run the phase exam at context 32k for all"

## Scope (this session)

PHASE 1 only:
- Download Qwen3.6-35B-A3B UD-Q5_K_M (~26.5 GB)
- Add config entries for Qwen3.6 (think-off and think-on, both 32k)
- Add config entry for Gemma4-26B-MXFP4 at 32k (new, existing 16k entry stays)
- Run Exam v1 + v2, 3 seeds each:
  - qwen36-35b-q5km-think-off (32k)
  - qwen36-35b-q5km-think-on  (32k, budget 8192)
  - gemma4-26b-mxfp4-32k      (32k, think-off)
- Report numbers, update LOCAL_LLM_CODING_BENCHMARKS.md

## Explicitly NOT done (deferred, parked)

- gpt-oss-20b re-run (user dealbreaker on legalese/self-censoring)
- Other models' 32k context regressions
- llama-server settings review across config.yaml
- Qwen3.6 MXFP4 quant (only after Q5 looks good)
- Qwen3.5 retune with more context + thinking-on
- pi-lean / OpenCode-style agentic harness build
- Ryzen 8500G fallback server deployment

## Guardrails

- No git pushes, no deletions of results, no touching unrelated config entries
- Stop if smoke test fails (arch probably `qwen35moe`, same family, b8708 should support)
- Stop if disk < 30 GB free
- 10 min timeout per generation (sweep.sh default)
- Each run writes to `results/{exam}/{model}/seed{N}/result.json`; no clobbering existing data
