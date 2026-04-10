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

// --- BufferedSink implementation for resilience ---

// BufferedSink wraps a MetricSink, buffers metrics when the underlying sink fails,
// and flushes them periodically.
type BufferedSink struct {
	sink             MetricSink
	buffer           []Metric
	bufferCapacity   int
	flushInterval    time.Duration
	flushTicker      *time.Ticker
	stopChan         chan struct{}
	mu                sync.Mutex
}

func NewBufferedSink(sink MetricSink, capacity int, flushInterval time.Duration) *BufferedSink {
	return &BufferedSink{
		sink:           sink,
		buffer:         make([]Metric, 0, capacity),
		bufferCapacity: capacity,
		flushInterval:  flushInterval,
		stopChan:       make(chan struct{}),
	}
}

// Write attempts to write to the underlying sink. If it fails, it buffers the metric.
func (bs *BufferedSink) Write(m Metric) error {
	err := bs.sink.Write(m)
	if err == nil {
		return nil // Success, do nothing more
	}

	// Sink failed, buffer the metric
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if len(bs.buffer) < bs.bufferCapacity {
		// Buffer has space, simply append
		bs.buffer = append(bs.buffer, m)
		return fmt.Errorf("sink failed, buffered: %w", err)
	}

	// Buffer is full, replace a random element (Least Recently Used approximation via random replacement)
	randomIndex := rand.Intn(bs.bufferCapacity)
	bs.buffer[randomIndex] = m
	log.Printf("buffer full, replacing metric at index %d", randomIndex)
	return fmt.Errorf("sink failed and buffer full, replaced metric: %w", err)
}

// Start initiates the background flushing goroutine.
func (bs *BufferedSink) Start(flushInterval time.Duration) {
	bs.flushTicker = time.NewTicker(flushInterval)
	go bs.runFlusher()
}

// Stop halts the background goroutine.
func (bs *BufferedSink) Stop() {
	bs.flushTicker.Stop()
	close(bs.stopChan)
}

// runFlusher periodically attempts to flush the buffer.
func (bs *BufferedSink) runFlusher() {
	for {
		select {
		case <-bs.flushTicker.C:
			bs.flushBufferedMetrics()
		case <-bs.stopChan:
			log.Println("BufferedSink flusher stopping. Attempting final flush...")
			bs.flushBufferedMetrics() // Final attempt on stop
			return
		}
	}
}

// flushBufferedMetrics attempts to send all metrics in the buffer.
func (bs *BufferedSink) flushBufferedMetrics() {
	bs.mu.Lock()
	if len(bs.buffer) == 0 {
		bs.mu.Unlock()
		return
	}
	
	// Copy buffer contents to process, then clear the original buffer
	metricsToSend := make([]Metric, len(bs.buffer))
	copy(metricsToSend, bs.buffer)
	bs.buffer = bs.buffer[:0] 
	bs.mu.Unlock()
	
	log.Printf("Attempting to flush %d buffered metrics...", len(metricsToSend))

	// Attempt to write all metrics sequentially
	successfulWrites := 0
	for _, m := range metricsToSend {
		err := bs.sink.Write(m)
		if err == nil {
			successfulWrites++
		} else {
			// If an individual write fails, we stop flushing this batch
			// and put the remaining metrics (including the failed one) back into the buffer 
			// to be retried later, or simply log and drop them based on strictness.
			// For simplicity here, if we fail mid-batch, we put the remaining back.
			bs.mu.Lock()
			// Put back all metrics from the point of failure onwards
			bs.buffer = append(bs.buffer, metricsToSend[successfulWrites:]...)
			bs.mu.Unlock()
			log.Printf("Warning: Failed to write metric during batch flush. Remaining metrics returned to buffer. Error: %v", err)
			return
		}
	}
	
	log.Printf("Successfully flushed %d buffered metrics.", successfulWrites)
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

// --- Main loop ---

func run(scraper MetricScraper, bufferedSink *BufferedSink, interval time.Duration) {
	// The scraper runs every 'interval', but the sink handles the persistence/buffering.
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

		// Write to the buffered sink
		err = bufferedSink.Write(m)
		
		// Log based on what the sink reported (which is usually "success" or "buffered")
		if err != nil && !bytes.Contains([]byte(err.Error()), []byte("buffered")) {
			// If the error is not explicitly about buffering, it means the initial write failed 
			// and the buffer was full/replacement occurred, so we log the failure/replacement.
			log.Printf("metric storage failure (buffered/dropped): %v", err)
		}
	}
}

func main() {
	var (
		inverterHost string
		sinkURL      string
		interval     time.Duration
		bufferSize   int
		flushPeriod  time.Duration
	)
	flag.StringVar(&inverterHost, "inverter-host", "192.168.0.11", "inverter hostname or IP")
	flag.StringVar(&sinkURL, "sink-url", "http://localhost:8086", "metrics sink base URL")
	flag.DurationVar(&interval, "interval", 5*time.Second, "scrape interval (e.g. 5s, 100ms)")
	flag.IntVar(&bufferSize, "buffer-size", 1000, "maximum capacity of the metric buffer")
	flag.DurationVar(&flushPeriod, "flush-period", 10*time.Second, "how often to attempt flushing the buffer")
	flag.Parse()

	// 1. Initialize the actual network sink
	networkSink := NewHTTPSink(sinkURL)
	
	// 2. Wrap it with the resilient buffer
	bufferedSink := NewBufferedSink(networkSink, bufferSize, flushPeriod)

	// 3. Start the background flusher
	bufferedSink.Start(flushPeriod)

	// Set random seed for replacement logic
	rand.Seed(time.Now().UnixNano())

	scraper := NewHTTPScraper(inverterHost)

	log.Printf("Starting Resilience System:")
	log.Printf("  Scrape Interval: %v", interval)
	log.Printf("  Buffer Size: %d", bufferSize)
	log.Printf("  Flush Period: %v", flushPeriod)
	log.Printf("  Scraper Target: %s", scraper.URL)

	// Run the main collection loop
	run(scraper, bufferedSink, interval)
}
