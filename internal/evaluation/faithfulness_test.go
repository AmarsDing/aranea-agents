package evaluation

import (
	"testing"

	"aranea-agents/internal/biz"
	evalset "trpc.group/trpc-go/trpc-agent-go/evaluation/evalset"
)

func TestLexicalFaithfulnessSupported(t *testing.T) {
	score := lexicalFaithfulness("The server rack is in room A", "server rack lives in room A next to UPS")
	if score < 0.4 {
		t.Fatalf("expected overlap, got %v", score)
	}
}

func TestApplyFaithfulnessNAWithoutRetrieval(t *testing.T) {
	res := biz.EvalCaseResult{ActualOutput: "hello", ScoresJSON: "{}"}
	applyFaithfulness(&res, &evalset.Invocation{})
	if _, ok := biz.ParseEvalScores(res.ScoresJSON)[metricFaithfulness]; ok {
		t.Fatal("no retrieval must leave faithfulness as N/A")
	}
}

func TestApplyFaithfulnessScoresRetrieval(t *testing.T) {
	res := biz.EvalCaseResult{ActualOutput: "rack in room A", ScoresJSON: "{}"}
	applyFaithfulness(&res, &evalset.Invocation{
		Tools: []*evalset.Tool{{
			Name:   "knowledge_search",
			Result: map[string]any{"chunks": []any{map[string]any{"content": "the rack is in room A"}}},
		}},
	})
	v, ok := biz.ParseEvalScores(res.ScoresJSON)[metricFaithfulness]
	if !ok || v <= 0 {
		t.Fatalf("expected faithfulness score, got ok=%v v=%v json=%s", ok, v, res.ScoresJSON)
	}
}
