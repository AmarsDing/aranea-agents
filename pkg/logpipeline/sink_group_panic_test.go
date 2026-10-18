package logpipeline

import (
	"sync/atomic"
	"testing"
	"time"
)

// panicOnceSink panics on the first Write, records subsequent writes.
type panicOnceSink struct {
	writes   atomic.Int32
	panicked atomic.Bool
}

func (s *panicOnceSink) Write(LogEntry) {
	if s.panicked.CompareAndSwap(false, true) {
		panic("intentional test panic")
	}
	s.writes.Add(1)
}
func (s *panicOnceSink) Flush()      {}
func (s *panicOnceSink) Close() error { return nil }

// TestSinkGroup_PanicCountedAndGroupSurvives pins the 2026-10-18 audit P1-1 fix:
// a panicking Sink.Write must be counted (visible via Stats) and must NOT kill
// the SinkGroup goroutine — subsequent entries still flow through.
func TestSinkGroup_PanicCountedAndGroupSurvives(t *testing.T) {
	sink := &panicOnceSink{}
	sg := NewSinkGroup(sink, 16, DropNewest, "panic-test")
	defer sg.Close()

	for i := 0; i < 3; i++ {
		if err := sg.Emit(LogEntry{Message: "m", Timestamp: time.Now()}); err != nil {
			t.Fatalf("Emit failed: %v", err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := sg.Stats()
		if st.Panics == 1 && sink.writes.Load() == 2 {
			return // panic counted, loop survived, later entries written
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected Panics=1 and 2 surviving writes, got panics=%d writes=%d",
		sg.Stats().Panics, sink.writes.Load())
}
