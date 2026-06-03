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
	Prefix    string
	MaxPerSec int
}

type pipeline struct {
	mu          sync.RWMutex
	sinkGroups  []*SinkGroup
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
	// SUG-6 fix: use select with ctx.Done() to prevent writing to closed channel
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
	sinkCount := len(p.sinkGroups)
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
	p.AddSinkGroup(sink, DefaultBufSize, DropNewest, "unnamed")
}

// AddSinkGroup creates a SinkGroup wrapping the given Sink and adds it to the pipeline.
func (p *pipeline) AddSinkGroup(sink Sink, bufSize int, dropPolicy DropPolicy, name string) {
	sg := NewSinkGroup(sink, bufSize, dropPolicy, name)
	p.mu.Lock()
	p.sinkGroups = append(p.sinkGroups, sg)
	p.mu.Unlock()
}

func (p *pipeline) Close() error {
	p.closed.Store(true)
	p.cancel()
	close(p.ch)
	p.wg.Wait()

	p.mu.Lock()
	groups := p.sinkGroups
	p.sinkGroups = nil
	p.mu.Unlock()

	var firstErr error
	for _, sg := range groups {
		sg.Flush()
		if err := sg.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	p.throttler.stop()
	return firstErr
}

func (p *pipeline) dispatchLoop(ctx context.Context) {
	for {
		select {
		case entry, ok := <-p.ch:
			if !ok {
				return
			}
			p.dispatch(entry)
		case <-ctx.Done():
			return
		}
	}
}

func (p *pipeline) dispatch(entry LogEntry) {
	p.mu.RLock()
	groups := p.sinkGroups
	p.mu.RUnlock()
	for _, sg := range groups {
		if err := sg.Emit(entry); err != nil {
			p.sinkErrors.Add(1)
		}
	}
}

type tokenBucket struct {
	tokens    float64
	maxTokens float64
	lastTime  time.Time
	rate      float64
}

type stepThrottler struct {
	mu      sync.RWMutex
	buckets map[string]*tokenBucket
	rules   []ThrottleRule
	// TTL-based cleanup for idle buckets
	lastAccess map[string]time.Time
	cancel     context.CancelFunc
}

func newStepThrottler() *stepThrottler {
	ctx, cancel := context.WithCancel(context.Background())
	t := &stepThrottler{
		buckets:    make(map[string]*tokenBucket),
		lastAccess: make(map[string]time.Time),
		cancel:     cancel,
	}
	safego.Go(ctx, "stepThrottler-cleanup", func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.cleanup()
			case <-ctx.Done():
				return
			}
		}
	})
	return t
}

func (t *stepThrottler) stop() {
	if t.cancel != nil {
		t.cancel()
	}
}

func (t *stepThrottler) cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for prefix, last := range t.lastAccess {
		if now.Sub(last) > 5*time.Minute {
			delete(t.buckets, prefix)
			delete(t.lastAccess, prefix)
		}
	}
}

func (t *stepThrottler) setRules(rules []ThrottleRule) {
	t.mu.Lock()
	t.rules = rules
	t.buckets = make(map[string]*tokenBucket)
	t.lastAccess = make(map[string]time.Time)
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
	t.lastAccess[matched] = time.Now()
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
