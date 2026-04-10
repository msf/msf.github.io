# Local LLM Benchmark Results

*April 2026* — Framework 13, Ryzen AI 370HX, Radeon 890M (16 CU RDNA 3.5), 64 GB DDR5-5600

Runtime: llama.cpp b8708 (Vulkan), llama-swap, `--reasoning off --flash-attn on --cache-type-k q4_0 --cache-type-v q4_0`

## Exam 1 — Three Go Programs (/15)

Write factorial, word-frequency, and file-tree-walker. Scored on compilation, correctness, error handling.

| Rank | Model | Score | Tok/s | Wall | Summary |
|------|-------|-------|-------|------|---------|
| 1 | qwen35-35b-q6k | 14/15 | 22.3 | 0:53 | factorial:4/5 wordfreq:5/5 filetreewalk:5/5 |
| 2 | qwen3-coder-draft | 13/15 | 26.7 | 0:35 | factorial:4/5 wordfreq:4/5 filetreewalk:5/5 |
| 3 | gpt-oss | 13/15 | 26.0 | 0:42 | factorial:4/5 wordfreq:4/5 filetreewalk:5/5 |
| 4 | qwen35-35b-mxfp4 | 13/15 | 22.3 | 0:44 | factorial:4/5 wordfreq:4/5 filetreewalk:5/5 |
| 5 | qwen35-35b | 13/15 | 21.5 | 0:46 | factorial:4/5 wordfreq:4/5 filetreewalk:5/5 |
| 6 | gemma4 | 13/15 | 18.0 | 0:52 | factorial:4/5 wordfreq:4/5 filetreewalk:5/5 |
| 7 | gemma4-mxfp4 | 13/15 | 18.1 | 0:53 | factorial:4/5 wordfreq:4/5 filetreewalk:5/5 |
| 8 | qwen35-9b | 13/15 | 13.0 | 0:53 | factorial:4/5 wordfreq:4/5 filetreewalk:5/5 |
| 9 | gemma4-e4b | 13/15 | 13.3 | 1:11 | factorial:4/5 wordfreq:4/5 filetreewalk:5/5 |
| 10 | glm-flash | 11/15 | 21.1 | 0:48 | factorial:4/5 wordfreq:2/5 filetreewalk:5/5 |
| 11 | glm-flash-reap | 10/15 | 20.0 | 0:38 | factorial:4/5 wordfreq:4/5 filetreewalk:2/5 |
| 12 | deepseek-coder | 10/15 | 27.1 | 0:46 | factorial:4/5 wordfreq:4/5 filetreewalk:2/5 |
| 13 | gemma3n-e4b | 9/15 | 13.4 | 1:00 | factorial:4/5 wordfreq:5/5 filetreewalk:0/5(build-fail) |
| 14 | qwen35-35b-q5km | 8/15 | 21.6 | 0:41 | factorial:4/5 wordfreq:4/5 filetreewalk:0/5(build-fail) |
| 15 | deepseek-r1-14b | 4/15 | 6.6 | 7:09 | factorial:4/5 wordfreq:0/5(build-fail) filetreewalk:0/5(build-fail) |

## Exam 2 — Resilience Test (/20)

Modify a 208-line Go metrics scraper to add buffering, random eviction, and background flush.

| Rank | Model | Score | Tok/s | Wall | A/3 | B/5 | C/7 | D/3 | E/2 | Summary |
|------|-------|-------|-------|------|-----|-----|-----|-----|-----|---------|
| 1 | qwen35-35b-q5km | 18/20 | 15.5 | 2:55 | 3/3 | 5/5 | 7/7 | 2/3 | 1/2 | A:3/3 B:5/5 C:7/7 D:2/3 E:1/2 |
| 2 | gpt-oss | 17/20 | 20.3 | 2:26 | 3/3 | 4/5 | 7/7 | 2/3 | 1/2 | A:3/3 B:4/5 C:7/7 D:2/3 E:1/2 |
| 3 | qwen35-35b | 17/20 | 21.2 | 2:36 | 3/3 | 5/5 | 6/7 | 2/3 | 1/2 | A:3/3 B:5/5 C:6/7 D:2/3 E:1/2 |
| 4 | gemma4 | 17/20 | 14.7 | 3:05 | 3/3 | 4/5 | 7/7 | 2/3 | 1/2 | A:3/3 B:4/5 C:7/7 D:2/3 E:1/2 |
| 5 | qwen3-coder-draft | 16/20 | 24.4 | 1:42 | 3/3 | 5/5 | 6/7 | 2/3 | 0/2 | A:3/3 B:5/5 C:6/7 D:2/3 E:0/2 |
| 6 | deepseek-coder | 16/20 | 18.5 | 2:10 | 3/3 | 4/5 | 7/7 | 2/3 | 0/2 | A:3/3 B:4/5 C:7/7 D:2/3 E:0/2 |
| 7 | gemma4-mxfp4 | 15/20 | 14.4 | 3:10 | 3/3 | 3/5 | 7/7 | 2/3 | 0/2 | A:3/3 B:3/5 C:7/7 D:2/3 E:0/2 |
| 8 | glm-flash | 0/20 | 12.0 | 3:01 | 0/3 | - | - | - | - | A:0/3 compile:NO |
| 9 | gemma3n-e4b | 0/20 | 10.0 | 3:27 | 0/3 | - | - | - | - | A:0/3 compile:NO |
| 10 | glm-flash-reap | 0/20 | 11.9 | 3:28 | 0/3 | - | - | - | - | A:0/3 compile:NO |
| 11 | qwen35-35b-mxfp4 | 0/20 | 15.1 | 3:33 | 0/3 | - | - | - | - | A:0/3 compile:NO |
| 12 | qwen35-35b-q6k | 0/20 | 17.9 | 4:21 | 0/3 | - | - | - | - | A:0/3 compile:NO |
| 13 | gemma4-e4b | 0/20 | 11.3 | 4:41 | 0/3 | - | - | - | - | A:0/3 compile:NO |

