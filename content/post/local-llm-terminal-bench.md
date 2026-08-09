---
title: "From one-shot prompts to a shell: running Terminal-Bench 2.1 on local models"
date: 2026-08-09T18:00:00+01:00
---

*August 2026*[^1]

*Part 5/5 — **Part 5** ← [Part 4](https://blog.mfilipe.eu/post/benchmarking_llms-v3-rebuild/) ← [Part 3](https://blog.mfilipe.eu/post/local-llm-coding-harder-test/) ← [Part 2](https://blog.mfilipe.eu/post/local-llm-performance-framework13/) ← [Part 1](https://blog.mfilipe.eu/post/benchmarking-local-llms-go-coding/)*

[^1]: Co-authored with Claude Opus 5.

[Part 4](https://blog.mfilipe.eu/post/benchmarking_llms-v3-rebuild/) ended with a
next step: *"move away from synthetic Go tasks toward evaluating models on
real-world tasks."* This post is that move. It describes what the new exam is,
how it runs, and what the first sweep produced. It is a description of an
exploration, not a verdict on which model anyone should use.

Everything here ran on one machine over about two days, with one model resident
on the GPU at a time.

## What was wrong with the old exam

Exams v1 through v3 all have the same shape. A prompt goes in, one file of Go
comes out, a test binary scores it. v3 made the scoring honest — real `go test
-race -json` execution instead of grepping the source for `sync.Mutex` — but the
shape never changed.

That shape measures one thing: whether a model can emit correct code in a single
turn with no feedback. It does not measure whether a model can read an error
message, try something else, inspect the filesystem, or notice it is in a loop.
Nothing in v3 gives the model a second chance at anything.

There is also a practical problem. Writing the tasks yourself means grading your
own homework — the exam drifts toward what you already thought was hard, and
scores are not comparable to anything outside your own repo.

## exam_v4: Terminal-Bench 2.1

[Terminal-Bench](https://www.tbench.ai/) is an external benchmark of tasks that
are solved in a terminal. Version 2.1 has 89 of them, each tagged easy, medium,
or hard.

The unit of work is a Docker container with a task set up inside it and a
verifier that decides afterwards whether the task was done. The model is not
asked to produce a file. It is given a goal and a shell, and it sends
keystrokes.

A trial looks like this:

1. Harbor builds the task's container.
2. The agent — `terminus-2` — opens a tmux session inside it.
3. The model receives the task description and the current terminal contents,
   and replies with JSON describing keys to press.
4. Those keys are typed into tmux. The new terminal output goes back to the
   model, truncated at 10,000 bytes.
5. Repeat until the model says it is finished, or the task's agent timeout
   fires.
6. The container is left alone and a separate verifier script runs against it,
   writing `1` or `0` to `verifier/reward.txt`.

Step 6 matters for reading results: the verifier runs even when the agent timed
out, so a task can raise `AgentTimeoutError` and still score 1. The reward file
is the source of truth, not the exception.

The tasks are genuinely varied — modernizing COBOL, configuring nginx request
logging, porting RStan to PyStan, compiling CompCert, cryptanalysis of FEAL,
building Doom for MIPS. Almost none of them are "write a function".

### What runs it

Two pieces beyond the model:

- **Harbor** (0.20.0) — the CLI that fetches tasks, builds containers,
  schedules trials, and collects results.
- **terminus-2** — the agent, i.e. the loop that turns model output into
  keystrokes. This is the part people usually mean by "harness". Harbor ships
  about thirty of them; `terminus-2` is the plain terminal one.

The model itself is reached over an OpenAI-compatible endpoint, so llama-swap on
`127.0.0.1:8090` drops straight in.

## Setup

- **Host:** `hopper` — AMD Ryzen 5 8500G, 32 GB DDR5, Debian 13 (trixie),
  kernel 7.0.9.
- **GPU:** Radeon AI PRO R9700 (Navi 48), 32 GiB VRAM, Vulkan/RADV.
- **Runtime:** llama.cpp b10025 behind llama-swap v211, configured as a single
  exclusive group so exactly one model is resident at a time. Harbor's first
  request to a different model name triggers the swap.
- **Context and thinking:** `-c 131072 --parallel 1 --reasoning-budget 8192`.
  Thinking is enabled on every arm, capped at 8192 tokens per turn, with
  `--predict 32768` bounding the whole turn.
- **KV cache:** `q8_0` for both K and V, flash attention on.

Five models, all Unsloth GGUFs, all with speculative decoding via
multi-token prediction:

| name | weights | quant | file | samplers |
|---|---|---|---|---|
| `qwen-35b-moe-mtp` | Qwen3.6-35B-A3B | UD-Q4_K_XL | 22 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `qwen-27b-mtp` | Qwen3.6-27B | UD-Q4_K_XL | 17 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `qwen-27b-mtp-q6` | Qwen3.6-27B | UD-Q6_K_XL | 25 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `gemma-31b-qat` | Gemma 4 31B QAT | UD-Q4_K_XL | 17 GB | temp 1.0 / top_p 0.95 / top_k 64 |
| `gemma-26b-moe` | Gemma 4 26B-A4B QAT | UD-Q4_K_XL | 14 GB | temp 1.0 / top_p 0.95 / top_k 64 |

Samplers are each vendor's published defaults, not tuned here. Qwen's MTP head
is embedded in the GGUF; Gemma 4's is a separate ~280 MB drafter file passed
with `--model-draft`, which landed in llama.cpp as the `GEMMA4_ASSISTANT` arch
in PR #23398.

## Picking 20 tasks out of 89

The full suite extrapolated to roughly 33 hours per model. Five models would be
a week of GPU time, so the sweep runs on a subset.

The first subset was 12 tasks picked by hand. `qwen-35b-moe-mtp` scored 10/12 —
83%. Looking at what had been picked, the selection skewed toward short agent
timeouts and toward the easy tier, because those are the tasks that are quick to
eyeball. It was thrown away.

The replacement, `strat20`, is drawn by a script instead:

- proportional to the real 89-task difficulty mix, which gives **1 easy /
  12 medium / 7 hard**;
- random *within* each tier, seed 42;
- materialized as a directory of symlinks into the task cache, so Harbor takes
  it as one `-p` argument.

Same model, same settings, new subset: 8/20 instead of 10/12. The generated set
is regenerated rather than curated, so it can be rebuilt byte-identically:

```
scripts/tb21-make-subset.py --name strat20 --size 20 --seed 42
```

## Running it

```
GIT_CONFIG_GLOBAL=/dev/null OPENAI_API_KEY=llama-swap-local \
harbor run \
  -a terminus-2 \
  -m openai/<model> \
  --ak api_base=http://127.0.0.1:8090/v1 \
  -n 1 -q \
  -o artifacts/results/exam_v4_tb21/jobs \
  -p artifacts/tb21-subsets/strat20
```

One attempt per task, one trial at a time.

Four things cost time to work out, all boring, all recorded so they cost nothing
next time:

- The `tb`/`harbor` CLI crashes on Python 3.14 — install with
  `uv tool install --python 3.12`.
- A global git `insteadOf` rule rewriting https to ssh breaks Harbor's dataset
  clone. `GIT_CONFIG_GLOBAL=/dev/null` on every invocation.
- `harbor download` writes relative to the working directory, so `-o` is
  required when the CWD is not writable.
- `OPENAI_API_KEY` must be set to *something*. llama-swap needs no auth, but
  litellm's `openai/` provider refuses to send a request with no key
  configured, and fails instantly with a credentials error.

## Scoring

Tier weights: **easy = 1, medium = 2, hard = 3**. `strat20`'s maximum is
`1×1 + 12×2 + 7×3 = 46` points.

A scoreboard script walks the job directories, reads every `reward.txt`, and
emits a markdown table plus a self-contained HTML page. It merges a model's jobs
per-task rather than per-job, so a single re-run trial updates one cell instead
of replacing a whole run.

## Results

| model | easy | medium | hard | score | tasks | runtime | per task |
|---|---|---|---|---|---|---|---|
| `qwen-35b-moe-mtp` | 1/1 | 5/12 | 2/7 | **17/46** (37%) | 8/20 | 6h08m | 18m |
| `qwen-27b-mtp` | 1/1 | 5/12 | 1/7 | **14/46** (30%) | 7/20 | 7h15m | 21m |
| `gemma-31b-qat` | 1/1 | 4/12 | 1/7 | **12/46** (26%) | 6/20 | 7h36m | 22m |
| `qwen-27b-mtp-q6` | 1/1 | 4/12 | 0/7 | **9/46** (20%) | 5/20 | 7h13m | 21m |
| `gemma-26b-moe` | 1/1 | 3/12 | 0/7 | **7/46** (15%) | 4/20 | 8h10m | 24m |

Total GPU time across the five arms: **36h24m**.

Per task:

| task | tier | 35B MoE | 27B | 31B QAT | 27B Q6 | 26B MoE |
|---|---|---|---|---|---|---|
| `cobol-modernization` | easy | ✅ | ✅ | ✅ | ✅ | ✅ |
| `break-filter-js-from-html` | medium | — | — | — | — | — |
| `caffe-cifar-10` | medium | ✅ | — | — | — | — |
| `chess-best-move` | medium | — | — | — | — | — |
| `compile-compcert` | medium | — | — | — | — | — |
| `distribution-search` | medium | ✅ | ✅ | — | ✅ | — |
| `dna-insert` | medium | — | — | — | — | — |
| `filter-js-from-html` | medium | — | — | — | — | — |
| `nginx-request-logging` | medium | ✅ | ✅ | ✅ | ✅ | ✅ |
| `portfolio-optimization` | medium | ✅ | ✅ | ✅ | ✅ | ✅ |
| `query-optimize` | medium | — | ✅ | ✅ | — | — |
| `rstan-to-pystan` | medium | — | — | — | — | — |
| `vulnerable-secret` | medium | ✅ | ✅ | ✅ | ✅ | ✅ |
| `bn-fit-modify` | hard | — | ✅ | ✅ | — | — |
| `cancel-async-tasks` | hard | ✅ | — | — | — | — |
| `circuit-fibsqrt` | hard | — | — | — | — | — |
| `feal-differential-cryptanalysis` | hard | — | — | — | — | — |
| `feal-linear-cryptanalysis` | hard | — | — | — | — | — |
| `make-doom-for-mips` | hard | — | — | — | — | — |
| `model-extraction-relu-logits` | hard | ✅ | — | — | — | — |

Four tasks were solved by every model. Seven were solved by none.

## Things worth writing down

**Runtime is a failure counter, not a speed measurement.** A solved task
finishes in about 220 seconds. A failed one burns its entire agent timeout. The
"per task" column therefore tracks pass rate inversely and says very little
about decode throughput.

**One trial was thrown out and re-run.** Partway through the `gemma-26b-moe`
job, an unrelated cron job on the same host asked llama-swap for a different
model, evicting the resident one four times between 08:00 and 08:05 UTC. The
task that was in flight, `break-filter-js-from-html`, was re-run on its own
afterwards and merged in. The other four jobs were checked against the swap log
and were clean.

**Q4 and Q6 of the same checkpoint agree on 18 of 20 tasks.**
`qwen-27b-mtp` and `qwen-27b-mtp-q6` are the same Qwen3.6-27B-MTP weights at two
quantization levels, with identical samplers, context, and drafter depth. The
two tasks they disagree on — `bn-fit-modify` and `query-optimize` — were both
solved by Q4 and not by Q6. Wall time was 7h15m against 7h13m. Q6 holds
6.7 GiB more weights in VRAM, leaving about 4 GiB of headroom instead of 11.

**n=20, one attempt each.** Differences of one or two tasks are inside the
sampling noise of this design. The whole five-model spread is ten tasks wide.

**The v3 ranking and the v4 ranking are not the same ranking.** On this same
host, exam_v3 scored `gemma-31b-qat` at a median 12/13 over five seeds, its
strongest result there; it sits third here. The two exams were not built to
measure the same thing, and no work has been done to calibrate one against the
other. Whether they correlate at all is an open question, not something this
sweep answers.

## What is next

- Run `strat20` more than once per model, so the noise floor is measured
  instead of asserted.
- Run the full 89 tasks on at least one model, to find out how much the subset
  distorts.
- Look at the trajectories of the seven tasks nobody solved, which is the part
  a pass/fail number throws away.

---

Full method, per-model llama-server flags, job index, and reproduction steps:
[`docs/reports/EXAM_V4_2026-08-09_TB21.md`](https://github.com/msf/msf.github.io/blob/main/blogpost/benchmarking_llms/docs/reports/EXAM_V4_2026-08-09_TB21.md).
