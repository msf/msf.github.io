// Resilient Scraper implementation.
//
// Buffers metrics in memory while the sink is unavailable, bounded by
// maxBufSize, and flushes opportunistically once writes succeed again.

package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

func NewScraper(maxBufSize int, source MetricSource, sink MetricSink, interval time.Duration, hangTimeout time.Duration) (Scraper, error) {
	if source == nil {
		return nil, fmt.Errorf("source is nil")
	}
	if sink == nil {
		return nil, fmt.Errorf("sink is nil")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("interval must be positive, got %v", interval)
	}
	if maxBufSize < 0 {
		return nil, fmt.Errorf("maxBufSize must be >= 0, got %d", maxBufSize)
	}
	if hangTimeout <= 0 {
		return nil, fmt.Errorf("hangTimeout must be positive, got %v", hangTimeout)
	}
	return &resilientScraper{
		source:      source,
		sink:        sink,
		interval:    interval,
		maxBufSize:  maxBufSize,
		hangTimeout: hangTimeout,
	}, nil
}

type resilientScraper struct {
	source      MetricSource
	sink        MetricSink
	interval    time.Duration
	maxBufSize  int
	hangTimeout time.Duration
}

func (s *resilientScraper) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Buffer owned by this goroutine alone.
	buf := make([]Metric, 0, s.maxBufSize)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		data, err := s.readWithTimeout(ctx)
		if err != nil || data == nil {
			if err != nil && ctx.Err() == nil {
				log.Printf("source read: %v", err)
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		m := collect(data)
		power := extractPower(data)
		if err := power.Validate(); err != nil {
			log.Printf("power validation: %v", err)
		} else {
			m.Fields["TotalPower_W"] = power.Total()
		}

		if err := s.writeWithTimeout(ctx, m); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.enqueue(&buf, m)
			continue
		}
		s.drain(ctx, &buf)
	}
}

// readWithTimeout runs source.Read in a goroutine and returns whichever
// completes first: the result, a timeout, or ctx cancellation. On ctx cancel,
// the inner Read goroutine is leaked — acceptable since the interface doesn't
// take ctx and we must not block Run indefinitely.
func (s *resilientScraper) readWithTimeout(ctx context.Context) (*InverterData, error) {
	type result struct {
		data *InverterData
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		d, err := s.source.Read()
		ch <- result{d, err}
	}()

	timer := time.NewTimer(s.hangTimeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-timer.C:
		return nil, fmt.Errorf("read timed out after %v", s.hangTimeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *resilientScraper) writeWithTimeout(ctx context.Context, m Metric) error {
	ch := make(chan error, 1)
	go func() {
		ch <- s.sink.Write(m)
	}()
	timer := time.NewTimer(s.hangTimeout)
	defer timer.Stop()
	select {
	case err := <-ch:
		return err
	case <-timer.C:
		return fmt.Errorf("write timed out after %v", s.hangTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *resilientScraper) enqueue(buf *[]Metric, m Metric) {
	if s.maxBufSize == 0 {
		log.Printf("metric dropped: buffer disabled")
		return
	}
	if len(*buf) < s.maxBufSize {
		*buf = append(*buf, m)
		return
	}
	// Buffer full: make room by discarding the front, keeping the most recent
	// maxBufSize metrics.
	*buf = append((*buf)[1:], m)
}

// drain flushes buffered metrics opportunistically. Stops on first failure
// (or ctx cancel) and preserves unsent tail for the next drain attempt.
func (s *resilientScraper) drain(ctx context.Context, buf *[]Metric) {
	sent := 0
	for _, m := range *buf {
		if ctx.Err() != nil {
			break
		}
		if err := s.writeWithTimeout(ctx, m); err != nil {
			break
		}
		sent++
	}
	if sent == 0 {
		return
	}
	remaining := copy(*buf, (*buf)[sent:])
	*buf = (*buf)[:remaining]
}
