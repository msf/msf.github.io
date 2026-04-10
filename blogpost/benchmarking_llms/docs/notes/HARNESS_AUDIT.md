# Exam v2 harness audit

Reviewer: pi. Date: 2026-04-17. Reviewer had no prior context on this harness.
User warned it hadn't been code-reviewed.

Triggered by: seeing 3 tests (BufferBounded, BufferSizeZero, RaceDetector) fail
on every model and every seed. Systematic failures across independent models
are evidence of harness issues, not model issues.

## Bugs found (ordered by impact)

### BUG 1 (HIGH) — `bufSize` is a package-level global set by `buildArgs`, races

```go
var (
    bufSize    = 10
    offlineSec = 15
    fastMode   = false
)

func buildArgs(port string, buf int) []string {
    // mutates package globals based on the scraper's -h output
    bufSize = 50      // or 10
    offlineSec = 5    // or 15
}
```

`buildArgs` is called by every test that calls `startHarness`. Tests read
`bufSize` and `offlineSec` as package globals. With `go test` running tests
sequentially within a package this is *currently* safe, but:

- **TestScenario hardcodes `bs := bufSize` then compares `flushed >= bs`** for
  FlushOnReconnect AND `flushed > bufSize+10` for BufferBounded. But the test
  sends `buf=bufSize` to `startHarness` — which re-runs buildArgs and can
  overwrite `bufSize`. On first call the global is 10 (initial). On second
  call, if the binary has a "duration" interval flag, bufSize becomes 50.
  Between `bs := bufSize` (10) and the later comparison `bufSize+10` (60),
  the value changes.

  Actually re-reading: `bs` captures once at the top. `bufSize+10` references
  the global. If buildArgs has already mutated bufSize to 50 by the time the
  subtest runs, `bufSize+10 = 60`, not `bs+10 = 20`. So BufferBounded reads
  the post-mutation value. **That's fine here because buildArgs runs during
  startHarness before the subtests.**

  BUT the same `bufSize` is shared across `TestScenario`, `TestMultipleOutageCycles`,
  `TestBufferSizeZero`, `TestBufferSizeOne` because they all call `startHarness`
  which calls `buildArgs`. All use the same `bufSize`. That's intentional.

  **Real problem:** the logic to set bufSize=50 vs 10 uses `strings.Contains(low, "duration")`.
  Many models write `-interval` as `time.Duration` (so their `-h` line literally
  contains "duration") — bufSize=50. Others write it as int seconds — bufSize=10.
  Scoring therefore **depends on the model's flag type**, with tolerances that
  aren't symmetric.

  Verdict: not a bug per se, but a methodological smell. Flag in writeup.

### BUG 2 (HIGH) — BufferBounded reads `flushed` after a 1s sleep that may include more scrape+flush rounds

```go
if !poll(15*time.Second, func() bool { return h.count() >= bs }) { ... }
time.Sleep(1 * time.Second)
flushed := h.count()
...
t.Run("BufferBounded", func(t *testing.T) {
    max := bufSize + 10
    if flushed > max {
        t.Fatalf("not bounded: got %d, expected <= %d", flushed, max)
    }
})
```

At 10ms scrape interval (fastMode=true), during a 5s offline + 1s grace period
after reconnect, the scraper can:
1. Fill the buffer to its cap (bufSize=50) during offline
2. On reconnect, flush 50 metrics AND continue scraping online
3. During the 1s post-poll sleep, another ~100 metrics come in ONLINE and are sent directly

So `flushed` at the end counts buffered-then-flushed + new-online metrics.
Test then checks `flushed > bufSize+10`. In seed123 log earlier:
`phase3: 114 metrics flushed, 114 in dump` — 114 >> 60. The test fails, but
not because buffer isn't bounded. Test fails because it conflates "flushed
buffer contents" with "total metrics received during phase 3."

**This is a real bug.** The mock server's /control/count should be reset or
the test should compute `delta from phase2 boundary`. Instead it compares
against `bufSize+10` which assumes only buffer contents arrive after reconnect.

This bug **penalizes fast-scraping implementations** — models that use tight
intervals to buffer more aggressively get punished more. Models that don't
start scraping immediately post-reconnect would pass. That's backward.

### BUG 3 (HIGH) — TestBufferSizeZero semantics are ambiguous

```go
func TestBufferSizeZero(t *testing.T) {
    h := startHarness(t, 0)
    h.post("/control/online")
    time.Sleep(500 * time.Millisecond)
    h.post("/control/offline")
    time.Sleep(3 * time.Second)
    if !h.alive() {
        t.Fatal("crashed with buffer-size=0")
    }
}
```

What's the correct behavior with `-buffer-size 0`?

Reasonable interpretations:
- (A) No buffering — metrics drop immediately when sink unreachable, scraper keeps running
- (B) Invalid input — exit with error 2
- (C) Unlimited buffer (0 as "no limit")

The test only checks `h.alive()`. That means (A) passes, (C) passes, (B) fails.
But the model doesn't know which interpretation is expected. The problem
statement says "buffer must have a fixed maximum capacity ... Add a -buffer-size
flag (default 1000)." A buffer of size 0 is a degenerate case not covered.

Many models crash with `rand.Intn(0)` — classic panic. The test catches this.
But models that validate input and exit cleanly with error also fail.

**Verdict: test is reasonable for catching panics but punishes defensive coding.**
Should be softened to: accept either `alive == true` OR `exit code == 2`.

### BUG 4 (MEDIUM) — TestRaceDetector picks up `binDir` path weirdly

```go
binDir := *scraperBin
for !strings.HasSuffix(binDir, "/build") && binDir != "/" {
    binDir = binDir[:strings.LastIndex(binDir, "/")]
}
srcFile := binDir + "/scraper.go"
```

This walks up from the binary path looking for a `/build` suffixed directory.
Assumes eval.sh layout where scraper is at `<workdir>/build/scraper` and
source is at `<workdir>/build/scraper.go`. Brittle but OK for current harness.

If scraperBin doesn't contain `/build` anywhere, loop runs until `binDir == "/"`
and srcFile becomes `/scraper.go`. Then `os.Stat(srcFile)` fails, `t.Skipf(...)`.
That means RaceDetector silently skips, and the overall test result is PASS
(skipped counts as not-failed in Go test output, but our eval.sh parsing:

```bash
case "$line" in
    *"--- PASS:"*) pass=$((pass + 1)) ;;
    *"--- FAIL:"*) fail=$((fail + 1)) ;;
    *"--- SKIP:"*) skip=$((skip + 1)) ;;
esac
```

Skip counts in the denominator but not numerator — **model gets penalized for a
harness config issue**.

### BUG 5 (MEDIUM) — TestRaceDetector false negatives via -race build crashes

```go
if out, err := buildCmd.CombinedOutput(); err != nil {
    t.Skipf("race build failed: %v\n%s", out)
}
```

If the -race build fails (e.g. link error), test Skips. Per bug 4, Skip goes
in max but not score. Some real-world Go code compiles without -race but
fails with -race due to atomic alignment issues on arm/amd — but on amd64 this
shouldn't bite. Still: silent Skip = score denominator inflation.

### BUG 6 (LOW) — eval.sh extraction strategy

In eval.sh:
```
# Strategy 1: last ```go block
content=$(awk '/^```go/ { capture=1; block=""; next } /^```/ && capture { ...')
```

But Qwen3.6 seed42's response wraps its code with **`#START scraper.go#` markers
INSIDE the ```go block**. The awk extracts the ```go contents including
the #START/#END lines. Then:

```
cp "$gofile" "$build_dir/scraper.go"
```

The scraper.go now contains lines like `#START scraper.go#` at the top.
Those are invalid Go syntax. Compile would fail. But seed42 compiled...
Let me check. Actually seed42 result shows "compile:FAIL" in the sweep.
That's this bug. The model followed exam_v1 format conventions (
`#START filename.go#` markers) and got compile-failed because eval.sh
doesn't strip them when inside a ```go block.

### BUG 7 (LOW) — Silent eval harness failures produce score=0, max=0

The observed "0/0" results: if the harness's `go test` invocation completes
with no PASS/FAIL lines parseable (e.g. immediate panic, wrong working directory),
score and max are both 0. Score/max=0 is indistinguishable from "harness didn't run"
vs "there are no tests" vs "everything skipped."

Fix: eval.sh should require max >= 1, else emit a distinct "eval:FAIL" status.

## Summary

| Bug | Severity | Who it penalizes | Fix effort |
|-----|----------|------------------|-----------:|
| 1   | medium   | models using int-second intervals get stricter bufSize | low |
| 2   | **high** | models that buffer+flush fast | medium |
| 3   | medium   | models that validate input defensively | low |
| 4   | medium   | any model, silently | low |
| 5   | low      | any model, silently | low |
| 6   | **high** | models that use exam_v1 markers in v2 | low (eval.sh patch) |
| 7   | low      | sweep reproducibility | low |

**The killer is BUG 2.** It likely explains why EVERY model failed BufferBounded.
It's not a model quality signal, it's a test definition bug.

## Recommendation

**Do not re-run the other models yet.** Fix the bugs, re-run only on saved
response.txt files (cheap), then decide if any model's ranking flipped
materially. If bugs 2+6 were inflating failure rates evenly across all models,
rankings might not change. If they penalize some architectures more
(e.g. thinking models buffer more aggressively), they could flip the
Qwen3.6 vs Gemma4 story.

**What I can do without approval:** re-score existing response.txt files
after harness fixes.

**What needs approval:** patching harness_test.go and eval.sh in a way
that invalidates blog results.
