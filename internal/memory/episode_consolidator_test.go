package memory

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

// --- Fakes ---

// fakeL2RecallStore implements biz.L2RecallStore for testing.
type fakeL2RecallStore struct {
	mu       sync.Mutex
	rows     [][]byte
	readErr  error
	readCall int
}

func (f *fakeL2RecallStore) ListEpisodeRowsForRecall(_ context.Context, _ string, _ string, _ int32) ([][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readCall++
	if f.readErr != nil {
		return nil, f.readErr
	}
	out := make([][]byte, len(f.rows))
	copy(out, f.rows)
	return out, nil
}

func (f *fakeL2RecallStore) RecallL2Episodes(_ context.Context, _ string, _ string, _ string, _ []float32, _ int32) ([][]byte, error) {
	return nil, nil
}

// fakeL3FactWriter implements biz.L3FactWriter for testing.
type fakeL3FactWriter struct {
	mu         sync.Mutex
	upserted   []biz.FactUpsert
	upsertErr  error
	deleteErr  error
	clearErr   error
	invalErr   error
	invalTxErr error
}

func (f *fakeL3FactWriter) UpsertFactRow(_ context.Context, in biz.FactUpsert) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	f.upserted = append(f.upserted, in)
	return []byte(`{}`), nil
}

func (f *fakeL3FactWriter) DeleteFactRow(_ context.Context, _ string) error { return f.deleteErr }

func (f *fakeL3FactWriter) DeleteFactRowsByIDs(_ context.Context, _ []string) (int, error) {
	return 0, nil
}

func (f *fakeL3FactWriter) ClearFactsByScope(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, f.clearErr
}

func (f *fakeL3FactWriter) InvalidateFact(_ context.Context, _ string) ([]byte, error) {
	return nil, f.invalErr
}

func (f *fakeL3FactWriter) InvalidateAndUpsertFactTx(_ context.Context, _ string, _ biz.FactUpsert) ([]byte, error) {
	return nil, f.invalTxErr
}

// fakeActionLogWriter implements biz.MemoryActionLogWriter for testing.
type fakeActionLogWriter struct {
	mu       sync.Mutex
	records  []biz.MemoryPolicyRecord
	writeErr error
}

func (f *fakeActionLogWriter) WriteMemoryActionLog(_ context.Context, rec biz.MemoryPolicyRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeErr != nil {
		return f.writeErr
	}
	f.records = append(f.records, rec)
	return nil
}

// --- Helpers ---

func makeEpisodeJSON(id, title, outcome string, importance float64) []byte {
	return []byte(`{"id":"` + id + `","title":"` + title + `","outcome_summary":"` + outcome + `","importance":` + floatToStr(importance) + `}`)
}

// floatToStr is a minimal float-to-string helper for test JSON.
func floatToStr(f float64) string {
	if f == 0 {
		return "0"
	}
	if f == 0.8 {
		return "0.8"
	}
	if f == 0.5 {
		return "0.5"
	}
	return "0.9"
}

// --- Tests ---

// TestEpisodeConsolidator_NilLLM verifies that nil LLM is a no-op.
func TestEpisodeConsolidator_NilLLM(t *testing.T) {
	c := NewEpisodeConsolidator(
		&fakeL2RecallStore{rows: [][]byte{makeEpisodeJSON("ep-1", "Task completed", "done", 0.8)}},
		&fakeL3FactWriter{},
		&fakeActionLogWriter{},
		nil,
		loggateway.NewNoop(),
	)
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// TestEpisodeConsolidator_NilDeps verifies that nil episodeReader/factWriter is a no-op.
func TestEpisodeConsolidator_NilDeps(t *testing.T) {
	c := NewEpisodeConsolidator(nil, nil, nil, &fakeModel{}, loggateway.NewNoop())
	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

// TestEpisodeConsolidator_EmptyEpisodes verifies that no episodes is a no-op.
func TestEpisodeConsolidator_EmptyEpisodes(t *testing.T) {
	reader := &fakeL2RecallStore{rows: nil}
	writer := &fakeL3FactWriter{}
	c := NewEpisodeConsolidator(reader, writer, &fakeActionLogWriter{}, &fakeModel{}, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := len(writer.upserted); got != 0 {
		t.Errorf("expected 0 facts upserted, got %d", got)
	}
}

// TestEpisodeConsolidator_ReadFailure verifies that read failure is graceful (returns nil).
func TestEpisodeConsolidator_ReadFailure(t *testing.T) {
	reader := &fakeL2RecallStore{readErr: errors.New("database unavailable")}
	writer := &fakeL3FactWriter{}
	c := NewEpisodeConsolidator(reader, writer, &fakeActionLogWriter{}, &fakeModel{}, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error on read failure (graceful), got %v", err)
	}
	if got := len(writer.upserted); got != 0 {
		t.Errorf("expected 0 facts on read failure, got %d", got)
	}
}

// TestEpisodeConsolidator_LLMFailure verifies that LLM failure is graceful.
func TestEpisodeConsolidator_LLMFailure(t *testing.T) {
	reader := &fakeL2RecallStore{rows: [][]byte{makeEpisodeJSON("ep-1", "Task A", "done", 0.8)}}
	writer := &fakeL3FactWriter{}
	llm := &fakeModel{err: errors.New("LLM unavailable")}
	c := NewEpisodeConsolidator(reader, writer, &fakeActionLogWriter{}, llm, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error on LLM failure, got %v", err)
	}
	if got := len(writer.upserted); got != 0 {
		t.Errorf("expected 0 facts on LLM failure, got %d", got)
	}
}

// TestEpisodeConsolidator_MalformedJSON verifies that malformed LLM output is graceful.
func TestEpisodeConsolidator_MalformedJSON(t *testing.T) {
	reader := &fakeL2RecallStore{rows: [][]byte{makeEpisodeJSON("ep-1", "Task A", "done", 0.8)}}
	writer := &fakeL3FactWriter{}
	llm := &fakeModel{response: buildLLMResponse("not valid json")}
	c := NewEpisodeConsolidator(reader, writer, &fakeActionLogWriter{}, llm, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error on malformed JSON, got %v", err)
	}
	if got := len(writer.upserted); got != 0 {
		t.Errorf("expected 0 facts on malformed JSON, got %d", got)
	}
}

// TestEpisodeConsolidator_Success verifies the happy path: LLM extracts facts,
// they are persisted, and action_log entries are written.
func TestEpisodeConsolidator_Success(t *testing.T) {
	reader := &fakeL2RecallStore{rows: [][]byte{
		makeEpisodeJSON("ep-1", "User preference for Go", "discovered", 0.8),
		makeEpisodeJSON("ep-2", "Project uses SQLite", "documented", 0.5),
	}}
	writer := &fakeL3FactWriter{}
	logWriter := &fakeActionLogWriter{}
	llmResp := buildLLMResponse(`{"facts":[
		{"statement":"User prefers Go for backend development","importance":0.9,"confidence":0.9,"source_episode_id":"ep-1","reason":"durable preference"},
		{"statement":"Project database is SQLite","importance":0.5,"confidence":0.8,"source_episode_id":"ep-2","reason":"project attribute"}
	]}`)
	llm := &fakeModel{response: llmResp}
	c := NewEpisodeConsolidator(reader, writer, logWriter, llm, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := len(writer.upserted); got != 2 {
		t.Fatalf("expected 2 facts upserted, got %d", got)
	}
	// Verify first fact.
	f1 := writer.upserted[0]
	if f1.Statement != "User prefers Go for backend development" {
		t.Errorf("unexpected statement: %q", f1.Statement)
	}
	if f1.SourceEpisodeID != "ep-1" {
		t.Errorf("expected source_episode_id=ep-1, got %q", f1.SourceEpisodeID)
	}
	if f1.Importance != 0.9 {
		t.Errorf("expected importance=0.9, got %f", f1.Importance)
	}
	// Verify action_log entries.
	if got := len(logWriter.records); got != 2 {
		t.Fatalf("expected 2 action_log entries, got %d", got)
	}
	if logWriter.records[0].Action != "episode_extract_fact" {
		t.Errorf("unexpected action: %q", logWriter.records[0].Action)
	}
}

// TestEpisodeConsolidator_LowImportanceSkipped verifies that facts with
// importance < 0.3 (default threshold) are skipped.
func TestEpisodeConsolidator_LowImportanceSkipped(t *testing.T) {
	reader := &fakeL2RecallStore{rows: [][]byte{
		makeEpisodeJSON("ep-1", "Minor task", "done", 0.2),
	}}
	writer := &fakeL3FactWriter{}
	llmResp := buildLLMResponse(`{"facts":[
		{"statement":"Low value fact","importance":0.1,"confidence":0.9,"source_episode_id":"ep-1","reason":"test"},
		{"statement":"Valuable fact","importance":0.8,"confidence":0.9,"source_episode_id":"ep-1","reason":"test"}
	]}`)
	llm := &fakeModel{response: llmResp}
	c := NewEpisodeConsolidator(reader, writer, &fakeActionLogWriter{}, llm, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := len(writer.upserted); got != 1 {
		t.Fatalf("expected 1 fact (low importance skipped), got %d", got)
	}
	if writer.upserted[0].Statement != "Valuable fact" {
		t.Errorf("expected only high-importance fact, got %q", writer.upserted[0].Statement)
	}
}

// TestEpisodeConsolidator_CustomMinImportance verifies that SetMinImportance
// changes the threshold (Phase 6A-06 T8).
func TestEpisodeConsolidator_CustomMinImportance(t *testing.T) {
	reader := &fakeL2RecallStore{rows: [][]byte{
		makeEpisodeJSON("ep-1", "Task A", "done", 0.5),
	}}
	writer := &fakeL3FactWriter{}
	llmResp := buildLLMResponse(`{"facts":[
		{"statement":"Medium fact","importance":0.4,"confidence":0.9,"source_episode_id":"ep-1","reason":"test"},
		{"statement":"High fact","importance":0.8,"confidence":0.9,"source_episode_id":"ep-1","reason":"test"}
	]}`)
	llm := &fakeModel{response: llmResp}
	c := NewEpisodeConsolidator(reader, writer, &fakeActionLogWriter{}, llm, loggateway.NewNoop())
	c.SetMinImportance(0.5) // raise threshold from 0.3 to 0.5

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// 0.4 < 0.5 → skipped; 0.8 >= 0.5 → kept
	if got := len(writer.upserted); got != 1 {
		t.Fatalf("expected 1 fact (0.4 skipped by custom threshold), got %d", got)
	}
	if writer.upserted[0].Statement != "High fact" {
		t.Errorf("expected only high-importance fact, got %q", writer.upserted[0].Statement)
	}
}

// TestEpisodeConsolidator_DisableImportanceFilter verifies that
// SetMinImportance(0) disables the filter entirely (Phase 6A-06 T8).
func TestEpisodeConsolidator_DisableImportanceFilter(t *testing.T) {
	reader := &fakeL2RecallStore{rows: [][]byte{
		makeEpisodeJSON("ep-1", "Task A", "done", 0.8),
	}}
	writer := &fakeL3FactWriter{}
	llmResp := buildLLMResponse(`{"facts":[
		{"statement":"Low fact","importance":0.01,"confidence":0.9,"source_episode_id":"ep-1","reason":"test"},
		{"statement":"High fact","importance":0.8,"confidence":0.9,"source_episode_id":"ep-1","reason":"test"}
	]}`)
	llm := &fakeModel{response: llmResp}
	c := NewEpisodeConsolidator(reader, writer, &fakeActionLogWriter{}, llm, loggateway.NewNoop())
	c.SetMinImportance(0) // disable filter

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := len(writer.upserted); got != 2 {
		t.Fatalf("expected 2 facts (filter disabled), got %d", got)
	}
}

// TestEpisodeConsolidator_EmptyFacts verifies that an empty facts array from
// the LLM is a valid no-op.
func TestEpisodeConsolidator_EmptyFacts(t *testing.T) {
	reader := &fakeL2RecallStore{rows: [][]byte{
		makeEpisodeJSON("ep-1", "Task A", "done", 0.8),
	}}
	writer := &fakeL3FactWriter{}
	llm := &fakeModel{response: buildLLMResponse(`{"facts":[]}`)}
	c := NewEpisodeConsolidator(reader, writer, &fakeActionLogWriter{}, llm, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := len(writer.upserted); got != 0 {
		t.Errorf("expected 0 facts, got %d", got)
	}
}

// TestEpisodeConsolidator_NilActionLogWriter verifies that a nil
// actionLogWriter does not cause panics.
func TestEpisodeConsolidator_NilActionLogWriter(t *testing.T) {
	reader := &fakeL2RecallStore{rows: [][]byte{
		makeEpisodeJSON("ep-1", "Task A", "done", 0.8),
	}}
	writer := &fakeL3FactWriter{}
	llmResp := buildLLMResponse(`{"facts":[
		{"statement":"Some fact","importance":0.8,"confidence":0.9,"source_episode_id":"ep-1","reason":"test"}
	]}`)
	llm := &fakeModel{response: llmResp}
	c := NewEpisodeConsolidator(reader, writer, nil, llm, loggateway.NewNoop())

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := c.ConsolidateEpisodes(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got := len(writer.upserted); got != 1 {
		t.Errorf("expected 1 fact, got %d", got)
	}
}

// TestSleepTime_WithEpisodeConsolidator verifies that SleepTimeService calls
// EpisodeConsolidator when wired, even when trpcmemory phase has no work.
func TestSleepTime_WithEpisodeConsolidator(t *testing.T) {
	// trpcmemory phase: no memories (empty).
	ms := &fakeMemoryService{
		entries: map[string][]*trpcmemory.Entry{
			"user-1": {},
		},
	}
	// Episode phase: 1 episode, 1 fact extracted.
	reader := &fakeL2RecallStore{rows: [][]byte{
		makeEpisodeJSON("ep-1", "Task A", "done", 0.8),
	}}
	factWriter := &fakeL3FactWriter{}
	llmResp := buildLLMResponse(`{"facts":[
		{"statement":"Durable fact","importance":0.8,"confidence":0.9,"source_episode_id":"ep-1","reason":"test"}
	]}`)
	llm := &fakeModel{response: llmResp}
	ec := NewEpisodeConsolidator(reader, factWriter, &fakeActionLogWriter{}, llm, loggateway.NewNoop())

	svc := NewSleepTimeService(ms, nil, nil, loggateway.NewNoop())
	svc.SetEpisodeConsolidator(ec)

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	if err := svc.Consolidate(context.Background(), uk); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// trpcmemory phase: no mutations (empty memories).
	if len(ms.added) != 0 || len(ms.updated) != 0 || len(ms.deleted) != 0 {
		t.Errorf("expected no trpcmemory mutations")
	}
	// Episode phase: 1 fact extracted.
	if got := len(factWriter.upserted); got != 1 {
		t.Fatalf("expected 1 fact from episode consolidation, got %d", got)
	}
}

// TestSleepTime_ReadFailureSkipsEpisodePhase verifies that when the trpcmemory
// read fails (retryable error), the episode phase is skipped to prevent
// duplicate facts on retry.
func TestSleepTime_ReadFailureSkipsEpisodePhase(t *testing.T) {
	ms := &fakeMemoryService{readErr: errors.New("database unavailable")}
	reader := &fakeL2RecallStore{rows: [][]byte{
		makeEpisodeJSON("ep-1", "Task A", "done", 0.8),
	}}
	factWriter := &fakeL3FactWriter{}
	ec := NewEpisodeConsolidator(reader, factWriter, &fakeActionLogWriter{}, &fakeModel{response: buildLLMResponse(`{"facts":[]}`)}, loggateway.NewNoop())

	svc := NewSleepTimeService(ms, nil, nil, loggateway.NewNoop())
	svc.SetEpisodeConsolidator(ec)

	uk := trpcmemory.UserKey{AppName: "agent-1", UserID: "user-1"}
	// Read failure should return an error (retryable).
	if err := svc.Consolidate(context.Background(), uk); err == nil {
		t.Fatal("expected error on read failure, got nil")
	}
	// Episode phase should be skipped (0 facts).
	if got := len(factWriter.upserted); got != 0 {
		t.Errorf("expected 0 facts (episode phase skipped on read failure), got %d", got)
	}
}
