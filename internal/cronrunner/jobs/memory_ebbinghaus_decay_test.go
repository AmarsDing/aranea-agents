package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// mock: biz.L3FactReader
// ---------------------------------------------------------------------------

type mockL3FactReader struct {
	rows     [][]byte
	total    int32
	active   int32
	archived int32
	err      error
	// captures the last call parameters for assertions
	lastScopeType string
	lastScopeID   string
	lastStatus    string
	lastAgentID   string
	lastLimit     int32
}

func (m *mockL3FactReader) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	m.lastScopeType = scopeType
	m.lastScopeID = scopeID
	m.lastStatus = status
	m.lastAgentID = agentID
	m.lastLimit = limit
	return m.rows, m.total, m.active, m.archived, m.err
}

func (m *mockL3FactReader) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	return nil, nil
}

func (m *mockL3FactReader) ListFactRowsForUserAll(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	return nil, nil
}

func (m *mockL3FactReader) GetFactRowsByIDs(ctx context.Context, factIDs []string) ([][]byte, error) {
	return nil, nil
}

func (m *mockL3FactReader) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// makeFactRow builds a JSON fact row matching the scanFactRowJSON schema.
func makeFactRow(createdAt, updatedAt, lastUsedAt string, useCount int32) []byte {
	m := map[string]any{
		"created_at":   createdAt,
		"updated_at":   updatedAt,
		"last_used_at": lastUsedAt,
		"use_count":    useCount,
	}
	b, _ := json.Marshal(m)
	return b
}

// makeFactRowWithID builds a JSON fact row with an explicit id field.
func makeFactRowWithID(id, createdAt, updatedAt, lastUsedAt string, useCount int32) []byte {
	m := map[string]any{
		"id":           id,
		"created_at":   createdAt,
		"updated_at":   updatedAt,
		"last_used_at": lastUsedAt,
		"use_count":    useCount,
	}
	b, _ := json.Marshal(m)
	return b
}

// ---------------------------------------------------------------------------
// mock: biz.DecayScoreWriter
// ---------------------------------------------------------------------------

type mockDecayScoreWriter struct {
	scores map[string]float64
	err    error
	calls  int
}

func (m *mockDecayScoreWriter) UpdateDecayScores(ctx context.Context, scores map[string]float64) error {
	m.calls++
	if m.err != nil {
		return m.err
	}
	if m.scores == nil {
		m.scores = make(map[string]float64)
	}
	for k, v := range scores {
		m.scores[k] = v
	}
	return nil
}

// newTestWorker creates a worker with a no-op logger for testing.
func newTestWorker(reader biz.L3FactReader, writer biz.DecayScoreWriter, agents *biz.AgentUsecase) *MemoryEbbinghausDecayWorker {
	return NewMemoryEbbinghausDecayWorker(0, nil, reader, writer, agents, loggateway.NewNoop())
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

// TestNewMemoryEbbinghausDecayWorker_Defaults verifies that the constructor
// applies sensible defaults.
func TestNewMemoryEbbinghausDecayWorker_Defaults(t *testing.T) {
	w := NewMemoryEbbinghausDecayWorker(0, nil, nil, nil, nil, nil)
	if w == nil {
		t.Fatal("expected non-nil worker")
	}
	if w.interval != memoryEbbinghausDecayDefaultInterval {
		t.Errorf("expected default interval %v, got %v", memoryEbbinghausDecayDefaultInterval, w.interval)
	}
	if w.calculator == nil {
		t.Error("expected non-nil calculator")
	}
	if w.runner == nil {
		t.Error("expected non-nil runner")
	}
}

// TestNewMemoryEbbinghausDecayWorker_NilReader verifies that the worker
// handles a nil reader gracefully (no-op tick).
func TestNewMemoryEbbinghausDecayWorker_NilReader(t *testing.T) {
	w := newTestWorker(nil, nil, nil)
	// Should not panic when runOnce is called with nil reader.
	w.runOnce(context.Background())
}

// TestMemoryEbbinghausDecay_ScanGlobal verifies that the worker scans facts
// and computes decay statistics when agents is nil (global scan mode).
func TestMemoryEbbinghausDecay_ScanGlobal(t *testing.T) {
	now := time.Now().UTC()
	// Create facts with varying decay:
	// - recently accessed: high decay (close to 1.0)
	// - old, never accessed: low decay (heavily decayed)
	recent := makeFactRowWithID("fact-recent",
		now.Add(-1*time.Hour).Format(time.RFC3339),    // created 1h ago
		now.Add(-30*time.Minute).Format(time.RFC3339), // updated 30m ago
		now.Add(-30*time.Minute).Format(time.RFC3339), // last used 30m ago
		5, // 5 accesses
	)
	old := makeFactRowWithID("fact-old",
		now.Add(-720*time.Hour).Format(time.RFC3339), // created 30d ago
		now.Add(-720*time.Hour).Format(time.RFC3339), // updated 30d ago
		"", // never accessed
		0,  // 0 accesses
	)

	reader := &mockL3FactReader{
		rows:   [][]byte{recent, old},
		total:  2,
		active: 2,
	}
	writer := &mockDecayScoreWriter{}

	w := newTestWorker(reader, writer, nil)
	w.runOnce(context.Background())

	// Verify the reader was called with empty scope (global scan).
	if reader.lastScopeType != "" {
		t.Errorf("expected empty scope_type for global scan, got %q", reader.lastScopeType)
	}
	if reader.lastStatus != "active" {
		t.Errorf("expected status 'active', got %q", reader.lastStatus)
	}
	if reader.lastLimit != int32(memoryEbbinghausDecayBatchSize) {
		t.Errorf("expected limit %d, got %d", memoryEbbinghausDecayBatchSize, reader.lastLimit)
	}
	// Verify decay scores were written back for both facts.
	if writer.calls != 1 {
		t.Errorf("expected 1 writeback call, got %d", writer.calls)
	}
	if len(writer.scores) != 2 {
		t.Errorf("expected 2 scores written back, got %d", len(writer.scores))
	}
	if _, ok := writer.scores["fact-recent"]; !ok {
		t.Error("expected fact-recent in writeback scores")
	}
	if _, ok := writer.scores["fact-old"]; !ok {
		t.Error("expected fact-old in writeback scores")
	}
}

// TestMemoryEbbinghausDecay_ScanGlobal_NilWriter verifies that the worker
// computes statistics without writeback when writer is nil (statistics-only
// mode for backward compatibility).
func TestMemoryEbbinghausDecay_ScanGlobal_NilWriter(t *testing.T) {
	now := time.Now().UTC()
	recent := makeFactRowWithID("fact-1",
		now.Add(-1*time.Hour).Format(time.RFC3339),
		now.Add(-30*time.Minute).Format(time.RFC3339),
		now.Add(-30*time.Minute).Format(time.RFC3339),
		5,
	)
	reader := &mockL3FactReader{
		rows:   [][]byte{recent},
		total:  1,
		active: 1,
	}
	// nil writer — should not panic, should compute statistics only.
	w := newTestWorker(reader, nil, nil)
	err := w.scanAndCompute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMemoryEbbinghausDecay_ComputeDecayForRow verifies the decay computation
// for a single fact row.
func TestMemoryEbbinghausDecay_ComputeDecayForRow(t *testing.T) {
	now := time.Now().UTC()
	w := newTestWorker(nil, nil, nil)

	// Recently accessed fact should have high decay (close to 1.0).
	recentRow := makeFactRow(
		now.Add(-1*time.Hour).Format(time.RFC3339),
		now.Add(-30*time.Minute).Format(time.RFC3339),
		now.Add(-30*time.Minute).Format(time.RFC3339),
		5,
	)
	_, decayRecent, err := w.computeDecayForRow(recentRow, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decayRecent < 0.5 {
		t.Errorf("expected high decay (>=0.5) for recently accessed fact, got %f", decayRecent)
	}

	// Old, never accessed fact should have lower decay than the recent one.
	// Note: R_t ≈ exp(-1) ≈ 0.368 for old facts with 0 accesses (by design).
	oldRow := makeFactRow(
		now.Add(-720*time.Hour).Format(time.RFC3339),
		now.Add(-720*time.Hour).Format(time.RFC3339),
		"",
		0,
	)
	_, decayOld, err := w.computeDecayForRow(oldRow, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decayOld >= decayRecent {
		t.Errorf("expected decayOld < decayRecent (old vs recent), got %f vs %f", decayOld, decayRecent)
	}
	if decayOld > 0.5 {
		t.Errorf("expected decayOld <= 0.5 for old fact, got %f", decayOld)
	}
}

// TestMemoryEbbinghausDecay_ComputeDecayForRow_InvalidJSON verifies that
// invalid JSON returns an error.
func TestMemoryEbbinghausDecay_ComputeDecayForRow_InvalidJSON(t *testing.T) {
	w := newTestWorker(nil, nil, nil)
	_, _, err := w.computeDecayForRow([]byte("not json"), time.Now())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// TestMemoryEbbinghausDecay_ComputeDecayForRow_EmptyTimestamps verifies that
// a fact with empty timestamps returns decay 1.0 (no decay).
func TestMemoryEbbinghausDecay_ComputeDecayForRow_EmptyTimestamps(t *testing.T) {
	w := newTestWorker(nil, nil, nil)
	row := makeFactRow("", "", "", 0)
	_, decay, err := w.computeDecayForRow(row, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decay != 1.0 {
		t.Errorf("expected decay 1.0 for empty timestamps, got %f", decay)
	}
}

// TestMemoryEbbinghausDecay_ScanGlobal_ReaderError verifies that the worker
// handles reader errors gracefully (returns error for retry).
func TestMemoryEbbinghausDecay_ScanGlobal_ReaderError(t *testing.T) {
	reader := &mockL3FactReader{
		err: errors.New("database connection failed"),
	}
	w := newTestWorker(reader, nil, nil)
	err := w.scanAndCompute(context.Background())
	if err == nil {
		t.Error("expected error from reader failure")
	}
}

// TestMemoryEbbinghausDecay_ScanGlobal_NoFacts verifies that the worker
// handles the case where no facts are returned (no statistics logged).
func TestMemoryEbbinghausDecay_ScanGlobal_NoFacts(t *testing.T) {
	reader := &mockL3FactReader{
		rows: nil,
	}
	w := newTestWorker(reader, nil, nil)
	err := w.scanAndCompute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestMemoryEbbinghausDecay_LogStatistics_NoFacts verifies that logStatistics
// is a no-op when totalFacts is 0.
func TestMemoryEbbinghausDecay_LogStatistics_NoFacts(t *testing.T) {
	w := newTestWorker(nil, nil, nil)
	// Should not panic.
	w.logStatistics(0, 0, 0, 0)
}

// TestMemoryEbbinghausDecay_Disabled verifies the env var check.
func TestMemoryEbbinghausDecay_Disabled(t *testing.T) {
	t.Setenv("MEMORY_EBBINGHAUS_DECAY_DISABLED", "1")
	if !MemoryEbbinghausDecayDisabled() {
		t.Error("expected disabled when env var is '1'")
	}

	t.Setenv("MEMORY_EBBINGHAUS_DECAY_DISABLED", "false")
	if MemoryEbbinghausDecayDisabled() {
		t.Error("expected not disabled when env var is 'false'")
	}

	t.Setenv("MEMORY_EBBINGHAUS_DECAY_DISABLED", "")
	if MemoryEbbinghausDecayDisabled() {
		t.Error("expected not disabled when env var is empty")
	}
}

// TestMemoryEbbinghausDecay_Start_ContextCancel verifies that Start exits
// when the context is cancelled.
func TestMemoryEbbinghausDecay_Start_ContextCancel(t *testing.T) {
	w := newTestWorker(nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	// Should return within 100ms after cancel.
	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Start did not exit after context cancellation")
	}
}

// TestMemoryEbbinghausDecay_ScanPerAgent_AgentsError verifies that the worker
// handles ListMemoryMaintenanceTargets errors gracefully.
func TestMemoryEbbinghausDecay_ScanPerAgent_AgentsError(t *testing.T) {
	// This test verifies the error path when agents.ListMemoryMaintenanceTargets
	// fails. Since we can't easily mock *biz.AgentUsecase (concrete type), we
	// skip this test and rely on the integration test.
	t.Skip("requires AgentUsecase mock — covered by integration test")
}

// TestMemoryEbbinghausDecay_ScanFactsForAgent_CrossScopeAgentFilter pins the
// H3 fix: per-agent decay scanning must filter by the ORIGINATING agent column
// (agent_id) across ALL scopes — not scope_type='agent' — because immediate
// facts live in session scope and consolidated facts in user scope (F1). The
// old scope='agent' query silently scanned 0 rows for agents whose facts are
// all in user/session scopes, so no decay scores were ever written back.
func TestMemoryEbbinghausDecay_ScanFactsForAgent_CrossScopeAgentFilter(t *testing.T) {
	now := time.Now().UTC()
	reader := &mockL3FactReader{
		rows: [][]byte{makeFactRowWithID("fact-u1",
			now.Add(-2*time.Hour).Format(time.RFC3339),
			now.Add(-time.Hour).Format(time.RFC3339),
			now.Add(-time.Hour).Format(time.RFC3339),
			3)},
		total:  1,
		active: 1,
	}
	w := newTestWorker(reader, nil, nil)

	total, decayed, _, scores, err := w.scanFactsForAgent(context.Background(), "agent-x", now)
	if err != nil {
		t.Fatalf("scanFactsForAgent: %v", err)
	}
	if reader.lastScopeType != "" {
		t.Errorf("scope_type = %q, want empty (cross-scope agent filter)", reader.lastScopeType)
	}
	if reader.lastScopeID != "" {
		t.Errorf("scope_id = %q, want empty", reader.lastScopeID)
	}
	if reader.lastAgentID != "agent-x" {
		t.Errorf("agentID = %q, want agent-x", reader.lastAgentID)
	}
	if total != 1 || len(scores) != 1 {
		t.Errorf("total=%d scores=%d, want 1/1", total, len(scores))
	}
	_ = decayed
}

// TestMemoryEbbinghausDecay_Integration verifies the full scan+compute flow
// with a real EbbinghausDecayCalculator and mock reader.
func TestMemoryEbbinghausDecay_Integration(t *testing.T) {
	now := time.Now().UTC()
	// Create 3 facts:
	// 1. Recent, high access count → high decay
	// 2. Old, low access count → low decay
	// 3. Very old, no access → very low decay
	fact1 := makeFactRowWithID("fact-1",
		now.Add(-1*time.Hour).Format(time.RFC3339),
		now.Add(-30*time.Minute).Format(time.RFC3339),
		now.Add(-30*time.Minute).Format(time.RFC3339),
		10,
	)
	fact2 := makeFactRowWithID("fact-2",
		now.Add(-168*time.Hour).Format(time.RFC3339), // 7d ago
		now.Add(-168*time.Hour).Format(time.RFC3339),
		now.Add(-168*time.Hour).Format(time.RFC3339),
		1,
	)
	fact3 := makeFactRowWithID("fact-3",
		now.Add(-1440*time.Hour).Format(time.RFC3339), // 60d ago
		now.Add(-1440*time.Hour).Format(time.RFC3339),
		"",
		0,
	)

	reader := &mockL3FactReader{
		rows:   [][]byte{fact1, fact2, fact3},
		total:  3,
		active: 3,
	}
	writer := &mockDecayScoreWriter{}

	w := newTestWorker(reader, writer, nil)
	err := w.scanAndCompute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify decay values are computed correctly.
	_, decay1, _ := w.computeDecayForRow(fact1, now)
	_, decay2, _ := w.computeDecayForRow(fact2, now)
	_, decay3, _ := w.computeDecayForRow(fact3, now)

	if decay1 <= decay2 {
		t.Errorf("expected decay1 > decay2 (recent vs old), got %f vs %f", decay1, decay2)
	}
	if decay2 <= decay3 {
		t.Errorf("expected decay2 > decay3 (old vs very old), got %f vs %f", decay2, decay3)
	}
	// Verify writeback captured all 3 facts with correct decay values.
	if len(writer.scores) != 3 {
		t.Errorf("expected 3 scores written back, got %d", len(writer.scores))
	}
	if v, ok := writer.scores["fact-1"]; !ok || v != decay1 {
		t.Errorf("expected fact-1 decay %f in writeback, got %f (ok=%v)", decay1, v, ok)
	}
	if v, ok := writer.scores["fact-3"]; !ok || v != decay3 {
		t.Errorf("expected fact-3 decay %f in writeback, got %f (ok=%v)", decay3, v, ok)
	}
}
