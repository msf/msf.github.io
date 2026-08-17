---
title: "Terminal-Bench 2.1 on a consumer GPU: what is a little machine actually worth?"
date: 2026-08-13T18:00:00+01:00
_build:
  list: never      # published, but not linked from the home page or RSS
  render: always
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
- **Runtime:** llama-swap v211, configured as a single exclusive group so
  exactly one model is resident at a time; Harbor's first request to a different
  model name triggers the swap. llama.cpp b10025 for the `strat20` runs,
  b10362 for `domain30` — the bump was forced by Muse Glimmer's architecture and
  every flag in use was re-checked against the new `--help` before switching.
- **Context and thinking:** `-c 131072 --parallel 1 --reasoning-budget 8192`.
  Thinking on for every arm, capped at 8192 tokens per turn, `--predict 32768`
  bounding the whole turn.
- **KV cache:** `q8_0` for both K and V, flash attention on.

Seven models, six of them Unsloth GGUFs with speculative decoding via
multi-token prediction, plus Meta's own build of Muse Glimmer. Every model here
runs with drafting on:

| name | weights | quant | file | samplers |
|---|---|---|---|---|
| `qwen36-35b-moe` | Qwen3.6-35B-A3B | UD-Q4_K_XL | 22 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `qwen36-27b` | Qwen3.6-27B | UD-Q4_K_XL | 17 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `qwen36-27b-q6` | Qwen3.6-27B | UD-Q6_K_XL | 25 GB | temp 0.7 / top_p 0.95 / top_k 20 |
| `qwen38-27b` | Qwen3.8-27B | UD-Q4_K_XL | 17 GB | temp 1.0 / top_p 0.95 / top_k 20 |
| `gemma-31b-qat` | Gemma 4 31B QAT | UD-Q4_K_XL | 17 GB | temp 1.0 / top_p 0.95 / top_k 64 |
| `gemma-26b-moe` | Gemma 4 26B-A4B QAT | UD-Q4_K_XL | 14 GB | temp 1.0 / top_p 0.95 / top_k 64 |
| `muse-glimmer-30b` | Muse Glimmer 30B | kquant-dynamic | 18 GB | temp 1.0 / top_p 0.95 / top_k 64 |

Samplers are each vendor's published defaults, not tuned here — Qwen3.8's are
Unsloth's *thinking-mode* set, which differs from their instruct-mode one on
temperature and top_p. Qwen's MTP head is embedded in the GGUF; Gemma 4's is a
separate ~280 MB drafter file passed with `--model-draft`, which landed in
llama.cpp as the `GEMMA4_ASSISTANT` arch in PR #23398.

A warning worth passing on, since it cost a full sweep here: `--spec-type`
defaults to `none`. Qwen3.8 ships its MTP head baked into the *default* GGUF,
unlike 3.6 which needed a separate upload — so the weights load, the file size
looks right, and the drafter is silently never used. It shows up only as decode
running at 25 t/s instead of 42. Check the tensor table, not the filename.

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

## Choosing the tasks

The full suite is roughly 33 hours per model. Seven models is over a week of
GPU time, so the sweep runs on a subset — and choosing that subset turned out to
matter more than anything else in this post.

The first attempt was 12 tasks picked by hand. The 35B MoE scored 10/12, 83%.
The selection had skewed toward short agent timeouts and the easy tier, because
those are the tasks that are quick to eyeball. Discarded: hand-picking the
subset repeats the problem with writing your own exam, one level up.

The replacement, `strat20`, was drawn by a script — proportional to the real
89-task difficulty mix, random within each tier, seed 42. That removed the
selection bias, rebuilt byte-identically, and produced a defensible table. It is
in [Appendix B](#appendix-b-strat20-the-earlier-random-subset), and it is the
version of this post I nearly published.

Then I read the tasks, and stopped trusting it.

### It is a proxy for someone else's job

Modernizing COBOL. Differential cryptanalysis of a FEAL cipher. Designing PCR
primers for site-directed mutagenesis. Fitting Bayesian networks in R.
Reproducing a CIFAR-10 training run on Caffe 1.0. Those are real tasks and
somebody should benchmark them; a model failing them tells me nothing about
whether it can help me recover a detached-HEAD git repository or configure
nginx.

A benchmark is a proxy for the work. Drawing proportionally from all 89 tasks
gave me an unbiased sample of a job that is not mine.

So the second subset, `domain30`, draws from an in-domain pool instead: systems
work, self-hosted infrastructure, security, and the kind of Linux/Go/Rust
plumbing this machine exists for. Out go exotic and legacy languages,
cryptanalysis and pure maths, wet-lab science, Bayesian statistics, ML research
and training, and rendering math — 37 tasks on subject matter.

### Reading the tests before trusting them

Three more came out after reading the pass criteria rather than the titles.

**`sparql-university`** estimates 800 minutes of expert time — and 10,000
minutes for a junior — then grants the agent 900 seconds. A 53× shortfall. Pass
is exact set-equality against a hardcoded result, and reaching it requires
knowing which countries were EU members on a particular date. That is memorized
world knowledge, not engineering, and no amount of SPARQL skill recovers it.

**`count-dataset-tokens`** has a defect in the test itself:

```python
expected_output = "79586"
assert expected_output in actual_output
```

A substring check. `179586` passes. In the other direction it needs live
HuggingFace dataset and tokenizer downloads mid-task and demands one exact
integer that any tokenizer-version drift changes. Brittle and
false-positive-prone at the same time.

**`break-filter-js-from-html`** requires headless Chrome and Selenium to observe
an `alert()` firing after the filter runs — an adversarial XSS bypass that turns
on recalling one specific trick. I have direct evidence of its instability: it
passed in one run and timed out in another, same model, same evening, configs
identical but for a speculative-decoding flag.

Two more stayed in, flagged rather than dropped. `code-from-image` wants an
exact SHA-256 after transcribing code from an image, no partial credit, with a
text-only agent that must install its own OCR — it was solved by nobody.
`polyglot-rust-c` is the only task in all 89 tagged `no-verified-solution`:
nobody has shown it is solvable.

The difficulty labels are not calibrated either. `configure-git-webserver` is
"hard" with a 15-minute expert estimate; `torch-pipeline-parallelism` is also
"hard" at 240 minutes. Sixteen times the work, same label, same budget.

### One timeout policy instead of none

The timeouts are the clearest structural problem. Across the 89 tasks the agent
budget ranges from 900 to 12,000 seconds, and the ratio of budget to the task's
*own* expert-time estimate spans **0.06× to 3.3×**. There is no policy; the
numbers look chosen per task and never reconciled.

`build-pov-ray` is what that costs. Measured on the first domain run: it
consumed its full **200.5 minutes** and failed, while the other four tasks in
that run finished in **10 minutes combined**. One task, 95% of the elapsed time,
for a guaranteed zero.

So every task now gets the same 20 minutes. If an agent has not finished in 20
minutes it is looping, out of its depth, or on a task that does not belong in a
30-task examination — and `terminus-2` has no context management, so long runs
degrade rather than progress. Harbor only offers timeout *multipliers*, which
scale proportionally and cannot flatten an outlier, so the subset is
materialized as copies with `[agent] timeout_sec` rewritten. The verifier's
budget is deliberately left alone: starving it would turn slow test suites into
fake failures, which measures the harness rather than the model. `build-pov-ray`
now fails in 20.5 minutes instead of 200.5 — the same information for a tenth of
the electricity.

### What the filter costs

It makes the benchmark easier, and that is worth stating rather than
discovering later:

| pool | n | hard | hard % | median expert |
|---|---:|---:|---:|---:|
| full 89 | 89 | 30 | 34% | 60 min |
| excluded | 40 | 18 | 45% | 90 min |
| kept (in-domain) | 49 | 12 | 24% | 45 min |

The exclusions took 18 of the benchmark's 30 hard tasks — 60% of them. That is
inherent to the filter rather than a mistake in execution: Terminal-Bench's hard
tier is concentrated in exactly the science, maths and exotic-language work the
domain filter removes. Difficulty and domain-relevance are correlated here, and
you cannot narrow one without paying in the other.

The first cut was 21 tasks. Nine more medium tasks were added afterwards as a
strict superset, which is why the subset is 30: because nothing was removed, the
seven models already scored only had to run the nine new tasks, 1h30m each
instead of a full re-run. It rebuilds in one command:

```
scripts/tb21-make-subset.py --name domain30 --size 30 --seed 42 \
    --exclude-offdomain --cap-agent-timeout 1200
```

## Running it

One attempt per task, one trial at a time, so only one model is ever resident on
the GPU. Tier weights are **easy 1, medium 2, hard 3**, so `domain30`'s maximum
is `1×1 + 24×2 + 5×3 = 64` points.

## Results

![exam_v4 weighted scores on domain30: qwen36-27b-q6 41/64, qwen36-27b 40/64, qwen38-27b 36/64, qwen36-35b-moe 35/64, muse-glimmer-30b 32/64, gemma-26b-moe 29/64, gemma-31b-qat 28/64, split by difficulty tier](/images/exam-v4/exam-v4-scores.svg)

| model | easy | medium | hard | score | tasks | total runtime | per task |
|---|---|---|---|---|---|---|---|
| `qwen36-27b-q6` | 1/1 | 17/24 | 2/5 | **41/64** (64%) | 20/30 | 4h54m | 9m |
| `qwen36-27b` | 1/1 | 15/24 | 3/5 | **40/64** (62%) | 19/30 | 4h44m | 9m |
| `qwen38-27b` | 1/1 | 16/24 | 1/5 | **36/64** (56%) | 18/30 | 6h06m | 12m |
| `qwen36-35b-moe` | 1/1 | 14/24 | 2/5 | **35/64** (55%) | 17/30 | 4h16m | 8m |
| `muse-glimmer-30b` | 1/1 | 14/24 | 1/5 | **32/64** (50%) | 16/30 | 5h01m | 10m |
| `gemma-26b-moe` | 1/1 | 11/24 | 2/5 | **29/64** (45%) | 14/30 | 4h33m | 9m |
| `gemma-31b-qat` | 1/1 | 12/24 | 1/5 | **28/64** (44%) | 14/30 | 4h48m | 9m |

34h25m of GPU time for 210 model-task attempts. Every model solves between 14
and 20 of the 30 tasks — so **between 44% and 64% of my own kind of work is
solvable by a model running on one 32 GB consumer card.** That is the number
this whole series has been circling, and `strat20` put it at 15–37%.

### Wall-clock cost

![Wall-clock runtime per model on domain30: 4h16m to 6h06m for 30 tasks](/images/exam-v4/exam-v4-runtime.svg)

The spread is 4h16m to 6h06m, much tighter than `strat20`'s 6h08m–8h10m despite
half again as many tasks — that is the 20-minute cap doing its work. The 30
tasks sum to 8h12m of agent timeout, the ceiling a model that failed everything
would reach.

The ordering is no longer a simple failure counter. `qwen38-27b` is both the
slowest arm and mid-table; `qwen36-35b-moe` is the fastest and fourth. Measured
during the earlier runs, decode rate explains less of this than turn-taking
style does: the MoE models decode at 110–118 tokens/s against 42–49 for the
dense ones, yet Muse Glimmer emitted about a third fewer completion tokens than
Gemma while taking nearly three times as many turns — many short commands and a
look at the result, rather than one long block and an inspection. Runtime tracks
*total tokens generated* more closely than it tracks either speed or score.

### Per task

Columns, left to right: `qwen36-27b-q6`, `qwen36-27b`, `qwen38-27b`, `qwen36-35b-moe`, `muse-glimmer-30b`, `gemma-26b-moe`, `gemma-31b-qat`.

| task | tier | 3.6 Q6 | 3.6 Q4 | 3.8 | 3.6 MoE | Muse | G26 | G31 |
|---|---|---|---|---|---|---|---|---|
| `fix-git` | easy | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `build-cython-ext` | medium | — | — | — | — | — | — | — |
| `build-pov-ray` | medium | — | — | — | — | — | — | — |
| `code-from-image` | medium | ✅ | ✅ | ✅ | — | ✅ | — | ✅ |
| `compile-compcert` | medium | — | — | — | — | — | — | — |
| `constraints-scheduling` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `crack-7z-hash` | medium | ✅ | ✅ | — | — | ✅ | ✅ | ✅ |
| `custom-memory-heap-crash` | medium | — | — | — | — | — | — | — |
| `db-wal-recovery` | medium | — | — | ✅ | — | — | — | — |
| `extract-elf` | medium | ✅ | — | ✅ | ✅ | — | — | — |
| `git-multibranch` | medium | ✅ | — | ✅ | ✅ | ✅ | ✅ | ✅ |
| `headless-terminal` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `hf-model-inference` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `kv-store-grpc` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | — | ✅ |
| `large-scale-text-editing` | medium | ✅ | ✅ | — | ✅ | ✅ | — | — |
| `log-summary-date-ranges` | medium | — | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `mailman` | medium | — | — | — | — | ✅ | — | — |
| `multi-source-data-merger` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `nginx-request-logging` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| `openssl-selfsigned-cert` | medium | ✅ | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| `qemu-startup` | medium | ✅ | ✅ | — | ✅ | — | — | — |
| `regex-log` | medium | ✅ | ✅ | ✅ | ✅ | — | ✅ | ✅ |
| `sqlite-db-truncate` | medium | ✅ | — | ✅ | — | — | — | — |
| `sqlite-with-gcov` | medium | ✅ | ✅ | ✅ | — | ✅ | — | — |
| `vulnerable-secret` | medium | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `cancel-async-tasks` | hard | — | ✅ | — | — | — | — | — |
| `configure-git-webserver` | hard | ✅ | ✅ | — | ✅ | — | ✅ | ✅ |
| `fix-code-vulnerability` | hard | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| `torch-pipeline-parallelism` | hard | — | — | — | — | — | — | — |
| `video-processing` | hard | — | — | — | — | — | — | — |

Six tasks were solved by every model and six by none, leaving **18 of 30 that
actually discriminate**. On `strat20` that number was 7 of 20. Same benchmark,
same agent, same models — a subset chosen with the tests read rather than
sampled blind separates the field more than twice as well.

The six nobody solved are `build-cython-ext`, `build-pov-ray`,
`compile-compcert`, `custom-memory-heap-crash`, `torch-pipeline-parallelism` and
`video-processing`. Three of those are "build a large foreign C/C++ project from
source", which is a coherent weakness rather than a scattering.

## `strat20` versus `domain30`

The two subsets are drawn from the same 89 tasks with the same agent and score
the same seven models. They overlap in **4 tasks**.

| | `strat20` | `domain30` |
|---|---|---|
| tasks | 20 | 30 |
| drawn from | all 89 | 49 in-domain |
| easy / medium / hard | 1 / 12 / 7 | 1 / 24 / 5 |
| agent timeout, as run | 15–60 min, no policy | **20 min, uniform** |
| total agent budget | 10h30m | 8h12m |
| median expert estimate | 120 min | 35 min |
| solved by every model | 3 of 20 | 6 of 30 |
| solved by none | 10 of 20 | 6 of 30 |
| **discriminating tasks** | **7 of 20 (35%)** | **18 of 30 (60%)** |
| score range across models | 15–37% | 44–64% |
| runtime per model | 6h08m–8h10m | 4h16m–6h06m |

And the rankings they produce:

| model | `strat20` | rank | `domain30` | rank |
|---|---:|---:|---:|---:|
| `qwen36-27b-q6` | 9/46 | 6 | **41/64** | **1** |
| `qwen36-27b` | 14/46 | 3 | **40/64** | **2** |
| `qwen38-27b` | 14/46 | 3 | **36/64** | **3** |
| `qwen36-35b-moe` | 17/46 | **1** | 35/64 | 4 |
| `muse-glimmer-30b` | 10/46 | 5 | 32/64 | 5 |
| `gemma-26b-moe` | 7/46 | 7 | 29/64 | 6 |
| `gemma-31b-qat` | 12/46 | 4 | 28/64 | **7** |

Spearman's rank correlation between the two columns is **ρ = 0.20** (p = 0.67).
The subset that won `strat20` is fourth here; the subset that came sixth is
first. Two honest, script-drawn samples of the same benchmark, run on the same
box with the same agent, rank the same seven models very nearly independently.

Which one is right depends on what you are asking. For "how do these models do
on Terminal-Bench", `strat20`. For "which of these should I run on my machine
next week", `domain30`. What is not defensible is quoting either as though it
were a property of the model.

## Conclusions

**Qwen3.8 is not materially better.** Released mid-August and run at the same
size, quant and samplers as its 3.6 predecessor, it lands third at 36/64 against
40/64 — behind both Qwen3.6 27B builds — while being the slowest arm in the
sweep at 6h06m. Its shape is consistent across every subset tried: competitive
on the medium tier (16/24, the best of the three 27Bs) and alone at the bottom
on hard (1/5). On exam_v3 in
[Part 5](https://blog.mfilipe.eu/post/local-llm-dense-models-r9700/) it beat 3.6
by six median points. The two exams disagree, and the reading I will defend is
that 3.8 writes better one-shot code and is no better at driving a terminal.

**Q6 edges Q4, and not by enough to matter.** Same Qwen3.6-27B checkpoint, two
quantizations, identical samplers and drafter depth. Q6 takes it 41/64 to 40/64
on the largest sample here. But the sign is not stable: Q4 led by 5 points on
`strat20` and by 3 on the first 21-task cut, and Q6 by 1 on the full 30. A
difference that changes direction when you add nine tasks is not a difference.
Q6 costs 6.7 GiB more VRAM, leaving about 4 GiB of headroom on this card instead
of 11, and on a machine with a history of OOM kills that is the whole argument.

**The Gemma 4 models are not good at this.** They finish sixth and seventh, and
`gemma-31b-qat` is last — the same model that wins exam_v3 outright with a
median of 12/13 where the best Qwen manages 6. Its medium tier here is 12/24
against the leaders' 15–17, so the shortfall is broad rather than a couple of
unlucky hard tasks. Across `strat20` and `domain30` the Gemmas occupy the bottom
in both. One-shot code synthesis and agentic competence are close to unrelated,
and Gemma is the sharpest illustration of it.

**Extending and re-targeting the exam moved the results toward what everyone
else reports.** Three checks, none of which `strat20` passed:

- Public evaluations place Qwen3.6 27B dense ahead of the 35B-A3B MoE at this
  size class, and general guidance says dense beats a 3B-active MoE when you are
  this compute-constrained. `domain30` puts the dense 27Bs first and second and
  the MoE fourth. `strat20` had the MoE first and the dense 27B third; exam_v3
  had the dense 27B *last* with a median of 0.
- Meta publish **51.7%** for Muse Glimmer 30B on Terminal-Bench 2.1 with
  terminus-2. It scores **50%** here. On `strat20` it scored 22% — less than
  half the vendor's own number, which I published at the time with the caveat
  that the gap was unexplained. Most of the gap was the task selection.
- The 30-task table's spread, 44–64%, is a believable band for quantized local
  models on agentic work. A table where no model clears 40% and ten of twenty
  tasks go unsolved by anyone was mostly measuring tasks nobody could do.

**This is the real lesson, and it was a lesson rather than a plan.** Adopting a
maintained third-party benchmark was supposed to end the work of maintaining my
own — that was the whole argument for abandoning exam_v3 at the end of Part 5.
It did not. Open-source benchmarks have bugs: an assertion that accepts the
wrong answer, a task nobody has shown is solvable, per-task budgets varying 13×
with no stated policy, difficulty labels that put a 15-minute task and a
240-minute one in the same tier. Using one blind, and quoting the number it
produces, is misleading in a way that is hard to detect from the number alone.

The externally-written harness is still worth having — Harbor and terminus-2 are
far better than anything I would write. But the *task selection and the run
configuration are mine to own*, and they turned out to matter more than any
model difference in this post. That is the opposite of what I expected when I
started Part 6.

## Caveats

**n=1 per task.** Every number here is a single attempt. Three individual tasks
were observed flipping outcome between otherwise-identical reruns during this
work. First-versus-last is a real difference; adjacent rows are not, and the
noise floor of this design is still unmeasured — the same omission that made
exam_v3's rankings noise in Part 5.

**`domain30` is easier than `strat20` by construction**, and narrower on
purpose. It is not a general benchmark and its scores should not be compared to
published Terminal-Bench numbers, with the Muse Glimmer agreement above being an
observation rather than a validation.

**The 20-minute cap is my choice, not the benchmark's.** It makes some tasks
unwinnable that a longer budget might have allowed. That is deliberate — it is
also uniform, which the benchmark's own budgets are not.

## What is next

- Run the subset more than once per model, so the noise floor is measured rather
  than assumed. This is the top of the list and has been since Part 5.
- Re-weight toward the hard tier. Proportional sampling from a pool that is only
  24% hard gives 5 hard tasks out of 30, and those five carry a quarter of the
  weighted score.
- Run the full 89 on one model, to quantify how much any subset distorts.
- Test Muse Glimmer at `xhigh` reasoning, the strength its own model card
  recommends for agentic work, instead of the `high` default used here.

## Appendix A: the 30 `domain30` tasks

Drawn with seed 42 from the 49-task in-domain pool, with every agent timeout
capped at 20 minutes. The "expert" column is Terminal-Bench's own estimate in
minutes, left uncapped — the gap between it and the 20-minute budget is the
point.

| task | tier | agent timeout | expert | what it asks |
|---|---|---:|---:|---|
| `fix-git` | easy | 15m | 5 | Evaluates the ability to recover lost Git commits from a detached HEAD state and merge them back into the master branch. |
| `build-cython-ext` | medium | 15m | 60 | Evaluates the ability to compile and install a Python package with Cython extensions from source while fixing NumPy 2.x compatibility issues. |
| `build-pov-ray` | medium | 20m | 60 | Evaluates the ability to locate, download, patch, and compile legacy POV-Ray 2.2 raytracer from 1990s source archives on a modern system. |
| `code-from-image` | medium | 20m | 30 | Evaluates an agent's ability to extract code from an image using OCR or vision models, implement the pseudocode logic with cryptographic hashing, and produce the correct output. |
| `compile-compcert` | medium | 20m | 60 | Evaluates the ability to build the CompCert verified C compiler from source with proper configuration for the host architecture and dependencies. |
| `constraints-scheduling` | medium | 20m | 15 | Find an optimal 1-hour meeting slot for three people with complex availability constraints by parsing ICS calendars and applying constraint satisfaction with tie-breaking preferences. |
| `crack-7z-hash` | medium | 20m | 5 | Evaluates the ability to crack a password-protected 7z archive using John the Ripper and extract secret contents. |
| `custom-memory-heap-crash` | medium | 20m | 30 | Evaluates the ability to debug and fix a C++ program that crashes in release mode due to a static initialization order issue with custom memory allocators and STL locale facets. |
| `db-wal-recovery` | medium | 15m | 45 | Tests the ability to decrypt an XOR-encrypted SQLite WAL file and recover complete database contents including write-ahead log changes. |
| `extract-elf` | medium | 15m | 30 | Evaluates ability to parse ELF binary format and extract memory values from executable sections using Node.js. |
| `git-multibranch` | medium | 15m | 180 | Evaluates the ability to set up a Git server with SSH authentication, implement post-receive hooks for automated multi-branch deployment, and configure Nginx to serve branch-specific content over HTTPS. |
| `headless-terminal` | medium | 15m | 120 | Implement a Python class that provides a headless terminal interface supporting interactive bash shells, modifier keys, startup file sourcing, and state persistence between commands. |
| `hf-model-inference` | medium | 15m | 20 | Evaluates the ability to download a Hugging Face transformer model, create a Flask API for sentiment analysis, and run the service in the background with proper error handling. |
| `kv-store-grpc` | medium | 15m | 15 | Evaluates the ability to build and deploy a gRPC-based key-value store server with Protocol Buffers, including service definition, code generation, implementation, and background process management. |
| `large-scale-text-editing` | medium | 20m | 40 | Evaluates the ability to efficiently transform a 1-million-row CSV file using keystroke-efficient Vim macros with strict command restrictions. |
| `log-summary-date-ranges` | medium | 15m | 75 | Evaluates the ability to analyze date-stamped log files, calculate counts across multiple date ranges, and generate structured CSV output. |
| `mailman` | medium | 20m | 60 | Evaluates the ability to configure a functional mailing list server by integrating postfix and mailman3 with proper join/leave/announce workflows. |
| `multi-source-data-merger` | medium | 15m | 30 | Evaluates an agent's ability to merge multi-format data sources (JSON, CSV, Parquet) with inconsistent schemas, applying field mappings and priority-based conflict resolution to produce standardized outputs. |
| `nginx-request-logging` | medium | 15m | 20 | Evaluates the ability to install and configure Nginx with advanced request logging, rate limiting, and custom error pages. |
| `openssl-selfsigned-cert` | medium | 15m | 20 | Evaluates an agent's ability to generate self-signed TLS certificates using OpenSSL, manage cryptographic keys with proper permissions, and create verification scripts. |
| `qemu-startup` | medium | 15m | 30 | Evaluates the agent's ability to configure and start a QEMU virtual machine with telnet-accessible serial console, requiring knowledge of QEMU command-line options, network configuration, and system readiness verification. |
| `regex-log` | medium | 15m | 45 | Tests the ability to construct a complex regular expression that matches dates in log lines containing valid IPv4 addresses while handling edge cases and boundary conditions. |
| `sqlite-db-truncate` | medium | 15m | 60 | Evaluates the ability to recover data from a corrupted SQLite database using binary file analysis and data recovery techniques. |
| `sqlite-with-gcov` | medium | 15m | 30 | Evaluates the ability to compile SQLite from source with gcov instrumentation and make it available in the system PATH. |
| `vulnerable-secret` | medium | 15m | 20 | Evaluates the agent's ability to analyze a binary executable, identify and exploit a buffer overflow vulnerability to bypass authentication, and extract a hidden secret flag. |
| `cancel-async-tasks` | hard | 15m | 120 | Evaluates the ability to implement async task concurrency control with proper cleanup on cancellation, including the edge case of queued tasks. |
| `configure-git-webserver` | hard | 15m | 15 | Evaluates the ability to configure a Git server with automatic deployment to an nginx web server using post-receive hooks. |
| `fix-code-vulnerability` | hard | 15m | 120 | Evaluates the ability to identify and fix a CRLF injection vulnerability (CWE-93) in HTTP header handling code by adding input validation to reject control characters. |
| `torch-pipeline-parallelism` | hard | 15m | 240 | Evaluates the ability to implement pipeline parallel training for LLaMA using PyTorch distributed primitives with all-forward-all-backward scheduling. |
| `video-processing` | hard | 20m | 400 | Evaluates the ability to build a computer vision script that analyzes hurdle jump videos and extracts takeoff/landing frame numbers using OpenCV. |

## Appendix B: `strat20`, the earlier random subset

The first defensible subset: 20 tasks drawn proportionally from all 89 with seed
42, no domain filter and no timeout policy. Kept because it is what this post
originally reported, and because the difference between the two tables is the
argument of the post.

Model names changed with a config cleanup between the two runs — speculative
decoding is now on by default and no longer stated in the name, and the Qwen
entries carry their version. `qwen-27b-mtp` is `qwen36-27b`, `qwen-35b-moe-mtp`
is `qwen36-35b-moe`, `qwen-27b-mtp-q6` is `qwen36-27b-q6`.

| model | easy | medium | hard | score | tasks | runtime | per task |
|---|---|---|---|---|---|---|---|
| `qwen-35b-moe-mtp` | 1/1 | 5/12 | 2/7 | **17/46** (37%) | 8/20 | 6h08m | 18m |
| `qwen-27b-mtp` | 1/1 | 5/12 | 1/7 | **14/46** (30%) | 7/20 | 7h15m | 21m |
| `qwen38-27b` | 0/1 | 4/12 | 2/7 | **14/46** (30%) | 6/20 | 7h46m | 23m |
| `gemma-31b-qat` | 1/1 | 4/12 | 1/7 | **12/46** (26%) | 6/20 | 7h36m | 22m |
| `muse-glimmer-30b` | 1/1 | 3/12 | 1/7 | **10/46** (22%) | 5/20 | 6h42m | 20m |
| `qwen-27b-mtp-q6` | 1/1 | 4/12 | 0/7 | **9/46** (20%) | 5/20 | 7h13m | 21m |
| `gemma-26b-moe` | 1/1 | 3/12 | 0/7 | **7/46** (15%) | 4/20 | 8h10m | 24m |

No model clears 40%, and ten of the twenty tasks were solved by nobody. Three
tasks were solved by everybody. The hard tier saw five solves across 49
model-task pairs.

The 20 tasks:

| task | tier | agent timeout | expert | what it asks |
|---|---|---:|---:|---|
| `cobol-modernization` | easy | 15m | 20 | Evaluates the ability to reverse-engineer and reimplement a COBOL program's business logic in Python with exact output reproduction. |
| `break-filter-js-from-html` | medium | 20m | 20 | Evaluates the agent's ability to bypass an HTML sanitization filter by crafting malicious HTML that triggers JavaScript execution after filtering. |
| `caffe-cifar-10` | medium | 60m | — | Evaluates the ability to install and configure BVLC Caffe 1.0.0, train a CNN on CIFAR-10 for exactly 500 iterations in CPU-only mode, and achieve specified accuracy thresholds. |
| `chess-best-move` | medium | 15m | 45 | Evaluates the agent's ability to analyze a chess position from an image, use a chess engine to find the best move(s), and handle multiple valid solutions. |
| `compile-compcert` | medium | 40m | 60 | Evaluates the ability to build the CompCert verified C compiler from source with proper configuration for the host architecture and dependencies. |
| `distribution-search` | medium | 60m | 120 | Tests the ability to find a probability distribution satisfying precise dual KL divergence constraints through numerical optimization. |
| `dna-insert` | medium | 30m | 30 | Evaluates the ability to design PCR primers for site-directed mutagenesis by analyzing plasmid sequences and applying molecular biology constraints on primer length and melting temperature. |
| `filter-js-from-html` | medium | 30m | 45 | Evaluates the agent's ability to create a robust XSS filter that removes JavaScript from HTML files while preserving legitimate HTML structure and content. |
| `nginx-request-logging` | medium | 15m | 20 | Evaluates the ability to install and configure Nginx with advanced request logging, rate limiting, and custom error pages. |
| `portfolio-optimization` | medium | 60m | 120 | Evaluates the ability to implement a high-performance C extension for Python that performs portfolio risk and return calculations at least 1.2x faster than a pure Python baseline while maintaining numerical accuracy. |
| `query-optimize` | medium | 15m | 60 | Evaluates the ability to optimize a slow SQL query with correlated subqueries by rewriting it using CTEs and window functions while preserving exact output. |
| `rstan-to-pystan` | medium | 30m | 180 | Evaluates the ability to convert an RStan Gaussian Process script to functionally equivalent PyStan 3.10.0 code, including complex installation, hyperparameter mapping, and numerical verification of posterior estimates. |
| `vulnerable-secret` | medium | 15m | 20 | Evaluates the agent's ability to analyze a binary executable, identify and exploit a buffer overflow vulnerability to bypass authentication, and extract a hidden secret flag. |
| `bn-fit-modify` | hard | 60m | 480 | Evaluates the ability to recover a Bayesian Network DAG structure from data, perform causal interventions, and sample from the modified network. |
| `cancel-async-tasks` | hard | 15m | 120 | Evaluates the ability to implement async task concurrency control with proper cleanup on cancellation, including the edge case of queued tasks. |
| `circuit-fibsqrt` | hard | 60m | 960 | Evaluates the agent's ability to implement complex mathematical functions (Fibonacci of integer square root) using only combinational and sequential logic gates in a hardware description format. |
| `feal-differential-cryptanalysis` | hard | 30m | 480 | Evaluates the ability to implement differential cryptanalysis on a FEAL-like cipher to recover a round key through chosen plaintext attacks. |
| `feal-linear-cryptanalysis` | hard | 30m | 960 | Evaluates the ability to perform linear cryptanalysis on a FEAL-like cipher to recover encryption keys from known plaintext-ciphertext pairs. |
| `make-doom-for-mips` | hard | 15m | 480 | Evaluates ability to cross-compile the DOOM game engine for MIPS architecture using LLVM toolchain and verify execution in a JavaScript emulator. |
| `model-extraction-relu-logits` | hard | 15m | 480 | Extracts hidden layer weights from a black-box ReLU neural network by querying outputs and identifying critical points where neurons activate. |
