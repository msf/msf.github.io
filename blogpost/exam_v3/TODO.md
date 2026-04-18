# exam_v3 — TODO

## Re-score the sweep

exam_v3 is a significant rework of exam_v2 (new interfaces, new prompt, new harness). Old blog-sweep results under `../results/exam_v2/` do NOT transfer. Every model that mattered needs a fresh run.

### Models to re-run

Local (via llama-swap at `localhost:8080`):
- [ ] gemma4-26b (MXFP4)
- [ ] gemma4-31b (Q4_K_M)
- [ ] gemma4-e4b (Q8_0)
- [ ] qwen35-9b (Q4_K_M)
- [ ] qwen35-35b (Q4_K_M)
- [ ] qwen35-35b-mxfp4
- [ ] qwen35-35b-q5km
- [ ] qwen35-35b-q6k
- [ ] qwen35-35b-think (reasoning mode)
- [ ] qwen3-coder
- [ ] qwen3-coder-draft
- [ ] gpt-oss

Hosted (API):
- [ ] Claude Opus 4.7
- [x] Claude Sonnet 4.5 — scored 11/13 (seed1)
- [x] GPT-5.4 — scored 9/13 (seed1)

### Per-model protocol

1. Run at `seed1` first (cheap sanity check).
2. If score is unstable at `seed1`, add `seed42` and `seed123` to estimate variance.
3. Save responses to `../results/exam_v3/<model-id>/seed<N>/response.txt`.
4. Save scores to `../results/exam_v3/<model-id>/seed<N>/score.json`.

### Current results

| Model | seed1 | seed42 | seed123 |
|---|---|---|---|
| gpt-5.4 | 9/13 | — | — |
| sonnet-4.5 | 11/13 | — | — |
| opus-4.7 | — | — | — |
| reference-solution | 13/13 | — | — |
| skeleton (baseline) | 1/13 | — | — |

## Known grader gaps

- `TestEvictionNotContiguous` passes random eviction, fails FIFO/LIFO. Both GPT-5.4 and Sonnet 4.5 failed here — may warrant an explicit prompt hint about eviction strategy if the sweep shows near-universal failure. For now, leave as a real signal axis.
- `TestNewScraperValidation` is a cheap-to-fix gate. Models that miss it are likely just pattern-copying the skeleton's buggy `NewScraper` instead of reading the requirements.

## Other

- The reference `scraper_solution.go` lives next to the exam. Do NOT ship it to models. eval.sh extracts from the response file; grader_test.go doesn't import it.
- `eval.sh` requires `jq`. Fails loudly if missing.
