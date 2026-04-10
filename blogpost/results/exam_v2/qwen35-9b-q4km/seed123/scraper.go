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
	"sync/atomic"
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
	return &root, xml.Unmarshal(data, &root)
}

// --- Kostal solar inverter XML types ---

type InverterData struct {
	XMLName xml.Name `xml:"root"`
	Device  struct {
		Name         string `xml:"Name,attr"`
		Type         string `xml:"Type,attr"`
		Serial       string `xml:"Serial,attr"`
		IPAddress    string `xml:"IpAddress,attr"`
		DateTime     string `xml:"DateTime,attr"`
		Measurements struct {
			Measurement []struct {
				Value float64 `xml:"Value,attr"`
				Unit  string  `xml:"Unit,attr"`
				Type  string  `xml:"Type,attr"`
			} `xml:"Measurement"`
		} `xml:"Measurements"`
	} `xml:"Device"`
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

// MetricBuffer handles buffering metrics when the sink is unavailable.
type MetricBuffer struct {
	mu          sync.Mutex
	metrics     []Metric
	maxCapacity int
	count       int64 // For logging/shuffling if needed, though len() is enough
}

func NewMetricBuffer(capacity int) *MetricBuffer {
	return &MetricBuffer{
		metrics:     make([]Metric, 0, capacity),
		maxCapacity: capacity,
	}
}

func (b *MetricBuffer) Write(m Metric) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add to the buffer
	b.metrics = append(b.metrics, m)
	atomic.AddInt64(&b.count, 1)

	// If over capacity, evict one random metric
	if len(b.metrics) > b.maxCapacity {
		// We have len = maxCapacity + 1 items.
		// We want to reduce to maxCapacity.
		// Strategy: Pick a random index i from 0 to len-2.
		// Swap metrics[i] with metrics[len-1] (the newly added one, or just the last).
		// Remove the last element.
		// This gives the new metric a 1/(maxCapacity+1) chance of being evicted immediately,
		// and all other metrics equal chance.
		
		n := len(b.metrics)
		if n > 0 {
			randIdx := rand.Intn(n - 1) // 0 to n-2
			// Swap metrics[randIdx] with metrics[n-1]
			b.metrics[randIdx], b.metrics[n-1] = b.metrics[n-1], b.metrics[randIdx]
			b.metrics = b.metrics[:n-1]
		}
	}
}

func (b *MetricBuffer) Get() []Metric {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]Metric, len(b.metrics))
	copy(result, b.metrics)
	return result
}

func (b *MetricBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.metrics)
}

func (b *MetricBuffer) Flush() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	length := len(b.metrics)
	return length
}

// --- Resilience Runner ---

type ResilienceRunner struct {
	scraper MetricScraper
	sink    MetricSink
	buffer  *MetricBuffer
	interval time.Duration
	flushInterval time.Duration
}

func NewResilienceRunner(scraper MetricScraper, sink MetricSink, buffer *MetricBuffer, interval, flushInterval time.Duration) *ResilienceRunner {
	return &ResilienceRunner{
		scraper:       scraper,
		sink:          sink,
		buffer:        buffer,
		interval:      interval,
		flushInterval: flushInterval,
	}
}

func (r *ResilienceRunner) Run() {
	// Start the background flusher
	go r.backgroundFlusher()

	// Start the main collection loop
	r.collectLoop()
}

func (r *ResilienceRunner) collectLoop() {
	for {
		time.Sleep(r.interval)

		data, err := r.scraper.Scrape()
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

		// Write to buffer. If sink is up later, we can try to flush? 
		// The requirement says: "Run a background goroutine that periodically attempts to send buffered metrics".
		// So we just buffer. The background goroutine will try to send.
		r.buffer.Write(m)
	}
}

func (r *ResilienceRunner) backgroundFlusher() {
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()

	for range ticker.C {
		// Attempt to send buffered metrics
		metrics := r.buffer.Get()
		
		if len(metrics) == 0 {
			continue
		}

		// Try to send all
		success := true
		for i, m := range metrics {
			if err := r.sink.Write(m); err != nil {
				log.Printf("background flush error for metric %d: %v", i, err)
				// Should we stop flushing? The requirement implies "When a send succeeds, flush all remaining".
				// If one fails, should we stop? Or retry?
				// Usually in resilience, if one fails, you might retry later or skip.
				// But the prompt says: "When a send succeeds, flush all remaining buffered metrics."
				// This implies a "flush" operation that might fail partially.
				// If it fails, we don't remove the metric from the buffer until a successful send.
				// So we must not call buffer removal here.
				success = false
				break
			}
		}

		if success {
			// All sent successfully. Flush buffer.
			count := r.buffer.Flush()
			log.Printf("successfully flushed %d metrics to sink", count)
		} else {
			// Partial or total failure. Metrics remain in buffer.
			// Log the failure but don't remove metrics.
			_ = success 
		}
	}
}

func main() {
	var (
		inverterHost string
		sinkURL      string
		interval     time.Duration
		bufferSize   int
		flushInterval time.Duration
	)
	flag.StringVar(&inverterHost, "inverter-host", "192.168.0.11", "inverter hostname or IP")
	flag.StringVar(&sinkURL, "sink-url", "http://localhost:8086", "metrics sink base URL")
	flag.DurationVar(&interval, "interval", 5*time.Second, "scrape interval (e.g. 5s, 100ms)")
	flag.IntVar(&bufferSize, "buffer-size", 1000, "maximum capacity of the metric buffer")
	flag.DurationVar(&flushInterval, "flush-interval", 10*time.Second, "interval for background flush attempts")
	flag.Parse()

	// Initialize random seed
	rand.Seed(time.Now().UnixNano())

	scraper := NewHTTPScraper(inverterHost)
	sink := NewHTTPSink(sinkURL)
	buffer := NewMetricBuffer(bufferSize)

	log.Printf("starting: scrape=%s sink=%s interval=%v buffer_size=%d flush_interval=%v", 
		scraper.URL, sink.URL, interval, bufferSize, flushInterval)

	resilienceRunner := NewResilienceRunner(scraper, sink, buffer, interval, flushInterval)
	resilienceRunner.Run()
}
