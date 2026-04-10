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
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// Metric represents a single timestamped measurement from the inverter.
type Metric struct {
	Timestamp  time.Time          `json:"timestamp"`
	DeviceName string             `json:"device_name"`
	Fields     map[string]float64 `json:"fields"`
}

// MetricSource fetches raw measurement data from a source (e.g. an inverter).
type MetricSource interface {
	Read() (*InverterData, error)
}

// MetricSink accepts metrics for storage or forwarding.
type MetricSink interface {
	Write(m Metric) error
}

// Scraper is the top-level service: it reads from a MetricSource and writes
// to a MetricSink on a fixed interval, until the context is cancelled.
type Scraper interface {
	Run(ctx context.Context) error
}

// NewScraper builds the default Scraper implementation.
//
// maxBufSize bounds in-memory state used to tolerate sink outages.
// source and sink are the data inputs and outputs.
// interval is the scrape period.
// hangTimeout is the upper bound on how long a single source.Read() or
// sink.Write() call may take before it must be abandoned. Calls exceeding
// this count as failures; the scraper must not block on them indefinitely.
func NewScraper(maxBufSize int, source MetricSource, sink MetricSink, interval time.Duration, hangTimeout time.Duration) (Scraper, error) {
	return &defaultScraper{
		source:      source,
		sink:        sink,
		interval:    interval,
		maxBufSize:  maxBufSize,
		hangTimeout: hangTimeout,
	}, nil
}

// defaultScraper is a minimal, non-resilient implementation: it has multiple bugs/innacuracies
// Extend this to satisfy the resilience spec.
type defaultScraper struct {
	source      MetricSource
	sink        MetricSink
	interval    time.Duration
	maxBufSize  int
	hangTimeout time.Duration
}

func (s *defaultScraper) Run(ctx context.Context) error {
	for {
		time.Sleep(s.interval)

		data, err := s.source.Read()
		if err != nil {
			log.Printf("source read error: %v", err)
		}

		m := collect(data)
		power := extractPower(data)
		if err := power.Validate(); err != nil {
			log.Printf("power validation: %v", err)
		} else {
			m.Fields["TotalPower_W"] = power.Total()
		}

		if err := s.sink.Write(m); err != nil {
			log.Printf("write error: %v, metric lost", err)
		}
	}
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

// HTTPSource fetches measurements from a Kostal inverter's XML endpoint.
type HTTPSource struct {
	URL    string
	client *http.Client
}

func NewHTTPSource(host string) *HTTPSource {
	return &HTTPSource{
		URL:    "http://" + host + "/measurements.xml",
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *HTTPSource) Read() (*InverterData, error) {
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

// --- Entry point ---

func main() {
	var (
		inverterHost string
		sinkURL      string
		interval     time.Duration
		maxBufSize   int
	)
	var hangTimeout time.Duration
	flag.StringVar(&inverterHost, "inverter-host", "192.168.0.11", "inverter hostname or IP")
	flag.StringVar(&sinkURL, "sink-url", "http://localhost:8086", "metrics sink base URL")
	flag.DurationVar(&interval, "interval", 5*time.Second, "scrape interval (e.g. 5s, 100ms)")
	flag.IntVar(&maxBufSize, "max-buf-size", 1000, "max metrics buffered in memory during sink outages")
	flag.DurationVar(&hangTimeout, "hang-timeout", 10*time.Second, "max time a single source/sink call may take before being abandoned")
	flag.Parse()

	source := NewHTTPSource(inverterHost)
	sink := NewHTTPSink(sinkURL)

	scraper, err := NewScraper(maxBufSize, source, sink, interval, hangTimeout)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("starting: source=%s sink=%s interval=%v max-buf-size=%d", source.URL, sink.URL, interval, maxBufSize)
	if err := scraper.Run(ctx); err != nil && err != context.Canceled {
		log.Fatalf("run: %v", err)
	}
}
