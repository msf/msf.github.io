# exam_v4 — Muse Glimmer 30B on the R9700

Run window: 2026-08-12 00:41:50 → 07:25:41 (Europe/Lisbon), 6h42m.
Machine: `hopper`. Numbers read back from
`artifacts/results/exam_v4_tb21/jobs/2026-08-12__00-41-50/`.

Adds a sixth model to the sweep in `EXAM_V4_2026-08-09_TB21.md`. Same subset
(`strat20`), same agent, same scoring — one deliberate difference in the
runtime, noted under *Caveats*.

## The model

Meta Superintelligence Lab's **Muse Glimmer 30B**, released 2026-08-09. First
dense 30B from Meta, marketed for local agentic work. Served as
`muse-glimmer-30b`.

Architecture read out of the GGUF on-box (the model card matched on every
field):

| | |
|---|---|
| params | ~29.6B dense (incl. vision encoder) |
| layers | 52 |
| hidden / FFN | 6656 / 19968, SwiGLU |
| heads (Q/KV) | 32 / 2, GQA 16:1, head dim 128 |
| attention | `[Local, Local, Local, Global]` × 13, SWA window 2048 |
| RoPE | θ = 500,000, local layers only |
| vocab / ctx | 202,048 / 131,072 |

Quant: `muse-glimmer-30B-kquant-dynamic.gguf` (18.3 GiB), the build Meta size
for 32 GB cards — 0.2% claimed degradation across 15 benchmarks, against 1.0%
for the 24 GB `kquant-17gb` build. The `mmproj` perception encoder was not
loaded; exam_v4 has no image input.

## Result

**10/46 (22%), 5 of 20 tasks, 6h42m.** Fourth of six.

| model | easy | medium | hard | score | tasks | runtime | per task |
|---|---|---|---|---|---|---|---|
| `qwen-35b-moe-mtp` | 1/1 | 5/12 | 2/7 | **17/46** (37%) | 8/20 | 6h08m | 18m |
| `qwen-27b-mtp` | 1/1 | 5/12 | 1/7 | **14/46** (30%) | 7/20 | 7h15m | 21m |
| `gemma-31b-qat` | 1/1 | 4/12 | 1/7 | **12/46** (26%) | 6/20 | 7h36m | 22m |
| **`muse-glimmer-30b`** | 1/1 | 3/12 | 1/7 | **10/46** (22%) | 5/20 | 6h42m | 20m |
| `qwen-27b-mtp-q6` | 1/1 | 4/12 | 0/7 | **9/46** (20%) | 5/20 | 7h13m | 21m |
| `gemma-26b-moe` | 1/1 | 3/12 | 0/7 | **7/46** (15%) | 4/20 | 8h10m | 24m |

Solved: `bn-fit-modify`, `cobol-modernization`, `distribution-search`,
`portfolio-optimization`, `vulnerable-secret`.

Failed: `break-filter-js-from-html`, `caffe-cifar-10`, `cancel-async-tasks`,
`chess-best-move`, `circuit-fibsqrt`, `compile-compcert`, `dna-insert`,
`feal-differential-cryptanalysis`, `feal-linear-cryptanalysis`,
`filter-js-from-html`, `make-doom-for-mips`, `model-extraction-relu-logits`,
`nginx-request-logging`, `query-optimize`, `rstan-to-pystan`.

### Where it sits

Against `gemma-31b-qat` (12) the two-point gap is **one task** — inside the
noise floor this design admits, per the standing rule not to rank models on a
2-task gap at n=20 with one attempt each. Against `qwen-35b-moe-mtp` (17) the
gap is 7 points / 3 tasks, which is more likely real but still unreplicated.

The honest summary: Muse Glimmer landed **mid-pack, not at the top**, and
nothing here supports a stronger claim in either direction.

## The gap against Meta's published number

Meta report **51.7%** on Terminal-Bench 2.1 with `terminus-2` — the same
benchmark and the same agent. We measured 22%. That is a large gap and it is
not explained by anything found in the run:

- **Not truncation.** Server logs show `truncated = 0` and generations running
  past the cap (`n_decoded = 8940` on one task) before stopping cleanly. The
  `--reasoning-budget 8192` did not sever responses.
- **Not harness errors.** Zero `AgentTimeoutError`, zero API errors, zero
  malformed-JSON rejections across 663 agent steps.
- **Not GPU contention.** The model stayed resident for all 400 minutes; VRAM
  flat at 21 GiB, no swaps, no evictions.
- **Not speed.** Sustained ~39 tok/s decode throughout.

Remaining candidate explanations, none verified here:

1. **Subset vs full set.** `strat20` is 20 tasks weighted 1 easy / 12 medium /
   7 hard. Meta's figure is presumably the full 89. The subset is deliberately
   harder than a curated pick — an earlier hand-picked 12-task set scored 83%
   where `strat20` scored 40% on the same model.
2. **`reasoning_strength`.** We ran the template default (`high`). The card
   recommends `high` *or* `xhigh` for agentic work; `xhigh` is untested here.
3. **Thinking cap.** 8192 tokens/turn is imposed on every arm of this sweep for
   comparability, not tuned per model. A model that "reasons at length" may pay
   more for it than the others do, even without hard truncation.
4. **Quantization.** 4-bit dynamic, claimed 0.2% degradation. Unlikely to
   account for a 30-point gap, and unverified on this benchmark.
5. **Harness/scaffold differences** between Meta's terminus-2 setup and ours.

Worth one follow-up run at `reasoning_strength: xhigh` before drawing any
conclusion about the model's agentic ability.

## Speculative decoding

Muse ships a DFlash block-diffusion drafter (`dflash-kquant.gguf`, 1.52 GiB;
5 layers, block size 16) and **no MTP head**, so the question was DFlash vs no
drafting rather than DFlash vs MTP.

Measured warm, 1200 tokens, temp 0, identical prompt/seed:

| config | tok/s | acceptance | mean accepted len |
|---|---|---|---|
| no drafter | 24.80 | — | — |
| `--spec-draft-n-max 4` | **48.43 / 48.19 / 48.46** | 45.1% | 2.80 |
| `--spec-draft-n-max 8` | 16.01 | 24.6% | 2.97 |
| `--spec-draft-n-max 16` | 16.42 | 14.5% | 3.16 |

**1.95x at n-max 4**, and it held up in production — the exam sustained ~39
tok/s over 400 minutes. n-max is a sharp optimum, not a plateau: 8 and 16 are
worse than running no drafter at all, because mean accepted length barely moves
(2.80 → 3.16) while drafted width quadruples. Same optimum the Gemma 4 MTP
entries landed on.

Drafter costs +2.02 GiB VRAM (18.77 → 20.79 GiB at `-c 131072`).

Note the model card documents only `-md <file> -ngld 99`. On b10362 that is
insufficient — `--spec-type` defaults to `none`, so the drafter loads and is
never used. `--spec-type draft-dflash` is required.

## Turn-taking: Muse is terse and incremental

Measured from the trajectories, across all 20 tasks:

| model | agent steps/task | completion tokens/task | tokens/step |
|---|---:|---:|---:|
| `muse-glimmer-30b` | **33.1** | **17,907** | **540** |
| `qwen-27b-mtp` | 14.6 | 22,780 | 1,560 |
| `gemma-31b-qat` | 12.0 | 26,683 | 2,224 |
| `qwen-35b-moe-mtp` | 20.4 | 30,002 | 1,471 |

Muse takes roughly **three times as many turns** as `gemma-31b-qat` while
emitting about **a third fewer tokens** — short commands, inspect, repeat, where
Gemma writes a long block and then checks it.

This explains the runtime anomaly. Muse finished second-fastest (6h42m) despite
solving fewer tasks than two models above it, and it is *not* because it decodes
faster: throughput sampled from the monitoring sidecar during the runs gives
Muse **42.3 tok/s** against `gemma-31b-qat`'s **46.4**, on comparable prefill
(345 vs 365 pp/s). Total tokens generated, not decode rate, set the wall clock.

Sidecar means over the exam windows (rough — real agentic load, prompt-cache
hits and swaps included):

| model | decode tok/s | prefill pp/s |
|---|---:|---:|
| `qwen-35b-moe-mtp` | 118 | 1256 |
| `gemma-26b-moe` | 110 | 949 |
| `qwen-27b-mtp` | 49.0 | 394 |
| `gemma-31b-qat` | 46.4 | 365 |
| `muse-glimmer-30b` | 42.3 | 345 |

`qwen-27b-mtp-q6` is absent — it ran before the sidecar scraped that series.

## Caveats

1. **Runtime differs from the other five arms.** Muse Glimmer needs llama.cpp
   **>= b10353** (arch merged 2026-08-10, PR #26841); the other five ran on
   **b10025**. This arm ran on **b10362**. Unavoidable — b10025 cannot load the
   file at all — but it is a real confound between this row and the rest of the
   table. The box moved wholesale to b10362 after the run.
2. **n=20, one attempt per task.** Unchanged from the 2026-08-09 sweep. Do not
   rank on a 2-task gap.
3. **VRAM sizing prediction was wrong before the run.** The plan of record
   assumed `kquant-dynamic` would not fit at `-c 131072` on a 32 GiB card,
   extrapolating from a Q6 Qwen3.6-27B that measured 29.85 GiB. Actual: 18.77
   GiB, ~13 GiB spare. The extrapolation ignored that SWA keeps 39 of 52 layers
   on a fixed 2048-token cache. VRAM does not transfer across attention
   patterns.

## Reproducing

```bash
LAB=~/play/msf.github.io/blogpost/benchmarking_llms
setsid nohup $LAB/scripts/tb21-run.sh muse-glimmer-30b strat20 \
  > /tmp/muse-glimmer-30b.log 2>&1 &
$LAB/scripts/tb21-scoreboard.py --html \
  $LAB/artifacts/results/exam_v4_tb21/scoreboard.html
```

Serving config: `/srv/selfhost/llm/llama-swap.yaml`, entry `muse-glimmer-30b`.
Wiki: `[[muse-glimmer-30b]]`, `[[2026-08-12-muse-glimmer-dflash-bench]]`.
