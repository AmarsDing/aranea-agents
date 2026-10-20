package biz

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"testing"
)

// TestFactWritePending_EndToEndRejectChain covers the reject half: a withheld
// DELETE stays rejected with the target fact never invalidated.
func TestFactWritePending_EndToEndRejectChain(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpDelete, TargetFactID: "fact-old"},
	}}
	p := contestedFixture(adj, pending, writer)
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	if res.Pended != 1 || len(pending.recs) != 1 {
		t.Fatalf("pipeline must withhold: res=%+v recs=%d", res, len(pending.recs))
	}

	dec := NewMemoryFactPendingDecider(pending, writer, nil, nil, nil, loggateway.NewNoop())
	applied, err := dec.Decide(context.Background(), pending.recs[0].ID, MemoryFactDecisionDeny, "admin", "误删")
	if !applied || err != nil {
		t.Fatalf("reject chain: applied=%v err=%v", applied, err)
	}
	if len(writer.invalidations) != 0 || len(writer.txUpserts) != 0 || len(writer.upserts) != 0 {
		t.Fatalf("reject must leave storage untouched: %+v", writer)
	}
	if pending.recs[0].Status != MemoryFactPendingStatusRejected || pending.recs[0].Approver != "admin" {
		t.Fatalf("row must record rejection trail: %+v", pending.recs[0])
	}
}

// fakeAllowRuleStore is an in-memory MemoryFactAllowRuleStore; err injects a
// store failure for the fail-closed branch.
type fakeAllowRuleStore struct {
	rules map[string]bool
	err   error
}

func newFakeAllowRuleStore() *fakeAllowRuleStore {
	return &fakeAllowRuleStore{rules: map[string]bool{}}
}

func allowRuleKey(agentID, verdict string) string { return agentID + "\x00" + verdict }

func (f *fakeAllowRuleStore) GrantAllowRule(_ context.Context, agentID, verdict, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.rules[allowRuleKey(agentID, verdict)] = true
	return nil
}

func (f *fakeAllowRuleStore) HasAllowRule(_ context.Context, agentID, verdict string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.rules[allowRuleKey(agentID, verdict)], nil
}

func (f *fakeAllowRuleStore) RevokeAllowRule(_ context.Context, agentID, verdict string) (bool, error) {
	k := allowRuleKey(agentID, verdict)
	if !f.rules[k] {
		return false, nil
	}
	delete(f.rules, k)
	return true, nil
}

func (f *fakeAllowRuleStore) ListAllowRules(_ context.Context, _ string, _ int) ([]MemoryFactAllowRule, error) {
	return nil, nil
}

func TestNormalizeMemoryFactDecision(t *testing.T) {
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"approve", MemoryFactDecisionApprove, true},
		{"approved", MemoryFactDecisionApprove, true},
		{"__aranea:tool_confirm:approve", MemoryFactDecisionApprove, true},
		{"deny", MemoryFactDecisionDeny, true},
		{"reject", MemoryFactDecisionDeny, true},
		{" Approve_Session ", MemoryFactDecisionApproveSession, true},
		{"__aranea:tool_confirm:approve_session", MemoryFactDecisionApproveSession, true},
		{"approve_always", MemoryFactDecisionApproveAlways, true},
		{"always", MemoryFactDecisionApproveAlways, true},
		{"", "", false},
		{"maybe", "", false},
	}
	for _, tc := range cases {
		got, ok := NormalizeMemoryFactDecision(tc.raw)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("Normalize(%q) = (%q,%v), want (%q,%v)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}

// TestMemoryFactPendingDecider_ApproveAlwaysPersistsRule pins E4: the
// approve_always tier replays the write AND persists an (agent_id, verdict)
// allow rule; plain approve must NOT persist one.
func TestMemoryFactPendingDecider_ApproveAlwaysPersistsRule(t *testing.T) {
	pending, writer, _ := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	rules := newFakeAllowRuleStore()
	dec := NewMemoryFactPendingDecider(pending, writer, nil, rules, nil, loggateway.NewNoop())
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApproveAlways, "admin", "")
	if !applied || err != nil {
		t.Fatalf("approve_always: applied=%v err=%v", applied, err)
	}
	if len(writer.txUpserts) != 1 {
		t.Fatalf("approve_always must replay: %+v", writer)
	}
	if !rules.rules[allowRuleKey("a1", MemoryFactPendingVerdictUpdate)] {
		t.Fatalf("allow rule must persist (a1, UPDATE): %+v", rules.rules)
	}
	if rules.rules[allowRuleKey("a1", MemoryFactPendingVerdictDelete)] {
		t.Fatal("rule must be verdict-scoped, DELETE must not be granted")
	}

	pending2, writer2, _ := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	dec2 := NewMemoryFactPendingDecider(pending2, writer2, nil, rules, nil, loggateway.NewNoop())
	applied, err = dec2.Decide(context.Background(), "mfp-1", MemoryFactDecisionApprove, "admin", "")
	if !applied || err != nil {
		t.Fatalf("approve: applied=%v err=%v", applied, err)
	}
	if len(rules.rules) != 1 {
		t.Fatalf("plain approve must not persist rules: %+v", rules.rules)
	}
}

// TestMemoryFactPendingDecider_ApproveSessionGrantsSession pins E4 session
// scope: grant lands only when the snapshot carries a source session, and is
// keyed (agent, verdict, session).
func TestMemoryFactPendingDecider_ApproveSessionGrantsSession(t *testing.T) {
	pending, writer, _ := newDeciderFixture(MemoryFactPendingVerdictUpdate)

	snap, _ := unmarshalFactWriteDecisionSnapshot(pending.recs[0].PayloadJSON)
	snap.Candidate.SourceSessionID = "sess-9"
	pending.recs[0].PayloadJSON = MarshalFactWriteDecisionSnapshot(snap)

	grants := NewMemoryFactSessionGrants()
	dec := NewMemoryFactPendingDecider(pending, writer, nil, nil, grants, loggateway.NewNoop())
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApproveSession, "admin", "")
	if !applied || err != nil {
		t.Fatalf("approve_session: applied=%v err=%v", applied, err)
	}
	if !grants.Has("a1", MemoryFactPendingVerdictUpdate, "sess-9") {
		t.Fatal("session grant must be recorded for (a1, UPDATE, sess-9)")
	}
	if grants.Has("a1", MemoryFactPendingVerdictUpdate, "sess-other") ||
		grants.Has("a1", MemoryFactPendingVerdictDelete, "sess-9") {
		t.Fatal("session grant must be (agent, verdict, session)-scoped")
	}

	pending2, writer2, _ := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	grants2 := NewMemoryFactSessionGrants()
	dec2 := NewMemoryFactPendingDecider(pending2, writer2, nil, nil, grants2, loggateway.NewNoop())
	applied, err = dec2.Decide(context.Background(), "mfp-1", MemoryFactDecisionApproveSession, "admin", "")
	if !applied || err != nil {
		t.Fatalf("session-less approve_session: applied=%v err=%v", applied, err)
	}
	if len(writer2.txUpserts) != 1 {
		t.Fatal("session-less approve_session must still replay")
	}
}

// TestMemoryFactWriteBypassed pins the gate-bypass matrix: session grant hit,
// always-rule hit, miss, and store-error fail-closed.
func TestMemoryFactWriteBypassed(t *testing.T) {
	ctx := context.Background()
	d := FactWriteDecision{Candidate: FactWriteCandidate{AgentID: "a1", SourceSessionID: "s1"}}
	grants := NewMemoryFactSessionGrants()
	grants.Grant("a1", MemoryFactPendingVerdictUpdate, "s1")
	rules := newFakeAllowRuleStore()
	rules.rules[allowRuleKey("a1", MemoryFactPendingVerdictDelete)] = true

	if !MemoryFactWriteBypassed(ctx, rules, grants, d, MemoryFactPendingVerdictUpdate) {
		t.Fatal("session grant must bypass")
	}
	if !MemoryFactWriteBypassed(ctx, rules, grants, d, MemoryFactPendingVerdictDelete) {
		t.Fatal("always rule must bypass")
	}
	if MemoryFactWriteBypassed(ctx, rules, grants, d, MemoryFactPendingVerdictContested) {
		t.Fatal("no grant/rule → must not bypass")
	}

	if MemoryFactWriteBypassed(ctx, nil, nil, d, MemoryFactPendingVerdictUpdate) {
		t.Fatal("nil stores must not bypass")
	}

	rulesErr := &fakeAllowRuleStore{err: errors.New("db down")}
	dNoSession := FactWriteDecision{Candidate: FactWriteCandidate{AgentID: "a1"}}
	if MemoryFactWriteBypassed(ctx, rulesErr, nil, dNoSession, MemoryFactPendingVerdictUpdate) {
		t.Fatal("store error must fail closed")
	}

	g2 := NewMemoryFactSessionGrants()
	g2.Grant("a1", MemoryFactPendingVerdictUpdate, "")
	if MemoryFactWriteBypassed(ctx, nil, g2, dNoSession, MemoryFactPendingVerdictUpdate) {
		t.Fatal("empty session must not hold a grant")
	}
}

// TestFactWriteGate_AllowRuleBypassesPend pins the pipeline bypass: an
// (agent, UPDATE) allow rule turns an adjudicated UPDATE into a direct write
// (no pending row), with the same write path as an ungated decision.
func TestFactWriteGate_AllowRuleBypassesPend(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	rules := newFakeAllowRuleStore()
	rules.rules[allowRuleKey("ag1", MemoryFactPendingVerdictUpdate)] = true
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		{Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpUpdate, TargetFactID: "fact-old"},
	}}
	p := contestedFixture(adj, pending, writer)
	p.allowRules = rules
	res := p.Apply(context.Background(), []FactWriteCandidate{pipelineCandidate("用户偏好编辑器为 Neovim")})
	if res.Updated != 1 || res.Pended != 0 {
		t.Fatalf("allow rule must bypass pend: %+v", res)
	}
	if len(pending.recs) != 0 {
		t.Fatalf("bypass must not create pending rows: %+v", pending.recs)
	}
	if len(writer.txUpserts) != 1 || writer.txOldIDs[0] != "fact-old" {
		t.Fatalf("bypass must execute the original UPDATE: %+v", writer)
	}
}

// TestFactWriteGate_SessionGrantBypassesPend pins session-scoped bypass:
// grant (agent, verdict, session) + candidate from that session → direct
// write; a candidate from another session still pends.
func TestFactWriteGate_SessionGrantBypassesPend(t *testing.T) {
	writer := &fakeFactWriter{}
	pending := &fakePendingStore{}
	grants := NewMemoryFactSessionGrants()
	grants.Grant("ag1", MemoryFactPendingVerdictUpdate, "sess-1")
	adj := &fakeAdjudicator{verdicts: []FactAdjudicationVerdict{
		// Ordinal:-1 = legacy statement-FIFO matching (P2-4: zero value 0 would
		// be read as a real ordinal and the two verdicts would collide at 0).
		{Ordinal: -1, Statement: "用户偏好编辑器为 Neovim", Operation: FactWriteOpUpdate, TargetFactID: "fact-old"},
		{Ordinal: -1, Statement: "用户偏好主题为暗色", Operation: FactWriteOpUpdate, TargetFactID: "fact-old"},
	}}
	p := contestedFixture(adj, pending, writer)
	p.sessionGrants = grants
	inSess := pipelineCandidate("用户偏好编辑器为 Neovim")
	inSess.SourceSessionID = "sess-1"
	offSess := pipelineCandidate("用户偏好主题为暗色")
	offSess.SourceSessionID = "sess-2"
	res := p.Apply(context.Background(), []FactWriteCandidate{inSess, offSess})
	if res.Updated != 1 || res.Pended != 1 {
		t.Fatalf("session bypass matrix: %+v, want Updated=1 Pended=1", res)
	}
}

// TestMemoryFactPendingDecider_UnknownDecisionNoop pins the fail-closed gate:
// an unrecognized decision string neither transitions the row nor replays.
func TestMemoryFactPendingDecider_UnknownDecisionNoop(t *testing.T) {
	pending, writer, dec := newDeciderFixture(MemoryFactPendingVerdictUpdate)
	applied, err := dec.Decide(context.Background(), "mfp-1", "maybe", "admin", "")
	if applied || err != nil {
		t.Fatalf("unknown decision must be a no-op: applied=%v err=%v", applied, err)
	}
	if pending.recs[0].Status != MemoryFactPendingStatusPending {
		t.Fatalf("row must stay pending: %+v", pending.recs[0])
	}
	if len(writer.txUpserts) != 0 || len(writer.upserts) != 0 || len(writer.invalidations) != 0 {
		t.Fatalf("unknown decision must not touch storage: %+v", writer)
	}
}

// TestMemoryFactPendingDecider_DeleteSoftArchiveAudit pins R3 3.4: an
// approved DELETE soft-archives (invalidate only, no row removal) and writes
// a delete_archived audit against the target fact for the restore trail.
func TestMemoryFactPendingDecider_DeleteSoftArchiveAudit(t *testing.T) {
	pending, writer, _ := newDeciderFixture(MemoryFactPendingVerdictDelete)
	alog := &fakeActionLog{}
	dec := NewMemoryFactPendingDecider(pending, writer, alog, nil, nil, loggateway.NewNoop())
	applied, err := dec.Decide(context.Background(), "mfp-1", MemoryFactDecisionApprove, "admin", "")
	if !applied || err != nil {
		t.Fatalf("approve DELETE: applied=%v err=%v", applied, err)
	}
	if len(writer.invalidations) != 1 || writer.invalidations[0] != "fact-old" {
		t.Fatalf("DELETE must invalidate (soft-archive) the target: %+v", writer)
	}
	var archived *MemoryPolicyRecord
	for i := range alog.recs {
		if alog.recs[i].Action == "fact_write_pending.delete_archived" {
			archived = &alog.recs[i]
		}
	}
	if archived == nil {
		t.Fatalf("delete_archived audit missing: %+v", alog.recs)
	}
	if archived.TargetKind != "memory_fact" || archived.TargetID != "fact-old" {
		t.Fatalf("archive audit must target the fact row: %+v", archived)
	}
}
