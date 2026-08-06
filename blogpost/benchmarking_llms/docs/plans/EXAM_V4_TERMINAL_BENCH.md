# exam_v4 — a runnable subset of terminal-bench

_Written 2026-08-06. Status: harness installed (`tb` 0.2.18), nothing run yet._

exam_v3 is a single 800-second shot at one Go file. It cannot see whether a
model can drive a tool loop. exam_v4 is that missing measurement, built as a
**subset of terminal-bench** rather than a new harness — we already burned a
night on home-grown harness bugs (`docs/notes/HARNESS_AUDIT.md`).

Supersedes the staged-agentic design that used to live in
`EXAM_V4_STAGED_AGENTIC.md` (that file now holds the exam_v3 re-run plan).
The local `agentic/` harness stays as-is; it is not being extended.

## Why terminal-bench and nothing else

`aider-polyglot` is a terminal-bench dataset, so picking terminal-bench gets
aider's 225 Exercism exercises *and* the agentic terminal tasks under one CLI,
one Docker path, one results format. Verified from the registry:

| dataset | version | tasks | languages |
|---|---|---|---|
| `terminal-bench-core` | 0.1.1 | 80 | language-agnostic shell/agentic |
| `aider-polyglot` | head | 225 | cpp 26, go 39, java 47, js 49, python 34, rust 30 |
| `quixbugs` | head | 80 | java 40, python 40 |

Registry lives at `laude-institute/terminal-bench` — the `harbor-framework`
org path 404s despite being the repo's current home. `tb` uses the
laude-institute URL by default (`terminal_bench/registry/client.py:90`), so
nothing needs overriding.

Rejected: aider's own `benchmark/` (same exercises, a second Docker image and
a second results format), and `mini-swe-agent` (SWE-bench only, Python only —
terminal-bench already carries `swebench-verified` under the same CLI).

Languages in scope: **Go, JavaScript, Python**. No cpp, java or rust.

## The two hard constraints, measured

### 1. Reasoning-on is impossible here

Task defaults are `max_agent_timeout_sec: 900.0` (15 min), confirmed in the
`task.yaml` of `hello-world`, `fibonacci-server`, `csv-to-parquet`, `fix-git`.

At the ~14 t/s these models decode, an 8192-token thinking block costs
**~585 s ≈ 9.8 min per turn**. That is ~1.5 turns inside a 15-minute budget.
A 20-turn agentic loop with reasoning on cannot finish.

→ exam_v4 runs the `-nothink` llama-swap aliases (`qwen36-moe-nothink`,
`gemma4-26b-qat-mtp-nothink`). This is a deliberate divergence from exam_v3,
which runs reasoning on. State it in any comparison.

### 2. The agent never manages its own context

`terminus-2` sizes context via `litellm.get_max_tokens(model_name)`
(`terminus_2.py:158-165`). Measured: `openai/qwen36-moe` **raises**
("isn't mapped yet"), so it falls back to `fallback_context_limit = 1_000_000`
and therefore never summarises or unwinds history — it appends until the
server rejects.

Each terminal observation is capped at 10 000 bytes ≈ 2.5k tokens
(`_limit_output_length`, default `max_bytes=10000`), so a 16k context dies
around turn 4.

→ Serve locals at their configured `-c 131072`. For the 122B, `-c 65536`
(KV q8_0 ≈ 3.2 GiB; weights 39.9 GiB + KV 43.1 GiB against a 57 GiB GTT).

Related, smaller: `lite_llm.py` never passes `max_tokens` to
`litellm.completion`, and raises `OutputLengthExceededError` on
`finish_reason == "length"`. Server-side `--predict 32768` should cover it —
but this is the same failure class that produced the structural 0/13 in
exam_v3, so check the first run's episode logs for it.

## The first subset — 5 scored tasks + 1 preflight

| # | task | dataset | language | why |
|---|---|---|---|---|
| 0 | `hello-world` | core 0.1.1 | — | plumbing preflight, **not scored** |
| 1 | `fix-git` | core 0.1.1 | — | pure agentic, expert est. 5 min |
| 2 | `csv-to-parquet` | core 0.1.1 | python | agentic + code, easy |
| 3 | `fibonacci-server` | core 0.1.1 | python/js | medium: write code, run a server, verify |
| 4 | `polyglot_go_book-store` | aider-polyglot | go | Go coding + edit-format compliance |
| 5 | `polyglot_python_bowling` | aider-polyglot | python | non-Go coding control |

Five scored tasks is deliberately small: per-task wall time on this box is
unmeasured, and a 20-turn loop at ~800 s/turn is the pessimistic case. Measure
task 0 and task 1 first, then decide whether 5 → 10 is affordable.

## How to run it

```zsh
export PATH="$HOME/.local/bin:$PATH"

# preflight: the oracle agent must pass. If it doesn't, no model score means
# anything (same role as exam_v3's 13/13 reference-response gate).
tb run --dataset terminal-bench-core==0.1.1 --agent oracle \
  -t hello-world -t fix-git --n-concurrent 1

# a model cell
tb run --dataset terminal-bench-core==0.1.1 \
  --agent terminus-2 \
  --model openai/qwen36-moe-nothink \
  -k api_base=http://127.0.0.1:8090/v1 \
  -t hello-world -t fix-git -t csv-to-parquet -t fibonacci-server \
  --n-concurrent 1 \
  --global-agent-timeout-sec 3600 \
  --output-path artifacts/results/exam_v4/<display>
```

`--n-concurrent 1` is mandatory: llama-swap serves one slot, and concurrent
trials would queue behind each other while their per-task clocks run.

## Non-negotiables

1. **Oracle preflight.** `--agent oracle` must pass every task in the subset
   before any model number is recorded.
2. **`--n-concurrent 1`.** See above.
3. **`-nothink` aliases.** See constraint 1.
4. **Raise `--global-agent-timeout-sec`.** The 900 s default is a hosted-model
   budget, not a local-model one. Start at 3600.
5. **Power profile must be `performance`.** Any timing recorded under
   `low-power` is not comparable. Needs sudo, see TODO.

## What "done" looks like

Per cell: tasks passed / 5, wall seconds per task, and the episode count at
which each failure happened (timeout vs. wrong answer vs. context blowup —
these are different findings and the run logs distinguish them).

Cells: `gemma4-26b-qat-mtp-nothink` (current best local), `qwen36-moe-nothink`,
Haiku 4.5 as the ceiling reference, and the 122B once exam_v3 has scored it.

## Open, unmeasured

- Per-task wall time on this hardware. Everything above is arithmetic, not
  measurement.
- Whether `aider-polyglot` tasks need a dataset download step separate from
  `terminal-bench-core` (they live in a different repo,
  `laude-institute/terminal-bench-datasets`).
- Whether `terminus-2`'s JSON parser survives these models' output. The
  parser has a `salvage_truncated_response` path, which suggests it often
  does not.
