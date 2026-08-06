# Runbook: exam_v3 and exam_v4 on an R9700 box

_Written 2026-08-07 by the agent that ran both exams on Framework 13 (Ryzen AI
HX 370, Radeon 890M iGPU, 62 GiB UMA). Target reader: a fresh agent with a
different machine — a discrete Radeon **AI PRO R9700, 32 GiB dedicated VRAM** —
and a different set of models._

Everything marked **measured** was executed on the Framework 13 and the number
is in `docs/reports/EXAM_V3_2026-08-06_TEMP_AND_HOSTED.md`. Everything marked
**inferred** is arithmetic that has not been run anywhere. Do not present
inferred numbers as findings.

---

## 0. What you are running, and why there are two exams

| exam | what it measures | shape | harness |
|---|---|---|---|
| `exam_v3` | one-shot Go coding: rewrite a concurrent scraper to be outage-resilient | 1 request, ~10k output tokens, scored /13 by `go test -race` | in-repo: `exam-driver.go` + `bench/exam_v3/` |
| `exam_v4` | agentic tool-loop: explore, act, read feedback, stop | 20+ turns per task, scored pass/fail per task | external: `terminal-bench` (`tb` CLI) |

They are independent. Do not conflate them in one sweep, and do not average
their scores together.

---

## 1. Hardware differences that change the recipe

The Framework 13 is a **UMA** box: the iGPU's GTT is carved out of the same
62 GiB the OS uses. The R9700 has **32 GiB of dedicated VRAM**. Three
consequences:

### 1.1 `--no-mmap` is right on R9700, wrong on UMA

**Measured on UMA:** `--no-mmap` made a 42.8 GB model *unloadable*. It stages
~40 GiB in host RAM *plus* ~40 GiB of GTT out of the same 62 GiB. Two attempts
sat at 35–40 GiB of GTT with `read_bytes` flat at 76 GB (1.8× the file — pages
evicted and re-read) and `stime` climbing ~100 s per 15 s wall: amdgpu
evict/restore thrash, not slow IO (the raw file reads at 2.3 GB/s). Killed at
941 s. With mmap on, the same model loaded in 45 s.

**Inferred for R9700:** dedicated VRAM removes the contention, so the existing
`--no-mmap` in the `llama-server` macro is correct there and should be left
alone. Keep it unless you observe swap growth during load.

### 1.2 `-ncmoe` is a UMA workaround — probably drop it

`-ncmoe N` keeps the MoE weights of the first N of 48 layers on CPU. On UMA
that *helps*, because CPU-side weights are read from page cache instead of
being duplicated into GTT.

**Measured on UMA**, Qwen3.5-122B-A10B UD-Q2_K_XL (42.8 GB), `-c 32768`:

| `-ncmoe` | decode t/s | GTT | note |
|---:|---:|---:|---|
| 24 | 3.10 | 24.7 GiB | |
| **20** | **5.47** | **27.7 GiB** | peak |
| 16 | 4.31 | 30.7 GiB | |
| 12 | — | — | fails to load (608 s timeout) |
| 8 | — | — | fails to load (608 s timeout) |

The curve is non-monotonic because two effects fight: fewer CPU layers means
more GPU compute but more GTT pressure.

**On R9700:** 32 GiB of VRAM will not hold 39.9 GiB of weights, so a 122B-class
model still needs *some* CPU offload — but the optimum will be at a different
`-ncmoe` and must be re-swept. Sweep script pattern is in this repo's history;
the loop is: for each N, start the server, wait for `/health`, send one 128-token
completion, record `timings.predicted_per_second`, kill, sleep 15 s.

**Sizing arithmetic for the 122B** (48 layers, 2 KV heads, head_dim 256):
KV at q8_0 is 0.80 GiB @16k, 1.59 GiB @32k, 3.19 GiB @64k, 6.38 GiB @128k.
Weights are 39.9 GiB. Adjust for your model.

### 1.3 Faster decode may make reasoning-on feasible for exam_v4

See §3.2. The whole reason exam_v4 must run `-nothink` on the Framework 13 is
decode speed. If the R9700 decodes fast enough, that constraint relaxes — but
**measure it before assuming it**, and record which mode you used.

---

## 2. exam_v3 — one-shot Go coding

### 2.1 Run it

```zsh
cd <repo>/blogpost/benchmarking_llms

# a local llama-swap cell
SEEDS="42 123" CELLS="B" TEMP=1.0 \
  RESULTS_DIR="$PWD/artifacts/results/exam_v3_r9700" \
  ./scripts/sweep-exam3-rocmfp4.sh

# a hosted reference cell (needs a key)
EXAM_API_KEY=sk-ant-... ./scripts/run-exam3-hosted.sh
```

`scripts/sweep-exam3-rocmfp4.sh` does preflight, warm-up, sequential cells,
teardown, and resumes on re-invocation. Cells are defined in `cell_spec()`;
add yours there as `<letter>) echo "display|endpoint|served-model-name"`.

**`RESULTS_DIR` must be new per arm.** `run_attempt` skips any cell/seed that
already has a `result.json`, so reusing a directory silently re-records old
numbers instead of running.

### 2.2 Five non-negotiables, all learned the hard way

1. **`max_tokens` MUST exceed `--reasoning-budget`.** With reasoning on and
   both at 8192, thinking consumes the entire allowance, the whole output lands
   in `reasoning_content`, and `content` comes back empty. `exam-driver.go`
   reads only `content` → a **structural 0/13 that looks like a model failure**.
   This cost us a whole 6-attempt sweep. Runner now defaults `MAX_TOKENS=16384`.
   If you change `--reasoning-budget`, re-check this.

2. **Preflight the grader.** `bench/exam_v3/make-reference-response.py` builds
   the answer a perfect model would emit; `eval.sh` must score it **13/13**. The
   runner does this and aborts on failure. If it doesn't score 13/13, no model
   number from that run means anything.

3. **Warm up and discard.** Cold prefill measured 1.07 t/s vs 13.10 warm — a 12×
   artifact on `wall_s`. The runner sends one throwaway request per cell.

4. **Match samplers across cells.** `exam-driver.go` sends `temperature`
   unconditionally (no `omitempty`), so the client value **overrides** the
   server's `--temp` on every cell. It never sends `top_p/top_k/min_p`, so those
   come from the server and must be set identically for every cell. We shipped an
   arm where a containerised cell ran llama.cpp defaults (`top_k 40`,
   `min_p 0.05`) against a llama-swap cell on `top_p 0.95 / top_k 20 /
   min_p 0.0` — that A-vs-B gap was partly a sampler gap, not a model gap.

5. **One temperature.** Two temps double wall time and temperature was not the
   discriminator: Gemma moved a lot (0→7, 6→11 going 1.0→0.6), both Qwen cells
   moved the *wrong* way, Haiku not at all. Pick one, state it, move on. If you
   want the per-model temp question, that is a separate experiment with a stated
   hypothesis — not a column in a model ranking.

### 2.3 The ceiling is 12/13, not 13/13

`TestNewScraperValidation` failed in **16/16 attempts** on 2026-08-06 —
including all four Haiku 4.5 runs — and in April's best 11/13 run. It has never
been passed by any model in this lab's recorded history.

The test requires `NewScraper` to return an error for nil source, nil sink, zero
interval, or negative `maxBufSize`. `prompt.txt`'s five numbered requirements
never ask for argument validation; the only hint is the `(Scraper, error)`
return in a signature you are told not to change, and the supplied original
always returns `nil`. The reference response scores 13/13 precisely because it
adds five guard clauses nobody asked for.

**Every exam_v3 score therefore carries a systematic −1.** Either fix the prompt
or drop the test before publishing anything. Until then, subtract 1 from the
denominator in your head.

Secondary, observed but not diagnosed: `TestLongOutage/EvictionNotContiguous`
failed 0/12 on scored attempts *and* in April's best run. Possibly the same
class of problem. Nobody has read that test yet.

### 2.4 Reference numbers to compare against (Framework 13, 2026-08-06)

Reasoning ON, seeds 42/123, `max_tokens 16384`, scored /13. **Timing measured
under `platform_profile: low-power` — treat t/s and wall_s as a floor, not a
comparable baseline.** Scores are unaffected.

| model | quant, size | t1.0 | t0.6 | decode t/s | wall s |
|---|---|---|---|---:|---:|
| Gemma 4 26B-A4B QAT + MTP | UD-Q4_K_XL, 14.25 GB | 0 / 6 | **7 / 11** | 14.1–14.3 | 744–825 |
| Qwen3.6-35B-A3B Unsloth | UD-Q4_K_XL, 22.85 GB | 7 / 5 | 6 / 0 | 11.3–12.3 | 864–960 |
| Qwen3.6-35B-A3B ACE-SABER | ROCmFP4, 19.05 GB | 7 / 0 | 6 / 0 | 13.3–14.5 | 659–833 |
| Qwen3.5-122B-A10B | UD-Q2_K_XL, 42.85 GB | 0 / 3 | not run | 5.9–6.1 | 679–888 |
| Haiku 4.5 (hosted) | — | 11 / 11 | 10 / 11 | n/a | 18–21 |

Headlines: **Gemma at 0.6 is the best local result ever recorded here (11/13,
i.e. the practical maximum).** The 122B at Q2_K_XL is the *worst* local
result — bigger did not help; 2-bit destroyed it, and it is 2× slower. Haiku is
4–5 points above the best local at ~1/40th the wall time.

Local attempts compiled 9/12; hosted 4/4. Every local compile failure was a
single-token-level Go error in an otherwise complete, well-fenced program
(unused `context` vars, `struct{...}` type mismatch, newline inside a string
literal, generic-inference mismatch). That is the signature to expect from
aggressive quantisation, and it is why a 0/13 needs its artifact read before you
call it a model quality result.

### 2.5 How to tell a real 0/13 from a harness 0/13

Read `artifacts/results/.../seed<N>/`:

- `response.txt` **empty**, `tokens` at exactly the reasoning budget →
  harness bug (§2.2.1). Not a model result.
- `response.txt` complete, clean ```go fences, `test.log` shows a compile
  error → genuine model failure. Record it.
- `summary` contains `race:DETECTED` → compiled, but `-race` aborted the run.
  Score is low because unrun tests count as failures (fixed denominator).
- `summary` is `extraction:FAIL` → the model didn't emit a fenced block. Check
  `eval.sh`'s `extract_code` before blaming the model.

---

## 3. exam_v4 — terminal-bench subset

Full design: `docs/plans/EXAM_V4_TERMINAL_BENCH.md`. **Status: harness
installed, nothing run.** You will be the first to produce a number.

### 3.1 Install and datasets

```zsh
uv tool install terminal-bench       # installs `tb`, needs ~/.local/bin on PATH
tb datasets list
```

`tb` defaults to the registry at `laude-institute/terminal-bench` — the
`harbor-framework` org path 404s despite being the repo's current home
(`terminal_bench/registry/client.py:90`). Nothing needs overriding.

Verified dataset inventory:

| dataset | version | tasks | languages |
|---|---|---|---|
| `terminal-bench-core` | 0.1.1 | 80 | language-agnostic shell/agentic |
| `aider-polyglot` | head | 225 | cpp 26, **go 39**, java 47, **js 49**, **python 34**, rust 30 |
| `quixbugs` | head | 80 | java 40, python 40 |

`aider-polyglot` **is** a terminal-bench dataset, which is why we run only
terminal-bench: you get aider's 225 Exercism exercises *and* the agentic tasks
under one CLI, one Docker path, one results format. Do not install aider's own
`benchmark/` (same exercises, second image, second format) or `mini-swe-agent`
(SWE-bench only, Python only; `swebench-verified` is already a `tb` dataset).

**Scope: Go, JavaScript, Python only.** No cpp, java, rust.

`aider-polyglot` lives in a different repo
(`laude-institute/terminal-bench-datasets`) — whether it needs a separate
download step is **untested**.

### 3.2 Two constraints that will bite you

**a) Reasoning-on may be impossible.** Task defaults are
`max_agent_timeout_sec: 900.0` (15 min), confirmed in the `task.yaml` of
`hello-world`, `fibonacci-server`, `csv-to-parquet`, `fix-git`. At the ~14 t/s
the Framework 13 decodes, an 8192-token thinking block costs **~585 s ≈ 9.8 min
per turn** — about 1.5 turns inside the budget. A 20-turn loop cannot finish.

On R9700, compute `8192 / <your measured decode t/s>`. If that is under ~60 s,
reasoning-on is viable and you get a strictly better measurement than we could.
Otherwise use the `-nothink` aliases and **say so in the writeup**, because
exam_v3 runs reasoning *on* — the two exams then differ in mode.

**b) The agent never manages its own context.** `terminus-2` sizes context via
`litellm.get_max_tokens(model_name)` (`terminus_2.py:158-165`). Measured:
`openai/qwen36-moe` **raises** ("isn't mapped yet"), so it falls back to
`fallback_context_limit = 1_000_000` and therefore **never summarises or unwinds
history** — it appends until the server rejects. Each terminal observation is
capped at 10 000 bytes ≈ 2.5k tokens (`_limit_output_length`), so a 16k context
dies around turn 4.

→ Serve locals at the largest context that fits. 131072 for a 14–23 GB model;
for a 40 GB model measure what fits in 32 GiB of VRAM after weights.

Related: `lite_llm.py` never passes `max_tokens` to `litellm.completion` and
raises `OutputLengthExceededError` on `finish_reason == "length"`. Server-side
`--predict 32768` should cover it, but this is the same failure class as
§2.2.1 — check the first run's episode logs.

### 3.3 The subset — 5 scored tasks + 1 preflight

| # | task | dataset | lang | why |
|---|---|---|---|---|
| 0 | `hello-world` | core 0.1.1 | — | plumbing preflight, **not scored** |
| 1 | `fix-git` | core 0.1.1 | — | pure agentic, expert est. 5 min |
| 2 | `csv-to-parquet` | core 0.1.1 | python | agentic + code, easy |
| 3 | `fibonacci-server` | core 0.1.1 | python/js | medium: write code, run a server, verify |
| 4 | `polyglot_go_book-store` | aider-polyglot | go | Go coding + edit-format compliance |
| 5 | `polyglot_python_bowling` | aider-polyglot | python | non-Go control |

Five is deliberately small. **Run tasks 0 and 1 first, measure the wall time,
then decide whether 5 → 10 is affordable.** Per-task wall time on any of our
hardware is still unmeasured — every duration estimate in these docs is
arithmetic.

### 3.4 Run it

```zsh
export PATH="$HOME/.local/bin:$PATH"

# preflight: the oracle agent must pass. Same role as exam_v3's 13/13 gate.
tb run --dataset terminal-bench-core==0.1.1 --agent oracle \
  -t hello-world -t fix-git --n-concurrent 1

# a model cell
tb run --dataset terminal-bench-core==0.1.1 \
  --agent terminus-2 \
  --model openai/<served-name> \
  -k api_base=http://127.0.0.1:8090/v1 \
  -t hello-world -t fix-git -t csv-to-parquet -t fibonacci-server \
  --n-concurrent 1 \
  --global-agent-timeout-sec 3600 \
  --output-path artifacts/results/exam_v4/<display>
```

Non-negotiables:

1. **Oracle preflight passes first.** No model number counts otherwise.
2. **`--n-concurrent 1`.** llama-swap serves one slot; concurrent trials queue
   behind each other while their per-task clocks keep running. The default is 4.
3. **Raise `--global-agent-timeout-sec`.** 900 s is a hosted-model budget.
4. Run **Haiku 4.5 as the ceiling reference** — it costs ~$0.03/task and tells
   you whether a task is hard or your plumbing is broken.

`terminus-2` accepts `max_episodes`, `parser_name`, `api_base`, `temperature`
via `-k`. Note `--agent-kwarg` is the long form of `-k`.

Untested: whether `terminus-2`'s JSON parser survives these models' output. It
has a `salvage_truncated_response` path, which suggests it often does not.

---

## 4. Reporting rules

- **Artifacts are gitignored** (`blogpost/.gitignore`: `benchmarking_llms/artifacts/*`).
  The report *is* the record. Put the tables in the doc, not just the run dir.
- New reports go in `docs/reports/`, plans in `docs/plans/`. See
  `AGENTS.md` in the lab root.
- State the machine, the power profile, the llama.cpp build, the temperature,
  and the reasoning mode. Any two of those differing makes numbers
  incomparable, and we have already published one table that mixed them.
- **Set the power profile before any timing run** — needs sudo, and an agent
  cannot do it:
  ```zsh
  echo performance | sudo tee /sys/firmware/acpi/platform_profile
  ```
  Every timing in the 2026-08-06 report was taken under `low-power`.
- Distinguish measured from inferred in the prose. Every wrong conclusion in
  this lab's history came from an inference presented as a measurement.

---

## 5. Things known to be wrong or unfinished

| item | state |
|---|---|
| `TestNewScraperValidation` unreachable | exam_v3 ceiling is 12/13; prompt or test must change |
| `TestLongOutage/EvictionNotContiguous` 0/12 | unread, possibly the same problem |
| 2026-08-06 timings taken under `low-power` | scores fine, t/s and wall_s not comparable |
| Gemma's 11/13 rests on one attempt | re-run at 0.6 with more seeds |
| exam_v4 has never been run | you are first |
| `aider-polyglot` download path | untested |
| 122B at Q2_K_XL | 0/13 and 3/13 — do not spend more time on 2-bit quants of this model |
