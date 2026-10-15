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
	recs []MemoryFactPendingRecord
	err  error
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

func (f *fakePendingStore) MarkDecided(_ context.Context, id, status, approver string, decidedAt int64) (bool, error) {
	// 真实单发语义：仅 pending 行可迁（与 repo 的 WHERE status='pending' 对齐）。
	for i := range f.recs {
		if f.recs[i].ID == id && f.recs[i].Status == MemoryFactPendingStatusPending {
			f.recs[i].Status = status
			f.recs[i].Approver = approver
			f.recs[i].DecidedAt = decidedAt
			return true, nil
		}
	}
	return false, nil
}

// --- RouteFactWriteDecision (pure) ---

func TestRouteFactWriteDecision(t *testing.T) {
	cases := []struct {
		name        string
		d           FactWriteDecision
		wantVerdict string
		wantPend    bool
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

// --- Phase 3.3: snapshot + decider ---

// TestFactWriteDecisionSnapshot_Roundtrip verifies the payload_json snapshot
// preserves the full candidate metadata (kind/confidence/importance/scope/
// source) needed for a faithful replay.
func TestFactWriteDecisionSnapshot_Roundtrip(t *testing.T) {
	orig := FactWriteDecision{
		Candidate: FactWriteCandidate{
			Statement: "值班电话 8899-1234", FactKind: "contact",
			Confidence: 0.9, Importance: 0.8, TagsJSON: `["duty"]`,
			ScopeType: "agent", ScopeID: "duty-phone", UserID: "u1", AgentID: "a1",
			SourceKind: "auto_memory", SourceEpisodeID: "ep-1",
		},
		Operation: FactWriteOpUpdate, TargetFactID: "fact-old",
	}
	snap := MarshalFactWriteDecisionSnapshot(orig)
	if snap == "" {
		t.Fatal("snapshot must not be empty")
	}
	got, ok := unmarshalFactWriteDecisionSnapshot(snap)
	if !ok {
		t.Fatal("unmarshal failed")
	}
	if got.TargetFactID != "fact-old" || got.Candidate.FactKind != "contact" ||
		got.Candidate.Confidence != 0.9 || got.Candidate.TagsJSON != `["duty"]` ||
		got.Candidate.ScopeID != "duty-phone" || got.Candidate.SourceEpisodeID != "ep-1" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if _, ok := unmarshalFactWriteDecisionSnapshot(""); ok {
		t.Fatal("empty payload must report ok=false")
	}
	if _, ok := unmarshalFactWriteDecisionSnapshot(`{"candidate":{"Statement":"  "}}`); ok {
		t.Fatal("blank statement must report ok=false")
	}
}

// newDeciderFixture builds one pending row with a full snapshot plus the
// decider under test.
func newDeciderFixture(verdict string) (*fakePendingStore, *fakeFactWriter, *MemoryFactPendingDecider) {
	cand := FactWriteCandidate{
		Statement: "用户偏好编辑器为 Neovim", FactKind: "preference",
		Confidence: 0.9, Importance: 0.7, ScopeType: "agent", ScopeID: "a1", AgentID: "a1",
	}
	d := FactWriteDecision{Candidate: cand, TargetFactID: "fact-old"}
	pending := &fakePendingStore{recs: []MemoryFactPendingRecord{{
		ID: "mfp-1", AgentID: "a1", FactKey: "fact-old", Verdict: verdict,
		ProposedBody: cand.Statement, PriorBody: "用户偏好编辑器为 VS Code",
		PayloadJSON: MarshalFactWriteDecisionSnapshot(d),
		Status:      MemoryFactPendingStatusPending, CreatedAt: 1787702400,
	}}}
	writer := &fakeFactWriter{}
	return pending, writer, NewMemoryFactPendingDecider(pending, writer, nil, nil, nil, loggateway.NewNoop())
}

func TestMemoryFactPendingDecider_ApproveUpdateReplays(t *testing.T) {
	pending, writer, dec := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApprove, "admin", "ok")
	if !applied || err != nil {
		t.Fatalf("approve: applied=%v err=%v", applied, err)
	}
	if len(writer.txUpserts) != 1 || writer.txOldIDs[0] != "fact-old" {
		t.Fatalf("UPDATE approve must replay invalidate+upsert on target: %+v", writer)
	}
	up := writer.txUpserts[0]
	if up.FactKind != "preference" || up.Confidence != 0.9 || up.ScopeID != "a1" {
		t.Fatalf("replay must preserve snapshot metadata: %+v", up)
	}
	if pending.recs[0].Status != MemoryFactPendingStatusApproved || pending.recs[0].Approver != "admin" {
		t.Fatalf("row must be approved with approver: %+v", pending.recs[0])
	}
}

func TestMemoryFactPendingDecider_ApproveDeleteReplays(t *testing.T) {
	_, writer, dec := newDeciderFixture(MemoryFactPendingVerdictDelete)
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApprove, "admin", "")
	if !applied || err != nil {
		t.Fatalf("approve: applied=%v err=%v", applied, err)
	}
	if len(writer.invalidations) != 1 || writer.invalidations[0] != "fact-old" {
		t.Fatalf("DELETE approve must invalidate target only: %+v", writer)
	}
	if len(writer.upserts) != 0 || len(writer.txUpserts) != 0 {
		t.Fatalf("DELETE approve must not upsert: %+v", writer)
	}
}

func TestMemoryFactPendingDecider_ApproveContestedAdds(t *testing.T) {
	_, writer, dec := newDeciderFixture(MemoryFactPendingVerdictContested)
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApprove, "admin", "确为新事实")
	if !applied || err != nil {
		t.Fatalf("approve: applied=%v err=%v", applied, err)
	}
	if len(writer.upserts) != 1 || writer.upserts[0].FactKind != "preference" {
		t.Fatalf("CONTESTED approve must plain-add from snapshot: %+v", writer)
	}
}

func TestMemoryFactPendingDecider_RejectNoReplay(t *testing.T) {
	pending, writer, dec := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionDeny, "admin", "重复")
	if !applied || err != nil {
		t.Fatalf("reject: applied=%v err=%v", applied, err)
	}
	if len(writer.txUpserts) != 0 || len(writer.upserts) != 0 || len(writer.invalidations) != 0 {
		t.Fatalf("reject must not touch storage: %+v", writer)
	}
	if pending.recs[0].Status != MemoryFactPendingStatusRejected {
		t.Fatalf("row must be rejected: %+v", pending.recs[0])
	}
}

func TestMemoryFactPendingDecider_DoubleDecisionSingleFire(t *testing.T) {
	_, writer, dec := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	applied, _ := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApprove, "admin", "")
	if !applied {
		t.Fatal("first decide must apply")
	}
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApprove, "admin2", "")
	if applied || err != nil {
		t.Fatalf("second decide must be no-op: applied=%v err=%v", applied, err)
	}
	if len(writer.txUpserts) != 1 {
		t.Fatalf("replay must be single-fire: %d tx upserts", len(writer.txUpserts))
	}
	// Unknown id: applied=false, no error.
	applied, err = dec.Decide(context.Background(), "mfp-absent", MemoryFactDecisionApprove, "admin", "")
	if applied || err != nil {
		t.Fatalf("unknown id: applied=%v err=%v", applied, err)
	}
	// Unknown decision tier: applied=false, row untouched.
	store, _, dec2 := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	applied, err = dec2.Decide(context.Background(), "mfp-1", "maybe", "admin", "")
	if applied || err != nil {
		t.Fatalf("unknown decision: applied=%v err=%v", applied, err)
	}
	if store.recs[0].Status != MemoryFactPendingStatusPending {
		t.Fatalf("unknown decision must not transition row: %+v", store.recs[0])
	}
}

// TestMemoryFactPendingDecider_LegacyRowMinimalReplay covers rows predating
// DDL 20261255 (no payload_json): approve still replays from top-level
// columns instead of failing.
func TestMemoryFactPendingDecider_LegacyRowMinimalReplay(t *testing.T) {
	_, writer, dec := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	// Strip the snapshot → legacy row.
	store, _, _ := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	store.recs[0].PayloadJSON = ""
	dec = NewMemoryFactPendingDecider(store, writer, nil, nil, nil, loggateway.NewNoop())
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApprove, "admin", "")
	if !applied {
		t.Fatalf("legacy approve must apply: applied=%v err=%v", applied, err)
	}
	if len(writer.txUpserts) != 1 || writer.txUpserts[0].Statement != "用户偏好编辑器为 Neovim" {
		t.Fatalf("legacy minimal replay mismatch: %+v", writer)
	}
}

// TestFactWriteGate_PendingStoresSnapshot pins the pipeline pend path storing
// the full decision snapshot into payload_json (R3 3.3 replay prerequisite).
func TestFactWriteGate_PendingStoresSnapshot(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpUpdate, TargetFactID: "fact-old"},
	}}
	p := contestedFixture(adj, pending, writer)
	p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	if len(pending.recs) != 1 {
		t.Fatalf("pending recs = %d, want 1", len(pending.recs))
	}
	snap, ok := unmarshalFactWriteDecisionSnapshot(pending.recs[0].PayloadJSON)
	if !ok {
		t.Fatalf("snapshot missing/invalid: %q", pending.recs[0].PayloadJSON)
	}
	if snap.TargetFactID != "fact-old" || snap.Candidate.Statement != "用户偏好编辑器为 Neovim" {
		t.Fatalf("snapshot mismatch: %+v", snap)
	}
}

// --- Phase 3 验证：管线扣留 → 决议回放端到端链 ---

// TestFactWritePending_EndToEndApproveChain closes the full R3 loop in one
// flow: pipeline withholds an adjudicated UPDATE (writing the payload snapshot
// itself), then the decider approves THAT record and replays the original
// bi-temporal write from the snapshot the pipeline stored — proving the two
// halves agree on the snapshot contract without a hand-built fixture.
func TestFactWritePending_EndToEndApproveChain(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpUpdate, TargetFactID: "fact-old"},
	}}
	p := contestedFixture(adj, pending, writer)
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	if res.Pended != 1 || len(pending.recs) != 1 {
		t.Fatalf("pipeline must withhold: res=%+v recs=%d", res, len(pending.recs))
	}
	if len(writer.txUpserts) != 0 {
		t.Fatalf("withheld write must not touch storage pre-decision: %+v", writer)
	}

	dec := NewMemoryFactPendingDecider(pending, writer, nil, nil, nil, loggateway.NewNoop())
	applied, err := dec.Decide(context.Background(), pending.recs[0].ID, MemoryFactDecisionApprove, "admin", "")
	if !applied || err != nil {
		t.Fatalf("approve chain: applied=%v err=%v", applied, err)
	}
	if len(writer.txUpserts) != 1 || writer.txOldIDs[0] != "fact-old" {
		t.Fatalf("approve must replay invalidate+upsert on target: %+v", writer)
	}
	if writer.txUpserts[0].Statement != "用户偏好编辑器为 Neovim" {
		t.Fatalf("replay must come from the pipeline-stored snapshot: %+v", writer.txUpserts[0])
	}
	if pending.recs[0].Status != MemoryFactPendingStatusApproved || pending.recs[0].Approver != "admin" {
		t.Fatalf("row must record approval trail: %+v", pending.recs[0])
	}
}

// TestFactWritePending_EndToEndRejectChain covers the reject half: a withheld
// DELETE stays rejected with the target fact never invalidated.

// ---------------------------------------------------------------------------
// Phase 3.4 — E4 四档决议（与工具 HITL 对齐）
// ---------------------------------------------------------------------------

// fakeAllowRuleStore is an in-memory MemoryFactAllowRuleStore; err injects a
// store failure for the fail-closed branch.

// TestMemoryFactPendingDecider_ApproveAlwaysPersistsRule pins E4: the
// approve_always tier replays the write AND persists an (agent_id, verdict)
// allow rule; plain approve must NOT persist one.

// Plain approve on a second row: replay but no rule.

// TestMemoryFactPendingDecider_ApproveSessionGrantsSession pins E4 session
// scope: grant lands only when the snapshot carries a source session, and is
// keyed (agent, verdict, session).

// Inject a source session into the snapshot.

// Legacy/session-less snapshot: replay still happens, grant skipped.

// TestMemoryFactWriteBypassed pins the gate-bypass matrix: session grant hit,
// always-rule hit, miss, and store-error fail-closed.

// Nil stores: never bypass.

// Store error: fail closed (pend, not bypass).

// Empty session never holds a grant.

// TestFactWriteGate_AllowRuleBypassesPend pins the pipeline bypass: an
// (agent, UPDATE) allow rule turns an adjudicated UPDATE into a direct write
// (no pending row), with the same write path as an ungated decision.

// TestFactWriteGate_SessionGrantBypassesPend pins session-scoped bypass:
// grant (agent, verdict, session) + candidate from that session → direct
// write; a candidate from another session still pends.

// TestMemoryFactPendingDecider_UnknownDecisionNoop pins the fail-closed gate:
// an unrecognized decision string neither transitions the row nor replays.

// TestMemoryFactPendingDecider_DeleteSoftArchiveAudit pins R3 3.4: an
// approved DELETE soft-archives (invalidate only, no row removal) and writes
// a delete_archived audit against the target fact for the restore trail.
