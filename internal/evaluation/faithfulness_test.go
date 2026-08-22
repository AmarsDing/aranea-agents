package evaluation

import (
	"context"
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
	applyFaithfulness(context.Background(), &res, &evalset.Invocation{}, nil)
	if _, ok := biz.ParseEvalScores(res.ScoresJSON)[metricFaithfulness]; ok {
		t.Fatal("no retrieval must leave faithfulness as N/A")
	}
}

func TestApplyFaithfulnessScoresRetrieval(t *testing.T) {
	res := biz.EvalCaseResult{ActualOutput: "rack in room A", ScoresJSON: "{}"}
	applyFaithfulness(context.Background(), &res, &evalset.Invocation{
		Tools: []*evalset.Tool{{
			Name:   "knowledge_search",
			Result: map[string]any{"chunks": []any{map[string]any{"content": "the rack is in room A"}}},
		}},
	}, nil)
	v, ok := biz.ParseEvalScores(res.ScoresJSON)[metricFaithfulness]
	if !ok || v <= 0 {
		t.Fatalf("expected faithfulness score, got ok=%v v=%v json=%s", ok, v, res.ScoresJSON)
	}
}

func TestParseFaithfulnessScore(t *testing.T) {
	cases := []struct {
		in   string
		want float32
	}{
		{`{"score": 0.75}`, 0.75},
		{"score: 0.4\nreason: grounded", 0.4},
		{"0.9", 0.9},
		{"score: 1.5", 1},
	}
	for _, tc := range cases {
		got, err := parseFaithfulnessScore(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("parse %q: got %v err=%v want %v", tc.in, got, err, tc.want)
		}
	}
	if _, err := parseFaithfulnessScore(""); err == nil {
		t.Fatal("empty response must fail")
	}
}
