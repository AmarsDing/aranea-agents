package logpipeline

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockSink struct {
	mu           sync.Mutex
	entries      []LogEntry
	writeCh      chan LogEntry
	panicOnWrite bool
	flushCount   int
	closeErr     error
}

func newMockSink() *mockSink {
	return &mockSink{
		writeCh: make(chan LogEntry, 256),
	}
}

func (s *mockSink) Write(entry LogEntry) {
	if s.panicOnWrite {
		panic("mock panic")
	}
	s.mu.Lock()
	s.entries = append(s.entries, entry)
	s.mu.Unlock()
	select {
	case s.writeCh <- entry:
	default:
	}
}

func (s *mockSink) Flush() {
	s.mu.Lock()
	s.flushCount++
	s.mu.Unlock()
}

func (s *mockSink) Close() error {
	return s.closeErr
}

func (s *mockSink) Entries() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]LogEntry, len(s.entries))
	copy(cp, s.entries)
	return cp
}

func (s *mockSink) WaitForWrite(timeout time.Duration) bool {
	select {
	case <-s.writeCh:
		return true
	case <-time.After(timeout):
		return false
	}
}

type blockingSink struct {
	unblock     chan struct{}
	writeCalled chan struct{}
	stopBlock   chan struct{}
}

func newBlockingSink() *blockingSink {
	return &blockingSink{
		unblock:     make(chan struct{}),
		writeCalled: make(chan struct{}, 256),
		stopBlock:   make(chan struct{}),
	}
}

func (s *blockingSink) Write(entry LogEntry) {
	s.writeCalled <- struct{}{}
	select {
	case <-s.unblock:
	case <-s.stopBlock:
	}
}

func (s *blockingSink) Flush() {}

func (s *blockingSink) Close() error { return nil }

func (s *blockingSink) AllowNext() {
	s.unblock <- struct{}{}
}

func (s *blockingSink) StopBlocking() {
	close(s.stopBlock)
}

func makeEntry(msg string) LogEntry {
	return LogEntry{
		Kind:      KindLog,
		Level:     "info",
		Message:   msg,
		Timestamp: time.Now(),
	}
}

func TestNewPipeline_DefaultBufSize(t *testing.T) {
	p := NewPipeline(0)
	defer p.Close()

	sink := newMockSink()
	p.AddSink(sink)

	p.Emit(makeEntry("default-buf"))
	if !sink.WaitForWrite(2 * time.Second) {
		t.Fatal("sink did not receive entry within timeout")
	}
}

func TestNewPipeline_CustomBufSize(t *testing.T) {
	p := NewPipeline(4)
	defer p.Close()

	sink := newMockSink()
	p.AddSink(sink)

	p.Emit(makeEntry("custom-buf"))
	if !sink.WaitForWrite(2 * time.Second) {
		t.Fatal("sink did not receive entry within timeout")
	}
}

func TestEmit_Dispatch(t *testing.T) {
	p := NewPipeline(64)
	defer p.Close()

	sink := newMockSink()
	p.AddSink(sink)

	entry := LogEntry{
		Kind:      KindFlow,
		Level:     "warn",
		Message:   "dispatch-test",
		SessionID: "sess-1",
		Fields:    map[string]any{"key": "val"},
		Timestamp: time.Now(),
	}
	p.Emit(entry)

	if !sink.WaitForWrite(2 * time.Second) {
		t.Fatal("sink did not receive entry within timeout")
	}

	entries := sink.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	got := entries[0]
	if got.Kind != entry.Kind || got.Level != entry.Level || got.Message != entry.Message || got.SessionID != entry.SessionID {
		t.Fatalf("entry mismatch: got Kind=%s Level=%s Message=%s SessionID=%s", got.Kind, got.Level, got.Message, got.SessionID)
	}
	if got.Fields["key"] != "val" {
		t.Fatalf("expected Fields[key]=val, got %v", got.Fields["key"])
	}
}

func TestEmit_NonBlocking(t *testing.T) {
	p := NewPipeline(1)

	bs := newBlockingSink()
	p.AddSink(bs)

	p.Emit(makeEntry("fill-1"))

	select {
	case <-bs.writeCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking sink never received first entry")
	}

	p.Emit(makeEntry("fill-2"))

	p.Emit(makeEntry("should-drop"))

	if dropped := p.Dropped(); dropped == 0 {
		t.Fatal("expected dropped > 0 when channel is full")
	}

	bs.StopBlocking()

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestAddSink(t *testing.T) {
	p := NewPipeline(64)
	defer p.Close()

	sink1 := newMockSink()
	p.AddSink(sink1)

	p.Emit(makeEntry("before-add"))
	if !sink1.WaitForWrite(2 * time.Second) {
		t.Fatal("sink1 did not receive entry")
	}

	sink2 := newMockSink()
	p.AddSink(sink2)

	p.Emit(makeEntry("after-add"))
	if !sink1.WaitForWrite(2 * time.Second) {
		t.Fatal("sink1 did not receive second entry")
	}
	if !sink2.WaitForWrite(2 * time.Second) {
		t.Fatal("sink2 did not receive entry after AddSink")
	}

	entries1 := sink1.Entries()
	entries2 := sink2.Entries()
	if len(entries1) != 2 {
		t.Fatalf("sink1 expected 2 entries, got %d", len(entries1))
	}
	if len(entries2) != 1 {
		t.Fatalf("sink2 expected 1 entry, got %d", len(entries2))
	}
}

func TestClose_DrainRemaining(t *testing.T) {
	p := NewPipeline(256)

	sink := newMockSink()
	p.AddSink(sink)

	const n = 20
	for i := 0; i < n; i++ {
		p.Emit(makeEntry("drain-entry"))
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	entries := sink.Entries()
	if len(entries) != n {
		t.Fatalf("expected %d entries after Close, got %d", n, len(entries))
	}
}

func TestDropped(t *testing.T) {
	p := NewPipeline(1)

	bs := newBlockingSink()
	p.AddSink(bs)

	p.Emit(makeEntry("block-sink"))

	select {
	case <-bs.writeCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("blocking sink never received entry")
	}

	const extra = 5
	for i := 0; i < extra; i++ {
		p.Emit(makeEntry("overflow"))
	}

	dropped := p.Dropped()
	if dropped == 0 {
		t.Fatal("expected some entries to be dropped")
	}

	bs.StopBlocking()

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestConcurrentEmit(t *testing.T) {
	p := NewPipeline(4096)
	defer p.Close()

	sink := newMockSink()
	p.AddSink(sink)

	const goroutines = 10
	const perGoroutine = 100
	var wg sync.WaitGroup
	var emitted atomic.Int64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				p.Emit(makeEntry("concurrent"))
				emitted.Add(1)
			}
		}()
	}
	wg.Wait()

	time.Sleep(500 * time.Millisecond)

	entries := sink.Entries()
	total := uint64(emitted.Load())
	dropped := p.Dropped()
	received := uint64(len(entries))

	if received+dropped != total {
		t.Fatalf("received(%d) + dropped(%d) != emitted(%d)", received, dropped, total)
	}
}

func TestSinkPanicIsolation(t *testing.T) {
	p := NewPipeline(64)
	defer p.Close()

	panicSink := &mockSink{panicOnWrite: true, writeCh: make(chan LogEntry, 256)}
	normalSink := newMockSink()
	p.AddSink(panicSink)
	p.AddSink(normalSink)

	p.Emit(makeEntry("panic-test"))

	if !normalSink.WaitForWrite(2 * time.Second) {
		t.Fatal("normal sink did not receive entry after panic sink")
	}

	entries := normalSink.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry in normal sink, got %d", len(entries))
	}
}
