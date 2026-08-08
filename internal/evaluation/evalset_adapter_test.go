package evaluation

import (
	"strings"
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

// P3-2 (pulled into P1): a case-level rubric in metadata_json.rubric must be
// forwarded to the framework as an EvalCaseRubric bound to the llm_as_judge
// metric instance, so the judge scores against the case's scoring standard.
func TestBizCaseToEvalCaseAttachesRubric(t *testing.T) {
	t.Parallel()
	meta := `{"rubric":"回答必须给出具体数字，且不得解释计算过程"}`
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1"}, []biz.EvalCase{
		{ID: "c1", Input: "1+1?", ExpectedOutput: "2", MetadataJSON: meta},
	}, loggateway.NewNoop())
	rubrics := es.EvalCases[0].Rubrics
	if len(rubrics) != 1 {
		t.Fatalf("expected 1 case rubric, got %d", len(rubrics))
	}
	r := rubrics[0]
	if r.MetricName != MetricLLMAsJudge {
		t.Fatalf("rubric must target %q, got %q", MetricLLMAsJudge, r.MetricName)
	}
	if r.Content == nil || r.Content.Text != "回答必须给出具体数字，且不得解释计算过程" {
		t.Fatalf("unexpected rubric content: %+v", r.Content)
	}
	if r.ID == "" {
		t.Fatal("rubric ID must be set (framework requires unique rubric IDs)")
	}
}

// The llm_rubric_response evaluator hard-requires at least one rubric per case
// ("llm judge rubrics are required"). Cases without a custom rubric must get a
// synthesized default: reference-consistency when expected output exists,
// generic quality otherwise.
func TestBizCaseToEvalCaseWithoutRubricGetsDefaultRubric(t *testing.T) {
	t.Parallel()
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1"}, []biz.EvalCase{
		{ID: "c1", Input: "hello", ExpectedOutput: "world"},
		{ID: "c2", Input: "free-form"},
	}, loggateway.NewNoop())

	withExpected := es.EvalCases[0].Rubrics
	if len(withExpected) != 1 {
		t.Fatalf("expected 1 default rubric, got %+v", withExpected)
	}
	if withExpected[0].MetricName != MetricLLMAsJudge {
		t.Fatalf("default rubric must target %q, got %q", MetricLLMAsJudge, withExpected[0].MetricName)
	}
	if withExpected[0].ID == "" {
		t.Fatal("default rubric ID must be set (structured output requires unique IDs)")
	}
	if !strings.Contains(withExpected[0].Content.Text, "world") {
		t.Fatalf("default rubric must embed expected output, got %q", withExpected[0].Content.Text)
	}

	withoutExpected := es.EvalCases[1].Rubrics
	if len(withoutExpected) != 1 {
		t.Fatalf("expected 1 generic default rubric, got %+v", withoutExpected)
	}
	if strings.Contains(withoutExpected[0].Content.Text, "参考答案") {
		t.Fatalf("generic default rubric must not reference an answer, got %q", withoutExpected[0].Content.Text)
	}
}

// A custom rubric is authoritative: it replaces the synthesized default rather
// than being appended alongside it.
func TestBizCaseToEvalCaseCustomRubricReplacesDefault(t *testing.T) {
	t.Parallel()
	meta := `{"rubric":"回答必须给出具体数字"}`
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1"}, []biz.EvalCase{
		{ID: "c1", Input: "1+1?", ExpectedOutput: "2", MetadataJSON: meta},
	}, loggateway.NewNoop())
	rubrics := es.EvalCases[0].Rubrics
	if len(rubrics) != 1 {
		t.Fatalf("custom rubric must replace default, got %d rubrics", len(rubrics))
	}
	if rubrics[0].Content.Text != "回答必须给出具体数字" {
		t.Fatalf("unexpected rubric text: %q", rubrics[0].Content.Text)
	}
}

// The framework fails a case whose rubric references a metric instance that
// was not registered for the run ("metric llm_as_judge not found"). When the
// run does not compute llm_as_judge, rubrics must be stripped instead.
func TestStripCaseRubricsWhenNoJudge(t *testing.T) {
	t.Parallel()
	meta := `{"rubric":"评分标准"}`
	es := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1"}, []biz.EvalCase{
		{ID: "c1", Input: "q", ExpectedOutput: "a", MetadataJSON: meta},
	}, loggateway.NewNoop())

	stripCaseRubricsWhenNoJudge(es, map[string]bool{MetricExactMatch: true})
	if len(es.EvalCases[0].Rubrics) != 0 {
		t.Fatal("rubrics must be stripped when llm_as_judge is not computed")
	}

	es2 := BizCasesToEvalSet(biz.EvalDataset{ID: "ds-1"}, []biz.EvalCase{
		{ID: "c1", Input: "q", ExpectedOutput: "a", MetadataJSON: meta},
	}, loggateway.NewNoop())
	stripCaseRubricsWhenNoJudge(es2, map[string]bool{MetricLLMAsJudge: true})
	if len(es2.EvalCases[0].Rubrics) != 1 {
		t.Fatal("rubrics must be kept when llm_as_judge is computed")
	}
}
