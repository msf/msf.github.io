# exam_v2 — deprecated

This version is frozen. Superseded by `../exam_v3/`.

## Why deprecated

The harness had multiple design flaws that made scoring unreliable:

- Subprocess-per-test model with HTTP mock, real ports, `time.Sleep`-based synchronization, and `-h` output parsing. 30–90s per submission.
- Tests coupled to submissions' flag syntax (`-interval` as `time.Duration` vs `int`), not to behavior. Scoring silently depended on the model's flag choices.
- `TestBufferBounded` conflated post-reconnect live scrapes with buffered flush contents, penalizing fast-scraping implementations.
- `TestBufferSizeZero` semantics ambiguous; accepted any of three contradictory interpretations.
- `TestRaceDetector` path-brittle; silently skipped on path mismatch and still counted in the denominator.
- Eval harness could emit `score=0, max=0` on certain failures, indistinguishable from "no tests ran."

See `../../docs/notes/HARNESS_AUDIT.md` for the full audit.

## What changed in exam_v3

- New factory signature `NewScraper(maxBufSize int, source MetricSource, sink MetricSink, interval time.Duration, hangTimeout time.Duration) (Scraper, error)` — resilience logic is implemented inside a returned `Scraper`, not a decorated `MetricSink`.
- New interface `MetricSource` (replaces `MetricScraper`) with `Read()` instead of `Scrape()`.
- `Scraper.Run(ctx)` with context-based cancellation.
- Tests run in-process via `go test -race -json`. No subprocess, no HTTP mock, no flag parsing. ~2–6s per submission.
- 13 scored leaf tests probing: input validation, reads-during-outage, graceful cancel, no-loss across transitions, short/long outage recovery, random eviction, survival under flaky sources, hang behavior.

## Results under exam_v2

Blog-sweep model responses remain at `../../artifacts/results/exam_v2/` for historical comparison. They are not comparable to exam_v3 scores: different prompt, different interfaces, different tests.
