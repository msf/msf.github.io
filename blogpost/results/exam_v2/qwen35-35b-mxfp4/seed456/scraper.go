package main

import (
	"bytes"
	"context"
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

// --- Resilient Buffer ---

// BufferedSink wraps a MetricSink and buffers metrics in memory during outages.
type BufferedSink struct {
	sink      MetricSink
	buffer    []Metric
	mu        sync.Mutex
	capacity  int
}

func NewBufferedSink(sink MetricSink, capacity int) *BufferedSink {
	return &BufferedSink{
		sink:     sink,
		buffer:   make([]Metric, 0, capacity),
		capacity: capacity,
	}
}

func (b *BufferedSink) Write(m Metric) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if buffer is full
	if len(b.buffer) >= b.capacity {
		// Randomly select one metric from the buffer and replace it with the new one
		randIdx := rand.Intn(len(b.buffer))
		b.buffer[randIdx] = m
		log.Printf("Buffer full (%d/%d), replacing metric at index %d", len(b.buffer), b.capacity, randIdx)
		return nil
	}

	// Add new metric
	b.buffer = append(b.buffer, m)
	return nil
}

// Flush attempts to send all buffered metrics to the underlying sink.
func (b *BufferedSink) Flush() error {
	b.mu.Lock()
	if len(b.buffer) == 0 {
		b.mu.Unlock()
		return nil
	}
	// Make a copy of the buffer to send to avoid holding the lock during network I/O
	metricsToSend := make([]Metric, len(b.buffer))
	copy(metricsToSend, b.buffer)
	// Clear the buffer after copying
	b.buffer = make([]Metric, 0, b.capacity)
	b.mu.Unlock()

	// Send metrics
	var lastErr error
	for _, m := range metricsToSend {
		if err := b.sink.Write(m); err != nil {
			lastErr = err
			log.Printf("Failed to flush metric: %v", err)
			continue
		}
	}
	
	if lastErr != nil {
		return fmt.Errorf("flush failed: %w", lastErr)
	}
	return nil
}

// --- Main loop ---

// startFlusher runs a background goroutine to periodically flush buffered metrics.
func startFlusher(ctx context.Context, sink *BufferedSink, flushInterval time.Duration) {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := sink.Flush(); err != nil {
				log.Printf("flush attempt failed: %v", err)
			} else {
				log.Printf("flush successful")
			}
		case <-ctx.Done():
			// Final flush on exit
			log.Println("Shutting down, performing final flush...")
			_ = sink.Flush()
			return
		}
	}
}

// runWithResilience runs the scraping loop and starts the flusher.
func runWithResilience(scraper MetricScraper, bufferedSink *BufferedSink, interval time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background flusher
	go startFlusher(ctx, bufferedSink, 10*time.Second)

	// Run the scraping loop
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

		// BufferedSink.Write never returns an error (it buffers or drops randomly)
		// The "metric lost" log is handled inside BufferedSink.Write when the buffer is full.
		_ = bufferedSink.Write(m)
	}
}

func main() {
	var (
		inverterHost string
		sinkURL      string
		interval     time.Duration
		bufferSize   int
	)
	flag.StringVar(&inverterHost, "inverter-host", "192.168.0.11", "inverter hostname or IP")
	flag.StringVar(&sinkURL, "sink-url", "http://localhost:8086", "metrics sink base URL")
	flag.DurationVar(&interval, "interval", 5*time.Second, "scrape interval (e.g. 5s, 100ms)")
	flag.IntVar(&bufferSize, "buffer-size", 1000, "maximum number of metrics to buffer")
	flag.Parse()

	scraper := NewHTTPScraper(inverterHost)
	// Create the underlying sink
	underlyingSink := NewHTTPSink(sinkURL)
	// Wrap it with the buffer
	bufferedSink := NewBufferedSink(underlyingSink, bufferSize)

	log.Printf("starting: scrape=%s sink=%s interval=%v buffer-capacity=%d", scraper.URL, sinkURL, interval, bufferSize)
	
	// Initialize random seed for replacement logic
	rand.Seed(time.Now().UnixNano())

	runWithResilience(scraper, bufferedSink, interval)
}
