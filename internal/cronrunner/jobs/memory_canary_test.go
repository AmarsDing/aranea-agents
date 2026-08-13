package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ── fakes ─────────────────────────────────────────────────

type canaryFakeConsolidationWriter struct {
	result *biz.ConsolidationResult
	err    error
	writes []biz.MemoryFactWrite
}

func (f *canaryFakeConsolidationWriter) UpsertFactsAndEpisodeBatch(ctx context.Context, facts []biz.MemoryFactWrite, ep *biz.EpisodeWrite) (*biz.ConsolidationResult, error) {
	f.writes = append(f.writes, facts...)
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &biz.ConsolidationResult{
		FactRows:     [][]byte{[]byte(`{"id":"canary-fact-1"}`)},
		FactsWritten: 1,
	}, nil
}

type canaryFakeL3Reader struct {
	// recallFn receives the 1-based call index; return rows per stage.
	recallFn     func(call int, query string, minScore float64) ([][]byte, error)
	calls        int
	lastQuery    string
	lastMinScore float64
}

func (f *canaryFakeL3Reader) RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, emb []float32, limit int32, minScore float64) ([][]byte, error) {
	f.calls++
	f.lastQuery = query
	f.lastMinScore = minScore
	if f.recallFn != nil {
		return f.recallFn(f.calls, query, minScore)
	}
	return nil, nil
}

// Unused L3FactReader methods — panic if unexpectedly called.
func (f *canaryFakeL3Reader) ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword, agentID string, limit, offset int32) ([][]byte, int32, int32, int32, error) {
	panic("unexpected call")
}
func (f *canaryFakeL3Reader) ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	panic("unexpected call")
}
func (f *canaryFakeL3Reader) ListFactRowsForUserAll(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error) {
	panic("unexpected call")
}
func (f *canaryFakeL3Reader) GetFactRowsByIDs(ctx context.Context, ids []string) ([][]byte, error) {
	panic("unexpected call")
}

type canaryFakeL3Writer struct {
	invalidated []string
	err         error
}

func (f *canaryFakeL3Writer) InvalidateFact(ctx context.Context, factID string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.invalidated = append(f.invalidated, factID)
	return []byte(`{"id":"` + factID + `"}`), nil
}

func (f *canaryFakeL3Writer) UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error) {
	panic("unexpected call")
}
func (f *canaryFakeL3Writer) DeleteFactRow(ctx context.Context, factID string) error {
	panic("unexpected call")
}
func (f *canaryFakeL3Writer) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
	panic("unexpected call")
}
func (f *canaryFakeL3Writer) ClearFactsByScope(ctx context.Context, scopeType, scopeID, userID string) ([]string, error) {
	panic("unexpected call")
}
func (f *canaryFakeL3Writer) InvalidateAndUpsertFactTx(ctx context.Context, oldFactID string, in biz.FactUpsert) ([]byte, error) {
	panic("unexpected call")
}

type canaryFakeFlowLog struct {
	errors []string
}

func (f *canaryFakeFlowLog) LogFlowStart(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
}
func (f *canaryFakeFlowLog) LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
}
func (f *canaryFakeFlowLog) LogFlowWarn(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
}
func (f *canaryFakeFlowLog) LogFlowError(ctx context.Context, sessionID, stepID, message string, pairs ...biz.LogPair) {
	f.errors = append(f.errors, stepID+": "+message)
}

// ── helpers ───────────────────────────────────────────────

func canaryOKRecallRow(factID, token string, total float64) []byte {
	return []byte(fmt.Sprintf(`{"id":%q,"statement":"金丝雀记忆测试：我的代号是 %s","scores":{"total":%v}}`, factID, token, total))
}

func newCanaryWorker(writer biz.MemoryConsolidationWriter, reader biz.L3FactReader, facts biz.L3FactWriter, status *biz.MemoryCanaryStatus, flowLog biz.FlowLogWriter) *MemoryCanaryWorker {
	return NewMemoryCanaryWorker(0, writer, reader, facts, status, flowLog, loggateway.NewNoop())
}

// ── tests ─────────────────────────────────────────────────

// TestMemoryCanary_RunOnce_Pass exercises the full write → recall → archive
// loop: fact is written, recalled with scores.total >= 0.55, then invalidated
// and no longer recallable. Status records OK, no alarm is emitted, and the
// canary fact is cleaned up via invalidation.
func TestMemoryCanary_RunOnce_Pass(t *testing.T) {
	writer := &canaryFakeConsolidationWriter{}
	reader := &canaryFakeL3Reader{}
	reader.recallFn = func(call int, query string, minScore float64) ([][]byte, error) {
		if call == 1 {
			// Recall stage: production default minScore must be used.
			if minScore != MemoryCanaryMinScore {
				t.Errorf("recall minScore = %v, want %v", minScore, MemoryCanaryMinScore)
			}
			return [][]byte{canaryOKRecallRow("canary-fact-1", query, 0.66)}, nil
		}
		// Archive stage: after invalidation the fact must be gone.
		return nil, nil
	}
	facts := &canaryFakeL3Writer{}
	status := biz.NewMemoryCanaryStatus()
	flowLog := &canaryFakeFlowLog{}

	w := newCanaryWorker(writer, reader, facts, status, flowLog)
	w.RunOnce(context.Background())

	if len(writer.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(writer.writes))
	}
	write := writer.writes[0]
	if write.ScopeType != "canary" || write.ScopeID == "" {
		t.Fatalf("canary scope = %q/%q, want dedicated canary scope", write.ScopeType, write.ScopeID)
	}
	if !strings.Contains(write.Statement, "FALCON-") {
		t.Fatalf("statement %q missing FALCON token", write.Statement)
	}
	if reader.calls != 2 {
		t.Fatalf("recall calls = %d, want 2 (recall + post-archive verify)", reader.calls)
	}
	if len(facts.invalidated) != 1 || facts.invalidated[0] != "canary-fact-1" {
		t.Fatalf("invalidated = %v, want [canary-fact-1]", facts.invalidated)
	}
	snap := status.Snapshot()
	if snap.RunsTotal != 1 || snap.FailedTotal != 0 || snap.ConsecutiveFailures != 0 {
		t.Fatalf("snapshot = %+v, want 1 OK run", snap)
	}
	if len(flowLog.errors) != 0 {
		t.Fatalf("flow alarms = %v, want none on pass", flowLog.errors)
	}
}

func TestMemoryCanary_RunOnce_WriteFails(t *testing.T) {
	writer := &canaryFakeConsolidationWriter{err: errors.New("42702 column reference version is ambiguous")}
	reader := &canaryFakeL3Reader{}
	facts := &canaryFakeL3Writer{}
	status := biz.NewMemoryCanaryStatus()
	flowLog := &canaryFakeFlowLog{}

	w := newCanaryWorker(writer, reader, facts, status, flowLog)
	w.RunOnce(context.Background())

	snap := status.Snapshot()
	if snap.FailedTotal != 1 || snap.LastFailStage != "write" {
		t.Fatalf("snapshot = %+v, want write-stage failure", snap)
	}
	if len(flowLog.errors) != 1 {
		t.Fatalf("flow alarms = %v, want 1", flowLog.errors)
	}
	if reader.calls != 0 {
		t.Fatalf("recall must not run after write failure, calls = %d", reader.calls)
	}
}

func TestMemoryCanary_RunOnce_RecallMissesFact(t *testing.T) {
	writer := &canaryFakeConsolidationWriter{}
	reader := &canaryFakeL3Reader{} // returns nothing: fused recall dropped the fact
	facts := &canaryFakeL3Writer{}
	status := biz.NewMemoryCanaryStatus()
	flowLog := &canaryFakeFlowLog{}

	w := newCanaryWorker(writer, reader, facts, status, flowLog)
	w.RunOnce(context.Background())

	snap := status.Snapshot()
	if snap.LastFailStage != "recall" {
		t.Fatalf("LastFailStage = %q, want recall", snap.LastFailStage)
	}
	if len(flowLog.errors) != 1 {
		t.Fatalf("flow alarms = %v, want 1", flowLog.errors)
	}
	// Cleanup: the written fact must still be invalidated to avoid residue.
	if len(facts.invalidated) != 1 {
		t.Fatalf("invalidated = %v, want cleanup after recall failure", facts.invalidated)
	}
}

// TestMemoryCanary_RunOnce_RecallScoreBelowThreshold reproduces Bug A: the
// fact comes back but scores.total is 0 (or below the production default
// minScore), meaning it would be filtered out before prompt injection.
func TestMemoryCanary_RunOnce_RecallScoreBelowThreshold(t *testing.T) {
	writer := &canaryFakeConsolidationWriter{}
	reader := &canaryFakeL3Reader{}
	reader.recallFn = func(call int, query string, minScore float64) ([][]byte, error) {
		return [][]byte{canaryOKRecallRow("canary-fact-1", query, 0.0)}, nil
	}
	facts := &canaryFakeL3Writer{}
	status := biz.NewMemoryCanaryStatus()
	flowLog := &canaryFakeFlowLog{}

	w := newCanaryWorker(writer, reader, facts, status, flowLog)
	w.RunOnce(context.Background())

	if snap := status.Snapshot(); snap.LastFailStage != "recall" {
		t.Fatalf("LastFailStage = %q, want recall (score below threshold)", snap.LastFailStage)
	}
	if len(facts.invalidated) != 1 {
		t.Fatalf("invalidated = %v, want cleanup", facts.invalidated)
	}
}

// TestMemoryCanary_RunOnce_ArchiveStillRecallable catches a broken
// valid_until filter: after InvalidateFact the fact must disappear.
func TestMemoryCanary_RunOnce_ArchiveStillRecallable(t *testing.T) {
	writer := &canaryFakeConsolidationWriter{}
	reader := &canaryFakeL3Reader{}
	reader.recallFn = func(call int, query string, minScore float64) ([][]byte, error) {
		// Both calls return the fact: invalidation did not filter it out.
		return [][]byte{canaryOKRecallRow("canary-fact-1", query, 0.66)}, nil
	}
	facts := &canaryFakeL3Writer{}
	status := biz.NewMemoryCanaryStatus()
	flowLog := &canaryFakeFlowLog{}

	w := newCanaryWorker(writer, reader, facts, status, flowLog)
	w.RunOnce(context.Background())

	if snap := status.Snapshot(); snap.LastFailStage != "archive" {
		t.Fatalf("LastFailStage = %q, want archive", snap.LastFailStage)
	}
	if len(flowLog.errors) != 1 {
		t.Fatalf("flow alarms = %v, want 1", flowLog.errors)
	}
}

func TestMemoryCanary_RunOnce_InvalidateFails(t *testing.T) {
	writer := &canaryFakeConsolidationWriter{}
	reader := &canaryFakeL3Reader{}
	reader.recallFn = func(call int, query string, minScore float64) ([][]byte, error) {
		if call == 1 {
			return [][]byte{canaryOKRecallRow("canary-fact-1", query, 0.66)}, nil
		}
		return nil, nil
	}
	facts := &canaryFakeL3Writer{err: errors.New("db down")}
	status := biz.NewMemoryCanaryStatus()
	flowLog := &canaryFakeFlowLog{}

	w := newCanaryWorker(writer, reader, facts, status, flowLog)
	w.RunOnce(context.Background())

	if snap := status.Snapshot(); snap.LastFailStage != "archive" {
		t.Fatalf("LastFailStage = %q, want archive (invalidate error)", snap.LastFailStage)
	}
}

func TestMemoryCanary_ConsecutiveFailuresAccumulate(t *testing.T) {
	writer := &canaryFakeConsolidationWriter{err: errors.New("boom")}
	status := biz.NewMemoryCanaryStatus()
	w := newCanaryWorker(writer, &canaryFakeL3Reader{}, &canaryFakeL3Writer{}, status, &canaryFakeFlowLog{})

	w.RunOnce(context.Background())
	w.RunOnce(context.Background())
	if got := status.ConsecutiveFailures(); got != 2 {
		t.Fatalf("ConsecutiveFailures() = %d, want 2", got)
	}
}

func TestMemoryCanaryWorker_NilDepsNoPanic(t *testing.T) {
	w := NewMemoryCanaryWorker(0, nil, nil, nil, nil, nil, loggateway.NewNoop())
	w.RunOnce(context.Background())
	// Start must return immediately instead of blocking on nil deps.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Start(ctx)
}

func TestMemoryCanaryDisabled(t *testing.T) {
	t.Setenv("MEMORY_CANARY_DISABLED", "1")
	if !MemoryCanaryDisabled() {
		t.Fatal("MemoryCanaryDisabled() = false, want true")
	}
	t.Setenv("MEMORY_CANARY_DISABLED", "0")
	if MemoryCanaryDisabled() {
		t.Fatal("MemoryCanaryDisabled() = true, want false")
	}
}
