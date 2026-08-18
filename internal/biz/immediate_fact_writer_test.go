package biz

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type fakeConsolidationWriter struct {
	res    *ConsolidationResult
	err    error
	writes []MemoryFactWrite
}

func (f *fakeConsolidationWriter) UpsertFactsAndEpisodeBatch(_ context.Context, writes []MemoryFactWrite, _ *EpisodeWrite) (*ConsolidationResult, error) {
	f.writes = append(f.writes, writes...)
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

// R1：即时事实严禁 session scope（L3ScopeTargets 无 session case，写入即死数据）。
// identity/preference → user scope；instruction/domain/general → agent scope。
func TestImmediateFactWriter_ScopeByFactKind(t *testing.T) {
	fw := &fakeConsolidationWriter{res: &ConsolidationResult{}}
	w := NewImmediateFactWriter(fw, nil, loggateway.NewNoop())
	if err := w.writeFactsSync(context.Background(), "sess-1", "agent-1", "user-1", "msg-1", []FactMark{
		{Type: "identity", Confidence: "high", Content: "用户叫张三"},
		{Type: "preference", Confidence: "high", Content: "用户偏好咖啡"},
		{Type: "instruction", Confidence: "high", Content: "回复要简洁"},
		{Type: "domain_knowledge", Confidence: "medium", Content: "机房空调 24 度"},
		{Type: "unknown_type", Confidence: "low", Content: "杂项"},
	}); err != nil {
		t.Fatalf("writeFactsSync: %v", err)
	}
	want := []struct{ scopeType, scopeID string }{
		{"user", "user-1"},
		{"user", "user-1"},
		{"agent", "agent-1"},
		{"agent", "agent-1"},
		{"agent", "agent-1"},
	}
	if len(fw.writes) != len(want) {
		t.Fatalf("writes = %d, want %d", len(fw.writes), len(want))
	}
	for i, wnt := range want {
		if fw.writes[i].ScopeType != wnt.scopeType || fw.writes[i].ScopeID != wnt.scopeID {
			t.Errorf("write[%d] scope = %s/%s, want %s/%s", i, fw.writes[i].ScopeType, fw.writes[i].ScopeID, wnt.scopeType, wnt.scopeID)
		}
	}
}

// userID 为空时 identity/preference 安全降级到 agent scope，保证可召回。
func TestImmediateFactWriter_UserScopeFallsBackToAgent(t *testing.T) {
	fw := &fakeConsolidationWriter{res: &ConsolidationResult{}}
	w := NewImmediateFactWriter(fw, nil, loggateway.NewNoop())
	if err := w.writeFactsSync(context.Background(), "sess-1", "agent-1", "", "msg-1", []FactMark{
		{Type: "identity", Confidence: "high", Content: "用户叫张三"},
	}); err != nil {
		t.Fatalf("writeFactsSync: %v", err)
	}
	if len(fw.writes) != 1 || fw.writes[0].ScopeType != "agent" || fw.writes[0].ScopeID != "agent-1" {
		t.Fatalf("empty userID must fall back to agent scope, got %+v", fw.writes)
	}
}

func TestImmediateFactWriter_CanonicalizesEmployeeIDAsUserIdentity(t *testing.T) {
	fw := &fakeConsolidationWriter{res: &ConsolidationResult{}}
	w := NewImmediateFactWriter(fw, nil, loggateway.NewNoop())
	if err := w.writeFactsSync(context.Background(), "sess-1", "agent-1", "user-1", "msg-1", []FactMark{
		{Type: "preference", Confidence: "high", Content: "我的工号是 DIAG-20260818-A7。"},
		{Type: "preference", Confidence: "high", Content: "喜欢简洁的回答"},
	}); err != nil {
		t.Fatalf("writeFactsSync: %v", err)
	}
	if len(fw.writes) != 2 {
		t.Fatalf("writes = %d, want 2", len(fw.writes))
	}
	if fw.writes[0].FactKind != "user_identity" || fw.writes[0].ScopeType != "user" {
		t.Fatalf("employee-id preference must canonicalize to user_identity/user, got %+v", fw.writes[0])
	}
	if fw.writes[0].Statement != "我的工号是 DIAG-20260818-A7" {
		t.Fatalf("trailing punctuation must be stripped, got %q", fw.writes[0].Statement)
	}
	if fw.writes[1].FactKind != "preference" {
		t.Fatalf("plain preference must stay preference, got %q", fw.writes[1].FactKind)
	}
}
