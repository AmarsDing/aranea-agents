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
