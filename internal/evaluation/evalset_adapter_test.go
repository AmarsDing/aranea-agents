package evaluation

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestBizCasesToEvalSet(t *testing.T) {
	t.Parallel()
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1", Name: "test"}, []biz.EvalCase{
		{ID: "c1", Input: "hello", ExpectedOutput: "world"},
	}, loggateway.NewNoop())
	if es.EvalSetID != "ds-1" || len(es.EvalCases) != 1 {
		t.Fatalf("unexpected evalset: %+v", es)
	}
	if es.EvalCases[0].EvalID != "c1" || len(es.EvalCases[0].Conversation) != 1 {
		t.Fatalf("eval id %q conv %d", es.EvalCases[0].EvalID, len(es.EvalCases[0].Conversation))
	}
}

func TestBizCasesToEvalSetMultiTurn(t *testing.T) {
	t.Parallel()
	meta := `{"turns":[{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"bye"}]}`
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1"}, []biz.EvalCase{{
		ID: "c2", Input: "ignored", ExpectedOutput: "farewell", MetadataJSON: meta,
	}}, loggateway.NewNoop())
	if len(es.EvalCases) != 1 || len(es.EvalCases[0].Conversation) != 2 {
		t.Fatalf("expected 2 invocations, got %d", len(es.EvalCases[0].Conversation))
	}
}

// ISSUE-005: the framework hard-requires SessionInput on every eval case
// (local inference fails with "session input is nil" otherwise). The adapter
// must always populate it, with metadata overrides for user_id/state.
func TestBizCasesToEvalSetSetsSessionInput(t *testing.T) {
	t.Parallel()
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1"}, []biz.EvalCase{
		{ID: "c1", Input: "hello", ExpectedOutput: "world"},
	}, loggateway.NewNoop())
	if len(es.EvalCases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(es.EvalCases))
	}
	si := es.EvalCases[0].SessionInput
	if si == nil {
		t.Fatal("SessionInput must be set; framework inference requires it")
	}
	if si.AppName != AppName {
		t.Fatalf("expected AppName %q, got %q", AppName, si.AppName)
	}
	if si.UserID == "" {
		t.Fatal("expected a default UserID")
	}
	if si.State == nil {
		t.Fatal("expected non-nil State map")
	}
}

func TestBizCasesToEvalSetSessionInputMetadataOverride(t *testing.T) {
	t.Parallel()
	meta := `{"session_user_id":"user-42","session_state":{"tenant":"acme"}}`
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1"}, []biz.EvalCase{
		{ID: "c1", Input: "hello", ExpectedOutput: "world", MetadataJSON: meta},
	}, loggateway.NewNoop())
	si := es.EvalCases[0].SessionInput
	if si == nil {
		t.Fatal("SessionInput must be set")
	}
	if si.UserID != "user-42" {
		t.Fatalf("expected metadata user override, got %q", si.UserID)
	}
	if si.State["tenant"] != "acme" {
		t.Fatalf("expected metadata state override, got %v", si.State)
	}
}
