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

// --- Buffered Sink Implementation ---

// BufferedSink wraps a MetricSink and adds buffering when the sink is unreachable
type BufferedSink struct {
	sink     MetricSink
	buffer   []Metric
	capacity int
	mutex  sync.RWMutex
}

func NewBufferedSink(sink MetricSink, capacity int) *BufferedSink {
	return &BufferedSink{
		sink:     sink,
		buffer:   make([]Metric, 0, capacity),
		capacity: capacity,
	}
}

// Write attempts to write a metric to the sink. If the sink is not
// reachable, it buffers the metric instead
func (bs *BufferedSink) Write(m Metric) error {
	bs.mutex.Lock()
	
	// Try to write to the actual sink first
	if err := bs.sink.Write(m); err == nil {
		bs.mutex.Unlock()
		return nil
	}
	
	// If the sink failed, add to the buffer
	if len(bs.buffer) < bs.capacity {
		// Buffer has space, just add the metric
		bs.buffer = append(bs.buffer, m)
		bs.mutex.Unlock()
		return nil
	}
	
	// Buffer is full, replace a random metric with the new one
	randomIndex := rand.Intn(len(bs.buffer))
	bs.buffer[randomIndex] = m
	bs.mutex.Unlock()
	return nil
}

// flush attempts to send all buffered metrics to the sink
func (bs *BufferedSink) flush() error {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()
	
	// Send all buffered metrics
	for len(bs.buffer) > 0 {
		// Send one metric
		m := bs.buffer[0]
		if err := bs.sink.Write(m); err != nil {
			return err // Stop if we encounter an error
		}
		
		// Remove the sent metric
		bs.buffer = bs.buffer[1:]
	}
	
	return nil
}

// flushAll flushes all buffered metrics and returns the number of flushed metrics
func (bs *BufferedSink) flushAll() int {
	bs.mutex.Lock()
	defer bs.mutex.Unlock()
	
	count := len(bs.buffer)
	
	// Send all buffered metrics
	for len(bs.buffer) > 0 {
		m := bs.buffer[0]
		if err := bs.sink.Write(m); err == nil {
			bs.buffer = bs.buffer[1:]
		} else {
			break // Stop if we encounter an error
		}
	}
	
	return count
}

// --- Main loop with background flushing ---

func run(scraper MetricScraper, sink *BufferedSink, interval time.Duration, flushInterval time.Duration) {
	// Start background goroutine to flush buffer periodically
	go func() {
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()
		
		for range ticker.C {
			flushed := sink.flushAll()
			if flushed > 0 {
				log.Printf("flushed %d metrics to sink", flushed)
			}
		}
	}()
	
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
		bufferSize     int
		flushInterval  time.Duration
	)
	flag.StringVar(&inverterHost, "inverter-host", "192.168.0.11", "inverter hostname or IP")
	flag.StringVar(&sinkURL, "sink-url", "http://localhost:8086", "metrics sink base URL")
	flag.DurationVar(&interval, "interval", 5*time.Second, "scrape interval (e.g. 5s, 100ms)")
	flag.IntVar(&bufferSize, "buffer-size", 1000, "maximum number of metrics to buffer")
	flag.DurationVar(&flushInterval, "flush-interval", 30*time.Second, "buffer flush interval")
	flag.Parse()

	scraper := NewHTTPScraper(inverterHost)
	sink := NewBufferedSink(NewHTTPSink(sinkURL), bufferSize)

	log.Printf("starting: scrape=%s sink=%s interval=%v buffer=%d flush=%v", scraper.URL, sink.(*BufferedSink).sink.(*HTTPSink).URL, interval, bufferSize, flushInterval)
	run(scraper, sink, interval, flushInterval)
}
