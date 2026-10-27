package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/decide"
	"aranea-agents/pkg/loggateway"
)

type shadowCapture struct{ recs []decision.Record }

func (c *shadowCapture) Emit(_ context.Context, r decision.Record) { c.recs = append(c.recs, r) }

// newShadowStore builds a no-DB store with the shadow recorder armed. emb is
// the interface type on purpose: a typed-nil *evalIntentTestEmbedder would
// wrap a nil pointer in the interface and defeat intentRouter's nil guard.
func newShadowStore(emb biz.EmbeddingService) (*evalMemoryStore, *shadowCapture) {
	s := &evalMemoryStore{emb: emb, lg: loggateway.NewNoop()}
	cap := &shadowCapture{}
	s.SetShadowRecorder(decide.NewRecorder(cap, loggateway.NewNoop()))
	return s, cap
}

// T8：读侧 route_mode 双记——达阈最优意图为 key、五意图 margin 全量入
// scores、词法前置模式快照入 state（P29 分歧分析输入）。
func TestShadowRouteMode_EmitsMarginsAndLexicalSnapshot(t *testing.T) {
	s, cap := newShadowStore(newEvalIntentTestEmbedder(nil))
	lexical := biz.EvalQueryAnalysis{Enumerative: true, Aggregate: true}
	s.shadowRouteMode(context.Background(), "user-1", "列出我提过的所有城市",
		evalIntentTestAxis(2, 1), lexical)
	if len(cap.recs) != 1 {
		t.Fatalf("want 1 shadow record, got %d", len(cap.recs))
	}
	rec := cap.recs[0]
	if rec.Category != decision.CategoryMemoryShadow {
		t.Fatalf("category = %q", rec.Category)
	}
	if rec.Outcome != "q=route_mode;k=enumerative" {
		t.Fatalf("outcome = %q", rec.Outcome)
	}
	shadow, _ := rec.Metadata["shadow"].(map[string]any)
	if shadow["question"] != "route_mode" || shadow["plane"] != "read" {
		t.Fatalf("shadow meta = %v", shadow)
	}
	inc, _ := shadow["incumbent"].(decide.Incumbent)
	if len(inc.Scores) != 5 {
		t.Fatalf("want margins for 5 read intents, got %v", inc.Scores)
	}
	if inc.Scores["enumerative"] != 1 {
		t.Fatalf("enumerative margin = %v, want 1", inc.Scores["enumerative"])
	}
	state, _ := shadow["state"].(map[string]any)
	if state["user_message"] != "列出我提过的所有城市" {
		t.Fatalf("state lost query: %v", state)
	}
	modes, _ := state["lexical_modes"].([]string)
	// evalModeSet 输出序：chronological, aggregate, enumerative, ...
	if len(modes) != 2 || modes[0] != "aggregate" || modes[1] != "enumerative" {
		t.Fatalf("lexical snapshot = %v", modes)
	}
}

// T8：VIR 盲区编码——五意图全不达阈时 key=no_recall（近似编码，harvest
// 金标时按最终应答质量复核，见 questionsets.json note）。
func TestShadowRouteMode_AllBelowThresholdEncodesNoRecall(t *testing.T) {
	s, cap := newShadowStore(newEvalIntentTestEmbedder(nil))
	s.shadowRouteMode(context.Background(), "u", "今天天气怎么样",
		evalIntentTestAxis(7, 1), biz.EvalQueryAnalysis{})
	if len(cap.recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(cap.recs))
	}
	if cap.recs[0].Outcome != "q=route_mode;k=no_recall" {
		t.Fatalf("outcome = %q", cap.recs[0].Outcome)
	}
}

// T8：关闭态零行为——无 recorder / 无 emb / 无 embedding 均安全无记录。
func TestShadowRouteMode_DisabledPathsAreSilent(t *testing.T) {
	s := newIntentTestStore(newEvalIntentTestEmbedder(nil))
	s.shadowRouteMode(context.Background(), "u", "q", evalIntentTestAxis(0, 1), biz.EvalQueryAnalysis{})

	s2, cap2 := newShadowStore(newEvalIntentTestEmbedder(nil))
	s2.shadowRouteMode(context.Background(), "u", "q", nil, biz.EvalQueryAnalysis{})
	if len(cap2.recs) != 0 {
		t.Fatalf("empty embedding must not emit, got %d", len(cap2.recs))
	}

	s3, cap3 := newShadowStore(nil)
	s3.shadowRouteMode(context.Background(), "u", "q", evalIntentTestAxis(0, 1), biz.EvalQueryAnalysis{})
	if len(cap3.recs) != 0 {
		t.Fatalf("nil embedder (no router) must not emit, got %d", len(cap3.recs))
	}
}

// T8：写侧 correction_gate 双记——margin/阈值齐全、词法 cue 共存入 state。
func TestShadowCorrectionGate_EmitsMarginAndLexicalCue(t *testing.T) {
	s, cap := newShadowStore(newEvalIntentTestEmbedder(nil))
	s.shadowCorrectionGate(context.Background(), "user-1", "其实我改成周三了",
		evalIntentTestAxis(4, 1), true)
	if len(cap.recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(cap.recs))
	}
	rec := cap.recs[0]
	if rec.Outcome != "q=correction_gate;k=update_fact" {
		t.Fatalf("outcome = %q", rec.Outcome)
	}
	shadow, _ := rec.Metadata["shadow"].(map[string]any)
	inc, _ := shadow["incumbent"].(decide.Incumbent)
	if inc.Threshold != 0.72 {
		t.Fatalf("threshold = %v, want 0.72", inc.Threshold)
	}
	if inc.Method != "vir" {
		t.Fatalf("method = %v", inc.Method)
	}
	state, _ := shadow["state"].(map[string]any)
	if state["lexical_cue"] != true {
		t.Fatalf("lexical cue lost: %v", state)
	}
}

func TestShadowCorrID_DeterministicAndDistinct(t *testing.T) {
	a := shadowCorrID("route_mode", "u", "q")
	b := shadowCorrID("route_mode", "u", "q")
	if a != b {
		t.Fatalf("retry must collapse to one correlation: %s vs %s", a, b)
	}
	if shadowCorrID("route_mode", "u", "q2") == a {
		t.Fatal("distinct queries must not share correlation")
	}
	if shadowCorrID("correction_gate", "u", "q") == a {
		t.Fatal("distinct questions must not share correlation")
	}
}
