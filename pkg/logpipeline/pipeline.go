package logpipeline

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/safego"
)

type EntryKind string

const (
	KindLog  EntryKind = "log"
	KindFlow EntryKind = "flow"
	KindStep EntryKind = "step"
)

type logLevel int

const (
	logLevelDebug logLevel = iota
	logLevelInfo
	logLevelWarn
	logLevelError
)

func parseLogLevel(s string) logLevel {
	switch s {
	case "debug":
		return logLevelDebug
	case "info":
		return logLevelInfo
	case "warn":
		return logLevelWarn
	case "error":
		return logLevelError
	default:
		return logLevelInfo
	}
}

type LogEntry struct {
	Kind       EntryKind
	Level      string
	Message    string
	Fields     map[string]any
	Timestamp  time.Time
	SessionID  string
	StepID     string
	TraceID    string
	RunID      string
	Phase      string
	Severity   string
	DurationMS int64
	SpanID     string
}

type Sink interface {
	Write(entry LogEntry)
	Flush()
	Close() error
}

type Pipeline interface {
	Emit(entry LogEntry)
	AddSink(sink Sink)
	Close() error
	Dropped() uint64
	Throttled() uint64
	Stats() PipelineStats
	SetThrottleRules(rules []ThrottleRule)
}

type PipelineStats struct {
	Dropped    uint64
	Throttled  uint64
	ChanLen    int
	ChanCap    int
	SinkCount  int
	SinkErrors uint64
}

type ThrottleRule struct {
	Prefix     string
	MaxPerSec  int
}

type pipeline struct {
	mu          sync.RWMutex
	sinks       []Sink
	ch          chan LogEntry
	wg          sync.WaitGroup
	cancel      context.CancelFunc
	dropped     atomic.Uint64
	throttled   atomic.Uint64
	sinkErrors  atomic.Uint64
	throttler   *stepThrottler
	closed      atomic.Bool
}

const DefaultBufSize = 4096

func NewPipeline(bufSize int) Pipeline {
	if bufSize <= 0 {
		bufSize = DefaultBufSize
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &pipeline{
		ch:        make(chan LogEntry, bufSize),
		cancel:    cancel,
		throttler: newStepThrottler(),
	}
	p.wg.Add(1)
	safego.Go(ctx, "logpipeline-dispatcher", func() {
		defer p.wg.Done()
		p.dispatchLoop(ctx)
	})
	return p
}

func (p *pipeline) Emit(entry LogEntry) {
	if p.closed.Load() {
		p.dropped.Add(1)
		return
	}
	if p.throttler.shouldThrottle(entry.StepID) {
		p.throttled.Add(1)
		return
	}
	select {
	case p.ch <- entry:
	default:
		p.dropped.Add(1)
	}
}

func (p *pipeline) Dropped() uint64 {
	return p.dropped.Load()
}

func (p *pipeline) Throttled() uint64 {
	return p.throttled.Load()
}

func (p *pipeline) Stats() PipelineStats {
	p.mu.RLock()
	sinkCount := len(p.sinks)
	p.mu.RUnlock()
	return PipelineStats{
		Dropped:    p.dropped.Load(),
		Throttled:  p.throttled.Load(),
		ChanLen:    len(p.ch),
		ChanCap:    cap(p.ch),
		SinkCount:  sinkCount,
		SinkErrors: p.sinkErrors.Load(),
	}
}

func (p *pipeline) SetThrottleRules(rules []ThrottleRule) {
	p.throttler.setRules(rules)
}

func (p *pipeline) AddSink(sink Sink) {
	p.mu.Lock()
	p.sinks = append(p.sinks, sink)
	p.mu.Unlock()
}

func (p *pipeline) Close() error {
	p.closed.Store(true)
	p.cancel()
	close(p.ch)
	p.wg.Wait()
	p.mu.Lock()
	sinks := p.sinks
	p.mu.Unlock()
	var firstErr error
	for _, s := range sinks {
		s.Flush()
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (p *pipeline) dispatchLoop(ctx context.Context) {
	for {
		entry, ok := <-p.ch
		if !ok {
			return
		}
		p.dispatch(entry)
	}
}

func (p *pipeline) dispatch(entry LogEntry) {
	p.mu.RLock()
	sinks := p.sinks
	p.mu.RUnlock()
	for _, s := range sinks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					p.sinkErrors.Add(1)
				}
			}()
			s.Write(entry)
		}()
	}
}

type tokenBucket struct {
	tokens   float64
	maxTokens float64
	lastTime time.Time
	rate     float64
}

type stepThrottler struct {
	mu      sync.RWMutex
	buckets map[string]*tokenBucket
	rules   []ThrottleRule
}

func newStepThrottler() *stepThrottler {
	return &stepThrottler{
		buckets: make(map[string]*tokenBucket),
	}
}

func (t *stepThrottler) setRules(rules []ThrottleRule) {
	t.mu.Lock()
	t.rules = rules
	t.buckets = make(map[string]*tokenBucket)
	t.mu.Unlock()
}

func (t *stepThrottler) shouldThrottle(stepID string) bool {
	if stepID == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.rules) == 0 {
		return false
	}
	matched := ""
	matchedMaxPerSec := 0
	for _, r := range t.rules {
		if strings.HasPrefix(stepID, r.Prefix) {
			if len(r.Prefix) > len(matched) {
				matched = r.Prefix
				matchedMaxPerSec = r.MaxPerSec
			}
		}
	}
	if matched == "" {
		return false
	}
	b, ok := t.buckets[matched]
	if !ok {
		b = &tokenBucket{
			tokens:     1,
			maxTokens:  float64(matchedMaxPerSec),
			lastTime:   time.Now(),
			rate:       float64(matchedMaxPerSec),
		}
		t.buckets[matched] = b
	}
	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	b.lastTime = now
	b.tokens += elapsed * b.rate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}
	if b.tokens < 1 {
		return true
	}
	b.tokens--
	return false
}
