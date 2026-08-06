# exam_v3 re-run: ROCmFP4 vs Unsloth vs Gemma, two temperatures, plus a hosted control

_Date: 2026-08-06. Machine: Framework 13, Ryzen AI HX 370, Radeon 890M, 62 GiB._
_Lab git sha at run time: `c76d2ca`. Exam, prompt, evaluator and driver unchanged from April._

16 attempts. Answers the question in `docs/plans/EXAM_V4_STAGED_AGENTIC.md` (ROCmFP4
vs Unsloth), plus two things that question turned out to depend on: sampling
temperature, and whether the exam's ceiling is really 13.

## Runs

| arm | run id | results dir | temp |
|---|---|---|---|
| aborted | `20260806-160135` | — (cleared) | 1.0, `max_tokens 8192` — harness bug, see below |
| local A | `20260806-162444` | `artifacts/results/exam_v3/` | 1.0 |
| local B | `20260806-183911-temp06` | `artifacts/results/exam_v3_temp06/` | 0.6 |
| hosted | `20260806-203809-hosted` | `artifacts/results/exam_v3_hosted/` | 0.6 and 1.0 |

Artifacts are gitignored, so the tables below are the record.

## Cells

| cell | model | serving | size |
|---|---|---|---|
| A | `Qwen3.6-35B-A3B-NSC-ACE-SABER-MTP-F16-to-ROCmFP4-STRIX_LEAN.gguf` | container `:18080`, HIP gfx1150 | 19.05 GB |
| B | `Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf` (Unsloth) | llama-swap `qwen36-moe`, Vulkan | 22.85 GB |
| C | `gemma-4-26B-A4B-it-qat-UD-Q4_K_XL.gguf` + MTP drafter | llama-swap `gemma4-26b-qat-mtp`, Vulkan | 14.25 + 0.25 GB |
| H | `claude-haiku-4-5-20251001` | `https://api.anthropic.com`, OpenAI-compat | hosted |

Reasoning ON for all local cells (April used `--reasoning off`). Seeds 42 and 123.

## Results

| cell | model | temp 1.0 seed42 | temp 1.0 seed123 | temp 0.6 seed42 | temp 0.6 seed123 |
|---|---|---:|---:|---:|---:|
| A | ROCmFP4 ACE-SABER 35B-A3B | 7/13 | 0/13 | 6/13 | 0/13 |
| B | Unsloth UD-Q4_K_XL 35B-A3B | 7/13 | 5/13 | 6/13 | 0/13 |
| C | Gemma 4 26B-A4B QAT | 0/13 | 6/13 | **7/13** | **11/13** |
| H | Haiku 4.5 | 11/13 | 11/13 | 10/13 | 11/13 |

Throughput and wall time:

| cell | temp | seed | tok | t/s | wall s |
|---|---|---|---:|---:|---:|
| A | 1.0 | 42 | 10817 | 13.32 | 833.4 |
| A | 1.0 | 123 | 9702 | 14.33 | 691.0 |
| A | 0.6 | 42 | 9115 | 14.49 | 658.5 |
| A | 0.6 | 123 | 10288 | 13.91 | 752.4 |
| B | 1.0 | 42 | 10624 | 11.29 | 959.7 |
| B | 1.0 | 123 | 10735 | 11.65 | 939.6 |
| B | 0.6 | 42 | 10106 | 11.95 | 864.3 |
| B | 0.6 | 123 | 10767 | 12.25 | 897.7 |
| C | 1.0 | 42 | 11488 | 14.16 | 824.8 |
| C | 1.0 | 123 | 10323 | 14.14 | 743.9 |
| C | 0.6 | 42 | 11138 | 14.34 | 790.9 |
| C | 0.6 | 123 | 11146 | 14.17 | 800.5 |
| H | 0.6 | 42 | 3736 | n/a | 19.7 |
| H | 0.6 | 123 | 3342 | n/a | 19.0 |
| H | 1.0 | 42 | 3226 | n/a | 17.9 |
| H | 1.0 | 123 | 3869 | n/a | 21.4 |

Hosted `tps` is 0 because the Anthropic response has no llama.cpp `timings` block.
Cost of the hosted arm: ~$0.11 total.

## Findings

**1. ROCmFP4 is not worth it.** Cell A never beat cell B at either temperature. It
*is* faster than Unsloth here (13.3–14.5 vs 11.3–12.3 t/s), which contradicts the
plan's prior that ROCmFP4 is slower — the §7b speeds in `ROCMFPX_QWOPUS_SETUP.md`
were measured without reasoning and at short generations. At ~10k tokens with
reasoning on, Vulkan Unsloth lands well below its quoted ~22 t/s. The caveat from
the plan stands: ACE-SABER is a re-tune, not a requant, so the A/B gap is not
attributable to the quantisation alone.

**2. Temperature is not a global knob — it is per-model, and it is large.** Gemma
went 0→7 and 6→11 moving from 1.0 to 0.6. Both Qwen cells got *worse* at 0.6,
with cell B losing 5 points on seed123. Unsloth's own 0.6 recommendation for
Qwen3.6 thinking mode is contradicted on this exam. Any table that fixes one
temperature across models is measuring the temperature as much as the model.

*Confound:* cell A's 0.6 column also gained sampler-parity flags (see 4), so its
1.0→0.6 delta is not temp-only. Cells B and C changed temperature only.

**3. Temperature is noise for Haiku** (11/11/11 and one 10), and Haiku is 4–5
points above the best local run at ~1/40th the wall time (≈20 s vs ≈800 s).

**4. Cell A and cell B were not sampler-matched in the temp-1.0 arm.** The
container command in `scripts/sweep-exam3-rocmfp4.sh` set no `--top-p/--top-k/--min-p`,
so cell A ran llama.cpp defaults (`top_k 40`, `min_p 0.05`) while cell B ran
`top_p 0.95 / top_k 20 / min_p 0.0` from `~/play/llama/config.yaml`. Non-negotiable
#4 of the plan was therefore not met in that arm. Fixed in the script; cell A's
0.6 arm is matched. Note the container logs
`device 'ROCm0' does not have support for op TOP_K` — top-k is applied on CPU.

**5. The compile-failure hypothesis was wrong.** 2 of 6 local attempts failed
structurally at each temperature; 0.6 only moved *which* cells failed (A/123 and
C/42 at 1.0; A/123 and B/123 at 0.6). All failures were genuine model errors,
verified by reading the artifacts:

- A/123 @1.0: `struct{*InverterData; error}` vs named-field channel type.
- C/42 @1.0: newline inside a string literal at line 328.
- B/123 @0.6: `withTimeout` generic inference, `func() error` vs `func() (T, error)`.
- A/123 @0.6: compiled, but `DATA RACE` on `closechan` in `Run()` — scored 0
  because the race aborts the run after 2 leaf tests.

Hosted compiled 4/4; local compiled 9/12.

## The exam has a hidden requirement — the real ceiling is 12/13

`TestNewScraperValidation` failed in **16/16 attempts today**, including all four
Haiku runs, and in April's best 11/13 run. It has never been passed by any model
in this lab's recorded history.

The test requires `NewScraper` to return an error for nil source, nil sink, zero
interval, or negative `maxBufSize`. `prompt.txt`'s five numbered requirements never
ask for argument validation. The only hint is the `(Scraper, error)` return in a
signature the prompt forbids changing, and the supplied original always returns
`nil` error. `make-reference-response.py` scores 13/13 precisely because it adds
five validation branches nobody asked for.

**Every exam_v3 score in this lab carries a systematic −1.** Either state the
requirement in the prompt or drop the test.

Second suspect, observed but not diagnosed: `TestLongOutage/EvictionNotContiguous`
passed 0/12 across all attempts that compiled, including all four Haiku runs and
April's best.

Per-test pass rates across the 16 attempts:

| test | pass / observed |
|---|---:|
| `TestNewScraperValidation` | 0 / 13 |
| `TestLongOutage/EvictionNotContiguous` | 0 / 12 |
| `TestLongOutage/FullBufferFlushed` | 5 / 12 |
| `TestMultipleShortOutagesNoLoss` | 5 / 12 |
| `TestNoLossAcrossTransitions` | 5 / 12 |
| `TestShortOutageNoLoss` | 5 / 12 |
| `TestLongOutage/BoundedBuffer` | 8 / 12 |
| `TestHangBehavior/ReadsProgressDespiteHungWrite` | 11 / 12 |
| `TestSurvivesUnderLoad` | 11 / 12 |
| `TestReadsDuringOutage` | 12 / 13 |
| `TestGracefulCancel` | 12 / 12 |
| `TestHangBehavior/CancelDuringHungRead` | 12 / 12 |
| `TestHangBehavior/CancelDuringHungWrite` | 12 / 12 |

## Harness bug found and fixed mid-run

The first launch (`20260806-160135`) scored cell A seed42 0/13 with an **empty**
`response.txt` despite 8192 tokens generated.

Root cause: `max_tokens 8192` equals `--reasoning-budget 8192`, so thinking
consumed the entire allowance, the whole output landed in `reasoning_content`, and
`content` came back empty. `exam-driver.go` reads only `content`. April's run was
immune because it used `--reasoning off`.

Confirmed by probe: at `max_tokens 16384` the same server returns `finish=stop`
with 3053 chars of reasoning **and** 191 chars of content.

Fix in `scripts/sweep-exam3-rocmfp4.sh`: `MAX_TOKENS` default 8192 → 16384,
`ATTEMPT_TIMEOUT` 15m → 30m (16384 tok at ~13 t/s is ~21 min), sweep budget 4h → 6h.

**Rule for any future reasoning-on run: `max_tokens` must exceed
`--reasoning-budget`, or every score is a structural zero.**

## Harness changes

- `exam-driver.go`: `-api-key` flag (defaults to `$EXAM_API_KEY`), sets
  `Authorization: Bearer …` only when non-empty, so llama-swap cells are
  unaffected. Enables any OpenAI-compatible hosted endpoint.
- `scripts/sweep-exam3-rocmfp4.sh`: token/timeout/budget defaults above, plus
  `--temp 0.6 --top-p 0.95 --top-k 20 --min-p 0.0 --presence-penalty 0.0` on the
  container for sampler parity with llama-swap.
- `scripts/run-exam3-hosted.sh` (new): hosted runner, same 13/13 reference-response
  preflight gate as the local sweep, key never echoed or logged. Verified: no key
  material in any log or artifact.

## What to do next

1. Fix `TestNewScraperValidation` (state the requirement, or drop the test) before
   any further exam_v3 numbers are published.
2. Read `TestLongOutage/EvictionNotContiguous`; 0/12 with a 13/13 reference smells
   like the same class of problem.
3. Re-run Gemma at 0.6 on more seeds — 11/13 is the best local score recorded and
   rests on one attempt.
4. Do not spend more time on ROCmFP4 for this model. The clean tune-vs-quant
   control (`CHADROCK3.6-35B-UNCENSORED-MTP-STRIX-LEAN`) remains undownloaded and
   is now low value.
