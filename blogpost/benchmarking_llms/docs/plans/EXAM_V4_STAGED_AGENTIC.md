# Re-run exam_v3 on the ROCmFP4 MoE — task for a fresh session

_Written 2026-08-06. Status: not run. **Nothing new to build.**_

> Filename is stale. This file used to hold a staged-agentic "exam_v4" design.
> That was dropped as over-engineering; the plan below replaced it. The old
> design is in git at commit `10f6678` if it is ever wanted again.

## The question

Is `jcbtc`'s ROCmFP4 quant of Qwen3.6-35B-A3B better or worse than the Unsloth
quant of the same model, for Go coding on this laptop?

Nothing else. Not agentic ability, not a new harness. **Use `exam_v3` exactly as
committed** — same `prompt.txt`, same `eval.sh`, same `exam-driver.go`, /13.

## Cells

| cell | display | serving | model | size |
|---|---|---|---|---|
| A | `rocmfp4-moe-35b-a3b` | container on `:18080` | `Qwen3.6-35B-A3B-NSC-ACE-SABER-MTP-F16-to-ROCmFP4-STRIX_LEAN.gguf` | 19.047 GB, ~4.29 bpw |
| B | `qwen36-moe-unsloth` | llama-swap `:8090`, model `qwen36-moe` | `Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf` | 22.854 GB, ~5.15 bpw |
| C | `gemma4-26b-qat` | llama-swap `:8090`, model `gemma4-26b-qat-mtp` | `gemma-4-26B-A4B-it-qat-UD-Q4_K_XL.gguf` + MTP drafter | 14.25 + 0.25 GB |

A vs B is the question — same base model, same MTP head, different quant and
tune. C is the control: Gemma topped the April exam_v3 table at 11/13, but this
is the QAT rebuild, not April's MXFP4 build, so it must be re-measured rather
than quoted.

Seeds `42 123`. A and B cannot be resident at once (17.7 + 21.3 GiB on a 62 GiB
box) — run cells sequentially, tearing down between them.

## How to run it

A runner already exists: `scripts/sweep-exam3-rocmfp4.sh` (committed, `1dbc5d7`
+ `596132e`). It does preflight, warm-up, sequential cells, teardown, and
resumes on re-invocation.

```bash
cd ~/play/msf.github.io/blogpost/benchmarking_llms
SEEDS="42 123" CELLS="A B C" ./scripts/sweep-exam3-rocmfp4.sh
```

If you'd rather not trust that script, the manual equivalent per cell is:

```bash
go run ./exam-driver.go \
  -endpoint <http://127.0.0.1:8090 | http://127.0.0.1:18080> \
  -prompt bench/exam_v3/prompt.txt \
  -eval bench/exam_v3/eval.sh \
  -out artifacts/results/exam_v3 \
  -seed 42 -temp 1.0 -max-tokens 8192 -timeout 15m \
  <served-model-name>
```

Cell A's container command is in `docs/notes/ROCMFPX_QWOPUS_SETUP.md` §6, with
the flags matched to llama-swap's `qwen36-moe` entry.

## Non-negotiables

1. **Preflight the grader.** `bench/exam_v3/make-reference-response.py` builds
   the response a perfect model would emit; `eval.sh` must score it **13/13**.
   If it doesn't, no model score from that run means anything. The runner does
   this and aborts on failure.
2. **Warm up and discard.** Cold prefill measured 1.07 t/s vs 13.10 warm — a 12x
   artifact on `wall_s`.
3. **Stop `llama-fim.service`** for the duration; restore it after.
4. **Match server config across cells.** Parity was proven by identical prompt
   tokenization (10088 both sides). Container flags must mirror llama-swap's
   `qwen36-moe`: `--flash-attn on --cache-type-k/v q8_0 -ngl 99 --no-mmap
   --ctx-checkpoints 0 --jinja --parallel 1 -c 131072 --reasoning on
   --reasoning-budget 8192 --spec-type draft-mtp --spec-draft-n-max 3
   --temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0`.

## Deviations from the April 2026 clean rerun — state these in any writeup

The April numbers in `bench/exam_v3/REPORT.md` are historical context, **not a
control**:

- April used `--reasoning off`. This run uses reasoning **on**, because that's
  how the box actually serves now.
- Gemma is the QAT rebuild, not April's MXFP4 build. Different weights.
- llama.cpp release, llama-swap version, and MTP drafters all moved since April.

That is exactly why cell C exists: it is the control *for this run*.

## What "done" looks like

A table of score /13 per cell per seed, plus wall seconds and tok/s, written to
`artifacts/results/exam_v3/<display>/seed<N>/result.json` and summarised in a
`status.tsv`. Two seeds means best/worst, not a mean worth defending.

Expected cost: 6 attempts, ~1–2 h wall.

## Known-good speed numbers already measured (no need to redo)

From `docs/notes/ROCMFPX_QWOPUS_SETUP.md` §7b, on this laptop:

| model | backend | decode t/s |
|---|---|---|
| ACE-SABER 35B-A3B MoE ROCmFP4, no MTP | HIP | 9.78 |
| ACE-SABER 35B-A3B MoE ROCmFP4, MTP | HIP | 14.6 |
| Qwen3.6-35B-A3B UD-Q4_K_XL (Unsloth) | Vulkan | ~22 |
| Gemma 4 26B-A4B MXFP4 (April build) | Vulkan | 18.6 |

So ROCmFP4 is already known to be **slower**. The only open question is whether
it is *better*, at 3.8 GB less on disk. If it scores worse and runs slower,
that's the end of the line for this quant on this box.

## Caveat that bounds the conclusion

ACE-SABER is a chadrock **re-tune**, not a requant of Unsloth's weights. Quant
and tune both vary, so an A-vs-B gap cannot be attributed to ROCmFP4 alone. The
clean control would be a third cell with
`CHADROCK3.6-35B-UNCENSORED-MTP-STRIX-LEAN` (19.047 GB, byte-identical size,
different tune) — see `docs/notes/JCBTC_MOE_CANDIDATES.md`. Not downloaded.

## Prior art to read first

- `docs/notes/ROCMFPX_QWOPUS_SETUP.md` — build, run, and the measured speeds.
- `docs/notes/JCBTC_MOE_CANDIDATES.md` — the model catalogue and why ACE-SABER.
- `bench/exam_v3/REPORT.md` — the April table this is compared against.
- `machines/framework13.md` — box conventions, and the caveat that exam_v3 on
  this machine punishes Qwen-family locals for reasons still unresolved.
