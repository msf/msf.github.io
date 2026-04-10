# Subagent task: code-review and simplicity review of exam_v2 evaluation

## The goal this evaluation is supposed to serve

We benchmark local LLMs on a code-modification task. The exam prompt asks the model to modify `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/scraper.go` (a ~208 line Go metrics scraper) to add **buffered resilience during network outages**: a fixed-size in-memory buffer, random eviction when full, and a background-goroutine flush on reconnect. The spec is in `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/prompt.txt`.

The model writes a modified Go program. We want to **programmatically grade the submission** fast, reliably, and fairly across different model outputs.

## What's broken right now (why this review exists)

- A full evaluation of ONE submission takes ~30-90 seconds. With 3 seeds × ~10 configs, sweeps take hours.
- The harness uses: subprocess launches, real HTTP client/server, goroutines, `time.Sleep` calls of 5-15 seconds, `go test -race` rebuilds, and "polling with convergence detection."
- Tests are timing-sensitive and flake under CPU load.
- A human reviewer says: "This shouldn't be compute intensive. CPUs are idle. Each test of ~200 lines of code takes too long individually. This smells like AI slop on the test code itself."
- The previous pi session burned 7 hours patching 7 bugs in this harness without questioning whether the design was the problem.

## Your job

Fresh, critical review. **Assume the current design is wrong.** Your output should propose a simpler replacement, not defend the existing code.

## Files to read (in this order)

1. `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/prompt.txt` — the task given to the LLM. Understand what correct model output looks like.
2. `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/scraper.go` — the original 208-line scraper. The model receives this and modifies it.
3. `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/eval.sh` — the current orchestrator. ~100 lines of bash.
4. `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/harness/harness_test.go` — the actual test suite. ~600 lines of Go. Read carefully; this is the main source of complexity.
5. `~/play/msf.github.io/blogpost/benchmarking_llms/bench/exam_v2/mock/main.go` — the mock HTTP server (inverter + sink + control plane).
6. `~/play/msf.github.io/blogpost/benchmarking_llms/docs/notes/HARNESS_AUDIT.md` — previous bug list, for context on what's already been patched.
7. One or two saved submissions as reference for what model outputs actually look like:
   - `~/play/msf.github.io/blogpost/benchmarking_llms/artifacts/results/exam_v2/qwen36-35b-q5km-thinkon/seed42/response.txt` (a good one)
   - `~/play/msf.github.io/blogpost/benchmarking_llms/artifacts/results/exam_v2/qwen36-35b-q5km-thinkoff/seed42/response.txt` (has `#START` markers)

## Questions to answer (be direct, not diplomatic)

1. **Is the architecture justified?** The current design: compile model's Go code → run as subprocess → launch real HTTP mock → orchestrate online/offline state via HTTP POST → poll HTTP counters with timing heuristics. Is this architecture justified by the requirements, or is most of it incidental complexity?

2. **What could it be instead?** Propose at least one radically simpler alternative. Examples to consider (not exhaustive):
   - Static analysis of the submission (AST-walk for required patterns) — rejected previously as "grep-scoring inflates results," but is there a middle ground?
   - Import the model's code as a Go package/type, instantiate the `ResilientSink` (or whatever the model named it) directly, call `Write()` in a tight loop with a `Sink` stub — no subprocess, no HTTP, no sleep.
   - Extract the model's `BufferedSink` type via `go/parser` and unit-test it with an in-process fake sink.
   - A hybrid: one quick in-process behavioral test for most properties, keep subprocess only for what needs it (signal handling, -race).
   
3. **What's actually slow?** Profile mentally: which specific parts of the current test are the real time sinks?
   - The 5s offline sleep + 5s flush window in TestScenario?
   - Multiple `startHarness` calls (each spawns mock + scraper, waits for port ready)?
   - `go test -race` build in TestRaceDetector (compiles the entire 200-line program with race instrumentation)?
   - The retry-once logic in eval.sh when harness flakes?
   
4. **What's the minimum set of behaviors to verify?** The spec in prompt.txt asks for 3 things: (A) buffer during outage, (B) random eviction when full, (C) background flush on reconnect. Plus the constraint that existing interfaces are preserved and scraping continues during outages.
   - Are 10 tests really needed, or are we testing implementation details?
   - Which tests give signal, which are noise? Be opinionated.
   
5. **What's the brittleness audit?** The harness uses `-h` output parsing to find flag names (`-interval` vs `-scrape-interval`, `duration` vs `int`). It makes tolerance assumptions (bufSize+5, bufSize-2). It uses a 5s wait window. Each of these is a coupling point between harness and model behavior. List them and say which should be design invariants (fix the spec) vs tolerances (fix the test).

6. **Is the prompt itself good?** Read prompt.txt. Does it over-specify details that force a particular implementation, or under-specify in ways that make different correct implementations fail different tests? Propose prompt edits if the ambiguity is the source of test brittleness.

## Constraints on your output

- **Do not edit any code yet.** This is review-only. Propose changes; don't apply them.
- **Do not run the existing tests.** Reading the code is enough to form opinions.
- **Be concrete.** "Use a better design" is useless. "Move TestScenario, FlushOnReconnect, BufferBounded, EvictionRandom into a single 1-second in-process test that uses reflection to find the ResilientSink type, instantiates it with a FakeSink stub, writes N metrics while stub is offline, then flips stub to online and asserts on stub.Received() — skips HTTP, skips subprocess, skips timing heuristics" is useful.
- **Short output.** One page max. Assume the reader knows Go well.

## Output format

```markdown
## Verdict
(One sentence: is the current design appropriate for the task?)

## What's the real problem
(2-4 bullet points on root causes, not symptoms)

## Proposed redesign
(Concrete alternative, with named types, functions, and time estimate of per-evaluation run)

## Prompt changes (if any)
(Quote the current prompt, propose diff)

## Migration plan
(Ordered list of steps to go from current to proposed, each with approximate effort)

## Open questions for the human
(Things that need a design decision)
```
