// Integration test harness for exam_v2.
//
// Runs the model's compiled scraper binary against a mock server.
// Score = subtests passed.
//
// Speed: scraper runs with -interval 10ms. Buffer fills in milliseconds.
// Flush/retry intervals are auto-detected and set to 1s (minimum for int flags).
// Total suite: ~30s.
//
// Usage:
//
//	go test -v -count=1 -timeout 120s . \
//	    -scraper-bin ./build/scraper -mock-bin ./mockserver
package harness_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

var (
	scraperBin = flag.String("scraper-bin", "", "path to compiled scraper binary")
	mockBin    = flag.String("mock-bin", "", "path to mock server binary")
)

// Tuned per detected scrape interval. Set by probeScraper (TestMain).
var (
	bufSize    = 10 // adjusted based on interval
	offlineSec = 15 // seconds offline to overflow buffer
	fastMode   = false
)

// TestMain runs before any test. Probes the scraper's -h output so package-level
// globals (bufSize, offlineSec, fastMode) are correct BEFORE tests read them.
// BUG 1 fix: previously tests did `bs := bufSize` before the first
// startHarness call, capturing the initial default (10) instead of the
// interval-tuned value (50). All subsequent comparisons used the stale value.
func TestMain(m *testing.M) {
	flag.Parse()
	if *scraperBin != "" {
		// Calling buildArgs with dummy port has the side effect of setting globals.
		_ = buildArgs("0", 10)
	}
	os.Exit(m.Run())
}

type harness struct {
	t       *testing.T
	mock    *exec.Cmd
	port    string
	base    string
	scraper *exec.Cmd
	exited  chan error
}

func startHarness(t *testing.T, buf int) *harness {
	t.Helper()
	if *scraperBin == "" || *mockBin == "" {
		t.Fatal("must set -scraper-bin and -mock-bin")
	}

	portFile := t.TempDir() + "/mock.port"
	mock := exec.Command(*mockBin, portFile)
	mock.Stderr = io.Discard
	if err := mock.Start(); err != nil {
		t.Fatalf("start mock: %v", err)
	}

	var port string
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		if data, err := os.ReadFile(portFile); err == nil && len(data) > 0 {
			port = strings.TrimSpace(string(data))
			break
		}
	}
	if port == "" {
		mock.Process.Kill()
		t.Fatal("mock didn't start")
	}

	args := buildArgs(port, buf)
	t.Logf("scraper args: %v", args)

	scraper := exec.Command(*scraperBin, args...)
	scraper.Stderr = io.Discard
	if err := scraper.Start(); err != nil {
		mock.Process.Kill()
		t.Fatalf("start scraper: %v", err)
	}

	h := &harness{
		t:       t,
		mock:    mock,
		port:    port,
		base:    "http://127.0.0.1:" + port,
		scraper: scraper,
		exited:  make(chan error, 1),
	}
	go func() { h.exited <- scraper.Wait() }()

	t.Cleanup(func() {
		if h.scraper != nil && h.scraper.Process != nil {
			h.scraper.Process.Kill()
			h.scraper.Wait()
		}
		mock.Process.Kill()
		mock.Wait()
	})
	return h
}

func buildArgs(port string, buf int) []string {
	help, _ := exec.Command(*scraperBin, "-h").CombinedOutput()
	helpStr := string(help)

	args := []string{
		"-inverter-host", "127.0.0.1:" + port,
		"-sink-url", "http://127.0.0.1:" + port,
	}

	// Detect interval flag type and set fastest possible.
	// Scale buffer/timing accordingly: 10ms → bufSize=50, 1s → bufSize=10.
	intervalSet := false
	for _, line := range strings.Split(helpStr, "\n") {
		low := strings.ToLower(line)
		if (strings.Contains(low, "interval") || strings.Contains(low, "scrape")) &&
			!strings.Contains(low, "flush") && !strings.Contains(low, "retry") {
			if f := firstFlag(line); f != "" {
				if strings.Contains(low, "duration") {
					args = append(args, f, "10ms")
					fastMode = true
					bufSize = 50
					offlineSec = 5
				} else {
					args = append(args, f, "1")
					fastMode = false
					bufSize = 10
					offlineSec = 15
				}
				intervalSet = true
				break
			}
		}
	}
	if !intervalSet {
		args = append(args, "-interval", "1")
		bufSize = 10
		offlineSec = 15
	}

	// Find buffer flag
	bufFlag := "-buffer-size"
	for _, line := range strings.Split(helpStr, "\n") {
		low := strings.ToLower(line)
		if strings.Contains(low, "buf") && !strings.Contains(low, "flush") {
			if f := firstFlag(line); f != "" {
				bufFlag = f
				break
			}
		}
	}
	args = append(args, bufFlag, fmt.Sprintf("%d", buf))

	// Find and minimize any flush/retry interval flags
	for _, line := range strings.Split(helpStr, "\n") {
		low := strings.ToLower(line)
		if strings.Contains(low, "flush") || strings.Contains(low, "retry") {
			if f := firstFlag(line); f != "" {
				if strings.Contains(low, "duration") {
					args = append(args, f, "200ms")
				} else {
					args = append(args, f, "1")
				}
			}
		}
	}
	return args
}

func firstFlag(line string) string {
	for _, word := range strings.Fields(strings.TrimSpace(line)) {
		if strings.HasPrefix(word, "-") {
			return strings.TrimRight(word, "= ")
		}
	}
	return ""
}

func (h *harness) alive() bool {
	select {
	case <-h.exited:
		return false
	default:
		return true
	}
}

func (h *harness) post(path string) {
	resp, err := http.Post(h.base+path, "", nil)
	if err != nil {
		h.t.Helper()
		h.t.Fatalf("POST %s: %v", path, err)
	}
	resp.Body.Close()
}

func (h *harness) count() int {
	resp, err := http.Get(h.base + "/control/count")
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var r struct{ Count int }
	json.NewDecoder(resp.Body).Decode(&r)
	return r.Count
}

type mockMetric struct {
	Fields map[string]float64 `json:"fields"`
}

func (h *harness) metrics() []mockMetric {
	resp, err := http.Get(h.base + "/control/metrics")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var m []mockMetric
	json.Unmarshal(data, &m)
	return m
}

// scrapeN returns how many /measurements.xml requests the mock has served.
// Used to snapshot phase boundaries so the harness can distinguish buffered
// metrics (scrape_n <= boundary_at_reconnect) from post-reconnect live metrics.
func (h *harness) scrapeN() int {
	resp, err := http.Get(h.base + "/control/scrape_n")
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var r struct{ ScrapeN int `json:"scrape_n"` }
	json.NewDecoder(resp.Body).Decode(&r)
	return r.ScrapeN
}

func poll(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// --- Main scenario: online → offline → reconnect ---

func TestScenario(t *testing.T) {
	bs := bufSize
	h := startHarness(t, bs)
	t.Logf("config: bufSize=%d, offlineSec=%d, fastMode=%v", bs, offlineSec, fastMode)

	// Phase 1: online — verify metrics flow
	h.post("/control/online")
	h.post("/control/reset")

	minOnline := 3
	if !poll(15*time.Second, func() bool { return h.count() >= minOnline }) {
		t.Fatalf("phase1: only %d metrics arrived (expected >= %d)", h.count(), minOnline)
	}
	t.Logf("phase1: %d metrics arrived", h.count())

	t.Run("OnlineFlow", func(t *testing.T) {
		if h.count() < minOnline {
			t.Fatalf("expected >= %d, got %d", minOnline, h.count())
		}
	})

	// Phase 2: offline — buffer fills, nothing reaches sink
	h.post("/control/offline")
	h.post("/control/reset")

	// Wait long enough to overflow buffer: offlineSec covers bufSize*2 scrapes + flush attempts
	time.Sleep(time.Duration(offlineSec) * time.Second)

	if !h.alive() {
		t.Fatal("scraper died during offline phase")
	}
	t.Logf("phase2: %d metrics at sink (should be 0), scraper alive", h.count())

	t.Run("BuffersDuringOutage", func(t *testing.T) {
		if c := h.count(); c != 0 {
			t.Fatalf("expected 0, got %d", c)
		}
	})

	// Phase 3: reconnect — verify flush.
	// BUG 2 fix: the test cannot perfectly distinguish "metrics that came out
	// of the buffer" from "metrics scraped live after reconnect" because both
	// arrive at the same sink. The previous version waited 1s of grace, which
	// at 10ms scrape interval adds 100 live scrapes on top of a bufSize=50
	// buffer, blowing the bound unfairly. Fix: reset count at reconnect, wait
	// only a short window (~300ms) for the buffer flush burst to complete,
	// then snapshot. Live scrapes during 300ms add at most ~30 at 10ms interval,
	// well within the bufSize+60 tolerance. At 1s interval (slow mode) only ~0-1
	// live scrapes land in that window.
	h.post("/control/reset")
	boundary := h.scrapeN() // every scrape so far predates reconnect
	h.post("/control/online")

	// Wait up to 5s for buffer to flush. Covers the 1s default flush interval
	// and some headroom for batched flushes. Scrapers with flush interval > 5s
	// will fail this test (intentional: flushes should be prompt).
	time.Sleep(5 * time.Second)
	total := h.count()
	flushedMetrics := h.metrics()

	// BUG 2 fix: the mock tags each scrape with a monotonically increasing
	// OwnConsumedPower field (100 * scrape_n). A metric with field value <=
	// 100*boundary was scraped BEFORE reconnect, so it could only reach the
	// sink via the buffer. Values > 100*boundary are post-reconnect live
	// scrapes. This cleanly separates "buffered" from "online" without
	// timing heuristics.
	boundaryPower := float64(boundary) * 100.0
	bufferedCount := 0
	liveCount := 0
	for _, m := range flushedMetrics {
		if p, ok := m.Fields["OwnConsumedPower_W"]; ok {
			if p <= boundaryPower {
				bufferedCount++
			} else {
				liveCount++
			}
		}
	}
	t.Logf("phase3: total=%d (buffered=%d, live=%d), boundary_scrape_n=%d",
		total, bufferedCount, liveCount, boundary)

	t.Run("FlushOnReconnect", func(t *testing.T) {
		// Fewer than bufSize-2 buffered metrics flushing means the buffer didn't
		// drain. -2 tolerance allows for eviction timing edge cases.
		min := bufSize - 2
		if bufferedCount < min {
			t.Fatalf("expected >= %d buffered flushed, got %d", min, bufferedCount)
		}
	})

	t.Run("BufferBounded", func(t *testing.T) {
		// The buffer must not have exceeded its configured cap. bufSize+5
		// tolerance covers off-by-one and race-window edge cases. An unbounded
		// buffer implementation would flush hundreds of pre-boundary scrapes.
		max := bufSize + 5
		if bufferedCount > max {
			t.Fatalf("not bounded: got %d buffered, expected <= %d", bufferedCount, max)
		}
	})

	t.Run("EvictionRandom", func(t *testing.T) {
		var powers []float64
		for _, m := range flushedMetrics {
			if p, ok := m.Fields["OwnConsumedPower_W"]; ok {
				powers = append(powers, p)
			}
		}
		if len(powers) < 5 {
			t.Fatalf("insufficient data: %d values", len(powers))
		}
		sort.Float64s(powers)
		mid := (powers[0] + powers[len(powers)-1]) / 2
		below, above := 0, 0
		for _, p := range powers {
			if p < mid {
				below++
			} else {
				above++
			}
		}
		minSide := below
		if above < minSide {
			minSide = above
		}
		ratio := float64(minSide) / float64(len(powers))
		if ratio < 0.1 {
			t.Fatalf("not random: %d below / %d above (ratio %.2f)", below, above, ratio)
		}
	})
}

// --- Multiple outage cycles: buffer survives repeated transitions ---

func TestMultipleOutageCycles(t *testing.T) {
	bs := bufSize
	h := startHarness(t, bs)
	h.post("/control/online")
	h.post("/control/reset")

	poll(10*time.Second, func() bool { return h.count() >= 2 })

	for cycle := 0; cycle < 3; cycle++ {
		// Offline phase
		h.post("/control/offline")
		h.post("/control/reset")
		time.Sleep(time.Duration(offlineSec) * time.Second)
		if !h.alive() {
			t.Fatalf("crashed during outage cycle %d", cycle)
		}
		if h.count() != 0 {
			t.Fatalf("cycle %d: metrics leaked during outage (%d)", cycle, h.count())
		}

		// Online phase — verify flush
		h.post("/control/online")
		if !poll(15*time.Second, func() bool { return h.count() >= bs }) {
			// partial flush
		}
		flushed := h.count()
		if flushed < 1 {
			t.Fatalf("cycle %d: no metrics flushed", cycle)
		}
		t.Logf("cycle %d: %d metrics flushed", cycle, flushed)
	}
}

// --- Edge cases ---

func TestBufferSizeZero(t *testing.T) {
	// BUG 3 fix: accept both "buffer-size=0 runs without buffering" AND
	// "buffer-size=0 exits cleanly with a non-zero code within ~1s" as valid.
	// Prior test only checked h.alive() which penalized implementations that
	// defensively validate input and exit rather than risk rand.Intn(0) panics.
	h := startHarness(t, 0)
	h.post("/control/online")
	time.Sleep(500 * time.Millisecond)

	select {
	case err := <-h.exited:
		// Exited within 500ms of starting — treat as valid rejection of bad input.
		// Accept any exit, including panic, because a panic at startup on invalid
		// flag is an arguably-acceptable behavior for a CLI tool.
		// Mark scraper=nil so Cleanup doesn't try to kill it.
		h.scraper = nil
		t.Logf("scraper exited quickly with buffer-size=0 (err=%v) — OK (validated input)", err)
		return
	default:
	}

	// Still running — verify it survives an outage without panicking.
	h.post("/control/offline")
	time.Sleep(3 * time.Second)
	if !h.alive() {
		t.Fatal("crashed mid-run with buffer-size=0 (neither rejected input nor survived)")
	}
}

func TestBufferSizeOne(t *testing.T) {
	h := startHarness(t, 1)
	h.post("/control/online")
	h.post("/control/reset")
	poll(5*time.Second, func() bool { return h.count() >= 1 })

	h.post("/control/offline")
	h.post("/control/reset")
	time.Sleep(3 * time.Second)

	h.post("/control/online")
	if !poll(5*time.Second, func() bool { return h.count() >= 1 }) {
		t.Fatalf("no flush with buffer-size=1, got %d", h.count())
	}
}

// Race detector: recompile with -race and run the core scenario.
// Catches unprotected concurrent access to buffer/sink.
func TestRaceDetector(t *testing.T) {
	if *scraperBin == "" {
		t.Skip("no scraper-bin")
	}
	// Find the source file next to the binary. eval.sh writes it to
	// <workdir>/build/scraper.go alongside the binary.
	binPath := *scraperBin
	binDir := binPath[:strings.LastIndex(binPath, "/")]
	srcFile := binDir + "/scraper.go"
	if _, err := os.Stat(srcFile); err != nil {
		// BUG 4 fix: treat as failure not skip. Skips count in max but not score,
		// silently penalizing models. Surface the path problem explicitly.
		t.Fatalf("race harness can't find source at %s: %v (binDir=%s, scraperBin=%s)", srcFile, err, binDir, *scraperBin)
	}

	// Build with -race
	raceDir := t.TempDir()
	raceBin := raceDir + "/scraper-race"
	// Copy source and create module
	src, _ := os.ReadFile(srcFile)
	os.WriteFile(raceDir+"/scraper.go", src, 0644)
	os.WriteFile(raceDir+"/go.mod", []byte("module exam\ngo 1.23\n"), 0644)

	buildCmd := exec.Command("go", "build", "-race", "-o", raceBin, ".")
	buildCmd.Dir = raceDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		// BUG 5 fix: race build failures are scraper bugs (e.g. concurrent
		// map writes caught by -race linker), not infra issues. Report as
		// FAIL so they count against the score.
		t.Fatalf("race build failed: %v\n%s", err, out)
	}

	// Run quick scenario with race binary
	portFile := t.TempDir() + "/mock.port"
	mock := exec.Command(*mockBin, portFile)
	mock.Stderr = io.Discard
	if err := mock.Start(); err != nil {
		t.Fatalf("start mock: %v", err)
	}
	t.Cleanup(func() { mock.Process.Kill(); mock.Wait() })

	var port string
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		if data, err := os.ReadFile(portFile); err == nil && len(data) > 0 {
			port = strings.TrimSpace(string(data))
			break
		}
	}
	if port == "" {
		t.Fatal("mock didn't start")
	}
	base := "http://127.0.0.1:" + port

	// Build args for race binary
	args := buildArgs(port, 10)
	scraper := exec.Command(raceBin, args...)
	var stderrBuf strings.Builder
	scraper.Stderr = &stderrBuf
	if err := scraper.Start(); err != nil {
		t.Fatalf("start race binary: %v", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- scraper.Wait() }()
	t.Cleanup(func() {
		if scraper != nil && scraper.Process != nil {
			scraper.Process.Kill()
			scraper.Wait()
		}
	})

	// Quick scenario: online, offline, online
	post := func(path string) {
		resp, err := http.Post(base+path, "", nil)
		if err == nil {
			resp.Body.Close()
		}
	}
	post("/control/online")
	time.Sleep(3 * time.Second)
	post("/control/offline")
	time.Sleep(5 * time.Second)
	post("/control/online")
	time.Sleep(3 * time.Second)

	// Check for race
	scraper.Process.Kill()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
	}
	scraper = nil

	stderr := stderrBuf.String()
	if strings.Contains(stderr, "DATA RACE") {
		// Count distinct races
		races := strings.Count(stderr, "DATA RACE")
		t.Fatalf("detected %d data race(s):\n%s", races, stderr)
	}
}

func TestGracefulShutdown(t *testing.T) {
	h := startHarness(t, 10)
	h.post("/control/online")
	poll(3*time.Second, func() bool { return h.count() >= 1 })

	h.scraper.Process.Signal(syscall.SIGINT)
	select {
	case <-h.exited:
		// clean
	case <-time.After(5 * time.Second):
		t.Fatal("didn't exit within 5s of SIGINT")
	}
	h.scraper = nil
}
