package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

// Metric represents a single timestamped measurement from the inverter.
type Metric struct {
	Timestamp  time.Time          `json:"timestamp"`
	DeviceName string             `json:"device_name"`
	Fields     map[string]float64 `json:"fields"`
}

// MetricSink accepts metrics for storage or forwarding.
type MetricSink interface {
	Write(m Metric) error
}

// MetricScraper fetches raw measurement data from a source.
type MetricScraper interface {
	Scrape() (*InverterData, error)
}

// --- HTTP implementations ---

// HTTPSink sends metrics as JSON POST to a remote endpoint.
type HTTPSink struct {
	URL    string
	client *http.Client
}

func NewHTTPSink(url string) *HTTPSink {
	return &HTTPSink{
		URL:    url,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (w *HTTPSink) Write(m Metric) error {
	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	resp, err := w.client.Post(w.URL+"/write", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("sink returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// HTTPScraper fetches measurements from a Kostal inverter's XML endpoint.
type HTTPScraper struct {
	URL    string
	client *http.Client
}

func NewHTTPScraper(host string) *HTTPScraper {
	return &HTTPScraper{
		URL:    "http://" + host + "/measurements.xml",
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *HTTPScraper) Scrape() (*InverterData, error) {
	resp, err := s.client.Get(s.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var root InverterData
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	return &root, nil
}

// --- Buffered Sink Implementation ---

// BufferedSink implements MetricSink, buffering metrics when the underlying sink fails.
type BufferedSink struct {
	upstreamSink MetricSink
	buffer       []Metric
	maxCapacity  int
	mu           sync.Mutex
	stopChan     chan struct{}
	flushInterval time.Duration
}

func NewBufferedSink(upstream MetricSink, capacity int, flushInterval time.Duration) *BufferedSink {
	bs := &BufferedSink{
		upstreamSink: upstream,
		buffer:       make([]Metric, 0, capacity),
		maxCapacity:  capacity,
		stopChan:     make(chan struct{}),
		flushInterval: flushInterval,
	}
	go bs.flushMetrics()
	return bs
}

// Write adds a metric to the buffer or attempts to write directly.
func (bs *BufferedSink) Write(m Metric) error {
	err := bs.upstreamSink.Write(m)
	if err == nil {
		// Success, no buffering needed for this metric.
		return nil
	}

	// Sink failed, buffer the metric (or replace if full)
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if len(bs.buffer) < bs.maxCapacity {
		// Buffer not full, append
		bs.buffer = append(bs.buffer, m)
		return fmt.Errorf("sink failed, buffered metric")
	}

	// Buffer is full, randomly replace an existing metric
	if len(bs.buffer) > 0 {
		idxToReplace := rand.Intn(len(bs.buffer))
		bs.buffer[idxToReplace] = m // Replace existing metric
		return fmt.Errorf("sink failed, replaced metric in buffer")
	}
	return fmt.Errorf("sink failed, buffer full and empty (should not happen)")
}

// flushMetrics runs periodically to drain the buffer.
func (bs *BufferedSink) flushMetrics() {
	ticker := time.NewTicker(bs.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bs.tryFlush()
		case <-bs.stopChan:
			log.Println("BufferedSink: stopping flusher goroutine.")
			// Final flush on stop
			bs.tryFlush()
			return
		}
	}
}

// tryFlush attempts to write all buffered metrics to the upstream sink.
func (bs *BufferedSink) tryFlush() {
	bs.mu.Lock()
	if len(bs.buffer) == 0 {
		bs.mu.Unlock()
		return
	}

	// Take a copy of the metrics to flush and clear the original buffer for concurrent writing
	metricsToFlush := make([]Metric, len(bs.buffer))
	copy(metricsToFlush, bs.buffer)
	bs.buffer = make([]Metric, 0, bs.maxCapacity) // Reset buffer
	bs.mu.Unlock()

	log.Printf("BufferedSink: attempting to flush %d metrics...", len(metricsToFlush))
	
	// Attempt to send all metrics sequentially (for simplicity, though parallel sending could be added)
	successCount := 0
	for _, m := range metricsToFlush {
		if err := bs.upstreamSink.Write(m); err == nil {
			successCount++
		} else {
			// If a write fails during the flush, put it back into the buffer and stop flushing for now.
			// This ensures we don't lose any data if the network flickers during the flush cycle.
			log.Printf("BufferedSink: Flush failed for one metric (%v). Re-buffering all failed metrics.", err)
			
			bs.mu.Lock()
			// Re-add the failed metric and all subsequent metrics back to the buffer
			bs.buffer = append(bs.buffer, m)
			bs.buffer = append(bs.buffer, metricsToFlush[successCount+1:]...)
			bs.mu.Unlock()
			return
		}
	}
	
	log.Printf("BufferedSink: Successfully flushed and dropped %d metrics.", successCount)
}

// Stop signals the flusher goroutine to shut down.
func (bs *BufferedSink) Stop() {
	close(bs.stopChan)
}

// --- Domain logic ---

// collect extracts a Metric from parsed inverter data.
func collect(data *InverterData) Metric {
	m := Metric{
		Timestamp:  time.Now().UTC(),
		DeviceName: data.Device.Name,
		Fields:     make(map[string]float64),
	}
	for _, meas := range data.Device.Measurements.Measurement {
		key := fmt.Sprintf("%s_%s", meas.Type, meas.Unit)
		m.Fields[key] = meas.Value
	}
	return m
}

type kostalPower struct {
	gridConsumed float64
	gridInjected float64
	ownConsumed  float64
}

func (k kostalPower) Total() float64 {
	if k.gridConsumed > 0 {
		return k.gridConsumed + k.ownConsumed
	}
	return k.ownConsumed + k.gridInjected
}

func (k kostalPower) Validate() error {
	if k.ownConsumed < 0 || k.gridInjected < 0 || k.gridConsumed < 0 {
		return fmt.Errorf("invalid power %+v: values cannot be negative", k)
	}
	if (k.gridInjected == 0 && k.gridConsumed == 0) ||
		(k.gridInjected > 0 && k.gridConsumed > 0) {
		return fmt.Errorf("inconsistent power %+v: grid must be either injecting or consuming", k)
	}
	return nil
}

func extractPower(data *InverterData) kostalPower {
	var p kostalPower
	for _, m := range data.Device.Measurements.Measurement {
		switch m.Type {
		case "OwnConsumedPower":
			p.ownConsumed = m.Value
		case "GridConsumedPower":
			p.gridConsumed = m.Value
		case "GridInjectedPower":
			p.gridInjected = m.Value
		}
	}
	return p
}

// --- Main loop ---

func run(scraper MetricScraper, sink MetricSink, interval time.Duration) {
	// We need to track the scraper interval, but the sink flushing runs on its own timer.
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		data, err := scraper.Scrape()
		if err != nil {
			log.Printf("scrape error: %v", err)
			continue
		}

		m := collect(data)
		power := extractPower(data)
		if err := power.Validate(); err != nil {
			log.Printf("power validation: %v", err)
		} else {
			m.Fields["TotalPower_W"] = power.Total()
		}

		if err := sink.Write(m); err != nil {
			// This log message now reflects that the BufferedSink handled the failure
			log.Printf("write error (handled by buffer?): %v", err)
		}
	}
}

func main() {
	rand.Seed(time.Now().UnixNano()) // Seed random generator for eviction logic

	var (
		inverterHost string
		sinkURL      string
		scrapeInterval time.Duration
		bufferSize   int
		flushInterval time.Duration
	)
	flag.StringVar(&inverterHost, "inverter-host", "192.168.0.11", "inverter hostname or IP")
	flag.StringVar(&sinkURL, "sink-url", "http://localhost:8086", "metrics sink base URL")
	flag.DurationVar(&scrapeInterval, "interval", 5*time.Second, "scrape interval (e.g. 5s, 100ms)")
	flag.IntVar(&bufferSize, "buffer-size", 1000, "maximum size of the metric buffer")
	flag.DurationVar(&flushInterval, "flush-interval", 10*time.Second, "how often to attempt flushing buffered metrics")
	flag.Parse()

	scraper := NewHTTPScraper(inverterHost)
	
	// 1. Setup the primary (unreliable) sink
	upstreamSink := NewHTTPSink(sinkURL)

	// 2. Wrap it in the resilient buffered sink
	bufferedSink := NewBufferedSink(upstreamSink, bufferSize, flushInterval)
	
	// Handle graceful shutdown (optional but good practice)
	// In a real service, you'd trap OS signals here.
	
	log.Printf("starting: scrape=%s sink=%s interval=%v. Buffer Size: %d, Flush Interval: %v", 
		scraper.URL, sinkURL, scrapeInterval, bufferSize, flushInterval)
	
	// Run the main collection loop
	run(scraper, bufferedSink, scrapeInterval)

	// If run returns (which it shouldn't in this infinite loop setup), stop the buffer
	bufferedSink.Stop() 
}
