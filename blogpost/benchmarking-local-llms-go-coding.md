# I tried running local LLMs on my Framework 13 to see if they can write Go

*February 2025*

I have a Framework 13 with a Ryzen AI 370HX and a bunch of GGUF models accumulating in `~/.cache/llama.cpp/`. I wanted to know if any of them can actually write Go that compiles and runs. Not vibes, not leaderboard numbers -- `go build` says yes or no. Goal was to have some sense of where local models are in terms of practical capability, being limited in size and available ram/compute

So I spent an evening with opencode + Claude Opus 4.6 scripting a bench harness in bash and threw all my local models at it. Claude wrote the harness, I drove the design decisions and kept fixing the dumb mistakes -- there were plenty on both sides.

## Setup

Framework 13, RRyzen AI 370HX (so it has a Radeon 890M sharing VRAM w/ system RAM), 64GB. llama.cpp b7992 with `--temp 1 --seed 42 --jinja --single-turn`. 60 second timeout per task -- can I use a model with a very well scoped prompt to do real, simple Go code without errors?

## Three tasks

1. **Factorial** -- print 10!. The "hello world" of this bench. ~10 lines of Go.
2. **Word frequency** -- read stdin, count words case-insensitively, print top 10. Needs bufio, strings, sort.
3. **File tree walker** -- take a directory, walk it recursively, print files sorted by size descending. Needs os, filepath, real system interaction.

Scoring is automated: compiles (1pt) + runs (1pt) + correct output (2pt) + error handling (1pt) = 5pt max. Correctness checked against `tr|sort|uniq -c` for wordfreq and `find` for filetree. No human judgment (Opus came up with the scoring)

I iterated on the prompts a lot. First version just said "write a Go program that prints factorial of 10" and Qwen3 literally printed `10`. Fair enough -- I wasn't specific. Final prompts explicitly mention `package main` and hint at which stdlib packages to use, otherwise half the models hallucinate APIs that don't exist (`fmt.Stdin`, etc).

## The models (I picked these based on their size and their token generation using the Vulkan backend of llama.cpp, bigger ones on this laptop are too slow)

| Model | Params | Quant | Size |
|-------|--------|-------|------|
| DeepSeek-Coder-V2-Lite-Instruct | 16B MoE | Q8_0 | 16 GB |
| GLM-4.7-Flash-REAP | 23B MoE | Q4_K_M | 13 GB |
| gpt-oss-20b | 20B | mxfp4 | 12 GB |
| DeepSeek-R1-Distill-Qwen-14B | 14B | Q4_K_M | 8.5 GB |
| gemma-3n-E4B-it | ~8B E4B | Q8_0 | 7 GB |
| Qwen3-8B | 8B | Q4_K_M | 4.7 GB |
| Qwen2.5-Coder-3B | 3B | Q8_0 | 3.1 GB |
| gemma-3-4b-it | 4B | Q4_K_M | 2.4 GB |

## Results

```
Model                               Score  Avg Tok/s
Qwen3-8B-Q4_K_M                     13/15     59     factorial:4 wordfreq:4 filetree:5
DeepSeek-Coder-V2-Lite-Instruct     13/15     43     factorial:4 wordfreq:4 filetree:5
gpt-oss-20b-mxfp4                   13/15     46     factorial:4 wordfreq:4 filetree:5
gemma-3n-E4B-it-Q8_0                 9/15     38     factorial:4 wordfreq:0 filetree:5
GLM-4.7-Flash-REAP-23B-A3B           4/15     55     factorial:0 wordfreq:4 filetree:0
gemma-3-4b-it-Q4_K_M                 4/15     64     factorial:4 wordfreq:0 filetree:0
qwen2.5-coder-3b-q8_0                2/15     58     factorial:2 wordfreq:0 filetree:0
DeepSeek-R1-Distill-Qwen-14B         0/15      ?     factorial:0 wordfreq:0 filetree:0
```

### Factorial (trivial -- 6/8 passed)

```
Model                               Tok/s  Wall     Result
gemma-3-4b-it                         14    5.6s    EXACT
Qwen3-8B                              58    9.0s    EXACT
gemma-3n-E4B-it                       96   12.9s    EXACT
DeepSeek-Coder-V2-Lite                80   14.6s    EXACT
gpt-oss-20b                           97   18.3s    EXACT
qwen2.5-coder-3b                      58    6.5s    WRONG (printed "10! 3628800")
GLM-4.7-Flash-REAP                    82   38.0s    FAIL: duplicate main declaration
DeepSeek-R1-Distill-14B                ?   60.0s    FAIL: spent 60s thinking, zero code output
```

### Word Frequency (medium -- 4/8 passed)

```
Model                               Tok/s  Wall     Result
Qwen3-8B                              65   18.7s    EXACT
DeepSeek-Coder-V2-Lite                13   19.9s    EXACT
gpt-oss-20b                           27   34.7s    EXACT
GLM-4.7-Flash-REAP                    28   53.5s    EXACT
gemma-3-4b-it                         94   17.7s    FAIL: used fmt.Stdin (not a thing)
gemma-3n-E4B-it                        8   30.0s    FAIL: forgot to import os
qwen2.5-coder-3b                       ?   60.0s    FAIL: degenerate repetition loop
DeepSeek-R1-Distill-14B                ?   60.0s    FAIL: still thinking...
```

### File Tree (hard -- 4/8 passed)

```
Model                               Tok/s  Wall     Result
DeepSeek-Coder-V2-Lite                35   17.2s    EXACT
Qwen3-8B                              54   21.5s    EXACT (auto-fixed missing })
gemma-3n-E4B-it                       10   35.2s    EXACT
gpt-oss-20b                           15   51.5s    EXACT
gemma-3-4b-it                         85   15.9s    FAIL: wrong WalkDir callback signature
GLM-4.7-Flash-REAP                     ?   60.0s    FAIL: timeout
qwen2.5-coder-3b                       ?   60.0s    FAIL: forgot to import sort
DeepSeek-R1-Distill-14B                ?   60.0s    FAIL: yes, still thinking
```

## Takeaways

**Qwen3-8B at 4.7GB is the one to use.** Ties at 13/15 with models 3x its size, fastest wall time on every task. At Q4_K_M there might even be room to improve with a higher quant. If you're doing local LLM coding on a laptop, this is it.

**Size doesn't predict quality.** The 16GB DeepSeek-Coder-V2 and the 4.7GB Qwen3-8B land on the exact same score. The tiny models (gemma-3-4b at 2.4GB, qwen2.5-coder-3b at 3.1GB) can barely do factorial. There's a threshold somewhere around 8B params where models go from "mostly broken" to "actually useful".

**DeepSeek-R1 reasoning model scored 0/15.** It spent the entire 60 seconds on every task inside `<think>` blocks and never produced a line of code. The `/no_think` prompt hint didn't change anything. For quick interactive coding, reasoning models need either much longer timeouts or `--reasoning-budget 0` which defeats the purpose.

**Models hallucinate Go stdlib APIs with full confidence.** `fmt.Stdin`, wrong `filepath.WalkDir` signatures, importing packages then not using the right symbols. These are the errors that cost you time in practice -- the code looks right at a glance.

**The hardest part was the harness, not the benchmark.** Every model formats output differently: fenced code blocks, raw code, leaked internal tokens (`<|channel|>analysis<|message|>`), duplicate programs, missing closing braces. Getting robust extraction across all these formats took more iterations than the actual benchmark design.

## Run it yourself

The script auto-discovers all `.gguf` models in your cache:

```bash
LLAMA_CLI=/path/to/llama-cli ./bench.sh            # all models
MODEL_FILTER="Qwen3" ./bench.sh                     # one model
TIMEOUT=120 MODEL_DIR=/my/models ./bench.sh         # tweak
```

Raw outputs, Go source, and scoring metadata end up in `bench_results/<model>/<task>.*`.

Script: [github.com/msf/llm-code-bench](https://github.com/msf/llm-code-bench)

## What's next

**Exam mode** -- all three tasks in one prompt, one timeout. Like a real coding test where the model allocates its own time. The verbose models that barely finish one task in 60s would get destroyed.

**Quant comparison** -- same model at Q4 vs Q8. Does quantization affect code quality or just speed?

**More models, same test** -- keep the benchmark fixed, test new models as they drop. That's the point of automating it.
