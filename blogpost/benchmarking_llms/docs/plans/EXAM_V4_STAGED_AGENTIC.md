# exam_v4 — staged agentic delivery (design, for a fresh session)

_Written 2026-08-06. Status: **design only, nothing built.** Deliberately deferred —
see "Why this was not the next thing" at the bottom._

## The idea

`exam_v3` hands a model the whole resilience task in one prompt and scores the
single artifact. `exam_v4` keeps the identical repo, identical grader, and
identical 13-point scale, but delivers the work as **four scoped requests in one
conversation against one sandbox** — the way a person actually uses an agent.

Framing given to the model on request 1 (user's words, lightly tightened):

> You are being asked to fix and extend this code across several requests.
> Certain interfaces must not change, because an evaluation test suite runs
> against your changes. This is request 1 of 4. `scraper.go` contains a metric
> scraper; identify and fix the bugs in `Run()`.

Then request 2 arrives in the same conversation: "First task complete. Second
task: …". And so on.

What this measures that neither existing exam can:

1. **Incremental construction** — can it build on its own prior work.
2. **Regression** — does request 4 break what request 2 achieved. This is the
   most realistic agentic failure mode and we currently have zero visibility
   into it.
3. **Scoping discipline** — does it stay inside the current ask or run ahead.

## Stages and their gates

Base fixture: `bench/exam_v3/scraper.go` as-is — the buggy `defaultScraper`
(busy-loop on `time.Sleep`, ignores `ctx`, crashes on nil `data` when the source
errors, drops metrics on write failure). No `resilient.go`; the model creates it
or edits in place.

| stage | ask | gate tests | pts |
|---|---|---|---:|
| 1 | Fix the bugs in `Run()`: honour context cancellation, don't crash when the source fails. | `TestNewScraperValidation`, `TestGracefulCancel` | 2 |
| 2 | Buffer during sink outages — nothing lost across short outages. | `TestReadsDuringOutage`, `TestNoLossAcrossTransitions`, `TestShortOutageNoLoss`, `TestMultipleShortOutagesNoLoss` | 4 |
| 3 | Bound the buffer at `maxBufSize`; choose an eviction policy for multi-hour outages. | `TestLongOutage/{BoundedBuffer,FullBufferFlushed,EvictionNotContiguous}` | 3 |
| 4 | Source and sink can *hang*, not just fail. Survive it under load. | `TestSurvivesUnderLoad`, `TestHangBehavior/{CancelDuringHungRead,CancelDuringHungWrite,ReadsProgressDespiteHungWrite}` | 4 |

2 + 4 + 3 + 4 = 13. Same `bench/exam_v3/grader_test.go`, unchanged, so exam_v4
scores are directly comparable to the exam_v3 one-shot table.

**Advance unconditionally.** Do not gate stage N+1 on stage N passing — a model
that flunks stage 2 would otherwise produce no data at all. Record whether each
stage's own gate passed at the transition, and move on.

## Scoring

Run the **full 13-test grader after every stage**, not just at the end. That
yields three numbers per attempt:

- **final score /13** — quality, quantitative, same scale as exam_v3.
- **total wall seconds for all four stages** — the time metric. Total time to
  solve the problem is what counts; do not report tok/s or pp-vs-tg here.
- **regressions** — count of stages where the score *dropped* vs the previous
  stage. A model that reaches 9/13 at stage 3 and falls to 6/13 at stage 4 broke
  its own earlier work. Neither existing exam can see this.

Keep per-stage wall seconds and turns too — "which stage did it fall apart on"
is the narrative the blog post needs.

## What to build

Reuse `agentic/agent-driver.go` wholesale — the tool loop, sandbox, caps, and
`harness_config` recording are already correct and parity-proven. Additions:

- `agentic/tasks/exam_v4/repo/` — pristine `bench/exam_v3/scraper.go` +
  `grader_test.go` + `go.mod`.
- `agentic/tasks/exam_v4/stages/{01,02,03,04}.md` — the four request texts.
- `agentic/tasks/exam_v4/grade.sh` — returns `{"score":N,"max":13,"per_test":{…}}`
  instead of an exit code. Must restore pristine `grader_test.go` + `go.mod`
  first and reject added `*_test.go`, exactly as `bug-hunt-01/verify.sh` does.
- driver: stage loop, per-stage grade + timing, regression detection, and a
  per-stage turn cap (suggest 8 turns / 15 min, 60 min per attempt).

Suggested cells: 3 models × 2 seeds. Models as in the exam_v3 comparison —
`rocmfp4-moe`, `qwen36-moe`, `gemma4-26b-qat-mtp`.

## Relationship to bug-hunt-01

Stage 1 *is* a bug hunt on the same repo, so `agentic/tasks/bug-hunt-01` becomes
redundant as a model-comparison cell. Keep it: it is the fixture that proves the
harness discriminates (broken → FAIL, reference → PASS, two cheat paths → FAIL),
and `scripts/sweep-agentic.sh` uses exactly that as its blocking preflight.

## Why this was not the next thing

Recorded so a fresh session does not relitigate it.

- Two agentic runs in one session were invalidated by harness bugs (a 4096
  per-call token cap; `reasoning_content` dropped). exam_v2 → v3 was rebuilt for
  the same class of reason. exam_v4 would be the third bespoke instrument in a
  year.
- The question that actually prompted all of this — *is the ROCmFP4 MoE worth
  running on this laptop* — is answerable **today** by `exam_v3` one-shot with
  zero new code, and is directly comparable to the already-published table.
- A home-grown, n=1-task, 2-seed score has no external calibration. Before
  investing further in bespoke harnesses, consider running an off-the-shelf
  benchmark with a public leaderboard against the same llama-swap endpoint for
  one externally-comparable datapoint. Aider's polyglot benchmark is the
  candidate (~225 Exercism tasks, 2 attempts with test feedback, OpenAI-compatible
  endpoint). **Unverified from memory — check its current shape before relying
  on it.**

So the ordering agreed with the user:

1. exam_v3 one-shot on the three models. ← done first, see
   `scripts/sweep-exam3-rocmfp4.sh`
2. Optionally an externally-calibrated benchmark on the winner.
3. exam_v4, as its own project, framed around the question worth answering:
   **does a tool loop beat one-shot on an identical grader?**

## Prior art in this repo, read before building

- `agentic/README.md` — tool surface, caps, the four-way verifier validation.
- `agentic/agent-driver.go` — the loop to extend.
- `docs/notes/PHASE2_AGENTIC_HARNESS_DESIGN.md` — the April design that started
  this, including task categories that were considered and dismissed
  (log-investigation: weak signal; refactor: subjective).
- `docs/plans/ROCMFPX_TEST_RUNBOOK.md` §Tier 3 — the tiering this slots into,
  including the never-built 3.0 tool-adherence gate.
- `bench/exam_v3/REPORT.md` — the one-shot numbers exam_v4 must be compared to.
