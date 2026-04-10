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
	Timestamp   time.Time          `json:"timestamp"`
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

// --- Kostal solar inverter XML types ---

type InverterData struct {
	XMLName xml.Name `xml:"root"`
	Device  struct {
		Name          string `xml:"Name,attr"`
		Type          string `xml:"Type,attr"`
		Serial        string `xml:"Serial,attr"`
		IPAddress     string `xml:"IpAddress,attr"`
		DateTime       string `xml:"DateTime,attr"`
		Measurements struct {
			Measurement []struct {
				Value float64 `xml:"Value,attr"`
				Unit  string `xml:"Unit,attr"`
				Type  string `xml:"Type,attr"`
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

// --- Buffered sink implementation ---

// BufferedSink wraps a MetricSink and buffers metrics when the sink is unreachable.
type BufferedSink struct {
	inner  MetricSink
	mu      sync.Mutex
	buffer  []Metric
	maxSize int
}

// NewBufferedSink creates a new BufferedSink that wraps the provided MetricSink.
func NewBufferedSink(inner MetricSink, maxSize int) *BufferedSink {
	bs := &BufferedSink{
		inner:    inner,
		buffer:  make([]Metric, 0, maxSize),
		maxSize: maxSize,
	}
	// Start background goroutine that periodically attempts to flush the buffer.
	go bs.periodicFlush()
	return bs
}

// Write writes a metric to the underlying sink. If the sink is unreachable,
// the metric is buffered for later retry.
func (bs *BufferedSink) Write(m Metric) error {
	if err := bs.inner.Write(m); err != nil {
		// Buffer the metric.
		bs.mu.Lock()
		if len(bs.buffer) >= bs.maxSize {
			// Replace a random metric in the buffer.
			idx := rand.Intn(bs.maxSize)
			bs.buffer[idx] = m
		} else {
			bs.buffer = append(bs.buffer, m)
		}
		bs.mu.Unlock()
		// Do not return an error to avoid spamming logs.
		return nil
	}
	return nil
}

// periodicFlush runs a background goroutine that attempts to send buffered metrics.
func (bs *BufferedSink) periodicFlush() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		bs.flush()
	}
}

// flush attempts to send buffered metrics to the sink.
// If any metric is sent successfully, all buffered metrics are discarded.
func (bs *BufferedSink) flush() {
	// Copy the buffer to avoid holding the lock while sending.
	bs.mu.Lock()
	buf := make([]Metric, len(bs.buffer))
	copy(buf, bs.buffer)
	// Clear the buffer; we may re-add on failure.
	bs.buffer = bs.buffer[:0]
	bs.mu.Unlock()

	success := false
	for _, m := range buf {
		if err := bs.inner.Write(m); err == nil {
			success = true
			// Flush remaining metrics.
			bs.mu.Lock()
			bs.buffer = bs.buffer[:0]
			bs.mu.Unlock()
			return
		}
	}
	if !success {
		// No metric was sent successfully; restore the buffer.
		bs.mu.Lock()
		bs.buffer = append(bs.buffer, buf...)
		bs.mu.Unlock()
	}
}

// run executes the main loop, periodically scraping and sending metrics.
func run(scraper MetricScraper, sink MetricSink, interval time.Duration) {
	for {
		time.Sleep(interval)

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
			log.Printf("write error: %v, metric lost", err)
		}
	}
}

func main() {
	var (
		inverterHost string
		sinkURL      string
		interval     time.Duration
		bufferSize  int
	)
	flag.StringVar(&inverterHost, "inverter-host", "192.168.0.11", "inverter hostname or IP")
	flag.StringVar(&sinkURL, "sink-url", "http://localhost:8086", "metrics sink base URL")
	flag.DurationVar(&interval, "interval", 5*time.Second, "scrape interval (e.g. 5s, 100ms)")
	flag.IntVar(&bufferSize, "buffer-size", 1000, "maximum number of buffered metrics during sink outages")
	flag.Parse()

	scraper := NewHTTPScraper(inverterHost)
	baseSink := NewHTTPSink(sinkURL)
	sink := NewBufferedSink(baseSink, bufferSize)

	log.Printf("starting: scrape=%s sink=%s interval=%v bufferSize=%d", scraper.URL, sinkURL, interval, bufferSize)
	run(scraper, sink, interval)
}
