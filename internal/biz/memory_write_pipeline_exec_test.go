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
	err error
}

func (f *fakeEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []float32{0.1, 0.2}, nil
}

func factRowJSON(id, statement, kind string) []byte {
	b, _ := json.Marshal(map[string]any{"id": id, "statement": statement, "fact_kind": kind, "status": "active"})
	return b
}

type fakeFactRowReader struct {
	rows [][]byte
}

func (f *fakeFactRowReader) GetFactRowsByIDs(_ context.Context, _ []string) ([][]byte, error) {
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
	verdicts []FactAdjudicationVerdict
	err      error
	calls    int
}

func (f *fakeAdjudicator) AdjudicateFactWrites(_ context.Context, _, _ string, _ []FactAdjudicationItem) ([]FactAdjudicationVerdict, error) {
	f.calls++
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
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户现在只喝茶")})
	if adj.calls != 1 {
		t.Fatalf("adjudicator calls: got %d want 1", adj.calls)
	}
	if res.Updated != 1 {
		t.Fatalf("updated: got %d want 1", res.Updated)
	}
	if len(writer.txOldIDs) != 1 || writer.txOldIDs[0] != "f1" {
		t.Fatalf("tx invalidate old: got %v want [f1]", writer.txOldIDs)
	}
	if len(writer.txUpserts) != 1 || writer.txUpserts[0].Statement != "用户现在只喝茶" {
		t.Fatalf("tx upsert: got %+v", writer.txUpserts)
	}
}

func TestFactWritePipeline_ContestedAdjudicatedDelete(t *testing.T) {
	writer := &fakeFactWriter{}
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
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户不再喝咖啡")})
	if res.Deleted != 1 || len(writer.invalidations) != 1 || writer.invalidations[0] != "f1" {
		t.Fatalf("deleted: got %d invalidations %v, want 1/[f1]", res.Deleted, writer.invalidations)
	}
}

func TestFactWritePipeline_AdjudicatorNilFallsBackHeuristic(t *testing.T) {
	writer := &fakeFactWriter{}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.85, FactKind: "preference"},
		}},
		Embedder: &fakeEmbedder{},
		Reader:   &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢咖啡", "preference")}},
		Writer:   writer,
		LG:       loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户现在只喝茶")})
	if res.Added != 1 {
		t.Fatalf("contested without adjudicator must fall back to add, got %+v", res)
	}
}

func TestFactWritePipeline_AdjudicatorErrorFallsBackHeuristic(t *testing.T) {
	writer := &fakeFactWriter{}
	adj := &fakeAdjudicator{err: errors.New("llm down")}
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Searcher: &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{
			{FactID: "f1", Score: 0.85, FactKind: "preference"},
		}},
		Embedder:    &fakeEmbedder{},
		Reader:      &fakeFactRowReader{rows: [][]byte{factRowJSON("f1", "用户喜欢咖啡", "preference")}},
		Writer:      writer,
		Adjudicator: adj,
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户现在只喝茶")})
	if res.Added != 1 {
		t.Fatalf("adjudicator error must fall back to add, got %+v", res)
	}
}

func TestFactWritePipeline_VerdictTargetNotNeighborDowngradesToAdd(t *testing.T) {
	writer := &fakeFactWriter{}
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
		LG:          loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户现在只喝茶")})
	if res.Added != 1 || len(writer.upserts) != 1 {
		t.Fatalf("invalid target must downgrade to add, got %+v", res)
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
