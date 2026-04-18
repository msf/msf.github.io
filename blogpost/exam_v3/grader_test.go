// In-process grader for exam_v2 submissions.
//
// Compiled as part of package main alongside the submission's scraper.go.
// Drives NewScraper directly against stub MetricSource / MetricSink — no
// subprocess, no HTTP, no -h parsing. Whole suite runs with -race in a few
// seconds.
//
// Scored leaf tests (max = 13):
//   1  TestNewScraperValidation
//   2  TestReadsDuringOutage
//   3  TestGracefulCancel
//   4  TestNoLossAcrossTransitions
//   5  TestShortOutageNoLoss
//   6  TestMultipleShortOutagesNoLoss
//   7  TestLongOutage/BoundedBuffer
//   8  TestLongOutage/FullBufferFlushed
//   9  TestLongOutage/EvictionNotContiguous
//  10  TestSurvivesUnderLoad
//  11  TestHangBehavior/CancelDuringHungRead
//  12  TestHangBehavior/CancelDuringHungWrite
//  13  TestHangBehavior/ReadsProgressDespiteHungWrite
//
// Parent aggregate lines (TestLongOutage, TestHangBehavior) are filtered out
// by eval.sh.
package main

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	// Test-time clock. 10× ratio gives room for scheduler jitter under -race
	// while keeping the hang-related tests short.
	scrapeInterval = 1 * time.Millisecond
	hangTimeout    = 10 * time.Millisecond
	// flushGrace: time we give a submission to drain its buffer after reconnect.
	// Invariant: flushGrace > scrapeInterval * maxBuf for every test below
	// (max maxBuf is 50 → 250ms, so 500ms is 2× headroom). Submissions that
	// scale flush cadence to scrape rate pass easily; submissions that
	// hardcode slow timers (1s, 5s) regardless of scrape rate fail — a real
	// design flaw worth penalizing.
	flushGrace  = 500 * time.Millisecond
	cancelGrace = 300 * time.Millisecond
)

var errOffline = errors.New("sink offline")

// --- stubs ---

// stubSource tags each scrape with a monotonically increasing sequence number
// carried in OwnConsumedPower. Sink-side sequence observation distinguishes
// buffered-from-outage metrics from post-reconnect live metrics with no timing
// assumptions.
type stubSource struct {
	n       atomic.Int64
	errEvery atomic.Int64 // if > 0, return error every Nth call (0 = never)
}

func (s *stubSource) seq() int64 { return s.n.Load() }

func (s *stubSource) Read() (*InverterData, error) {
	n := s.n.Add(1)
	if every := s.errEvery.Load(); every > 0 && n%every == 0 {
		return nil, errors.New("flaky source")
	}
	d := &InverterData{}
	d.Device.Name = "stub"
	d.Device.Measurements.Measurement = []struct {
		Value float64 `xml:"Value,attr"`
		Unit  string  `xml:"Unit,attr"`
		Type  string  `xml:"Type,attr"`
	}{
		{Value: float64(n), Unit: "W", Type: "OwnConsumedPower"},
		{Value: 0, Unit: "W", Type: "GridConsumedPower"},
		{Value: float64(n), Unit: "W", Type: "GridInjectedPower"},
	}
	return d, nil
}

type stubSink struct {
	mu       sync.Mutex
	online   bool
	received []Metric
}

func newStubSink(online bool) *stubSink { return &stubSink{online: online} }

func (s *stubSink) setOnline(v bool) { s.mu.Lock(); s.online = v; s.mu.Unlock() }

func (s *stubSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.received)
}

func (s *stubSink) snapshot() []Metric {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Metric, len(s.received))
	copy(out, s.received)
	return out
}

func (s *stubSink) reset() {
	s.mu.Lock()
	s.received = nil
	s.mu.Unlock()
}

func (s *stubSink) Write(m Metric) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.online {
		return errOffline
	}
	s.received = append(s.received, m)
	return nil
}

// --- helpers ---

// seqsOf extracts OwnConsumedPower values (== source seq numbers) from metrics.
func seqsOf(ms []Metric) []int64 {
	out := make([]int64, 0, len(ms))
	for _, m := range ms {
		if v, ok := m.Fields["OwnConsumedPower_W"]; ok {
			out = append(out, int64(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// seqSet converts seqs to a set for O(1) membership.
func seqSet(xs []int64) map[int64]bool {
	s := make(map[int64]bool, len(xs))
	for _, x := range xs {
		s[x] = true
	}
	return s
}

// startScraper constructs and runs the scraper in a goroutine. Cleanup cancels
// ctx best-effort; it does NOT fail the test if Run doesn't return — that's
// TestGracefulCancel's job. A leaked Run goroutine dies when the test binary
// exits.
func startScraper(t *testing.T, maxBuf int, src MetricSource, sink MetricSink) context.CancelFunc {
	t.Helper()
	scraper, err := NewScraper(maxBuf, src, sink, scrapeInterval, hangTimeout)
	if err != nil {
		t.Fatalf("NewScraper: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("scraper.Run panicked: %v", r)
			}
		}()
		_ = scraper.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
	})
	return cancel
}

// waitFor polls cond until true or timeout.
func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func headTail(xs []int64) []int64 {
	if len(xs) <= 10 {
		return xs
	}
	out := append([]int64{}, xs[:5]...)
	out = append(out, -1)
	out = append(out, xs[len(xs)-5:]...)
	return out
}

// --- tests ---

// TestNewScraperValidation: factory must reject obviously invalid inputs.
func TestNewScraperValidation(t *testing.T) {
	src := &stubSource{}
	sink := newStubSink(true)

	cases := []struct {
		name        string
		maxBuf      int
		src         MetricSource
		sink        MetricSink
		interval    time.Duration
		hangTimeout time.Duration
	}{
		{"nil source", 10, nil, sink, scrapeInterval, hangTimeout},
		{"nil sink", 10, src, nil, scrapeInterval, hangTimeout},
		{"zero interval", 10, src, sink, 0, hangTimeout},
		{"negative maxBuf", -1, src, sink, scrapeInterval, hangTimeout},
	}

	for _, c := range cases {
		_, err := NewScraper(c.maxBuf, c.src, c.sink, c.interval, c.hangTimeout)
		if err == nil {
			t.Fatalf("NewScraper accepted invalid input (%s); expected error", c.name)
		}
	}
}

// TestReadsDuringOutage: source keeps being read while sink is offline. Any
// implementation that blocks on Write stalls the whole loop and fails this.
func TestReadsDuringOutage(t *testing.T) {
	src := &stubSource{}
	sink := newStubSink(false)
	startScraper(t, 10, src, sink)

	time.Sleep(20 * scrapeInterval)
	first := src.seq()
	time.Sleep(20 * scrapeInterval)
	second := src.seq()

	if second-first < 10 {
		t.Fatalf("source read stalled during outage: seq went %d -> %d in %v", first, second, 20*scrapeInterval)
	}
	if sink.count() != 0 {
		t.Fatalf("sink received %d metrics during outage (expected 0)", sink.count())
	}
}

// TestGracefulCancel: Run must return promptly after ctx cancellation.
// Fails any implementation that ignores ctx (e.g. time.Sleep-based loops).
func TestGracefulCancel(t *testing.T) {
	src := &stubSource{}
	sink := newStubSink(true)
	scraper, err := NewScraper(10, src, sink, scrapeInterval, hangTimeout)
	if err != nil {
		t.Fatalf("NewScraper: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("scraper.Run panicked: %v", r)
			}
		}()
		_ = scraper.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(cancelGrace):
		t.Fatalf("Run did not return within %v of cancel()", cancelGrace)
	}
}

// TestNoLossAcrossTransitions: run online → offline → online → offline →
// online, with each offline window well under maxBuf. All seqs produced during
// offline windows must eventually arrive at the sink.
func TestNoLossAcrossTransitions(t *testing.T) {
	src := &stubSource{}
	sink := newStubSink(true)
	maxBuf := 50
	startScraper(t, maxBuf, src, sink)

	if !waitFor(500*time.Millisecond, func() bool { return sink.count() >= 3 }) {
		t.Fatal("no online flow established")
	}

	offlineWindows := [][2]int64{}
	for i := 0; i < 2; i++ {
		sink.setOnline(false)
		a := src.seq()
		time.Sleep(15 * scrapeInterval) // ~15 scrapes offline, < maxBuf
		b := src.seq()
		offlineWindows = append(offlineWindows, [2]int64{a, b})

		sink.setOnline(true)
		time.Sleep(50 * time.Millisecond) // drain + live
	}

	// Wait for drain to complete.
	if !waitFor(flushGrace, func() bool {
		got := seqSet(seqsOf(sink.snapshot()))
		for _, w := range offlineWindows {
			for n := w[0] + 1; n <= w[1]; n++ {
				if !got[n] {
					return false
				}
			}
		}
		return true
	}) {
		got := seqsOf(sink.snapshot())
		t.Fatalf("lost metrics across transitions: offline windows %v, received seqs %v",
			offlineWindows, headTail(got))
	}
}

// TestShortOutageNoLoss: single outage well under maxBuf. All offline seqs
// must arrive after reconnect.
func TestShortOutageNoLoss(t *testing.T) {
	src := &stubSource{}
	sink := newStubSink(true)
	maxBuf := 50
	startScraper(t, maxBuf, src, sink)

	if !waitFor(500*time.Millisecond, func() bool { return sink.count() >= 3 }) {
		t.Fatal("no online flow established")
	}

	sink.setOnline(false)
	a := src.seq()
	time.Sleep(15 * scrapeInterval) // ~15 scrapes, < maxBuf=50
	b := src.seq()
	offlineScrapes := b - a

	if offlineScrapes < 5 {
		t.Fatalf("not enough offline scrapes (%d)", offlineScrapes)
	}
	if offlineScrapes >= int64(maxBuf) {
		t.Fatalf("outage too long for this test (%d >= %d)", offlineScrapes, maxBuf)
	}

	sink.reset()
	sink.setOnline(true)

	if !waitFor(flushGrace, func() bool {
		got := seqSet(seqsOf(sink.snapshot()))
		for n := a + 1; n <= b; n++ {
			if !got[n] {
				return false
			}
		}
		return true
	}) {
		got := seqsOf(sink.snapshot())
		t.Fatalf("short outage lost metrics: offline (%d, %d], received seqs %v",
			a, b, headTail(got))
	}
}

// TestMultipleShortOutagesNoLoss: 3 cycles of short outage + recovery. No loss
// in any cycle. Catches implementations that handle the first outage but leak
// state, fail to re-arm, or deadlock across cycles.
func TestMultipleShortOutagesNoLoss(t *testing.T) {
	src := &stubSource{}
	sink := newStubSink(true)
	maxBuf := 50
	startScraper(t, maxBuf, src, sink)

	if !waitFor(500*time.Millisecond, func() bool { return sink.count() >= 3 }) {
		t.Fatal("no online flow established")
	}

	for cycle := 0; cycle < 3; cycle++ {
		sink.setOnline(false)
		a := src.seq()
		time.Sleep(15 * scrapeInterval) // ~15 scrapes, < maxBuf=50
		b := src.seq()

		sink.setOnline(true)

		if !waitFor(flushGrace, func() bool {
			got := seqSet(seqsOf(sink.snapshot()))
			for n := a + 1; n <= b; n++ {
				if !got[n] {
					return false
				}
			}
			return true
		}) {
			got := seqsOf(sink.snapshot())
			t.Fatalf("cycle %d lost metrics: offline (%d, %d], received seqs %v",
				cycle, a, b, headTail(got))
		}
	}
}

// longOutageResult captures the observable state of a long-outage run: seq
// numbers that arrived at the sink which were scraped during the offline window.
type longOutageResult struct {
	maxBuf       int
	offlineStart int64
	offlineEnd   int64
	bufferedSeqs []int64
}

// runLongOutage: offline for ~10 × maxBuf scrapes, then reconnect and wait for
// drain. Returns seq numbers <= offlineEnd that reached the sink.
func runLongOutage(t *testing.T, maxBuf int) longOutageResult {
	t.Helper()
	src := &stubSource{}
	sink := newStubSink(true)
	startScraper(t, maxBuf, src, sink)

	if !waitFor(500*time.Millisecond, func() bool { return sink.count() >= 3 }) {
		t.Fatal("no online flow established")
	}

	sink.setOnline(false)
	offlineStart := src.seq()
	time.Sleep(time.Duration(10*maxBuf) * scrapeInterval) // 10× buffer worth of scrapes

	sink.reset()
	offlineEnd := src.seq()
	sink.setOnline(true)
	time.Sleep(flushGrace)

	all := seqsOf(sink.snapshot())
	var buf []int64
	for _, s := range all {
		if s <= offlineEnd {
			buf = append(buf, s)
		}
	}

	t.Logf("long outage: maxBuf=%d, offline=(%d,%d] (%d scrapes), buffered_count=%d, sample=%v",
		maxBuf, offlineStart, offlineEnd, offlineEnd-offlineStart, len(buf), headTail(buf))

	return longOutageResult{
		maxBuf:       maxBuf,
		offlineStart: offlineStart,
		offlineEnd:   offlineEnd,
		bufferedSeqs: buf,
	}
}

// TestLongOutage bundles three observations from a single long-outage run, as
// subtests. Subtests surface as independent PASS/FAIL lines for scoring, but
// only one long-outage scenario is executed.
func TestLongOutage(t *testing.T) {
	r := runLongOutage(t, 20)
	const tolerance = 3 // race window on the eviction path

	t.Run("BoundedBuffer", func(t *testing.T) {
		if len(r.bufferedSeqs) < 1 {
			t.Fatalf("no metrics buffered during long outage: got %d, expected ~%d", len(r.bufferedSeqs), r.maxBuf)
		}
		if len(r.bufferedSeqs) > r.maxBuf+tolerance {
			t.Fatalf("buffer unbounded: got %d buffered metrics, maxBuf=%d (+%d tolerance)",
				len(r.bufferedSeqs), r.maxBuf, tolerance)
		}
	})

	t.Run("FullBufferFlushed", func(t *testing.T) {
		min := r.maxBuf - tolerance
		if len(r.bufferedSeqs) < min {
			t.Fatalf("kept too few: got %d buffered metrics, expected >= %d (maxBuf=%d). Either nothing was buffered, nothing was flushed, or the buffer was under-utilized during a long outage.",
				len(r.bufferedSeqs), min, r.maxBuf)
		}
	})

	t.Run("EvictionNotContiguous", func(t *testing.T) {
		if len(r.bufferedSeqs) < 5 {
			t.Fatalf("too few buffered metrics (%d) to classify eviction", len(r.bufferedSeqs))
		}
		first := r.bufferedSeqs[0]
		last := r.bufferedSeqs[len(r.bufferedSeqs)-1]
		span := last - first + 1
		count := int64(len(r.bufferedSeqs))
		offlineRange := r.offlineEnd - r.offlineStart

		const contiguousTol = int64(3)
		if span <= count+contiguousTol {
			pattern := "FIFO-or-LIFO"
			if first <= r.offlineStart+contiguousTol {
				pattern = "FIFO (kept oldest)"
			} else if last >= r.offlineEnd-contiguousTol {
				pattern = "LIFO (kept newest)"
			}
			t.Fatalf("eviction appears %s: seqs span [%d, %d] (width %d) over offline range %d with %d kept",
				pattern, first, last, span, offlineRange, count)
		}
	})
}

// TestSurvivesUnderLoad: long run with flaky source + repeated outage cycles.
// Asserts the scraper stays alive, keeps reading, and makes progress. Catches
// implementations that crash on Read errors (NPE on nil data), deadlock after
// many cycles, or leak resources until Run panics.
func TestSurvivesUnderLoad(t *testing.T) {
	src := &stubSource{}
	src.errEvery.Store(5) // ~20% of Read calls fail
	sink := newStubSink(true)
	startScraper(t, 20, src, sink)

	// Drive 5 short outage cycles while the source is flaky.
	for cycle := 0; cycle < 5; cycle++ {
		sink.setOnline(false)
		time.Sleep(15 * scrapeInterval)
		sink.setOnline(true)
		time.Sleep(15 * scrapeInterval)
	}

	// Scraper must still be making progress: source seq advancing and sink
	// receiving new metrics in the final online window.
	sinkBefore := sink.count()
	srcBefore := src.seq()
	time.Sleep(100 * time.Millisecond)
	sinkAfter := sink.count()
	srcAfter := src.seq()

	if srcAfter-srcBefore < 10 {
		t.Fatalf("scraper stopped reading source after load: seq %d -> %d in 100ms", srcBefore, srcAfter)
	}
	if sinkAfter-sinkBefore < 5 {
		t.Fatalf("scraper stopped writing to sink after load: count %d -> %d in 100ms", sinkBefore, sinkAfter)
	}
}

// --- hang-behavior stubs ---

// hangingSource blocks Read on a channel until release is closed. Each Read
// call increments started; the caller can assert that Read was entered.
type hangingSource struct {
	release chan struct{}
	started atomic.Int64
}

func newHangingSource() *hangingSource { return &hangingSource{release: make(chan struct{})} }

func (h *hangingSource) Read() (*InverterData, error) {
	h.started.Add(1)
	<-h.release // block until released
	return nil, errors.New("released without data")
}

// hangingSink blocks Write until release is closed. It also tracks how many
// calls are currently blocked, so tests can observe concurrency.
type hangingSink struct {
	release  chan struct{}
	started  atomic.Int64
	finished atomic.Int64
}

func newHangingSink() *hangingSink { return &hangingSink{release: make(chan struct{})} }

func (h *hangingSink) Write(m Metric) error {
	h.started.Add(1)
	<-h.release
	h.finished.Add(1)
	return nil
}

// TestHangBehavior bundles three tests against sources/sinks that hang. All
// three exercise the same architectural requirement: the scrape loop must not
// block indefinitely on a single source/sink call, and ctx cancellation must
// propagate even through hung I/O.
func TestHangBehavior(t *testing.T) {
	t.Run("CancelDuringHungRead", func(t *testing.T) {
		src := newHangingSource()
		defer close(src.release) // let the leaked goroutine exit at end
		sink := newStubSink(true)

		scraper, err := NewScraper(10, src, sink, scrapeInterval, hangTimeout)
		if err != nil {
			t.Fatalf("NewScraper: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { _ = recover() }()
			_ = scraper.Run(ctx)
		}()

		// Wait until at least one Read has entered the hang.
		if !waitFor(200*time.Millisecond, func() bool { return src.started.Load() >= 1 }) {
			cancel()
			<-done
			t.Fatal("source.Read was never called")
		}

		cancel()
		select {
		case <-done:
		case <-time.After(cancelGrace):
			t.Fatalf("Run did not return within %v of cancel despite hung Read", cancelGrace)
		}
	})

	t.Run("CancelDuringHungWrite", func(t *testing.T) {
		src := &stubSource{}
		sink := newHangingSink()
		defer close(sink.release)

		scraper, err := NewScraper(10, src, sink, scrapeInterval, hangTimeout)
		if err != nil {
			t.Fatalf("NewScraper: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { _ = recover() }()
			_ = scraper.Run(ctx)
		}()

		// Wait until at least one Write has entered the hang.
		if !waitFor(200*time.Millisecond, func() bool { return sink.started.Load() >= 1 }) {
			cancel()
			<-done
			t.Fatal("sink.Write was never called")
		}

		cancel()
		select {
		case <-done:
		case <-time.After(cancelGrace):
			t.Fatalf("Run did not return within %v of cancel despite hung Write", cancelGrace)
		}
	})

	t.Run("ReadsProgressDespiteHungWrite", func(t *testing.T) {
		// Sink hangs on every Write. The scrape loop must continue to call
		// source.Read — i.e. hung writes must not serialize reads.
		src := &stubSource{}
		sink := newHangingSink()
		defer close(sink.release)

		scraper, err := NewScraper(10, src, sink, scrapeInterval, hangTimeout)
		if err != nil {
			t.Fatalf("NewScraper: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { _ = recover() }()
			_ = scraper.Run(ctx)
		}()
		defer func() {
			cancel()
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
			}
		}()

		// Wait for the first Write to start hanging.
		if !waitFor(100*time.Millisecond, func() bool { return sink.started.Load() >= 1 }) {
			t.Fatal("sink.Write was never called")
		}

		// With a 5× hang-to-scrape ratio, a simple synchronous impl stalls reads
		// at ~1 read per hangTimeout during the hang. Over 5×hangTimeout that's
		// ~5 reads. We assert >= 2 to catch true deadlock while tolerating
		// proportional slowdown — an impl that serializes reads on writes is
		// still acceptable as long as the loop makes progress.
		before := src.seq()
		time.Sleep(5 * hangTimeout)
		after := src.seq()

		if after-before < 2 {
			t.Fatalf("scrape loop appears deadlocked on hung Write: seq went %d -> %d over %v (expected >= 2 new reads)",
				before, after, 5*hangTimeout)
		}
	})
}
