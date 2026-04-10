# Phase 2 — Agentic harness design (draft, for user review)

Status: design only. Not built. Not run.
Author: pi (unattended). Date: 2026-04-17.

## Why

User feedback, verbatim:
> "the 16k context is a synthetic bulshit setting, in real world I will always
> need to allow a context of 64k or 128k.. I think testing for real pi-lean
> use (or pretend openclaw) is a real validation of the quality of the model..
> today the plateau is on RL and tuning for agentic usecases, the real gain
> is on using the models with tools, not just chatting w/ them.. my tests are
> too synthetic."

Translation: exam_v1/v2 measure single-shot code generation. They don't measure
what the user actually uses local LLMs for — pi-lean / OpenCode-style sessions
where the model reads files, runs commands, iterates. Qwen3.6's release notes
explicitly target that axis (tool calling, preserve_thinking, "developer role").

## Core tension

Agentic traces are non-reproducible. A tool call either hits or doesn't. The
whole trajectory diverges. The 3-seed / compile-rate methodology we use for
the exams does not port.

Two ways out, both useful:

1. **Frozen-task replay**: pre-recorded task with a frozen repo snapshot, the
   harness replays the same inputs, model produces tool calls, harness
   simulates/executes them, we score on outcome not trajectory.
2. **Real-world telemetry**: instrument pi-lean itself, log every session, mine
   logs for patterns (tool-call error rate, turn count, completion rate per
   model). No synthetic tasks; just observe the user's own work.

Option 2 is cheaper but requires multi-week data collection to compare models.
Option 1 is more up-front work but answers the question today.

**Proposal: Option 1 primary, Option 2 passive in parallel.**

## Task shape (the hard design question)

Tasks need to be:
- Self-contained: one repo snapshot, no external API or LLM calls
- Reasonably short: 5-20 turns, 30 min max wall time per attempt
- Deterministically scorable: a script says pass/fail at the end
- Representative: matches user's actual work (SRE/Go/systems)
- Resilient to model creativity: multiple correct solutions should all pass

**Candidate task categories** (need user to pick 3-5):

### A. Bug hunt (read-heavy, narrow fix)
Repo: a small Go project with a known bug (e.g., data race, off-by-one, wrong
error wrap). Model must read, find, fix. Score: `go test ./...` passes
post-fix, pre-fix it failed.

### B. Feature add (read + write, broader scope)
Existing Go repo + a written spec ("add CLI flag X that does Y"). Model must
understand codebase, add the feature, pass existing tests + new tests provided
by the harness.

### C. k8s/config debug (read-only diagnosis, no code change)
Provide a broken k8s manifest + pod logs + `kubectl describe` output. Model
must identify the root cause. Score: model's final answer matches a known
root-cause string (keyword match, e.g. "resource limits too low", "imagepullbackoff").

### D. Log investigation (read-heavy, output-is-answer)
Provide a bundle of logs from a multi-service failure. Model must identify
which service failed first and why. Score: keyword match on final summary.

### E. Refactor (write-heavy, tests must still pass)
Existing Go package + instruction ("extract this to an interface, keep all
tests passing"). Score: `go test ./...` still passes.

**Recommended starter set:** A + B + C. Skip E (subjective), skip D (too much
like exam v2 but worse signal).

## Harness shape

```
agentic-bench/
  tasks/
    bug-hunt-01/
      repo/           # git-frozen snapshot
      setup.sh        # prepare repo (e.g., `git checkout broken-state`)
      verify.sh       # scoring script, exits 0/1
      prompt.md       # task instructions for model
    feature-add-01/ ...
    k8s-debug-01/ ...
  harness/
    pi-driver.go      # wraps pi-lean invocation, captures session
    simulator.go      # OR: a minimal MCP-like tool server that emulates
                      # filesystem + bash + one domain tool
  runner.sh           # iterate tasks × models × N attempts
  results/
    {task}/{model}/attempt{N}/
      transcript.log
      tool_calls.json
      verify_output
      result.json
```

## Metrics (what we actually record)

Per attempt:
- `passed`: bool (from verify.sh exit code)
- `turns`: int (tool-call rounds)
- `total_tokens`: int
- `thinking_tokens`: int (if thinking on)
- `wall_s`: float
- `tool_errors`: int (malformed tool calls, wrong schema, hallucinated tools)
- `retry_count`: int (same failed tool call repeated)

Aggregated per (task, model):
- `pass_rate` over N attempts
- `median_turns` on passed attempts only
- `token_efficiency = passed ? total_tokens : null` (cheaper = better when both pass)
- `tool_error_rate = tool_errors / turns`

## Why this is NOT the exam harness with more turns

The exam scores compile+test. That's a quality signal. The agentic harness
adds orthogonal signals that the exam can't capture:
1. **Can the model stop**: an exam prompt is once-and-done. An agent has to
   decide when it's done. Qwen3.5 famously doesn't stop.
2. **Can the model plan**: an exam gives you the whole task in one prompt.
   An agent has to sequence actions, remember what it did, not go in loops.
3. **Can the model use tools correctly**: the exam has no tools. Agents
   live or die on tool-call schema adherence.

## Open questions (blocking build)

1. **Which 3 tasks?** User needs to accept/provide.
2. **What tool surface?** pi-lean with its real tools (read/bash/edit), or a
   restricted synthetic tool set the harness provides (MCP-mock)?
3. **How many attempts per (task, model)?** Token budget says 2. Statistical
   signal says 5. Compromise: 3.
4. **Do we need a separate "local Gemma4 agentic baseline"** before running
   Qwen3.6 through this? Probably yes — if Gemma4 can't do a task at all in
   agentic mode, that reframes our expectations for Qwen3.6.

## Next actions (NOT this session)

1. Get user to nominate 2-3 real task sources (repos, logs, manifests they
   actually debugged recently).
2. Build task `bug-hunt-01` end-to-end with one model (Gemma4 as baseline).
3. Wire verify.sh scoring, iterate until it's reproducible.
4. Add Qwen3.6. Then other models.
5. Decide: pi-lean wrapper vs. synthetic MCP tool surface.

## What this session produces

- This design doc. That's it for Phase 2.
- Phase 1 results (exam v1/v2) go in a separate report.
