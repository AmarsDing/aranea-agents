package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	sessionsess "aranea-agents/internal/biz/session"
	"aranea-agents/internal/conf"
	memtrpc "aranea-agents/internal/memory/trpc"
	"aranea-agents/pkg/loggateway"
)

// fakeDeadLetterSinkP203 is a test double for biz.MemoryDeadLetterSink.
type fakeDeadLetterSinkP203 struct {
	mu      sync.Mutex
	entries []fakeDeadLetterEntryP203
}

type fakeDeadLetterEntryP203 struct {
	Request biz.MemoryDeadLetterRequest
	Reason  biz.MemoryDeadLetterReason
	LastErr string
}

func (f *fakeDeadLetterSinkP203) WriteMemoryDeadLetter(r biz.MemoryDeadLetterRequest, reason biz.MemoryDeadLetterReason, lastErr string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, fakeDeadLetterEntryP203{Request: r, Reason: reason, LastErr: lastErr})
}

func (f *fakeDeadLetterSinkP203) Entries() []fakeDeadLetterEntryP203 {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]fakeDeadLetterEntryP203, len(f.entries))
	copy(cp, f.entries)
	return cp
}

// errorConsolidator is a MemoryConsolidator that always fails.
type errorConsolidator struct {
	err error
}

func (c *errorConsolidator) Extract(_ context.Context, _ biz.ConsolidateInput) ([]biz.MemoryProposal, error) {
	return nil, c.err
}

// noopConsolidationWriter is a minimal writer that does nothing (for P2-03 tests
// we only care about the consolidator error path, not write results).
type noopConsolidationWriter struct{}

func (noopConsolidationWriter) UpsertFactsAndEpisodeBatch(_ context.Context, _ []biz.MemoryFactWrite, _ *biz.EpisodeWrite) (*biz.ConsolidationResult, error) {
	return &biz.ConsolidationResult{}, nil
}

// buildRetryExhaustedWorker creates an AutoMemoryWorker wired with sessions,
// agents, and a failing consolidator so that retry exhaustion is triggered.
func buildRetryExhaustedWorker(t *testing.T, rc *conf.Runtime, sink biz.MemoryDeadLetterSink) *AutoMemoryWorker {
	t.Helper()
	const (
		sessID  = "sess-retry-exhaust"
		agentID = "agent-retry"
		userID  = "user-retry"
	)
	repo := fixedSessionRepo{
		sess: sessionsess.Session{ID: sessID, AgentID: agentID, UserID: userID},
		msgs: []sessionsess.ChatMessage{{
			ID: "msg-1", SessionID: sessID, Role: "user", ContentMarkdown: "hello",
		}},
	}
	sessionsUC := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, repo, loggateway.NewNoop(), nil)
	agentsUC := newMemoryEnabledAgentsUC(agentID)
	q := memtrpc.NewMemoryJobQueue(&conf.Runtime{}, 4, 0, loggateway.NewNoop())
	t.Cleanup(q.Close)
	w, err := NewAutoMemoryWorker(AutoMemoryWorkerConfig{
		RuntimeConf:    rc,
		Interval:       0,
		Sessions:       sessionsUC,
		Agents:         agentsUC,
		Writer:         noopConsolidationWriter{},
		Consolidator:   &errorConsolidator{err: errors.New("LLM extraction failed")},
		Queue:          q,
		DeadLetterSink: sink,
		Stats:          biz.NewMemoryWorkerStats(),
		Logger:         loggateway.NewNoop(),
	})
	if err != nil {
		t.Fatalf("NewAutoMemoryWorker: %v", err)
	}
	return w
}

// TestAutoMemoryWorker_RetryExhaustedWritesDeadLetter verifies that when
// retries are exhausted, the job is persisted to the dead-letter store
// with reason "retry_exhausted" (P2-03).
func TestAutoMemoryWorker_RetryExhaustedWritesDeadLetter(t *testing.T) {
	sink := &fakeDeadLetterSinkP203{}
	rc := &conf.Runtime{
		AutoMemory: &conf.Runtime_AutoMemory{
			MaxRetries: 1,
		},
	}
	w := buildRetryExhaustedWorker(t, rc, sink)

	req := memtrpc.AutoMemoryJobRequest{
		SessionID: "sess-retry-exhaust",
		AppName:   "agent-retry",
		Priority:  memtrpc.MemoryJobPriorityHigh,
	}
	// Call processWithRetry directly (it has its own defer AckDone).
	w.processWithRetry(context.Background(), req)

	entries := sink.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 dead-letter entry, got %d", len(entries))
	}
	if entries[0].Reason != biz.MemoryDeadLetterReasonRetryExhausted {
		t.Errorf("expected reason %q, got %q", biz.MemoryDeadLetterReasonRetryExhausted, entries[0].Reason)
	}
	if entries[0].Request.SessionID != "sess-retry-exhaust" {
		t.Errorf("expected session_id %q, got %q", "sess-retry-exhaust", entries[0].Request.SessionID)
	}
	if entries[0].LastErr == "" {
		t.Error("expected non-empty last_err")
	}
}

// TestAutoMemoryWorker_RetryExhaustedNoSink verifies that when no
// dead-letter sink is wired, retry exhaustion still works (no panic)
// and the job is only logged/metered (legacy behavior).
func TestAutoMemoryWorker_RetryExhaustedNoSink(t *testing.T) {
	rc := &conf.Runtime{
		AutoMemory: &conf.Runtime_AutoMemory{
			MaxRetries: 1,
		},
	}
	w := buildRetryExhaustedWorker(t, rc, nil)

	req := memtrpc.AutoMemoryJobRequest{
		SessionID: "sess-retry-exhaust",
		AppName:   "agent-retry",
		Priority:  memtrpc.MemoryJobPriorityHigh,
	}
	// Should not panic even without sink.
	w.processWithRetry(context.Background(), req)
}
