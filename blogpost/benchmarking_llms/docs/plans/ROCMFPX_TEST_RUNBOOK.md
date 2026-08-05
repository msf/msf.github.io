# ROCmFPX / Qwopus test runbook

_Date: 2026-08-03. Status: plan. Nothing in Tier 2/3 is built yet._

Setup instructions: `../notes/ROCMFPX_QWOPUS_SETUP.md`.
Model catalogue + MoE shortlist: `../notes/JCBTC_MOE_CANDIDATES.md`.
Prior agentic design (superseded by Tier 3 below): `../notes/PHASE2_AGENTIC_HARNESS_DESIGN.md`.

## What this runbook is for

We have a working ROCmFPX container on two machines and no methodology for it.
The existing harnesses (`exam_v1`, `exam_v3`) assume `llama-swap` on `:8080` and
measure single-shot code generation. This model family is served standalone,
targets tool-calling, and ships an MTP head — so the runbook has to cover a
correctness gate, a perf baseline, the existing exams, and the agentic tier
that's been missing since April.

**Key enabler:** `exam-driver.go` takes `-endpoint`. Point it straight at the
container's port. No `llama-swap` config change on either box.

## Machines

| id | GPU | target | serving | constraint |
|---|---|---|---|---|
| fw13 | Radeon 890M | `gfx1150` | container on `127.0.0.1:18080` | 89.6 GB/s bus is the ceiling |
| hopper | Radeon AI PRO R9700 | `gfx1201` | container on `127.0.0.1:18081` | model+KV must fit 31.9 GiB VRAM |

**hopper isolation rules (hard):**
- Never edit `/srv/selfhost/llm/llama-swap.yaml`. `-watch-config` is on; an edit
  kills the in-flight production model and hermes 5xx's.
- Port 18081, not 8090. Distinct container name. `docker rm` when done.
- No `GGML_HIP_ENABLE_UNIFIED_MEMORY` — host has 30 GiB with ~14 GiB used; GTT
  spill is not charged to any cgroup. See `selfhost/llm/triage/2026-05-07-oom-after-boot.md`.
- Run `curl 127.0.0.1:8090/unload` first to free VRAM, then confirm
  `amdgpu_vram_used_bytes` is near-idle before loading the experiment.

## Tier 0 — correctness gate (hopper only, blocking)

RDNA4 + ROCmFP4 is unvalidated by the publisher. The quant ftype is literally
named `STRIX_LEAN`; `vendors/hip.h:213` sends `gfx1201` down the `RDNA4` path,
not the `RDNA3_5` path the kernels were tuned on. **Do not believe a single
throughput number from hopper until this passes.**

Our current image can't run this — the Dockerfile change limited targets to
`llama-server llama-bench`. Build one validation image without the limit:

```bash
# on hopper, after the gfx1201 server image finishes
cd ~/play/ROCmFPX-qwopus
git stash                      # drop the target-limit patch for this build only
docker build --target full --build-arg CMAKE_HIP_ARCHITECTURES=gfx1201 \
  --build-arg JOBS=4 -t local/rocmfpx-qwopus:gfx1201-tests \
  -f .devops/strix-rocmfp4.Dockerfile .
git stash pop
```

Then, in the container with `/dev/kfd` + `/dev/dri`:

```bash
/app/test-backend-ops -o MUL_MAT -b ROCm0      # ROCmFP4 MMQ vs CPU reference
/app/test-quantize-fns                          # quant round-trip
```

| outcome | action |
|---|---|
| both pass | proceed to Tier 1 |
| `MUL_MAT` fails on ROCmFP4 types | rebuild at `gfx1200` (documented target) and retest |
| still fails | ROCmFP4 is fw13-only. Record it, run hopper on stock llama.cpp quants, stop. |

Cross-check on fw13 too, so a hopper failure is attributable to the arch and not
the commit.

## Tier 1 — perf baseline

`llama-bench`, then the server. Both machines, same model file.

```bash
docker run --rm --device=/dev/kfd --device=/dev/dri \
  --group-add video --group-add render --security-opt seccomp=unconfined \
  -v <models>:/models:ro --entrypoint /app/llama-bench <image> \
  -m /models/<file>.gguf -dev ROCm0 -ngl 999 -fa 1 -p 512 -n 128 -r 3
```

Record: `pp512`, `tg128`, VRAM/GTT peak, wall. `llama-bench` does not exercise
MTP — it is the **no-MTP floor**, which is exactly what we want as the denominator.

Then MTP via the server, using the `serve.sh`-derived command from the setup doc.
Per request, harvest from `print_timing`: `prompt_per_second`,
`predicted_per_second`, `draft_n`, `draft_n_accepted`, mean acceptance length.

**Discard the first request.** Measured on fw13: 1.07 t/s cold prompt vs 13.10
t/s warm — a 12x artifact. Warm with one throwaway call, then measure 3.

Fixed axes for every cell: `-c 8192` for exam parity (see Tier 2 note),
`-ctk q4_0 -ctv q4_0`, `-b 512 -ub 512`, `-dev ROCm0`, `--parallel 1`,
`temp 1.0`. fw13 on **power-saver** (tighter variance, per `machines/framework13.md`).

Cells:

| # | machine | model | MTP | thinking |
|---|---|---|---|---|
| 1 | fw13 | Qwopus 27B dense (done: 1.97 / 5.49 t/s) | off / on | off |
| 2 | fw13 | ACE-SABER 35B-A3B MoE 19.0 GB | off / on | off |
| 3 | hopper | Qwopus 27B dense | off / on | off |
| 4 | hopper | ACE-SABER 35B-A3B MoE | off / on | off |

**Prune rule:** if MoE-on-fw13 (cell 2) doesn't clear ~15 t/s with MTP, the whole
ROCmFPX line is dead on that laptop — stop testing fw13 and keep it on hopper only.

## Tier 2 — existing exams (quality, reproducible)

Reuses `exam_v1` (/15) and `exam_v3` (/13) unchanged. Seeds `42 123 456`.
`max_tokens=8192`, timeout `10m`.

```bash
cd ~/play/msf.github.io/blogpost/benchmarking_llms
go run ./exam-driver.go \
  -endpoint http://127.0.0.1:18080 \
  -prompt bench/exam_v3/prompt.txt \
  -eval bench/exam_v3/eval.sh \
  -out artifacts/results/exam_v3 \
  -seed 42 -max-tokens 8192 -timeout 10m \
  qwopus
```

Note `go` is **not installed on hopper**. Either install it there or run the
driver from fw13 against hopper's port over the tailnet — the eval compiles Go
locally, so driving remotely is the simpler path, and the endpoint is the only
machine-dependent bit.

Context caveat: exam_v3 was scored at 16k on the Vulkan stack. Our container runs
`-c 8192`. Raise to 16384 for exam runs or the comparison to the Gemma/Qwen table
in `bench/exam_v3/REPORT.md` is invalid. Check VRAM headroom on hopper first.

Interpretation guard: fw13 exam_v3 results are known to punish Qwen-family locals
(`7/13` best Qwen vs `11/13` Gemma) and the cause is unresolved. A bad ROCmFPX
score here is **not** evidence about the tune until Tier 3 corroborates.

## Tier 3 — agentic (the missing test)

Goal: measure what we actually use local models for — tool loops — without
pretending trajectories are reproducible. Three sub-tiers, cheapest first. Build
0 and 1; treat 2 as stretch.

### 3.0 Tool-call schema adherence (deterministic, ~2 min per model)

No repo, no loop. One request with a `tools` array, `tool_choice: auto`, a prompt
that unambiguously requires one call. 20 trials, varying seed.

Score per trial: (a) emitted a tool call at all, (b) valid JSON, (c) correct tool
name, (d) required args present and well-typed, (e) did **not** hallucinate a
tool outside the schema. Report as a 5-point rate over 20.

This is the highest signal-per-token test we can run and it discriminates hard —
a model that can't emit a clean call cannot do 3.1 at all. Run it first as a gate.
`chadrock3.6-27b-pi-agent` is tagged `tool-calling` + `no-thinking`; this is the
test that justifies downloading it.

### 3.1 Bounded tool loop, frozen repo, deterministic verifier

New `agent-driver.go` in the lab root, alongside `exam-driver.go`. Same shape:
`-endpoint`, `-task`, `-out`, `-seed`, hard caps.

Tool surface — deliberately three tools, no shell:

| tool | args | why |
|---|---|---|
| `list_files` | `dir` | forces exploration instead of guessing paths |
| `read_file` | `path` | read-only, the bulk of real agent turns |
| `write_file` | `path`, `content` | whole-file writes; no patch-format skill confound |
| `run_tests` | — | the only feedback signal; runs `go test ./...` in the sandbox |

No `bash`. A shell tool makes the trace unbounded and lets the model score by
accident. Sandbox is a temp copy of the frozen repo; the driver executes the
tools for real.

Hard caps per attempt: **20 turns**, **20 min wall**, **32k total tokens**.
Exceeding any cap = fail, recorded with the reason.

Task 1 — `bug-hunt-01`. Reuse `bench/exam_v3/scraper.go` as the repo, with
`scraper_solution.go` mutated to inject one known defect (candidate: the
`rand.Intn(0)` panic at `-buffer-size 0`, which `grader_test.go` already catches).
Pre-fix `go test` fails; post-fix it must pass. The grader already exists — that
is the whole reason to start here rather than a fresh repo.

Metrics per attempt, per the Phase 2 doc: `passed`, `turns`, `tool_errors`,
`retry_count` (identical failed call repeated), `total_tokens`, `wall_s`.
Aggregate per (task, model): `pass_rate` over 3 attempts, `median_turns` on
passes only, `tool_error_rate = tool_errors/turns`.

Baseline first: run **Gemma 4 26B MXFP4** (current best local, plain Vulkan via
llama-swap on fw13) before any ROCmFPX model. If Gemma can't pass it, the task is
mis-scoped and no ROCmFPX number means anything.

### 3.2 Stretch — second task for generalization

`k8s-debug-01` from the Phase 2 candidate list: broken manifest + pod logs +
`describe` output, read-only, scored on keyword match against a known root cause.
Cheap to build, tests diagnosis rather than codegen. Only after 3.1 is stable
across two models.

Explicitly **not** doing: real `pi` session instrumentation (multi-week data
collection), refactor tasks (subjective), log-investigation tasks (weak signal —
same conclusion as the April design doc).

## Execution order and time boxes

Learned from the aborted 7-hour April sweep: declare the stop clock up front and
write partial results.

| step | box | budget | gate to proceed |
|---|---|---|---|
| 1. Tier 0 correctness | hopper | 1 h (mostly build) | must pass or hopper is out |
| 2. Tier 1 baseline, dense both boxes | both | 1 h | — |
| 3. Download ACE-SABER MoE 19.0 GB | both | 25 min @ 15 MB/s | size + sha256 vs LFS oid |
| 4. Tier 1 baseline, MoE both boxes | both | 1 h | fw13 prune rule above |
| 5. Tier 3.0 tool adherence | best box | 30 min | gate for 3.1 |
| 6. Tier 2 exams, best 1–2 configs | best box | 2 h | — |
| 7. Tier 3.1 build + Gemma baseline | fw13 | 3 h | separate session |

Steps 1–4 are one session. Step 7 is its own. Do not start 3.1 in the same
session as the sweep — that is exactly how April got consumed by harness
debugging instead of measurement.

## Where output goes

- perf cells: `artifacts/results/rocmfpx/<machine>-<model>-<mtp>/`
- exams: existing `artifacts/results/exam_v3/<display>/seed{N}/`
- agentic: `artifacts/results/agentic/<task>/<model>/attempt{N}/` with
  `transcript.log`, `tool_calls.json`, `verify_output`, `result.json`
- logs: `artifacts/logs/rocmfpx/<run-id>/`
- write-up target: a Part 4 blog post, dense-vs-MoE on two AMD generations plus
  the first agentic numbers.

## Open questions for the user

1. Tier 3.1 tool surface — is the no-`bash`, four-tool set acceptable, or do you
   want the real `pi` tool schema so results transfer to actual use?
2. Is `bug-hunt-01` derived from `exam_v3/scraper.go` good enough, or do you want
   to nominate a real repo/incident you debugged recently (the April doc asked
   this and it never got answered)?
3. Does hopper get `go` installed, or do we always drive it remotely from fw13?
