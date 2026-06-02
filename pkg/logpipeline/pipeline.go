package logpipeline

import (
	"context"
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
}

type pipeline struct {
	mu      sync.RWMutex
	sinks   []Sink
	ch      chan LogEntry
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	dropped atomic.Uint64
}

const DefaultBufSize = 4096

func NewPipeline(bufSize int) Pipeline {
	if bufSize <= 0 {
		bufSize = DefaultBufSize
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &pipeline{
		ch:     make(chan LogEntry, bufSize),
		cancel: cancel,
	}
	p.wg.Add(1)
	safego.Go(ctx, "logpipeline-dispatcher", func() {
		defer p.wg.Done()
		p.dispatchLoop(ctx)
	})
	return p
}

func (p *pipeline) Emit(entry LogEntry) {
	select {
	case p.ch <- entry:
	default:
		p.dropped.Add(1)
	}
}

func (p *pipeline) Dropped() uint64 {
	return p.dropped.Load()
}

func (p *pipeline) AddSink(sink Sink) {
	p.mu.Lock()
	p.sinks = append(p.sinks, sink)
	p.mu.Unlock()
}

func (p *pipeline) Close() error {
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
		select {
		case entry, ok := <-p.ch:
			if !ok {
				return
			}
			p.dispatch(entry)
		case <-ctx.Done():
			p.drain()
			return
		}
	}
}

func (p *pipeline) dispatch(entry LogEntry) {
	p.mu.RLock()
	sinks := p.sinks
	p.mu.RUnlock()
	for _, s := range sinks {
		func() {
			defer func() { recover() }()
			s.Write(entry)
		}()
	}
}

func (p *pipeline) drain() {
	for {
		select {
		case entry, ok := <-p.ch:
			if !ok {
				return
			}
			p.dispatch(entry)
		default:
			return
		}
	}
}
