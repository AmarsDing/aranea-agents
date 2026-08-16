package biz

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type fakeConsolidationWriter struct {
	res *ConsolidationResult
	err error
}

func (f *fakeConsolidationWriter) UpsertFactsAndEpisodeBatch(_ context.Context, _ []MemoryFactWrite, _ *EpisodeWrite) (*ConsolidationResult, error) {
	return f.res, f.err
}

type recordingIndexSyncer struct {
	mu   sync.Mutex
	rows [][]byte
	err  error
}

func (r *recordingIndexSyncer) SyncFactIndex(_ context.Context, _, _, _, _ string) error { return r.err }

func (r *recordingIndexSyncer) SyncFactIndexFromRow(_ context.Context, raw []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rows = append(r.rows, raw)
	return r.err
}

func (r *recordingIndexSyncer) syncedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

// P2-2：写入成功后逐行回采 embedding 索引（对齐 auto_memory 范式）。
func TestImmediateFactWriter_SyncsIndexFromFactRows(t *testing.T) {
	fw := &fakeConsolidationWriter{res: &ConsolidationResult{
		FactRows:     [][]byte{[]byte(`{"id":"f1","agent_id":"agent-1","statement":"s1"}`), []byte(`{"id":"f2"}`)},
		FactsWritten: 2,
	}}
	syncer := &recordingIndexSyncer{}
	w := NewImmediateFactWriter(fw, syncer, loggateway.NewNoop())
	if w == nil {
		t.Fatal("writer must be constructed when factWriter non-nil")
	}
	if err := w.writeFactsSync(context.Background(), "sess-1", "agent-1", "user-1", "msg-1", []FactMark{
		{Type: "preference", Confidence: "high", Content: "部署必须走灰度"},
	}); err != nil {
		t.Fatalf("writeFactsSync: %v", err)
	}
	if got := syncer.syncedCount(); got != 2 {
		t.Fatalf("SyncFactIndexFromRow calls = %d, want 2 (one per FactRow)", got)
	}
}

// nil indexSync 降级安全：写入成功即返回，不 panic。
func TestImmediateFactWriter_NilIndexSyncDegrades(t *testing.T) {
	fw := &fakeConsolidationWriter{res: &ConsolidationResult{FactRows: [][]byte{[]byte(`{"id":"f1"}`)}}}
	w := NewImmediateFactWriter(fw, nil, loggateway.NewNoop())
	if err := w.writeFactsSync(context.Background(), "sess-1", "agent-1", "user-1", "msg-1", []FactMark{
		{Type: "identity", Confidence: "medium", Content: "用户叫张三"},
	}); err != nil {
		t.Fatalf("writeFactsSync: %v", err)
	}
}

// 索引同步失败不阻断写入（reconciler cron 兜底），错误不回传。
func TestImmediateFactWriter_IndexSyncFailureDoesNotFailWrite(t *testing.T) {
	fw := &fakeConsolidationWriter{res: &ConsolidationResult{FactRows: [][]byte{[]byte(`{"id":"f1"}`)}, FactsWritten: 1}}
	syncer := &recordingIndexSyncer{err: errors.New("pgvector unavailable")}
	w := NewImmediateFactWriter(fw, syncer, loggateway.NewNoop())
	if err := w.writeFactsSync(context.Background(), "sess-1", "agent-1", "user-1", "msg-1", []FactMark{
		{Type: "instruction", Confidence: "low", Content: "回复尽量简洁"},
	}); err != nil {
		t.Fatalf("sync failure must not fail write, got %v", err)
	}
	if got := syncer.syncedCount(); got != 1 {
		t.Fatalf("SyncFactIndexFromRow calls = %d, want 1", got)
	}
}

// 写入失败直接返回错误，不触发索引同步。
func TestImmediateFactWriter_WriteFailureSkipsIndexSync(t *testing.T) {
	fw := &fakeConsolidationWriter{err: errors.New("db down")}
	syncer := &recordingIndexSyncer{}
	w := NewImmediateFactWriter(fw, syncer, loggateway.NewNoop())
	if err := w.writeFactsSync(context.Background(), "sess-1", "agent-1", "user-1", "msg-1", []FactMark{
		{Type: "preference", Confidence: "high", Content: "x"},
	}); err == nil {
		t.Fatal("expected write error to propagate")
	}
	if got := syncer.syncedCount(); got != 0 {
		t.Fatalf("index sync must not run on write failure, got %d calls", got)
	}
}
