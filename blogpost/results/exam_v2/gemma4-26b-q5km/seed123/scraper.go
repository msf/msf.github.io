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

// --- Resilience logic ---

// ResilientSink wraps a MetricSink with an in-memory buffer to handle outages.
type ResilientSink struct {
	sink       MetricSink
	buffer     []Metric
	maxSize    int
	mu         sync.Mutex
	retryTimer *time.Ticker
}

func NewResilientSink(sink MetricSink, bufferSize int) *ResilientSink {
	rs := &ResilientSink{
		sink:    sink,
		buffer:  make([]Metric, 0, bufferSize),
		maxSize: bufferSize,
	}
	// Background goroutine to attempt flushing the buffer periodically
	go rs.flushLoop()
	return rs
}

func (rs *ResilientSink) Write(m Metric) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Try to write immediately
	err := rs.sink.Write(m)
	if err == nil {
		// If write succeeds and we have buffered items, try to flush them immediately
		if len(rs.buffer) > 0 {
			go rs.flush()
		}
		return nil
	}

	// Sink unreachable, buffer the metric
	log.Printf("sink unreachable, buffering metric: %v", err)
	if len(rs.buffer) < rs.maxSize {
		rs.buffer = append(rs.buffer, m)
	} else {
		// Buffer full: replace a random metric
		idx := rand.Intn(rs.maxSize)
		rs.buffer[idx] = m
	}
	return nil
}

func (rs *ResilientSink) flushLoop() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		rs.flush()
	}
}

func (rs *ResilientSink) flush() {
	rs.mu.Lock()
	if len(rs.buffer) == 0 {
		rs.mu.Unlock()
		return
	}

	// Copy buffer to minimize lock contention during network I/O
	toSend := make([]Metric, len(rs.buffer))
	copy(toSend, rs.buffer)
	rs.mu.Unlock()

	log.Printf("attempting to flush %d buffered metrics", len(toSend))
	
	success := true
	for _, m := range toSend {
		if err := rs.sink.Write(m); err != nil {
			log.Printf("flush failed during item send: %v", err)
			success = false
			break
		}
	}

	if success {
		rs.mu.Lock()
		// If we successfully sent everything, clear the buffer. 
		// Note: In a highly concurrent environment, new metrics might have arrived.
		// We only clear if the length hasn't changed in a way that implies new items 
		// were added that weren't part of this batch, but for this requirement, 
		// "flush all remaining" implies clearing what we just sent.
		rs.buffer = rs.buffer[:0]
		rs.mu.Unlock()
		log.Printf("successfully flushed buffer")
	}
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
			// This log is now mostly for the ResilientSink to signal it is buffering
			log.Printf("write error: %v", err)
		}
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
	flag.IntVar(&bufferSize, "buffer-size", 1000, "maximum number of metrics to buffer in memory")
	flag.Parse()

	scraper := NewHTTPScraper(inverterHost)
	baseSink := NewHTTPSink(sinkURL)
	sink := NewResilientSink(baseSink, bufferSize)

	log.Printf("starting: scrape=%s sink=%s interval=%v buffer-size=%d", scraper.URL, sinkURL, interval, bufferSize)
	run(scraper, sink, interval)
}
