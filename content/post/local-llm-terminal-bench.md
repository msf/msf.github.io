---
title: "Terminal-Bench 2.1 on a consumer GPU: what is a little machine actually worth?"
date: 2026-08-13T18:00:00+01:00
---

*August 2026*[^1]

*Part 6/6 — **Part 6** ← [Part 5](https://blog.mfilipe.eu/post/local-llm-dense-models-r9700/) ← [Part 4](https://blog.mfilipe.eu/post/benchmarking_llms-v3-rebuild/) ← [Part 3](https://blog.mfilipe.eu/post/local-llm-coding-harder-test/) ← [Part 2](https://blog.mfilipe.eu/post/local-llm-performance-framework13/) ← [Part 1](https://blog.mfilipe.eu/post/benchmarking-local-llms-go-coding/)*

[^1]: Co-authored with Claude Opus 5.

## Why an external benchmark

[Part 5](https://blog.mfilipe.eu/post/local-llm-dense-models-r9700/) ended with
my own exam failing as a measuring stick. So this part swaps it for one I did
not write: Terminal-Bench 2.1, a real benchmark that other people build, review
and publish numbers against.

That changes what the question is. It is no longer "which model wins my little
Go exam", it is the thing I have actually been circling since post #1:

**My hardware is a small consumer machine. How much value can I really get out
of it, and what should I be running on it?**

A public benchmark is the only way to answer that honestly, because it puts my
box on the same axis as everyone else's numbers. If a model scores 37% here and
the published frontier numbers are far above it, that gap is the price of
running locally — and it is worth knowing the size of that price rather than
guessing at it.

The R9700 is also what makes this possible at all. Known open-source benchmarks
are usually not run by individuals because they take too long; at eleven times
the throughput of the laptop, a suite that needs many hours per model becomes
affordable. Such suites also carry a maturity a personal exam does not have — in
the tasks, in the methodology, and in the harness and tooling around them — and
adopting one ends the open-ended work of re-inventing an LLM testing harness
that was flawed anyway.

One qualification, since it applies to everything below. External does not mean
rigorous. Most published benchmarks, Terminal-Bench included, run one attempt
per task and report the pass rate as though it were a property of the model.
Seeds are rarely varied, and the resulting variance is neither measured nor
reported. What Part 5's exam_v3 table says about my own exam, it also says about
theirs; it is simply not usually stated.

## Terminal-Bench 2.1

[Terminal-Bench](https://www.tbench.ai/) 2.1 is a suite of 89 tasks, each solved
inside a terminal. Three things recommended it:

- The unit of work is a container and a goal rather than a prompt. The model
  installs, configures, builds, debugs and verifies; almost none of the tasks
  are "write a function".
- It is known, so the numbers are comparable to other people's numbers.
- Nobody publishes it for quantized local models. I want to test different
  quants and get a real feel for how they perform, instead of the
  full-precision published numbers — in the spirit of what Donato Capitella
  does for AMD Strix Halo
  ([kyuz0.github.io/amd-strix-halo-toolboxes](https://kyuz0.github.io/amd-strix-halo-toolboxes/))
  and what [pi-local-coding-bench.dev](https://pi-local-coding-bench.dev/)
  publishes: public, reproducible runs across models and quantizations.

The task mix is not exam_v3's, and it is not a description of my own work
either. There is no Go anywhere in the 89 tasks. Python appears in at least nine
of the twenty tasks used here; the remainder is SQL, JavaScript, R, C, a
hardware description language, and one COBOL program to be re-implemented.
Several are frankly exotic: differential cryptanalysis of a FEAL cipher,
cross-compiling Doom for MIPS. What the tasks share is not subject matter but
shape — multi-step work in a real environment, where the model has to check its
own progress. The shape is what is being tested here; the subject matter is
whatever the benchmark happens to contain.

### How a trial works

Three pieces. **Harbor** (0.20.0) is the test harness: it fetches the tasks,
builds a container per task, schedules the trials and collects the results.
**terminus-2** is the agent — the loop that opens a tmux session inside the
container, shows the model the task and the current terminal contents, and types
back whatever keys the model asks for. The **model** itself sits behind an
OpenAI-compatible endpoint, so llama-swap drops straight in.

The loop runs until the model declares itself finished or the task's timeout
fires. Scoring is separate: a verifier script runs against the container
afterwards and writes a 1 or a 0. That file is the source of truth, not the
agent's exit — a task can hit its timeout and still be scored as solved.

## Setup

- **Host:** `hopper` — AMD Ryzen 5 8500G, 32 GB DDR5, Debian 13 (trixie),
  kernel 7.0.9.
- **GPU:** Radeon AI PRO R9700 (Navi 48), 32 GiB VRAM, Vulkan/RADV.
- **Runtime:** llama.cpp b10025 behind llama-swap v211, configured as a single
  exclusive group so exactly one model is resident at a time. Harbor's first
  request to a different model name triggers the swap.
- **Context and thinking:** `-c 131072 --parallel 1 --reasoning-budget 8192`.
  Thinking on for every arm, capped at 8192 tokens per turn, `--predict 32768`
  bounding the whole turn.
- **KV cache:** `q8_0` for both K and V, flash attention on.

Six models, five of them Unsloth GGUFs with speculative decoding via
multi-token prediction, plus Meta's own build of Muse Glimmer:

| name | weights | quant | file | samplers |
|---|---|---|---|---|
| `qwen-35b-moe-mtp` | Qwen3.6-35B-A3B | UD-Q4_K_XL | 22 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `qwen-27b-mtp` | Qwen3.6-27B | UD-Q4_K_XL | 17 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `qwen-27b-mtp-q6` | Qwen3.6-27B | UD-Q6_K_XL | 25 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `gemma-31b-qat` | Gemma 4 31B QAT | UD-Q4_K_XL | 17 GB | temp 1.0 / top_p 0.95 / top_k 64 |
| `gemma-26b-moe` | Gemma 4 26B-A4B QAT | UD-Q4_K_XL | 14 GB | temp 1.0 / top_p 0.95 / top_k 64 |
| `muse-glimmer-30b` | Muse Glimmer 30B | kquant-dynamic | 18 GB | temp 1.0 / top_p 0.95 / top_k 64 |

Samplers are each vendor's published defaults, not tuned here. Qwen's MTP head
is embedded in the GGUF; Gemma 4's is a separate ~280 MB drafter file passed
with `--model-draft`, which landed in llama.cpp as the `GEMMA4_ASSISTANT` arch
in PR #23398.

Muse Glimmer is the odd one out on three counts. Its quant is Meta's own
"dynamic" build rather than an Unsloth one, sized by the vendor for a 32 GB
card and measured here at 18.8 GB of VRAM at full 131k context — three of every
four of its 52 layers use a fixed 2048-token sliding window, so context is
close to free. Its drafter is DFlash, a block-diffusion model that proposes 16
tokens per forward pass, not an MTP head; it needs `--spec-type draft-dflash`,
and passing the drafter file alone silently does nothing because `--spec-type`
defaults to `none`. And it is the one model here whose reasoning cannot be
switched off — the chat template opens the thinking channel unconditionally, so
only the strength (`low`/`medium`/`high`/`xhigh`, default `high`) and the token
budget are controllable.

## 20 tasks out of 89

The full suite extrapolates to roughly 33 hours per model. Six models is over a
week of GPU time, so the sweep runs on a subset.

The first subset was 12 tasks picked by hand. `qwen-35b-moe-mtp` scored 10/12 —
83%. On inspection the selection skewed toward short agent timeouts and the
easy tier, because those are the tasks that are quick to eyeball. It was
discarded: hand-picking the subset repeats the problem with writing the exam,
one level up.

The replacement, `strat20`, is drawn by a script:

- proportional to the real 89-task difficulty mix, giving **1 easy / 12 medium /
  7 hard**;
- random *within* each tier, seed 42;
- materialized as a directory of symlinks into the task cache, so Harbor takes
  it as one `-p` argument.

Same model, same settings, new subset: 8/20 instead of 10/12. Being generated
rather than curated, it rebuilds byte-identically:

```
scripts/tb21-make-subset.py --name strat20 --size 20 --seed 42
```

The 20 tasks are listed in full in the [appendix](#appendix-the-20-tasks).

## Running it

One attempt per task, one trial at a time, so only one model is ever resident on
the GPU. Getting Harbor to run hit a few snags; the specifics and the exact
invocation are in the
[run report](https://github.com/msf/msf.github.io/blob/main/blogpost/benchmarking_llms/docs/reports/EXAM_V4_2026-08-09_TB21.md).

## Results

Tier weights: **easy = 1, medium = 2, hard = 3**, so `strat20`'s maximum is
`1×1 + 12×2 + 7×3 = 46` points.

![exam_v4 weighted scores: qwen-35b-moe-mtp 17/46, qwen-27b-mtp 14/46, gemma-31b-qat 12/46, muse-glimmer-30b 10/46, qwen-27b-mtp-q6 9/46, gemma-26b-moe 7/46, split by difficulty tier](/images/exam-v4/exam-v4-scores.svg)

| model | easy | medium | hard | score | tasks | runtime | per task |
|---|---|---|---|---|---|---|---|
| `qwen-35b-moe-mtp` | 1/1 | 5/12 | 2/7 | **17/46** (37%) | 8/20 | 6h08m | 18m |
| `qwen-27b-mtp` | 1/1 | 5/12 | 1/7 | **14/46** (30%) | 7/20 | 7h15m | 21m |
| `gemma-31b-qat` | 1/1 | 4/12 | 1/7 | **12/46** (26%) | 6/20 | 7h36m | 22m |
| `muse-glimmer-30b` | 1/1 | 3/12 | 1/7 | **10/46** (22%) | 5/20 | 6h42m | 20m |
| `qwen-27b-mtp-q6` | 1/1 | 4/12 | 0/7 | **9/46** (20%) | 5/20 | 7h13m | 21m |
| `gemma-26b-moe` | 1/1 | 3/12 | 0/7 | **7/46** (15%) | 4/20 | 8h10m | 24m |

No model clears 40%. On a subset drawn to match the real difficulty mix the
hard tier is nearly untouched: five solves across 42 model-task pairs.

### The sixth row

Muse Glimmer 30B arrived after the rest and is the reason this table has six
rows. It is Meta's first dense 30B, published on 9 August and built explicitly
for local agentic work, which makes it the most on-topic model here. One
caveat: its architecture needs llama.cpp b10353 or newer, against b10025 for
the other five, so this is the single row that is not strictly like-for-like.

It lands mid-pack. The two-point gap to `gemma-31b-qat` is one task, which is
inside the noise this design admits, so the honest reading is "fourth, roughly
tied for third" rather than a clean ranking.

The more interesting number is the one that does not appear in the table. Meta
publish **51.7%** for this model on Terminal-Bench 2.1 with terminus-2 — the
same benchmark and the same agent. This run scored 22%. The run was clean:
nothing truncated, no agent errors, no evictions, ~39 tokens/s sustained across
the whole 6h42m, so the gap is not a broken harness. What differs is unmeasured.
`strat20` is 20 hard-weighted tasks rather than the full 89. Thinking is capped
at 8192 tokens per turn on every arm here. And the model ran at its default
reasoning strength of `high` rather than the `xhigh` its own card recommends for
agentic work — the cheapest of the three to test, and untested so far.

Publishing a number less than half the vendor's own is worth stating plainly
rather than smoothing over: on this subset, under this harness, with these
caps, it scored 22%. Whether that is the model or the setup is genuinely open.

### Wall-clock cost

![Wall-clock runtime per model: 6h08m to 8h10m for 20 tasks, with the slowest arm solving the fewest tasks and Muse Glimmer breaking the pattern by being fast and mid-scoring](/images/exam-v4/exam-v4-runtime.svg)

The six arms took 43h06m of GPU time for 120 model-task attempts, and the
ordering is broadly inverted: the slowest arm is the weakest model. A solved
task finishes in about 220 seconds; a failed one burns its entire agent timeout.
The runtime column is therefore mostly a failure counter rather than a speed
measurement. The 20 tasks sum to 630 minutes of agent timeout, the ceiling a
model that failed everything would reach: 10h30m.

Muse Glimmer is the exception that shows where the rule stops. It solved 5 tasks
— fewer than `gemma-31b-qat`'s 6 and `qwen-27b-mtp`'s 7 — yet finished faster
than both, second-quickest overall at 6h42m.

The obvious explanation would be that it decodes faster. It does not.

![Decode and prefill throughput per model measured during the exam runs: MoE models around 110-118 tokens/s decode, dense models 42-49, with prefill from 345 to 1256 prompt tokens/s](/images/exam-v4/exam-v4-throughput-by-model.svg)

Sampled during the runs themselves, Muse decodes at 42.3 tokens/s against
`gemma-31b-qat`'s 46.4 — slightly *slower*, on comparable prefill. The MoE
models are in a different class again at 110–118, which is the clearest thing
this figure shows: active-parameter count, not total size, sets decode speed.

What separates Muse is how much it says. It emitted 17,900 completion tokens per
task against Gemma's 26,700 — about a third fewer — while taking nearly three
times as many turns to do it, 33 per task against 12. That is roughly 540 tokens
per turn versus 2,200: many short commands, look at the result, another short
command, where Gemma writes a long block and then inspects. Fewer tokens at a
similar rate is less time, even though the turn count is higher.

So the runtime column is not purely a failure counter. It is closer to *total
tokens generated* divided by decode rate, and a model's turn-taking style moves
that as much as its score does.

This is why the subset exists at 20 rather than 89. It also means the cost of
the benchmark is set mostly by the timeouts of the tasks a model fails, so a
weaker model is usually more expensive to evaluate than a stronger one — unless,
like Muse, it is terse enough to lose in fewer words.

### Per task

| task | tier | 35B MoE | 27B | 31B QAT | Muse | 27B Q6 | 26B MoE |
|---|---|---|---|---|---|---|---|
| `cobol-modernization` | easy | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `break-filter-js-from-html` | medium | — | — | — | — | — | — |
| `caffe-cifar-10` | medium | ✅ | — | — | — | — | — |
| `chess-best-move` | medium | — | — | — | — | — | — |
| `compile-compcert` | medium | — | — | — | — | — | — |
| `distribution-search` | medium | ✅ | ✅ | — | ✅ | ✅ | — |
| `dna-insert` | medium | — | — | — | — | — | — |
| `filter-js-from-html` | medium | — | — | — | — | — | — |
| `nginx-request-logging` | medium | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| `portfolio-optimization` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `query-optimize` | medium | — | ✅ | ✅ | — | — | — |
| `rstan-to-pystan` | medium | — | — | — | — | — | — |
| `vulnerable-secret` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `bn-fit-modify` | hard | — | ✅ | ✅ | ✅ | — | — |
| `cancel-async-tasks` | hard | ✅ | — | — | — | — | — |
| `circuit-fibsqrt` | hard | — | — | — | — | — | — |
| `feal-differential-cryptanalysis` | hard | — | — | — | — | — | — |
| `feal-linear-cryptanalysis` | hard | — | — | — | — | — | — |
| `make-doom-for-mips` | hard | — | — | — | — | — | — |
| `model-extraction-relu-logits` | hard | ✅ | — | — | — | — | — |

Three tasks were solved by every model. Ten were solved by none.

`nginx-request-logging` is the reason the first number is three rather than
four: it was the one task every one of the original five solved, and Muse
Glimmer is the only model to have missed it. Meanwhile Muse solved
`bn-fit-modify`, a hard task that both `qwen-27b-mtp-q6` and `gemma-26b-moe`
failed. The overlap between models is much less orderly than the score column
suggests — being better on aggregate does not mean solving a superset.

## Things worth writing down

**Q4 and Q6 of the same checkpoint agree on 18 of 20 tasks.** `qwen-27b-mtp` and
`qwen-27b-mtp-q6` are the same Qwen3.6-27B-MTP weights at two quantization
levels, identical samplers, context, and drafter depth. The two tasks they
disagree on — `bn-fit-modify` and `query-optimize` — were both solved by Q4 and
not by Q6. Wall time was 7h15m against 7h13m. Q6 holds 6.7 GiB more weights in
VRAM, leaving about 4 GiB of headroom instead of 11. On this evidence the extra
bits buy nothing, subject to the sample-size caveat below.

**n=20, one attempt each.** Differences of one or two tasks are inside the
sampling noise of this design, and the whole six-model spread is ten points
wide. The noise floor of this design has not been measured, and the exam_v3
result in [Part 5](https://blog.mfilipe.eu/post/local-llm-dense-models-r9700/) is a
reminder of what that omission can hide. The ranking should be
read with that in mind.

**One trial was thrown out and re-run.** Partway through the `gemma-26b-moe`
job, an unrelated cron job on the same host asked llama-swap for a different
model, evicting the resident one four times between 08:00 and 08:05 UTC. The
task in flight, `break-filter-js-from-html`, was re-run on its own and merged
in. The other four jobs were checked against the swap log and were clean.

**The v3 ranking and the v4 ranking are not the same ranking, and now there are
numbers for it.** On this same host, [Part 5](https://blog.mfilipe.eu/post/local-llm-dense-models-r9700/)'s
exam_v3 run scored `gemma-31b-qat` at a median 12/13 over five seeds — its
strongest result there. It sits third here. Running
Muse Glimmer on both exams gave five models with a score on each, enough to
actually check instead of assert:

| model | exam_v3 median /13 | rank | exam_v4 /46 | rank |
|---|---:|---:|---:|---:|
| `gemma-31b-qat` | 12 | 1 | 12 | 3 |
| `muse-glimmer-30b` | 8 | 2 | 10 | 4 |
| `qwen-35b-moe-mtp` | 7 | 3 | 17 | 1 |
| `gemma-26b-moe` | 5 | 4 | 7 | 5 |
| `qwen-27b-mtp` | 0 | 5 | 14 | 2 |

Spearman's rank correlation is **−0.10**: no relationship. `qwen-27b-mtp` is the
clearest case — last on v3, where it failed to produce compilable Go in three of
five attempts, and second on v4, where it gets a terminal and can read its own
error messages. One unused import is a zero on v3 and a non-event on v4, because
the compiler tells the model and the model fixes it.

The honest caveat is that five models with ±6-point seed spread on the v3 side
cannot establish a *negative* correlation either; −0.10 is indistinguishable
from zero at this sample size. The defensible claim is narrower: there is no
evidence that one-shot code synthesis predicts agentic performance, and the
cases where they disagree disagree very loudly.

**Some hard tasks may be out of reach for reasons other than capability.** The
appendix lists each task's agent timeout beside the benchmark's own estimate of
how long an expert would need. `circuit-fibsqrt` allows 60 minutes for work
estimated at 16 hours. Failing that is not obviously a statement about the
model.

## What is next

- Run `strat20` more than once per model, so the noise floor is measured rather
  than assumed. This comes first, for the reasons given at the top.
- Run the full 89 tasks on at least one model, to find out how much the subset
  distorts.
- Look at the trajectories of the seven tasks nobody solved, which is the part a
  pass/fail number throws away.

## Appendix: the 20 tasks

`strat20`, drawn from Terminal-Bench 2.1 with seed 42. Descriptions and
timeouts are the benchmark's own, read from each task's `task.toml`. "Expert"
is Terminal-Bench's estimate of how long a human expert would take, in minutes.

| task | tier | agent timeout | expert | what it asks |
|---|---|---:|---:|---|
| `cobol-modernization` | easy | 15m | 20 | Reverse-engineer a COBOL program's business logic and reimplement it in Python with exact output reproduction. |
| `break-filter-js-from-html` | medium | 20m | 20 | Bypass an HTML sanitization filter by crafting malicious HTML that triggers JavaScript execution after filtering. |
| `caffe-cifar-10` | medium | 60m | — | Install and configure BVLC Caffe 1.0.0, train a CNN on CIFAR-10 for exactly 500 iterations CPU-only, hit accuracy thresholds. |
| `chess-best-move` | medium | 15m | 45 | Analyze a chess position from an image, use an engine to find the best move(s), handle multiple valid solutions. |
| `compile-compcert` | medium | 40m | 60 | Build the CompCert verified C compiler from source with correct configuration for the host architecture and dependencies. |
| `distribution-search` | medium | 60m | 120 | Find a probability distribution satisfying precise dual KL divergence constraints through numerical optimization. |
| `dna-insert` | medium | 30m | 30 | Design PCR primers for site-directed mutagenesis under molecular-biology constraints on primer length and melting temperature. |
| `filter-js-from-html` | medium | 30m | 45 | Build a robust XSS filter that strips JavaScript from HTML while preserving legitimate structure and content. |
| `nginx-request-logging` | medium | 15m | 20 | Install and configure Nginx with advanced request logging, rate limiting, and custom error pages. |
| `portfolio-optimization` | medium | 60m | 120 | Write a C extension for Python doing portfolio risk/return calculations ≥1.2× faster than pure Python, without losing accuracy. |
| `query-optimize` | medium | 15m | 60 | Rewrite a slow SQL query with correlated subqueries using CTEs and window functions, preserving exact output. |
| `rstan-to-pystan` | medium | 30m | 180 | Convert an RStan Gaussian Process script to equivalent PyStan 3.10.0, including install, hyperparameter mapping, and numerical verification. |
| `vulnerable-secret` | medium | 15m | 20 | Analyze a binary, exploit a buffer overflow to bypass authentication, extract a hidden flag. |
| `bn-fit-modify` | hard | 60m | 480 | Recover a Bayesian Network DAG from data, perform causal interventions, sample from the modified network. |
| `cancel-async-tasks` | hard | 15m | 120 | Implement async task concurrency control with correct cleanup on cancellation, including the queued-task edge case. |
| `circuit-fibsqrt` | hard | 60m | 960 | Implement Fibonacci-of-integer-square-root using only combinational and sequential logic gates in an HDL format. |
| `feal-differential-cryptanalysis` | hard | 30m | 480 | Implement differential cryptanalysis on a FEAL-like cipher to recover a round key via chosen-plaintext attacks. |
| `feal-linear-cryptanalysis` | hard | 30m | 960 | Perform linear cryptanalysis on a FEAL-like cipher to recover keys from known plaintext-ciphertext pairs. |
| `make-doom-for-mips` | hard | 15m | 480 | Cross-compile the DOOM engine for MIPS with an LLVM toolchain and verify execution in a JavaScript emulator. |
| `model-extraction-relu-logits` | hard | 15m | 480 | Extract hidden-layer weights from a black-box ReLU network by querying outputs and locating neuron activation critical points. |

Agent timeouts sum to 630 minutes. Expert estimates sum to 4,700 — about 78
hours — and that excludes `caffe-cifar-10`, which carries no estimate.

---

Full method, per-model llama-server flags, job index, and reproduction steps:
[`docs/reports/EXAM_V4_2026-08-09_TB21.md`](https://github.com/msf/msf.github.io/blob/main/blogpost/benchmarking_llms/docs/reports/EXAM_V4_2026-08-09_TB21.md).
The exam_v3 re-run behind Part 5:
[`docs/reports/EXAM_V3_2026-08-07_R9700.md`](https://github.com/msf/msf.github.io/blob/main/blogpost/benchmarking_llms/docs/reports/EXAM_V3_2026-08-07_R9700.md).
Charts are generated by
[`scripts/make-post-charts.py`](https://github.com/msf/msf.github.io/blob/main/blogpost/benchmarking_llms/scripts/make-post-charts.py).
