---
title: "Dense LLMs on a Radeon R9700, and the end of exam_v3"
date: 2026-08-09T18:00:00+01:00
---

*August 2026*[^1]

*Part 5/6 — [Part 6](https://blog.mfilipe.eu/post/local-llm-terminal-bench/) ← **Part 5** ← [Part 4](https://blog.mfilipe.eu/post/benchmarking_llms-v3-rebuild/) ← [Part 3](https://blog.mfilipe.eu/post/local-llm-coding-harder-test/) ← [Part 2](https://blog.mfilipe.eu/post/local-llm-performance-framework13/) ← [Part 1](https://blog.mfilipe.eu/post/benchmarking-local-llms-go-coding/)*

[^1]: Co-authored with Claude Opus 5.

## Re-running exam_v3 on the R9700

I got a Radeon R9700 GPU for my server/desktop machine to play with LLMs and so
wanted to test and compare my prior exams to what I could do on the new GPU.
Both to check the performance but also because this hardware allows running the
dense models at reasonable speeds (which were completely unviable on the laptop)
and see how much better they would be on my little made up exam_v3. What the
exam_v3 does is covered in the prior posts linked above.

So, to summarize, first let's test the best models that I tested on the
Framework 13 again and see throughput differences, and the throughput of dense
models on the GPU:

![Decode throughput: Framework 13 at 11.6 t/s versus R9700 at 126.7 t/s on the identical Qwen 35B-A3B MoE build, plus dense Gemma at 24.5 t/s rising to 56.3 t/s with a speculative-decoding drafter](/images/exam-v4/throughput-fw13-vs-r9700.svg)

In summary, on the R9700, the dense models are fast enough and become genuine
candidates for best models to use. The MoE become very fast models that can be
used when we really want something 2-3x faster than what we get with the dense
models. According to public testing and general knowledge, at these size of
models (20-40B params) dense models are preferred. MoE models are the future and
anything above 120B param is a MoE model, but when we're this compute/ram
restricted, a MoE with 3B or 5B active params simply degrades more than a 27B
dense model.

Another thing these tests cover is I tested with and without MTP on many models
to improve the throughput (and to validate it was indeed faster).

Models we will focus on (Q4 quantizations):

- Qwen 3.6, 27B, dense
- Qwen 3.6, 27B, dense (Q6 quant)
- Qwen 3.6, 35B, A3B MoE model
- Gemma4 31B, dense, Post-Trained Quantization (PTQ) unsloth
- Gemma4 31B-QAT, dense, (this quantized should perform better)
- Gemma4 26B, MoE model
- Meta Muse-Glimmer, 30B, dense

Two builds of Gemma 4 31B appear there. **PTQ** is Unsloth's post-training
quantization of the original Gemma 4 release: quantization applied to finished
weights. **QAT** is Google's later re-release, quantization-aware trained, where
quantization is part of training rather than something done to the model
afterwards.

So, how do the dense models score on the exam_v3?

![exam_v3 scores on the R9700: five seeds per model across seven models, showing that the spread within a model is as wide as the gaps between models, with Muse Glimmer spanning the full range from 0 to 12](/images/exam-v4/exam-v3-seed-variance.svg)

The results are all over the place, the only thing that aligns with outside
reality and theory is: indeed the Gemma4 QAT model performs measurably better
than the PTQ one — a median of 12 against 5, at or above 11 in four attempts out
of five against two out of five, on the same architecture and the same quant
format. Both compiled in all five attempts, so the gap is in the answers, not in
whether the code built.

The rest of that table are harder to reconcile with what is known/found about
these models on the internet:

- Public evaluations, and the private results others report, place Qwen3.6 27B
  dense ahead of the 35B-A3B MoE on the large majority of tests. This exam
  reverses that: the MoE has a median of 7, the dense 27B a median of 0. Three
  of the 27B's five attempts failed to compile at all.
- Gemma 4 E4B, by some distance the smallest model in the table, recorded a top
  score of 9. The 27B dense, the strongest of this group by public results,
  topped out at 6.
- Gemma 4 beat Qwen3.6 throughout. That part may well be real — the exam is Go,
  and Google may simply have the better Go training data — but a table that also
  produces the first two results is not in a position to establish it.

The variance is the underlying problem. Qwen3.6 35B-A3B scored 6, 6, 7, 10, 11
across five seeds, with nothing varying but the seed; Gemma 4 31B PTQ drew 5, 5,
5, 11, 12. Muse Glimmer is the worst case — **0, 5, 8, 11, 12**, the entire
usable range of the exam in five draws, and its zero is a single unused import:
364 lines of structurally sound Go with a complete `main()`, with `"sync"` left
in the import block unused, which Go's compiler treats as fatal. One dead line
is the difference between 0/13 and a near-ceiling 12/13.

So the spread within a model is as wide as the differences between models. A
score that moves five points on the seed cannot separate models that sit a point
or two apart, and no number published from this exam had ever carried that
uncertainty, because it had never been measured — which means the two-seed
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
   ever run here. A maintained third-party harness removes that work.
3. **The scoring gate is brutal in a way that has nothing to do with quality.**
   A single unused import is a zero, whatever the other 364 lines look like.
   That is a property of `go build`, not of the model.

So the sensible move is to stop maintaining my own exam and adopt one that other
people write, review and use. That is [Part 6](https://blog.mfilipe.eu/post/local-llm-terminal-bench/).
