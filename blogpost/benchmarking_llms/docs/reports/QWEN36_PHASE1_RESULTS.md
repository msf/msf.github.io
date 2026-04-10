# Qwen3.6-35B-A3B Phase 1 results — 2026-04-17

Run setup:
- Framework 13, Ryzen AI 370HX, Radeon 890M iGPU, 64GB, Vulkan
- llama.cpp b8708, llama-swap
- 3 seeds (42, 123, 456), temp per Qwen3.6-docs recommendation, 32k ctx
- Qwen3.6-35B-A3B-UD-Q5_K_M.gguf (26.5GB), from unsloth

## Result table

| Model                       | v1 mean | v1 range | v2 mean | v2 compile | tps  | v2 tok | v2 wall |
|-----------------------------|--------:|----------|--------:|-----------:|-----:|-------:|--------:|
| qwen36-35b-q5km thinkoff    | 14.0/15 | 14-14    | 4.0/10  | 2/3*       | 19.6 |  2315  |   128s  |
| gemma4-26b-mxfp4-32k        | 14.0/15 | 14-14    | 6.0/10  | 3/3        | 17.1 |  2442  |   150s  |
| qwen36-35b-q5km **thinkon** | 14.3/15 | 14-15    | **6.7/10** | **3/3**  | 19.3 | 10150  |   528s  |

*thinkoff seed 42 compile failed due to HARNESS BUG (model emitted `#START scraper.go#`
marker inside ```go fence; eval.sh didn't strip it). Model's generated code
was structurally correct — see harness audit.

## Headline findings

**1. Qwen3.6 thinking-on is the new best on this hardware.** 6.7/10 on exam v2,
3/3 compile rate, one perfect 15/15 on exam v1 seed 42 (no prior model reached
15). Cost: 4x generation time, ~4x tokens used. If you can tolerate 8-9 min
per response, it's the strongest local coding model we have.

**2. Qwen3.6 thinking-off matches prior Qwen3.5 family behavior under this
harness.** 4.0/10 — same as Gemma4 at 16k, but not better. The upside of the
Qwen3.6 release is almost entirely in thinking mode, which aligns with the
model's positioning ("agentic coding", preserve_thinking).

**3. Gemma4-26B-MXFP4 at 32k scored 6.0/10 where the blog post had it at 4.0/10.**
This is a **material delta** and I believe it's harness-related, not a context
upgrade effect. The blog used 16k ctx on this model; now I ran it at 32k. But
the blog's 4.0/10 was: mean across 3 seeds where 1 seed failed. This sweep had
no failed compiles for Gemma4. Two possibilities:
  - 32k ctx was the difference (unlikely — the response fits in 8k)
  - The sweep hit different variance on 3 new seeds and the blog number was
    partly noise.
  - A harness difference between then and now (see audit)

I'd want a rerun at 16k ctx before believing "32k made Gemma4 better." That's
cheap, ~6 min.

**4. Thinking mode is insanely expensive.** Qwen3.6 thinkon used ~10k tokens
on avg for exam_v2 (vs ~2.3k thinkoff). At ~20 tok/s that's 8 min vs 2 min.
For batch code gen that's pure loss. For agentic use (where a 5-min model
response is fine, but wrong-first-time costs a tool-call round trip = more
minutes) this may be a win.

## Harness issues discovered (see ../notes/HARNESS_AUDIT.md)

During this sweep, 3 of 18 cells returned "score=0/max=0" — a silent harness
failure (go test returned no parseable PASS/FAIL). All three were re-scored
from saved response.txt files and came out 6/10 or 7/10. No legitimate zeros.

Additional issues found reviewing harness_test.go:
- BUG 2 (high): TestScenario/BufferBounded miscounts — conflates buffered
  flush with new-online metrics. Penalizes all models uniformly. Explains why
  no model passes it under default 10ms scrape interval.
- BUG 6 (high): eval.sh doesn't strip exam_v1-style `#START filename#` markers
  when model embeds them inside a ```go fence. Cost Qwen3.6 thinkoff 1 of 3
  compiles.
- BUG 3/7 (medium): TestBufferSizeZero and "0/0" silent failures.

Fixes are NOT applied in this session. User explicitly warned the harness
hadn't been reviewed and fixing silently would invalidate blog results.

## Recommendation

**Keep Qwen3.6 thinking-on as the primary local coding model going forward.**
It is the best model on this hardware for quality, and its slowness is the
correct tradeoff if you're doing anything non-trivial (where rerunning a wrong
first attempt is worse than waiting 8 min for a right first attempt).

**Deprioritize Qwen3.6 thinking-off**. It's not worse than Qwen3.5 or Gemma4
but gains nothing to justify its 26.5GB footprint over Gemma4-26B-MXFP4 (15GB).

**Don't believe the 6.0/10 for Gemma4 yet** — need to rerun at 16k ctx to
separate "32k helps" from "noise / harness diff." If 16k gives the same
6.0/10, the blog post needs an update. If 16k gives 4.0/10, then 32k is a
real win for that model too.

**Fix harness bugs 2 + 6 + 7** before running any more comparisons. Then
re-score every existing response.txt (cheap). That re-scoring may well shift
the published blog numbers.

## What's in this session

- `/home/miguel/play/llama/config.yaml` — 3 new entries added
  (qwen36-35b-q5km-thinkoff, qwen36-35b-q5km-thinkon, gemma4-26b-mxfp4-32k)
- `/home/miguel/play/msf.github.io/blogpost/benchmarking_llms/scripts/sweep-qwen36.sh` — sweep wrapper
- `/home/miguel/play/msf.github.io/blogpost/benchmarking_llms/artifacts/results/exam_v{1,2}/qwen36-*/` and
  `/gemma4-26b-mxfp4-32k/` — 18 result cells (3 re-scored from flakes)
- `/home/miguel/play/msf.github.io/blogpost/benchmarking_llms/docs/notes/HARNESS_AUDIT.md` — detailed bug list
- `/home/miguel/play/msf.github.io/blogpost/benchmarking_llms/docs/notes/PHASE2_AGENTIC_HARNESS_DESIGN.md` — design for the
  real thing (agentic/tool-calling harness), NOT built
- `/home/miguel/play/msf.github.io/blogpost/benchmarking_llms/docs/notes/PARKING_LOT.md` — deferred items
- `/home/miguel/play/msf.github.io/blogpost/benchmarking_llms/docs/plans/unattended-qwen36-plan.md` — what was planned
- `/home/miguel/play/msf.github.io/blogpost/benchmarking_llms/artifacts/history/runtime-framework13/unattended-qwen36-sweep.log` — full sweep stdout
- `/home/miguel/play/msf.github.io/blogpost/benchmarking_llms/artifacts/history/runtime-framework13/unattended-qwen36-download.log` — download log
