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

// Tuned per detected scrape interval. Set in init-like fashion by buildArgs.
var (
	bufSize    = 10 // adjusted based on interval
	offlineSec = 15 // seconds offline to overflow buffer
	fastMode   = false
)

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

	// Phase 3: reconnect — verify flush
	h.post("/control/online")

	if !poll(15*time.Second, func() bool { return h.count() >= bs }) {
		t.Logf("phase3: only %d metrics flushed (wanted >= %d)", h.count(), bs)
	}
	time.Sleep(1 * time.Second)

	flushed := h.count()
	flushedMetrics := h.metrics()
	t.Logf("phase3: %d metrics flushed, %d in dump", flushed, len(flushedMetrics))

	t.Run("FlushOnReconnect", func(t *testing.T) {
		if flushed < bufSize {
			t.Fatalf("expected >= %d, got %d", bufSize, flushed)
		}
	})

	t.Run("BufferBounded", func(t *testing.T) {
		max := bufSize + 10
		if flushed > max {
			t.Fatalf("not bounded: got %d, expected <= %d", flushed, max)
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
	h := startHarness(t, 0)
	h.post("/control/online")
	time.Sleep(500 * time.Millisecond)
	h.post("/control/offline")
	time.Sleep(3 * time.Second)
	if !h.alive() {
		t.Fatal("crashed with buffer-size=0")
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
	// Find the source file next to the binary
	binDir := *scraperBin
	for !strings.HasSuffix(binDir, "/build") && binDir != "/" {
		binDir = binDir[:strings.LastIndex(binDir, "/")]
	}
	srcFile := binDir + "/scraper.go"
	if _, err := os.Stat(srcFile); err != nil {
		t.Skipf("can't find source at %s: %v", srcFile, err)
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
		t.Skipf("race build failed: %v\n%s", err, out)
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
		if scraper.Process != nil {
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
