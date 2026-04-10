// Mock server for exam_v2: combined inverter + sink + control API on one port.
//
// Endpoints:
//
//	GET  /measurements.xml   — mock Kostal inverter (always available)
//	POST /write              — mock metrics sink (toggleable)
//	POST /control/offline    — sink starts returning 503
//	POST /control/online     — sink starts accepting writes
//	POST /control/reset      — clear all received metrics
//	GET  /control/metrics    — dump received metrics as JSON array
//	GET  /control/count      — return {"count": N}
//	GET  /control/status     — return {"online": true/false}
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Metric struct {
	Timestamp  time.Time          `json:"timestamp"`
	DeviceName string             `json:"device_name"`
	Fields     map[string]float64 `json:"fields"`
	ReceivedAt time.Time          `json:"received_at"`
}

type server struct {
	mu      sync.Mutex
	metrics []Metric
	online  atomic.Bool
	scrapeN atomic.Int64
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/measurements.xml" && r.Method == "GET":
		s.handleScrape(w, r)
	case r.URL.Path == "/write" && r.Method == "POST":
		s.handleWrite(w, r)
	case r.URL.Path == "/control/offline" && r.Method == "POST":
		s.online.Store(false)
		w.WriteHeader(200)
		fmt.Fprintln(w, "sink offline")
	case r.URL.Path == "/control/online" && r.Method == "POST":
		s.online.Store(true)
		w.WriteHeader(200)
		fmt.Fprintln(w, "sink online")
	case r.URL.Path == "/control/reset" && r.Method == "POST":
		s.mu.Lock()
		s.metrics = nil
		s.mu.Unlock()
		w.WriteHeader(200)
		fmt.Fprintln(w, "metrics reset")
	case r.URL.Path == "/control/metrics" && r.Method == "GET":
		s.mu.Lock()
		data, _ := json.Marshal(s.metrics)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	case r.URL.Path == "/control/count" && r.Method == "GET":
		s.mu.Lock()
		n := len(s.metrics)
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"count":%d}`, n)
	case r.URL.Path == "/control/status" && r.Method == "GET":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"online":%v}`, s.online.Load())
	default:
		w.WriteHeader(404)
	}
}

func (s *server) handleScrape(w http.ResponseWriter, _ *http.Request) {
	n := s.scrapeN.Add(1)
	// Vary values so each metric is distinguishable by its OwnConsumedPower field.
	own := float64(n) * 100.0
	grid := float64(n) * 10.0
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<root>
  <Device Name="PIKO" Type="Inverter" Serial="123456" IpAddress="192.168.0.11" DateTime="%s">
    <Measurements>
      <Measurement Value="%.1f" Unit="W" Type="OwnConsumedPower"/>
      <Measurement Value="0" Unit="W" Type="GridConsumedPower"/>
      <Measurement Value="%.1f" Unit="W" Type="GridInjectedPower"/>
    </Measurements>
  </Device>
</root>`, time.Now().Format(time.RFC3339), own, grid)
	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(xml))
}

func (s *server) handleWrite(w http.ResponseWriter, r *http.Request) {
	if !s.online.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	var m Metric
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		w.WriteHeader(400)
		fmt.Fprintf(w, "bad json: %v", err)
		return
	}
	m.ReceivedAt = time.Now().UTC()
	s.mu.Lock()
	s.metrics = append(s.metrics, m)
	s.mu.Unlock()
	w.WriteHeader(200)
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port

	s := &server{}
	s.online.Store(true)

	// Write port to stdout so the orchestrator can discover it, then to a file.
	fmt.Println(port)
	if len(os.Args) > 1 {
		os.WriteFile(os.Args[1], []byte(fmt.Sprintf("%d", port)), 0644)
	}

	log.Printf("mock server listening on :%d", port)
	log.Fatal(http.Serve(ln, s))
}
