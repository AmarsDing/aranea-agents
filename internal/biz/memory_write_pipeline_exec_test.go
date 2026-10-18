package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// --- fakes (fakeNeighborSearcher shared with memory_conflict_test.go) ---

type fakeEmbedder struct {
	err   error
	calls int
}

func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return []float32{0.1, 0.2}, nil
}

// batchEmbedder 实现可选的批量口（P2-5），管线必须优先用它。
type batchEmbedder struct {
	fakeEmbedder
	batchCalls int
	batchSizes []int
}

func (b *batchEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	b.batchCalls++
	b.batchSizes = append(b.batchSizes, len(texts))
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		v, err := b.embedOne(t)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (b *batchEmbedder) embedOne(t string) ([]float32, error) {
	if b.err != nil {
		return nil, b.err
	}
	return []float32{0.1, 0.2}, nil
}

func factRowJSON(id, statement, kind string) []byte {
	b, _ := json.Marshal(map[string]any{"id": id, "statement": statement, "fact_kind": kind, "status": "active"})
	return b
}

type fakeFactRowReader struct {
	rows  [][]byte
	calls int
}

func (f *fakeFactRowReader) GetFactRowsByIDs(_ context.Context, _ []string) ([][]byte, error) {
	f.calls++
	return f.rows, nil
}

type fakeFactWriter struct {
	upserts       []FactUpsert
	invalidations []string
	txOldIDs      []string
	txUpserts     []FactUpsert
	err           error
}

func (f *fakeFactWriter) UpsertFactRow(_ context.Context, in FactUpsert) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.upserts = append(f.upserts, in)
	return factRowJSON(in.ID, in.Statement, in.FactKind), nil
}

func (f *fakeFactWriter) DeleteFactRow(_ context.Context, _ string) error { return nil }

func (f *fakeFactWriter) DeleteFactRowsByIDs(_ context.Context, _ []string) (int, error) {
	return 0, nil
}

func (f *fakeFactWriter) ClearFactsByScope(_ context.Context, _, _, _ string) ([]string, error) {
	return nil, nil
}

func (f *fakeFactWriter) InvalidateFact(_ context.Context, factID string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.invalidations = append(f.invalidations, factID)
	return factRowJSON(factID, "", ""), nil
}

func (f *fakeFactWriter) InvalidateAndUpsertFactTx(_ context.Context, oldFactID string, in FactUpsert) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.txOldIDs = append(f.txOldIDs, oldFactID)
	f.txUpserts = append(f.txUpserts, in)
	return factRowJSON(in.ID, in.Statement, in.FactKind), nil
}

type fakeAccessCounter struct {
	ids []string
	err error
}

func (f *fakeAccessCounter) IncrementFactRecalledCount(_ context.Context, ids []string) error {
	if f.err != nil {
		return f.err
	}
	f.ids = append(f.ids, ids...)
	return nil
}

type fakeAdjudicator struct {
	verdicts  []FactAdjudicationVerdict
	err       error
	calls     int
	lastItems []FactAdjudicationItem
	// fn：按 items 动态生成判决（P2-4 序数匹配测试用）；设置后覆盖 verdicts。
	fn func(items []FactAdjudicationItem) []FactAdjudicationVerdict
}

func (f *fakeAdjudicator) AdjudicateFactWrites(_ context.Context, _, _ string, items []FactAdjudicationItem) ([]FactAdjudicationVerdict, error) {
	f.calls++
	f.lastItems = items
	if f.fn != nil {
		return f.fn(items), f.err
	}
	return f.verdicts, f.err
}

type fakeActionLog struct {
	recs []MemoryPolicyRecord
}

func (f *fakeActionLog) WriteMemoryActionLog(_ context.Context, rec MemoryPolicyRecord) error {
	f.recs = append(f.recs, rec)
	return nil
}

// --- tests ---

func pipelineCandidate(stmt string) FactWriteCandidate {
	return FactWriteCandidate{
		Statement:  stmt,
		FactKind:   "preference",
		Confidence: 0.9,
		Importance: 0.7,
		ScopeType:  "user",
		ScopeID:    "u1",
		UserID:     "u1",
		AgentID:    "ag1",
		SourceKind: "auto_memory",
	}
}

func TestFactWritePipeline_GateDrops(t *testing.T) {
	writer := &fakeFactWriter{}
	alog := &fakeActionLog{}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Writer:    writer,
		ActionLog: alog,
		LG:        loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{
		{Statement: "临时事件", FactKind: "event", Confidence: 0.9},     // gate 1
		{Statement: "低置信", FactKind: "preference", Confidence: 0.3}, // gate 2
		{Statement: "  ", FactKind: "preference", Confidence: 0.9},  // empty
	})
	if res.Dropped != 3 {
		t.Fatalf("dropped: got %d want 3", res.Dropped)
	}
	if len(writer.upserts) != 0 {
		t.Fatalf("no writes expected, got %d", len(writer.upserts))
	}
	if len(alog.recs) != 3 {
		t.Fatalf("audit recs: got %d want 3", len(alog.recs))
	}
}

func TestFactWritePipeline_AddWhenNoNeighbors(t *testing.T) {
	writer := &fakeFactWriter{}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{},
		Embedder: &fakeEmbedder{},
		Reader:   &fakeFactRowReader{},
		Writer:   writer,
		LG:       loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户喜欢茶")})
	if res.Added != 1 || len(writer.upserts) != 1 {
		t.Fatalf("added: got %d upserts %d, want 1/1", res.Added, len(writer.upserts))
	}
	up := writer.upserts[0]
	if up.Statement != "用户喜欢茶" || up.FactKind != "preference" || up.ValidFrom == "" || up.Status != "active" {
		t.Fatalf("upsert shape wrong: %+v", up)
	}
	if len(res.FactRows) != 1 {
		t.Fatalf("FactRows: got %d, want 1 (index-sync contract)", len(res.FactRows))
	}
}

func TestFactWritePipeline_MergeBumpsAccessCount(t *testing.T) {
	writer := &fakeFactWriter{}
	access := &fakeAccessCounter{}
	adj := &fakeAdjudicator{}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.95, FactKind: "preference"},
		}},
		Embedder:    &fakeEmbedder{},
		Reader:      &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢茶", "preference")}},
		Writer:      writer,
		Access:      access,
		Adjudicator: adj,
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户喜欢喝茶")})
	if res.Merged != 1 {
		t.Fatalf("merged: got %d want 1", res.Merged)
	}
	if len(access.ids) != 1 || access.ids[0] != "f1" {
		t.Fatalf("access bump ids: got %v want [f1]", access.ids)
	}
	if len(writer.upserts) != 0 {
		t.Fatalf("merge must not insert, got %d upserts", len(writer.upserts))
	}
	if adj.calls != 0 {
		t.Fatalf("auto-merge must not call adjudicator, got %d calls", adj.calls)
	}
}

func TestFactWritePipeline_ContestedAdjudicatedUpdate(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户现在只喝茶", Operation: FactWriteOpUpdate, TargetFactID: "f1"},
	}}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.85, FactKind: "preference"},
		}},
		Embedder:    &fakeEmbedder{},
		Reader:      &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢咖啡", "preference")}},
		Writer:      writer,
		Adjudicator: adj,
		Pending:     pending,
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户现在只喝茶")})
	if adj.calls != 1 {
		t.Fatalf("adjudicator calls: got %d want 1", adj.calls)
	}
	// R3: adjudicated UPDATE no longer writes directly — it pends for approval.
	if res.Pended != 1 || res.Updated != 0 {
		t.Fatalf("R3 gate: got Pended=%d Updated=%d, want 1/0", res.Pended, res.Updated)
	}
	if len(writer.txOldIDs) != 0 || len(writer.txUpserts) != 0 {
		t.Fatalf("withheld update must not touch storage: %+v", writer)
	}
	if len(pending.recs) != 1 || pending.recs[0].Verdict != MemoryFactPendingVerdictUpdate ||
		pending.recs[0].FactKey != "f1" || pending.recs[0].PriorBody != "用户喜欢咖啡" {
		t.Fatalf("pending rec mismatch: %+v", pending.recs)
	}
}

func TestFactWritePipeline_ContestedAdjudicatedDelete(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户不再喝咖啡", Operation: FactWriteOpDelete, TargetFactID: "f1"},
	}}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.88, FactKind: "preference"},
		}},
		Embedder:    &fakeEmbedder{},
		Reader:      &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢咖啡", "preference")}},
		Writer:      writer,
		Adjudicator: adj,
		Pending:     pending,
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户不再喝咖啡")})
	// R3: adjudicated DELETE no longer invalidates directly — it pends.
	if res.Pended != 1 || res.Deleted != 0 || len(writer.invalidations) != 0 {
		t.Fatalf("R3 gate: got Pended=%d Deleted=%d invalidations=%v, want 1/0/[]",
			res.Pended, res.Deleted, writer.invalidations)
	}
	if len(pending.recs) != 1 || pending.recs[0].Verdict != MemoryFactPendingVerdictDelete || pending.recs[0].FactKey != "f1" {
		t.Fatalf("pending rec mismatch: %+v", pending.recs)
	}
}

// R3：adjudicator 未装配时 contested 候选不再 heuristic 直写——落 pending
// （CONTESTED）；无 pending 存储则 fail-closed 扣留（记 WriteErrs，绝不直写）。
func TestFactWritePipeline_AdjudicatorNilPendsContested(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.85, FactKind: "preference"},
		}},
		Embedder: &fakeEmbedder{},
		Reader:   &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢咖啡", "preference")}},
		Writer:   writer,
		Pending:  pending,
		LG:       loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户现在只喝茶")})
	if res.Pended != 1 || res.Added != 0 || len(writer.upserts) != 0 {
		t.Fatalf("contested without adjudicator must pend, got %+v", res)
	}
	if len(pending.recs) != 1 || pending.recs[0].Verdict != MemoryFactPendingVerdictContested {
		t.Fatalf("pending rec mismatch: %+v", pending.recs)
	}
}

func TestFactWritePipeline_AdjudicatorErrorPendsContested(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{err: errors.New("llm down")}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.85, FactKind: "preference"},
		}},
		Embedder:    &fakeEmbedder{},
		Reader:      &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢咖啡", "preference")}},
		Writer:      writer,
		Adjudicator: adj,
		Pending:     pending,
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户现在只喝茶")})
	if res.Pended != 1 || res.Added != 0 || len(writer.upserts) != 0 {
		t.Fatalf("adjudicator error must pend CONTESTED, got %+v", res)
	}
	if len(pending.recs) != 1 || pending.recs[0].AdjudicatorReason != "adjudicator_error" {
		t.Fatalf("pending rec mismatch: %+v", pending.recs)
	}
}

func TestFactWritePipeline_VerdictTargetNotNeighborPendsContested(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户现在只喝茶", Operation: FactWriteOpUpdate, TargetFactID: "stranger"},
	}}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.85, FactKind: "preference"},
		}},
		Embedder:    &fakeEmbedder{},
		Reader:      &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢咖啡", "preference")}},
		Writer:      writer,
		Adjudicator: adj,
		Pending:     pending,
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户现在只喝茶")})
	if res.Pended != 1 || res.Added != 0 || len(writer.upserts) != 0 {
		t.Fatalf("invalid target must pend CONTESTED (R3), got %+v", res)
	}
	if len(pending.recs) != 1 || pending.recs[0].Verdict != MemoryFactPendingVerdictContested {
		t.Fatalf("pending rec mismatch: %+v", pending.recs)
	}
}

func TestFactWritePipeline_EmbedErrorDegradesToAdd(t *testing.T) {
	writer := &fakeFactWriter{}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{},
		Embedder: &fakeEmbedder{err: errors.New("embed down")},
		Writer:   writer,
		LG:       loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户喜欢茶")})
	if res.Added != 1 {
		t.Fatalf("embed failure must degrade to add, got %+v", res)
	}
}

// P2-4（2026-10-18 审计）：同批次两条相同 statement 的 contested 候选，判决
// 必须按 ordinal 一一对应——旧的 statement→verdict map 会让两条候选共用同一
// 判决（交叉错配）。
func TestFactWritePipeline_DuplicateStatementsMatchDistinctVerdicts(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	access := &fakeAccessCounter{}
	adj := &fakeAdjudicator{fn: func(items []FactAdjudicationItem) []FactAdjudicationVerdict {
		out := make([]FactAdjudicationVerdict, 0, len(items))
		for i, it := range items {
			if i == 0 {
				out = append(out, FactAdjudicationVerdict{
					Ordinal: i, Statement: it.Candidate.Statement,
					Operation: FactWriteOpUpdate, TargetFactID: "f1",
				})
			} else {
				out = append(out, FactAdjudicationVerdict{
					Ordinal: i, Statement: it.Candidate.Statement,
					Operation: FactWriteOpNoop, TargetFactID: "f1",
				})
			}
		}
		return out
	}}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.85, FactKind: "preference"},
		}},
		Embedder:    &fakeEmbedder{},
		Reader:      &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢咖啡", "preference")}},
		Writer:      writer,
		Access:      access,
		Adjudicator: adj,
		Pending:     pending,
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{
		pipelineCandidate("用户现在只喝茶"), // ordinal 0 → adjudicated UPDATE → pend
		pipelineCandidate("用户现在只喝茶"), // ordinal 1 → adjudicated NOOP  → merge
	})
	if res.Pended != 1 || res.Merged != 1 || res.Added != 0 {
		t.Fatalf("ordinal mis-match: got Pended=%d Merged=%d Added=%d, want 1/1/0",
			res.Pended, res.Merged, res.Added)
	}
	if len(pending.recs) != 1 || pending.recs[0].Verdict != MemoryFactPendingVerdictUpdate {
		t.Fatalf("pending rec mismatch: %+v", pending.recs)
	}
	if len(access.ids) != 1 || access.ids[0] != "f1" {
		t.Fatalf("merge bump mismatch: %+v", access.ids)
	}
	// items 顺序即 ordinal——判决实现依赖该契约（服务端 prompt 同序）。
	if adj.calls != 1 || len(adj.lastItems) != 2 {
		t.Fatalf("adjudicator contract: calls=%d items=%d", adj.calls, len(adj.lastItems))
	}
}

// P2-5（2026-10-18 审计）：邻居检索必须跨候选批处理——fact 行读取整个批次
// 至多一次；embedder 提供批量口时必须走批量（每候选一次 Embed 的旧路径）。
func TestFactWritePipeline_NeighborRetrievalBatched(t *testing.T) {
	writer := &fakeFactWriter{}
	reader := &fakeFactRowReader{rows: [][]byte{
		factRowJSON("f1", "用户喜欢咖啡", "preference"),
	}}
	embedder := &batchEmbedder{}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.95, FactKind: "preference"}, // ≥0.92 同 kind → merge
		}},
		Embedder: embedder,
		Reader:   reader,
		Writer:   writer,
		LG:       loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{
		pipelineCandidate("用户喜欢喝咖啡"),
		pipelineCandidate("用户偏好拿铁"),
		pipelineCandidate("用户不喝奶茶"),
	})
	// 全部 merge：无新增写入。
	if res.Added != 0 || len(writer.upserts) != 0 {
		t.Fatalf("expected all merged, got Added=%d upserts=%d", res.Added, len(writer.upserts))
	}
	if reader.calls != 1 {
		t.Errorf("GetFactRowsByIDs calls: got %d, want 1 (batched across candidates)", reader.calls)
	}
	if embedder.batchCalls != 1 {
		t.Errorf("EmbedBatch calls: got %d, want 1; per-text Embed calls: %d", embedder.batchCalls, embedder.calls)
	}
	if len(embedder.batchSizes) != 1 || embedder.batchSizes[0] != 3 {
		t.Errorf("batch sizes: got %v, want [3]", embedder.batchSizes)
	}
}

// 无批量口的 embedder 退化为逐条 Embed（旧行为兼容）。
func TestFactWritePipeline_NeighborRetrievalFallbackPerTextEmbed(t *testing.T) {
	writer := &fakeFactWriter{}
	reader := &fakeFactRowReader{rows: [][]byte{
		factRowJSON("f1", "用户喜欢咖啡", "preference"),
	}}
	embedder := &fakeEmbedder{}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.95, FactKind: "preference"},
		}},
		Embedder: embedder,
		Reader:   reader,
		Writer:   writer,
		LG:       loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{
		pipelineCandidate("用户喜欢喝咖啡"),
		pipelineCandidate("用户偏好拿铁"),
	})
	if res.Added != 0 {
		t.Fatalf("expected all merged, got Added=%d", res.Added)
	}
	if reader.calls != 1 {
		t.Errorf("GetFactRowsByIDs calls: got %d, want 1", reader.calls)
	}
	if embedder.calls != 2 {
		t.Errorf("per-text Embed calls: got %d, want 2 (no batch interface)", embedder.calls)
	}
}
