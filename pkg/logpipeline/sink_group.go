package logpipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"aranea-agents/pkg/safego"
)

// DropPolicy controls how a SinkGroup handles a full channel buffer.
type DropPolicy int

const (
	DropNewest DropPolicy = iota // Drop the newest entry when buffer is full (default)
	DropBlock                    // Block the caller until buffer has space
)

// SinkGroup wraps a Sink with an independent goroutine and channel buffer.
// A slow Sink in one SinkGroup does not affect other SinkGroups.
type SinkGroup struct {
	sink       Sink
	ch         chan LogEntry
	wg         sync.WaitGroup
	dropped    atomic.Uint64
	dropPolicy DropPolicy
	name       string
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewSinkGroup creates a SinkGroup with the given Sink, buffer size, and drop policy.
func NewSinkGroup(sink Sink, bufSize int, dropPolicy DropPolicy, name string) *SinkGroup {
	if bufSize <= 0 {
		bufSize = 4096
	}
	ctx, cancel := context.WithCancel(context.Background())
	sg := &SinkGroup{
		sink:       sink,
		ch:         make(chan LogEntry, bufSize),
		dropPolicy: dropPolicy,
		name:       name,
		ctx:        ctx,
		cancel:     cancel,
	}
	sg.wg.Add(1)
	safego.Go(ctx, "sinkgroup-"+name, func() {
		defer sg.wg.Done()
		sg.run()
	})
	return sg
}

// run is the main loop that reads entries from the channel and writes to the Sink.
func (sg *SinkGroup) run() {
	for {
		entry, ok := <-sg.ch
		if !ok {
			return
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					// Sink.Write panicked; continue processing.
				}
			}()
			sg.sink.Write(entry)
		}()
	}
}

// Emit sends a LogEntry to the SinkGroup's channel.
// It returns nil on success or an error if the entry was dropped.
func (sg *SinkGroup) Emit(entry LogEntry) error {
	if sg.dropPolicy == DropBlock {
		select {
		case sg.ch <- entry:
			return nil
		case <-sg.ctx.Done():
			sg.dropped.Add(1)
			return errors.New("sinkgroup closed")
		}
	}
	// DropNewest policy
	select {
	case sg.ch <- entry:
		return nil
	default:
		sg.dropped.Add(1)
		return errors.New("sinkgroup buffer full, entry dropped")
	}
}

// Close stops the SinkGroup goroutine, drains remaining entries, and closes the Sink.
func (sg *SinkGroup) Close() error {
	sg.cancel()
	close(sg.ch)
	sg.wg.Wait()
	return sg.sink.Close()
}

// Flush flushes the underlying Sink.
func (sg *SinkGroup) Flush() {
	sg.sink.Flush()
}

// Stats returns per-SinkGroup statistics.
type SinkGroupStats struct {
	Name    string
	Dropped uint64
	ChanLen int
	ChanCap int
}

// Stats returns current statistics for this SinkGroup.
func (sg *SinkGroup) Stats() SinkGroupStats {
	return SinkGroupStats{
		Name:    sg.name,
		Dropped: sg.dropped.Load(),
		ChanLen: len(sg.ch),
		ChanCap: cap(sg.ch),
	}
}
