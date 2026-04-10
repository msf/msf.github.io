#START scraper.go#
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
	return &root, xml.Unmarshal(data, &root)
}

// --- Kostal solar inverter XML types ---

type InverterData struct {
	XMLName xml.Name `xml:"root"`
	Device struct {
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
		Timestamp:   time.Now().UTC(),
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

// --- Buffered Sink ---

// BufferedSink decorates an underlying MetricSink and buffers metrics when
// the sink is unreachable.
type BufferedSink struct {
	underlying   MetricSink
	buffer       []Metric
	capacity     int
	mu           sync.Mutex
	flushTicker  *time.Ticker
	stopChan     chan struct{}
}

func NewBufferedSink(base MetricSink, capacity int, flushInterval time.Duration) *BufferedSink {
	b := &BufferedSink{
		underlying: base,
		capacity:    capacity,
		flushTicker: time.NewTicker(flushInterval),
		stopChan:    make(chan struct{}),
	}
	go b.flushLoop()
	return b
}

func (b *BufferedSink) Write(m Metric) error {
	if err := b.underlying.Write(m); err == nil {
		return nil
	}

	// Underlying sink failed; buffer the metric.
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.buffer) < b.capacity {
		b.buffer = append(b.buffer, m)
	} else {
		// Randomly replace an existing metric.
		idx := rand.Intn(len(b.buffer))
		b.buffer[idx] = m
	}
	// No error returned – treat as success for the caller.
	return nil
}

// flushLoop periodically attempts to send all buffered metrics.
func (b *BufferedSink) flushLoop() {
	for {
		select {
		case <-b.flushTicker.C:
			b.attemptFlush()
		case <-b.stopChan:
			b.flushTicker.Stop()
			return
		}
	}
}

// attemptFlush sends all metrics currently in the buffer.
// If all writes succeed, the buffer is cleared.
// If any write fails, the buffer remains unchanged.
func (b *BufferedSink) attemptFlush() {
	// Copy buffer to avoid holding lock during network I/O.
	b.mu.Lock()
	localBuf := make([]Metric, len(b.buffer))
	copy(localBuf, b.buffer)
	b.mu.Unlock()

	if len(localBuf) == 0 {
		return
	}

	allSuccess := true
	for _, m := range localBuf {
		if err := b.underlying.Write(m); err != nil {
			allSuccess = false
			// Stop on first failure to avoid further network traffic.
			break
		}
	}

	if allSuccess {
		b.mu.Lock()
		b.buffer = b.buffer[:0]
		b.mu.Unlock()
	}
}

// Close stops the background flush goroutine.
func (b *BufferedSink) Close() {
	close(b.stopChan)
}

// --- Main loop ---

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

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	var (
		inverterHost string
		sinkURL        string
		interval      time.Duration
		bufferSize     int
	)

	flag.StringVar(&inverterHost, "inverter-host", "192.168.0.11", "inverter hostname or IP")
	flag.StringVar(&sinkURL, "sink-url", "http://localhost:8086", "metrics sink base URL")
	flag.DurationVar(&interval, "interval", 5*time.Second, "scrape interval (e.g. 5s, 100ms)")
	flag.IntVar(&bufferSize, "buffer-size", 1000, "maximum metrics buffer size when sink unreachable")
	flag.Parse()

	scraper := NewHTTPScraper(inverterHost)
	underlyingSink := NewHTTPSink(sinkURL)
	sink := NewBufferedSink(underlyingSink, bufferSize, interval)

	log.Printf("starting: scrape=%s sink=%s interval=%v buffer-size=%d", scraper.URL, sinkURL, interval, bufferSize)
	run(scraper, sink, interval)
}
#END scraper.go#
