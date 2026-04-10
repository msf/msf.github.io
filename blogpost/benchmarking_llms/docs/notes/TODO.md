# llama play project — TODO

Active issues and deferred work. Update as things move.

## Blocker — exam_v2 harness architecture

Current harness is slow, flaky, and timing-sensitive. See ../plans/SUBAGENT_EVAL_REVIEW_BRIEF.md
for context. Before running any more benchmarks:

1. Run the subagent review with that brief.
2. Act on its verdict. Expect to rewrite most of `../../bench/exam_v2/harness/`.
3. Re-score saved responses against new harness.

Until this is unblocked, further model benchmarking is throwing money at a
broken measuring stick.

## TODO — Pareto sweep (aborted 2026-04-18, user pulled plug at ~7h in)

Why: original goal was to compare tokens/sec vs exam quality across model/quant/draft
variants and find the Pareto frontier for coding use. The session got consumed
by harness bug-fixing instead.

**Precondition:** harness must be fast and deterministic first (see blocker).
Re-running the sweep against the current slow harness wastes hours.

### Cells to run (from the original plan)

Baseline already on disk or re-scored:
- Qwen3.6-35B Q5_K_M, thinking off, no draft (have responses, re-score after harness fix)
- Qwen3.6-35B Q5_K_M, thinking on,  no draft (have responses, re-score after harness fix)
- Gemma4-26B   MXFP4,  thinking off, no draft (have responses, re-score after harness fix)

Not yet tested:
- Qwen3.6-35B Q5_K_M, thinking off, **draft: Qwen3-0.6B Q8** (on disk)
- Qwen3.6-35B Q5_K_M, thinking on,  **draft: Qwen3-0.6B Q8**
- Qwen3.6-35B MXFP4,  thinking off, no draft              (**download: ~21.7 GB from unsloth/Qwen3.6-35B-A3B-GGUF**)
- Qwen3.6-35B MXFP4,  thinking on,  no draft
- Qwen3.6-35B MXFP4,  thinking on,  draft: Qwen3-0.6B Q8   (only if MXFP4-nodraft compiles on v2)
- Gemma4-26B   MXFP4, thinking off, draft: Gemma4-E4B Q8   (draft oversize but only Gemma4-compatible one on disk; may slow things down rather than speed up — worth measuring once)
- Gemma4-26B   MXFP4, thinking off, draft: Gemma4-E2B      (smaller if it exists on HF; requires download; verify compatibility first)

Explicitly OUT of scope per user:
- gpt-oss-20b (user dealbreaker on "legalese/self-censoring")

### Protocol per cell

- 2 exams (v1, v2) × 3 seeds (42, 123, 456)
- temp/top_p/etc per Qwen3.6 docs for Qwen3.6 runs, defaults for Gemma4
- Record: score, compile rate, tps, prompt tokens, output tokens, wall time
- Record draft acceptance rate from llama-server logs (for drafted variants)

### Autonomous pruning rules

- If a drafted variant runs slower (lower tps) than no-draft counterpart on
  exam v1 seed 42: skip remaining drafted seeds, log "draft hurt" data point.
- If a quant has 0/3 compile rate on exam v2: skip remaining v2 seeds.
- If llama-server logs show <10% draft acceptance: drop that draft pairing.
- Hard stop at pre-declared wall clock, write partial results.

### Time-box rules (learned the hard way)

- Harness fixes: max 1h. If more needed, stop and escalate.
- Downloads: max 1h total across all new models. Skip downloads if over.
- Each cell: max 15 min wall time. Kill and mark timeout otherwise.
- Reporting: always reserve last 30 min for writing up partial results.

## TODO — llama-server settings review across config.yaml

Still deferred (was already parked in PARKING_LOT.md). See that file for items.
Best done AFTER agentic harness exists (Phase 2), so we can actually test
"does ctx=64k help vs 32k" with something more realistic than exam_v2.

## TODO — Agentic / pi-lean benchmark harness

See PHASE2_AGENTIC_HARNESS_DESIGN.md. Needs user input: 2-3 real task sources
(repos, logs, manifests the user actually debugged). Blocked on that input.

Note: design doc was written before we learned the exam_v2 harness is a mess.
Lessons from that failure should flow back into Phase 2 design:
- Keep harness synchronous and deterministic wherever possible.
- Don't use real HTTP / subprocess / timing when a function call would do.
- If parallelism is needed, parallelize ACROSS cells, not WITHIN a cell.

## TODO — Ryzen 8500G fallback server

Deferred per PARKING_LOT.md. Phase 3 at earliest.

## TODO — Publish corrected blog numbers

Blog post at `blog.mfilipe.eu/blogpost/local-llm-coding-harder-test.html`
reports exam v2 scores that are now known-wrong due to harness bugs:

- BufferBounded was impossible to pass for fast scrapers (bug 2).
- FlushOnReconnect false-passed on scrapers with slow flush intervals (same bug,
  opposite direction).
- Silent skip of RaceDetector penalized models that had otherwise clean code.

New harness (once redesigned) gives materially different rankings. Once the
harness redesign lands and all existing responses are re-scored, either:
- Publish an "errata / we were wrong" post, OR
- Update the original post in place with a changelog at the top.

User to decide which. Don't edit blog text unattended.

## Cleanup

- `/mnt/ai-models/huggingface/hub/models--ggml-org--gemma-3-1b-it-GGUF` — partial download, remove.
- `/mnt/ai-models/huggingface/hub/models--unsloth--Nemotron-3-Nano-30B-A3B-GGUF` — partial download, remove.

## Lessons from 2026-04-17/18 unattended sessions

1. **"I have 9 hours" is a ceiling, not a budget.** Each phase needs its own
   cap. Watchdog kills the current phase when exceeded. Not doing this cost
   a full night.
2. **When you're fixing bugs in a measuring stick, stop and ask if the
   measuring stick itself is the problem.** 3+ failed attempts at fixing the
   same "bug" is the signal. Seven hours of harness patching produced a more
   correct but still bad harness.
3. **Test infrastructure is code.** Review it like code. Ask simplicity
   questions: does this need subprocesses? Does this need real HTTP? Does
   this need timing?
4. **Unattended sessions need a planning checkpoint in the middle.** At 50%
   time budget, I should have stopped, reviewed progress vs plan, and either
   continued or aborted. I didn't, and the sunk cost kept me pushing.

## References / other docs

- `../plans/SUBAGENT_EVAL_REVIEW_BRIEF.md` — hand to a fresh subagent to get a harness redesign proposal
- `HARNESS_AUDIT.md` — detailed list of 7 bugs found in the current harness
- `../reports/QWEN36_PHASE1_RESULTS.md` — post-sweep report (numbers are since-changed
  after harness fixes; summary narrative still correct)
- `PHASE2_AGENTIC_HARNESS_DESIGN.md` — draft design for pi-lean/agentic benchmark
- `PARKING_LOT.md` — longer-term deferred items
- `LOCAL_LLM_CODING_BENCHMARKS.md` — canonical benchmark notes (pre-Qwen3.6, pre-harness-fix)
- `../../artifacts/history/runtime-framework13/unattended-qwen36-sweep.log` — raw sweep output from 2026-04-17 run
