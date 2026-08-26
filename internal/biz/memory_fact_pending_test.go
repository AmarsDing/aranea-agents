package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// --- fake pending store ---

type fakePendingStore struct {
	recs       []MemoryFactPendingRecord
	err        error
	decideNext bool
}

func (f *fakePendingStore) InsertPending(_ context.Context, rec MemoryFactPendingRecord) error {
	if f.err != nil {
		return f.err
	}
	f.recs = append(f.recs, rec)
	return nil
}

func (f *fakePendingStore) GetPending(_ context.Context, id string) (MemoryFactPendingRecord, bool, error) {
	for _, r := range f.recs {
		if r.ID == id {
			return r, true, nil
		}
	}
	return MemoryFactPendingRecord{}, false, nil
}

func (f *fakePendingStore) ListPending(_ context.Context, _, _ string, _ int) ([]MemoryFactPendingRecord, error) {
	return f.recs, nil
}

func (f *fakePendingStore) MarkDecided(_ context.Context, _, _, _ string, _ int64) (bool, error) {
	return f.decideNext, nil
}

// --- RouteFactWriteDecision (pure) ---

func TestRouteFactWriteDecision(t *testing.T) {
	cases := []struct {
		name       string
		d          FactWriteDecision
		wantVerdict string
		wantPend   bool
	}{
		{"plain ADD direct", FactWriteDecision{Operation: FactWriteOpAdd}, "", false},
		{"NOOP untouched", FactWriteDecision{Operation: FactWriteOpNoop}, "", false},
		{"merge NOOP untouched", FactWriteDecision{Operation: FactWriteOpNoop, TargetFactID: "f1", Contested: true, Adjudicated: true}, "", false},
		{"UPDATE pends", FactWriteDecision{Operation: FactWriteOpUpdate, TargetFactID: "f1", Contested: true, Adjudicated: true}, MemoryFactPendingVerdictUpdate, true},
		{"DELETE pends", FactWriteDecision{Operation: FactWriteOpDelete, TargetFactID: "f1", Contested: true, Adjudicated: true}, MemoryFactPendingVerdictDelete, true},
		{"contested unadjudicated ADD pends", FactWriteDecision{Operation: FactWriteOpAdd, Contested: true}, MemoryFactPendingVerdictContested, true},
		{"contested adjudicated ADD direct", FactWriteDecision{Operation: FactWriteOpAdd, Contested: true, Adjudicated: true}, "", false},
		{"uncontested heuristic ADD direct", FactWriteDecision{Operation: FactWriteOpAdd, Contested: false}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict, pend := RouteFactWriteDecision(tc.d)
			if pend != tc.wantPend || verdict != tc.wantVerdict {
				t.Fatalf("Route(%+v) = (%q,%v), want (%q,%v)", tc.d, verdict, pend, tc.wantVerdict, tc.wantPend)
			}
		})
	}
}

// --- pipeline-level gate behavior ---

// contestedFixture builds a pipeline whose single candidate always lands in
// the contested band (neighbor score 0.85 < merge 0.92 ≥ contested 0.80).
func contestedFixture(adj FactWriteAdjudicator, pending MemoryFactPendingStore, writer *fakeFactWriter) *FactWritePipeline {
	return NewFactWritePipeline(FactWritePipelineDeps{
		Searcher:    &fakeNeighborSearcher{neighbors: []MemoryConflictNeighbor{{FactID: "fact-old", Score: 0.85}}},
		Embedder:    &fakeEmbedder{},
		Reader:      &fakeFactRowReader{rows: [][]byte{factRowJSON("fact-old", "用户偏好编辑器为 VS Code", "preference")}},
		Writer:      writer,
		Adjudicator: adj,
		Pending:     pending,
		LG:          loggateway.NewNoop(),
	})
}

func TestFactWriteGate_UpdateVerdictPends(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpUpdate, TargetFactID: "fact-old"},
	}}
	p := contestedFixture(adj, pending, writer)
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})

	if res.Pended != 1 || res.Updated != 0 || res.Added != 0 {
		t.Fatalf("result = %+v, want Pended=1 only", res)
	}
	if len(writer.txUpserts) != 0 || len(writer.upserts) != 0 {
		t.Fatalf("withheld write must NOT touch storage: %+v", writer)
	}
	if len(pending.recs) != 1 {
		t.Fatalf("pending recs = %d, want 1", len(pending.recs))
	}
	rec := pending.recs[0]
	if rec.Verdict != MemoryFactPendingVerdictUpdate || rec.FactKey != "fact-old" ||
		rec.PriorBody != "用户偏好编辑器为 VS Code" || rec.ProposedBody == "" ||
		rec.Status != MemoryFactPendingStatusPending || rec.AdjudicatorReason != "adjudicated_update" {
		t.Fatalf("pending rec mismatch: %+v", rec)
	}
}

func TestFactWriteGate_DeleteVerdictPends(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpDelete, TargetFactID: "fact-old"},
	}}
	p := contestedFixture(adj, pending, writer)
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	if res.Pended != 1 || res.Deleted != 0 || len(writer.invalidations) != 0 {
		t.Fatalf("delete must pend without invalidation: res=%+v writer=%+v", res, writer)
	}
	if pending.recs[0].Verdict != MemoryFactPendingVerdictDelete {
		t.Fatalf("verdict = %q, want DELETE", pending.recs[0].Verdict)
	}
}

func TestFactWriteGate_AdjudicatorErrorPendsContested(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{err: errors.New("llm down")}
	p := contestedFixture(adj, pending, writer)
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	if res.Pended != 1 || res.Added != 0 || len(writer.upserts) != 0 {
		t.Fatalf("adjudicator error must pend CONTESTED, not heuristic-ADD: res=%+v", res)
	}
	rec := pending.recs[0]
	if rec.Verdict != MemoryFactPendingVerdictContested || rec.AdjudicatorReason != "adjudicator_error" || rec.PriorBody != "" {
		t.Fatalf("contested rec mismatch: %+v", rec)
	}
}

func TestFactWriteGate_AdjudicatedAddWritesDirectly(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpAdd},
	}}
	p := contestedFixture(adj, pending, writer)
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	if res.Added != 1 || res.Pended != 0 || len(pending.recs) != 0 || len(writer.upserts) != 1 {
		t.Fatalf("adjudicated ADD must write directly: res=%+v pending=%d", res, len(pending.recs))
	}
}

func TestFactWriteGate_InvalidTargetPendsContested(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpUpdate, TargetFactID: "hallucinated-id"},
	}}
	p := contestedFixture(adj, pending, writer)
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	if res.Pended != 1 || res.Added != 0 || len(writer.upserts) != 0 {
		t.Fatalf("hallucinated target must pend CONTESTED: res=%+v", res)
	}
	rec := pending.recs[0]
	if rec.Verdict != MemoryFactPendingVerdictContested || !strings.HasPrefix(rec.AdjudicatorReason, "target_not_neighbor:") {
		t.Fatalf("rec mismatch: %+v", rec)
	}
}

func TestFactWriteGate_NilPendingStoreFailClosed(t *testing.T) {
	writer := &fakeFactWriter{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpUpdate, TargetFactID: "fact-old"},
	}}
	p := contestedFixture(adj, nil, writer)
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	// fail-closed: no pending store → write withheld anyway, never direct-written.
	if res.WriteErrs != 1 || res.Updated != 0 || len(writer.txUpserts) != 0 {
		t.Fatalf("nil pending store must withhold (fail-closed): res=%+v writer=%+v", res, writer)
	}
}

func TestFactWriteGate_UncontestedAddUnchanged(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	// No neighbors → not contested → plain ADD direct write.
	p := NewFactWritePipeline(FactWritePipelineDeps{
		Writer:  writer,
		Pending: pending,
		LG:      loggateway.NewNoop(),
	})
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好深色主题")})
	if res.Added != 1 || res.Pended != 0 || len(pending.recs) != 0 {
		t.Fatalf("uncontested ADD must be unchanged: res=%+v pending=%d", res, len(pending.recs))
	}
}
