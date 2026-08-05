# agentic — bounded tool-loop harness

_Built 2026-08-03. Driver + task validated. **No model has completed a run yet.**_

Measures what `exam_v1`/`exam_v3` can't: whether a model can drive a tool loop —
explore, form a hypothesis, act, read feedback, stop. Outcome is scored by a
verifier's exit code, not by the trajectory.

## Layout

```
agent-driver.go              # the loop. companion to ../exam-driver.go
tasks/bug-hunt-01/
  prompt.md                  # what the model is told
  repo/                      # frozen snapshot, copied to a temp sandbox per run
    scraper.go               #   skeleton: types, interfaces, HTTP impls, main
    resilient.go             #   implementation, with one injected defect
    grader_test.go           #   13-test spec, identical to bench/exam_v3's
    go.mod
  solution/resilient.go      # reference fix, NOT visible to the model
  verify.sh                  # scoring: exit 0 = passed
```

## Tool surface — four tools, no shell

`list_files(dir)`, `read_file(path)`, `write_file(path, content)`, `run_tests()`.

No `bash` on purpose: a shell tool makes the trace unbounded and lets a model
score by accident. `run_tests` runs `go test -race ./...` in the sandbox and is
the only feedback signal. Paths are confined to the sandbox (`resolve()`).

## Caps (exceeding any = recorded failure, not a crash)

20 turns · 20 min wall · token budget · 4096 tokens per completion.
`stop_reason` in `result.json` says which one fired.

## The defect in bug-hunt-01

`enqueue` evicts from the front of the buffer instead of at random, so a long
outage leaves a contiguous run of the newest metrics. Grader catches it as
`TestLongOutage/EvictionNotContiguous` — **12/13, exactly one failure**, and the
message names the pattern ("eviction appears LIFO (kept newest)"), so the model
has a real signal to work from.

## Verifier is anti-cheat, and that was tested

`verify.sh` restores pristine `grader_test.go` + `go.mod` over the agent's tree
before grading, and rejects any added `*_test.go`. Validated in four directions:

| case | result |
|---|---|
| repo as shipped | FAIL |
| correct fix applied | PASS |
| agent neuters `grader_test.go` | FAIL |
| agent adds `extra_test.go` | FAIL |

## Run it

Needs an OpenAI-compatible endpoint with **tool-calling** and **≥32k context**.
8192 is not enough — reading `grader_test.go` alone puts the transcript over it.

```bash
cd ~/play/msf.github.io/blogpost/benchmarking_llms/agentic

go run ./agent-driver.go \
  -task tasks/bug-hunt-01 \
  -endpoint http://127.0.0.1:8090 \
  -out ../artifacts/results/agentic \
  -seed 42 -max-turns 20 -max-tokens 60000 -timeout 20m \
  <model-alias>
```

Against llama-swap on this laptop, `<model-alias>` is one of `gemma4-26b-qat-mtp`,
`qwen36-moe`, etc. (`curl -s localhost:8090/v1/models | jq -r '.data[].id'`).
For the ROCmFPX container path, see `../docs/notes/ROCMFPX_QWOPUS_SETUP.md` §6 —
raise `-c` to 65536.

Outputs to `../artifacts/results/agentic/<task>/<model>/seed<N>/`:
`result.json`, `transcript.log`, `tool_calls.json`, `verify_output`.

## Do this first, before any ROCmFPX number

Run **`gemma4-26b-qat-mtp`** (current best local) as the baseline. If Gemma can't
pass, the task is mis-scoped and no other model's score means anything.

Then 3 seeds (42, 123, 456) per model. Aggregate: `pass_rate`, `median_turns` on
passes only, `tool_error_rate = tool_errors/turns`.

## Known-good so far

- Pristine repo (reference solution): 13/13.
- Shipped repo: 12/13, one failure, deterministic.
- Tool-calling confirmed working on `ACE-SABER 35B-A3B` ROCmFP4 via the container
  — emitted a clean `list_files({"dir":"."})`.
- One aborted run (my `-c 8192` misconfiguration, not a model result): the model
  made 5 well-formed tool calls, 0 tool errors, then blew the context. Results
  deleted rather than kept as a data point.

## Next task (not built)

`k8s-debug-01` from `../docs/notes/PHASE2_AGENTIC_HARNESS_DESIGN.md`: broken
manifest + pod logs + `describe`, read-only, keyword-matched root cause. Only
after `bug-hunt-01` is stable across two models.
