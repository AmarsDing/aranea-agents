package memoryremember

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// --- fakes ---

type fakeWriter struct {
	writes []biz.MemoryFactWrite
	result *biz.ConsolidationResult
	err    error
}

func (f *fakeWriter) UpsertFactsAndEpisodeBatch(_ context.Context, facts []biz.MemoryFactWrite, _ *biz.EpisodeWrite) (*biz.ConsolidationResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.writes = append(f.writes, facts...)
	if f.result != nil {
		return f.result, nil
	}
	return &biz.ConsolidationResult{FactRows: [][]byte{[]byte(`{"id":"fact-new-1"}`)}, FactsWritten: 1}, nil
}

type fakeDetector struct {
	decision biz.MemoryConflictDecision
	err      error
	// captured args
	kind      string
	statement string
	agentID   string
	userID    string
}

func (f *fakeDetector) DetectConflict(_ context.Context, agentID, userID, factKind, statement string) (biz.MemoryConflictDecision, error) {
	f.agentID, f.userID, f.kind, f.statement = agentID, userID, factKind, statement
	if f.err != nil {
		return biz.MemoryConflictDecision{Action: biz.ConflictActionNone}, f.err
	}
	return f.decision, nil
}

type fakeConflictStore struct {
	supersedeOld, supersedeNew []string
	markBatches                [][]string
	supersedeErr               error
	markErr                    error
}

func (f *fakeConflictStore) IncrementConflictCount(_ context.Context, _ string) (int32, error) {
	return 1, nil
}
func (f *fakeConflictStore) ListConflictingFacts(_ context.Context, _, _ string, _, _ int32) ([][]byte, int32, error) {
	return nil, 0, nil
}
func (f *fakeConflictStore) BatchIncrementConflictCounts(_ context.Context, ids []string) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.markBatches = append(f.markBatches, ids)
	return nil
}
func (f *fakeConflictStore) SupersedeFact(_ context.Context, oldID, newID string) error {
	if f.supersedeErr != nil {
		return f.supersedeErr
	}
	f.supersedeOld = append(f.supersedeOld, oldID)
	f.supersedeNew = append(f.supersedeNew, newID)
	return nil
}

// --- helpers ---

func ctxWithSession(userID, sessionID string) context.Context {
	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: sessionID, UserID: userID}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func callTool(t *testing.T, deps Deps, ctx context.Context, args string) rememberOutput {
	t.Helper()
	tl := NewRememberTool(deps)
	if tl == nil {
		t.Fatal("expected non-nil tool")
	}
	out, err := tl.Call(ctx, []byte(args))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	o, ok := out.(rememberOutput)
	if !ok {
		t.Fatalf("unexpected output type %T", out)
	}
	return o
}

func baseDeps() Deps {
	return Deps{
		Writer:        &fakeWriter{},
		AgentID:       "agent-1",
		ConflictStore: &fakeConflictStore{},
	}
}

// --- tests ---

func TestNewRememberTool_NilWriterReturnsNil(t *testing.T) {
	if tl := NewRememberTool(Deps{AgentID: "agent-1"}); tl != nil {
		t.Fatal("nil writer must yield nil tool (assembly skips registration)")
	}
}

func TestRemember_CreatesPreferenceFactByDefault(t *testing.T) {
	w := &fakeWriter{}
	deps := baseDeps()
	deps.Writer = w
	out := callTool(t, deps, ctxWithSession("user-1", "sess-1"), `{"statement":"喜欢简洁的回答"}`)
	if out.Action != "created" || out.FactID != "fact-new-1" || out.Kind != "preference" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(w.writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(w.writes))
	}
	fw := w.writes[0]
	if fw.Statement != "喜欢简洁的回答" || fw.FactKind != "preference" {
		t.Fatalf("unexpected fact write: %+v", fw)
	}
	if fw.ScopeType != "user" || fw.ScopeID != "user-1" || fw.UserID != "user-1" || fw.AgentID != "agent-1" {
		t.Fatalf("identity/scope mismatch: %+v", fw)
	}
	if fw.SourceKind != "explicit" || fw.Status != "active" {
		t.Fatalf("source/status mismatch: %+v", fw)
	}
	if fw.Confidence != 0.95 || fw.Importance != 0.8 {
		t.Fatalf("confidence/importance mismatch: %+v", fw)
	}
	if fw.SourceSessionID != "sess-1" {
		t.Fatalf("source session mismatch: %+v", fw)
	}
}

func TestRemember_ConstraintKindPassesThrough(t *testing.T) {
	w := &fakeWriter{}
	deps := baseDeps()
	deps.Writer = w
	out := callTool(t, deps, ctxWithSession("user-1", "sess-1"), `{"statement":"不要再使用 emoji","kind":"constraint"}`)
	if out.Kind != "constraint" {
		t.Fatalf("unexpected kind: %+v", out)
	}
	if w.writes[0].FactKind != "constraint" {
		t.Fatalf("write kind mismatch: %+v", w.writes[0])
	}
}

func TestRemember_InvalidKind(t *testing.T) {
	tl := NewRememberTool(baseDeps())
	if _, err := tl.Call(ctxWithSession("user-1", "sess-1"), []byte(`{"statement":"x","kind":"knowledge"}`)); err == nil {
		t.Fatal("invalid kind must error")
	}
}

func TestRemember_EmptyStatement(t *testing.T) {
	tl := NewRememberTool(baseDeps())
	if _, err := tl.Call(ctxWithSession("user-1", "sess-1"), []byte(`{"statement":"  "}`)); err == nil {
		t.Fatal("empty statement must error")
	}
}

func TestRemember_MissingInvocationUser(t *testing.T) {
	tl := NewRememberTool(baseDeps())
	if _, err := tl.Call(context.Background(), []byte(`{"statement":"x"}`)); err == nil {
		t.Fatal("missing invocation must error")
	}
	inv := trpcagent.NewInvocation()
	inv.Session = &trpcsession.Session{ID: "s", UserID: ""}
	ctx := trpcagent.NewInvocationContext(context.Background(), inv)
	if _, err := tl.Call(ctx, []byte(`{"statement":"x"}`)); err == nil {
		t.Fatal("empty session user must error")
	}
}

func TestRemember_SupersedeDecisionApplied(t *testing.T) {
	store := &fakeConflictStore{}
	det := &fakeDetector{decision: biz.MemoryConflictDecision{
		Action: biz.ConflictActionSupersede, TargetFactID: "fact-old-1", Score: 0.95,
	}}
	deps := baseDeps()
	deps.Detector = det
	deps.ConflictStore = store
	out := callTool(t, deps, ctxWithSession("user-1", "sess-1"), `{"statement":"喜欢极简回答"}`)
	if out.Action != "superseded" || out.TargetFactID != "fact-old-1" || out.FactID != "fact-new-1" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(store.supersedeOld) != 1 || store.supersedeOld[0] != "fact-old-1" || store.supersedeNew[0] != "fact-new-1" {
		t.Fatalf("supersede not applied: old=%v new=%v", store.supersedeOld, store.supersedeNew)
	}
	if det.agentID != "agent-1" || det.userID != "user-1" || det.kind != "preference" {
		t.Fatalf("detector args mismatch: %+v", det)
	}
}

func TestRemember_MarkConflictDecisionApplied(t *testing.T) {
	store := &fakeConflictStore{}
	det := &fakeDetector{decision: biz.MemoryConflictDecision{
		Action: biz.ConflictActionMarkConflict, TargetFactID: "fact-old-9", Score: 0.85,
	}}
	deps := baseDeps()
	deps.Detector = det
	deps.ConflictStore = store
	out := callTool(t, deps, ctxWithSession("user-1", "sess-1"), `{"statement":"有时喜欢详细解释"}`)
	if out.Action != "marked_conflict" || out.TargetFactID != "fact-old-9" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if len(store.markBatches) != 1 || len(store.markBatches[0]) != 1 || store.markBatches[0][0] != "fact-old-9" {
		t.Fatalf("mark conflict not applied: %+v", store.markBatches)
	}
}

func TestRemember_GovernanceFailureDoesNotFailWrite(t *testing.T) {
	store := &fakeConflictStore{supersedeErr: errors.New("db down")}
	det := &fakeDetector{decision: biz.MemoryConflictDecision{
		Action: biz.ConflictActionSupersede, TargetFactID: "fact-old-1", Score: 0.95,
	}}
	deps := baseDeps()
	deps.Detector = det
	deps.ConflictStore = store
	out := callTool(t, deps, ctxWithSession("user-1", "sess-1"), `{"statement":"x"}`)
	if out.FactID != "fact-new-1" {
		t.Fatalf("write must succeed despite governance failure: %+v", out)
	}
}

func TestRemember_WriterFailurePropagates(t *testing.T) {
	deps := baseDeps()
	deps.Writer = &fakeWriter{err: errors.New("db down")}
	tl := NewRememberTool(deps)
	if _, err := tl.Call(ctxWithSession("user-1", "sess-1"), []byte(`{"statement":"x"}`)); err == nil {
		t.Fatal("writer failure must propagate")
	}
}

func TestRemember_NoDetectorStillWrites(t *testing.T) {
	deps := baseDeps()
	deps.Detector = nil
	deps.ConflictStore = nil
	out := callTool(t, deps, ctxWithSession("user-1", "sess-1"), `{"statement":"x"}`)
	if out.Action != "created" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestRemember_OutputJSONShape(t *testing.T) {
	deps := baseDeps()
	tl := NewRememberTool(deps)
	out, err := tl.Call(ctxWithSession("user-1", "sess-1"), []byte(`{"statement":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		t.Fatal("output must marshal to JSON object")
	}
	for _, k := range []string{"fact_id", "action", "kind"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %q in %s", k, string(b))
		}
	}
}
