package evaluation

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestBizCasesToEvalSet(t *testing.T) {
	t.Parallel()
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1", Name: "test"}, []biz.EvalCase{
		{ID: "c1", Input: "hello", ExpectedOutput: "world"},
	})
	if es.EvalSetID != "ds-1" || len(es.EvalCases) != 1 {
		t.Fatalf("unexpected evalset: %+v", es)
	}
	if es.EvalCases[0].EvalID != "c1" {
		t.Fatalf("eval id %q", es.EvalCases[0].EvalID)
	}
}
