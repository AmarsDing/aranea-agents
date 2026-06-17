package event

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	frameworkwal "trpc.group/trpc-go/trpc-agent-go/event/wal"
)

// capturingLogger records log calls for test assertions.
type capturingLogger struct {
	mu      sync.Mutex
	entries []logEntry
}

type logEntry struct {
	level string
	msg   string
}

func (l *capturingLogger) Debug(msg string, _ ...loggateway.Field) {}
func (l *capturingLogger) Info(msg string, _ ...loggateway.Field)  {}
func (l *capturingLogger) Warn(msg string, _ ...loggateway.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{level: "warn", msg: msg})
}
func (l *capturingLogger) Error(msg string, _ ...loggateway.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{level: "error", msg: msg})
}
func (l *capturingLogger) With(_ ...loggateway.Field) loggateway.Logger { return l }

func (l *capturingLogger) hasMessageContaining(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if contains(e.msg, substr) {
			return true
		}
	}
	return false
}

func (l *capturingLogger) hasLevelMessage(level, substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if e.level == level && contains(e.msg, substr) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// failingWALStorage is a framework wal.Storage implementation that can fail
// on Insert or MarkPublished to simulate WAL failures.
type failingWALStorage struct {
	failInsert        bool
	failMarkPublished bool
	insertCalled      bool
	markCalled        bool
}

func (s *failingWALStorage) Insert(_ context.Context, _ frameworkwal.Entry) error {
	s.insertCalled = true
	if s.failInsert {
		return errors.New("simulated insert failure")
	}
	return nil
}

func (s *failingWALStorage) MarkPublished(_ context.Context, _ string, _ time.Time) error {
	s.markCalled = true
	if s.failMarkPublished {
		return errors.New("simulated mark failure")
	}
	return nil
}

func (s *failingWALStorage) ListUnpublished(_ context.Context) ([]frameworkwal.Entry, error) {
	return nil, nil
}

func (s *failingWALStorage) PurgePublished(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (s *failingWALStorage) Close() error { return nil }

// recordingBus is a contract.Bus that records published envelopes.
type recordingBus struct {
	mu        sync.Mutex
	published []contract.Envelope
}

func (b *recordingBus) Publish(_ context.Context, env contract.Envelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, env)
}

func (b *recordingBus) Subscribe(_ contract.SubscribeOptions) (<-chan contract.Envelope, func()) {
	ch := make(chan contract.Envelope)
	return ch, func() { close(ch) }
}

func (b *recordingBus) DropCount() uint64 { return 0 }

func (b *recordingBus) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.published)
}

// newTestInfraWithWAL builds an Infra with a WAL backed by failingWALStorage.
func newTestInfraWithWAL(t *testing.T, storage *failingWALStorage, lg loggateway.Logger) (*Infra, *recordingBus) {
	t.Helper()
	isCritical := func(env Envelope) bool {
		return contract.IsCriticalWBPFType(env.Type)
	}
	inner, err := frameworkwal.New[Envelope](storage, isCritical, nil)
	if err != nil {
		t.Fatalf("new framework wal: %v", err)
	}
	wal := &EventWAL{inner: inner, lg: lg}

	sessionBus := &recordingBus{}
	return &Infra{
		SessionBus: sessionBus,
		MonitorBus: &recordingBus{},
		WAL:        wal,
		lg:         lg,
		routing:    routingModeSplit,
	}, sessionBus
}

// TestInfraPublish_WALInsertFails_CriticalEventNotPublished verifies that when
// WAL insert fails, the Critical event is NOT published (WBPF: write before publish).
func TestInfraPublish_WALInsertFails_CriticalEventNotPublished(t *testing.T) {
	storage := &failingWALStorage{failInsert: true}
	lg := &capturingLogger{}
	infra, sessionBus := newTestInfraWithWAL(t, storage, lg)

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	infra.Publish(context.Background(), env)

	if sessionBus.count() != 0 {
		t.Errorf("event was published but WAL insert failed; got %d events, want 0", sessionBus.count())
	}
	if !storage.insertCalled {
		t.Error("WAL Insert was not called")
	}
}

// TestInfraPublish_WALInsertFails_LoggedAsDropped verifies that when WAL insert
// fails, the event is logged as "dropped" (Error level).
func TestInfraPublish_WALInsertFails_LoggedAsDropped(t *testing.T) {
	storage := &failingWALStorage{failInsert: true}
	lg := &capturingLogger{}
	infra, _ := newTestInfraWithWAL(t, storage, lg)

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	infra.Publish(context.Background(), env)

	if !lg.hasLevelMessage("error", "dropped") {
		t.Error("expected Error log containing 'dropped' when WAL insert fails")
	}
}

// TestInfraPublish_WALMarkFails_CriticalEventStillPublished verifies that when
// WAL insert succeeds but markPublished fails, the Critical event IS published
// (publish was called inside WriteBeforePublish before mark).
func TestInfraPublish_WALMarkFails_CriticalEventStillPublished(t *testing.T) {
	storage := &failingWALStorage{failMarkPublished: true}
	lg := &capturingLogger{}
	infra, sessionBus := newTestInfraWithWAL(t, storage, lg)

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	infra.Publish(context.Background(), env)

	if sessionBus.count() != 1 {
		t.Errorf("event should be published even when mark fails; got %d events, want 1", sessionBus.count())
	}
	if !storage.markCalled {
		t.Error("WAL MarkPublished was not called")
	}
}

// TestInfraPublish_WALMarkFails_NotLoggedAsDropped verifies that when WAL insert
// succeeds but markPublished fails, the event is NOT logged as "dropped" because
// it was actually published. This is the WBPF semantic violation fix: the event
// was published, so logging "dropped" is misleading.
func TestInfraPublish_WALMarkFails_NotLoggedAsDropped(t *testing.T) {
	storage := &failingWALStorage{failMarkPublished: true}
	lg := &capturingLogger{}
	infra, _ := newTestInfraWithWAL(t, storage, lg)

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	infra.Publish(context.Background(), env)

	if lg.hasMessageContaining("dropped") {
		t.Error("event was published (mark failed after publish) but logged as 'dropped'; " +
			"should log as published-with-mark-failure warning, not dropped")
	}
}

// TestInfraPublish_WALMarkFails_LoggedAsWarning verifies that when WAL insert
// succeeds but markPublished fails, a warning is logged (not error).
func TestInfraPublish_WALMarkFails_LoggedAsWarning(t *testing.T) {
	storage := &failingWALStorage{failMarkPublished: true}
	lg := &capturingLogger{}
	infra, _ := newTestInfraWithWAL(t, storage, lg)

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	infra.Publish(context.Background(), env)

	if !lg.hasLevelMessage("warn", "mark") {
		t.Error("expected Warn log containing 'mark' when markPublished fails after publish")
	}
}

// TestInfraPublish_WALSuccess_CriticalEventPublished verifies that when WAL
// succeeds fully, the Critical event is published normally.
func TestInfraPublish_WALSuccess_CriticalEventPublished(t *testing.T) {
	storage := &failingWALStorage{}
	lg := &capturingLogger{}
	infra, sessionBus := newTestInfraWithWAL(t, storage, lg)

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	infra.Publish(context.Background(), env)

	if sessionBus.count() != 1 {
		t.Errorf("event should be published on WAL success; got %d events, want 1", sessionBus.count())
	}
}

// TestInfraPublish_WALNil_CriticalEventPublished verifies that when WAL is nil,
// events are published directly (no WBPF protection).
func TestInfraPublish_WALNil_CriticalEventPublished(t *testing.T) {
	sessionBus := &recordingBus{}
	infra := &Infra{
		SessionBus: sessionBus,
		MonitorBus: &recordingBus{},
		WAL:        nil,
		lg:         &capturingLogger{},
		routing:    routingModeSplit,
	}

	env := contract.NewEnvelope(contract.EnvelopeTypeToolResult, "agent", "sess-1")
	infra.Publish(context.Background(), env)

	if sessionBus.count() != 1 {
		t.Errorf("event should be published when WAL is nil; got %d events, want 1", sessionBus.count())
	}
}
