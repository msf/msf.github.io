---
title: "Dense LLMs on a Radeon R9700, and the end of exam_v3"
date: 2026-08-09T18:00:00+01:00
_build:
  list: never      # published, but not linked from the home page or RSS
  render: always
---

*August 2026*[^1]

*Part 5/6 — [Part 6](https://blog.mfilipe.eu/post/local-llm-terminal-bench/) ← **Part 5** ← [Part 4](https://blog.mfilipe.eu/post/benchmarking_llms-v3-rebuild/) ← [Part 3](https://blog.mfilipe.eu/post/local-llm-coding-harder-test/) ← [Part 2](https://blog.mfilipe.eu/post/local-llm-performance-framework13/) ← [Part 1](https://blog.mfilipe.eu/post/benchmarking-local-llms-go-coding/)*

[^1]: Co-authored with Claude Opus 5.

## Re-running exam_v3 on the R9700

I got a Radeon R9700 for my server/desktop machine. What matters here isn't the
raw throughput, it's that dense models in the 20-40B range were completely
unviable on the laptop and on this card they aren't. So I re-ran exam_v3 (what it
does is covered in the prior posts linked above) on the models that did best on
the Framework 13, plus the dense models I couldn't run before.

First, the throughput:

![Decode throughput: Framework 13 at 11.6 t/s versus R9700 at 126.7 t/s on the identical Qwen 35B-A3B MoE build, plus dense Gemma at 24.5 t/s rising to 56.3 t/s with a speculative-decoding drafter](/images/exam-v4/throughput-fw13-vs-r9700.svg)

On the R9700 the dense models are fast enough to be genuine candidates. The MoEs
are very fast, worth reaching for when I want something 2-3x quicker than a dense
model gives me. Public testing and general guidance both say dense wins at this
size (20-40B params). MoE is where everything above 120B is going, but when you're
this compute and RAM restricted, a MoE with 3B or 5B active params simply degrades
more than a 27B dense model does.

These runs also test each model with and without multi-token prediction (MTP),
both to improve throughput and to confirm the drafter was indeed faster.

The models (Q4 quantizations):

- Qwen 3.6 27B, dense
- Qwen 3.6 35B-A3B, MoE
- Qwen 3.8 27B, dense (added later, after its August release)
- Gemma 4 31B, dense, post-training quantization (PTQ) by Unsloth
- Gemma 4 31B QAT, dense, quantization-aware trained by Google
- Gemma 4 26B-A4B, MoE
- Gemma 4 E4B, dense, the smallest model here by some distance
- Muse Glimmer 30B, dense (Meta)

Two builds of Gemma 4 31B appear there. PTQ is Unsloth's quantization applied to
the finished Gemma 4 weights. QAT is Google's later re-release, where quantization
is part of training rather than something done to the model afterwards, so it
should perform better.

So how do they score?

![exam_v3 scores on the R9700: five seeds per model across eight models, showing that the spread within a model is as wide as the gaps between models, with Muse Glimmer spanning the full range from 0 to 12](/images/exam-v4/exam-v3-seed-variance.svg)

The results are all over the place. The one thing that lines up with outside
reality and theory is that the Gemma 4 QAT build does perform measurably better
than the PTQ one. A median of 12 against 5, and at or above 11 in four attempts out
of five against two out of five. Same architecture, same quant format. Both compiled
in all five attempts, so the gap is in the answers, not in
whether the code built.

The rest of the chart is harder to reconcile with what is known about these
models:

- Public evaluations, and the private results others report, place Qwen3.6 27B
  dense ahead of the 35B-A3B MoE on the large majority of tests. This exam
  reverses that: the MoE has a median of 7, the dense 27B a median of 0. Three
  of the 27B's five attempts failed to compile at all.
- Gemma 4 E4B, by some distance the smallest model here, recorded a top score of
  9. The 27B dense, the strongest of this group by public results, topped out at
  6.
- Gemma 4 beat Qwen3.6 throughout. That part may well be real (the exam is Go,
  and Google may simply have the better Go training data) but a chart that also
  produces the first two results is not in a position to establish it.
- Qwen3.8 27B, added when it was released in August, scores worse than a new
  generation ought to: a median of 6 on the same quant and samplers, which is
  mid-table, behind every Gemma 4 dense build and behind its own predecessor's
  MoE sibling. It is a clear improvement on the Qwen3.6 27B it replaces (median 6
  against 0, and it compiled in four attempts out of five against two). But "beats
  the model that could not compile" is a low bar, and on a 13-point exam with this
  much seed spread it is not a result worth leaning on.

The variance is the underlying problem. Qwen3.6 35B-A3B scored 6, 6, 7, 10, 11
across five seeds, with nothing varying but the seed; Gemma 4 31B PTQ drew 5, 5,
5, 11, 12. Muse Glimmer is the worst case: **0, 5, 8, 11, 12**, the entire
usable range of the exam in five draws, and its zero is a single unused import:
364 lines of structurally sound Go with a complete `main()`, with `"sync"` left
in the import block unused, which Go's compiler treats as fatal. One dead line
is the difference between 0/13 and a near-ceiling 12/13.

So the spread within a model is as wide as the differences between models. A
score that moves five points on the seed cannot separate models that sit a point
or two apart, and no number published from this exam had ever carried that
uncertainty, because it had never been measured. Which means the three-seed
rankings published in earlier parts of this series were largely noise.

## exam_v3 is not a good measuring stick

At this point the conclusion is unavoidable: exam_v3 is unreliable and flawed by
design, and it is not a good measuring stick for testing these models.

Three specific reasons, beyond the variance:

1. **One-shot generation is no longer the interesting question.** Testing an
   agentic flow with reasoning enabled is closer to how these models are
   actually used. On the Framework 13, reasoning had to be off for runs to
   finish in reasonable time; on the R9700 it does not.
2. **The harness is a maintenance burden.** exam_v3's driver and grader are
   fragile, and there is nothing to be gained from re-inventing them. The exam
   carried a systematic −1 for months: one test required argument validation the
   prompt never asked for, and it failed 38 consecutive times across every model
   run here up to August 2026.[^2] A maintained third-party harness removes that
   work.
3. **The scoring gate is brutal in a way that has nothing to do with quality.**
   A single unused import is a zero, whatever the other 364 lines look like.
   That is a property of `go build`, not of the model.

So the sensible move is to stop maintaining my own exam and adopt one that other
people write, review and use. That is [Part 6](https://blog.mfilipe.eu/post/local-llm-terminal-bench/).

[^2]: The streak has since been broken: Qwen3.8-27B passed that test on three of
    five seeds on 2026-08-15, the first model here to do so. It does not change
    the conclusion: a test that goes unpassed 38 times before anyone clears it
    was measuring an omission in the prompt, not the models.
