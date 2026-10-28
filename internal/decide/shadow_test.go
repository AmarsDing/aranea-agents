package decide

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz/decision"
	"aranea-agents/pkg/loggateway"
)

type captureCollector struct{ recs []decision.Record }

func (c *captureCollector) Emit(_ context.Context, r decision.Record) { c.recs = append(c.recs, r) }

func newTestRecorder() (*Recorder, *captureCollector) {
	c := &captureCollector{}
	return NewRecorder(c, loggateway.NewNoop()), c
}

func TestRecorder_NilIsDisabledAndSafe(t *testing.T) {
	var r *Recorder
	if r.Enabled() {
		t.Fatal("nil recorder must report disabled")
	}
	r.Emit(context.Background(), ShadowEvent{QuestionID: "route_mode"}) // must not panic

	r2 := NewRecorder(nil, loggateway.NewNoop())
	if r2.Enabled() {
		t.Fatal("nil collector must report disabled")
	}
	r2.Emit(context.Background(), ShadowEvent{QuestionID: "route_mode"})
}

func TestRecorder_EmitBuildsValidRecord(t *testing.T) {
	r, c := newTestRecorder()
	r.Emit(context.Background(), ShadowEvent{
		CorrelationID: "corr-1",
		QuestionID:    "route_mode",
		State:         map[string]any{"user_message": "按时间顺序列出我的体检记录"},
		Incumbent: Incumbent{
			Key:    "chronological",
			Scores: map[string]float64{"chronological": 0.83, "aggregate": 0.41},
			Method: "vir",
		},
		SessionID:   "sess-1",
		WorkspaceID: "ws-1",
	})
	if len(c.recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(c.recs))
	}
	rec := c.recs[0]
	if err := rec.Validate(); err != nil {
		t.Fatalf("emitted record fails M80 contract: %v", err)
	}
	if rec.Category != decision.CategoryMemoryShadow {
		t.Fatalf("category = %q, want memory_shadow", rec.Category)
	}
	if rec.Outcome != "q=route_mode;k=chronological" {
		t.Fatalf("outcome = %q", rec.Outcome)
	}
	if rec.Confidence == nil || *rec.Confidence != 0.83 {
		t.Fatalf("confidence = %v, want incumbent winning margin 0.83", rec.Confidence)
	}
	if rec.ActorKey != "system:mem_shadow" || rec.ActorType != decision.ActorSystem {
		t.Fatalf("actor = %s/%s", rec.ActorType, rec.ActorKey)
	}
	if rec.SourceRef.SessionID != "sess-1" || rec.WorkspaceID != "ws-1" {
		t.Fatalf("attribution lost: %+v / %q", rec.SourceRef, rec.WorkspaceID)
	}
	shadow, ok := rec.Metadata["shadow"].(map[string]any)
	if !ok {
		t.Fatalf("metadata.shadow missing: %v", rec.Metadata)
	}
	if shadow["correlation_id"] != "corr-1" {
		t.Fatalf("correlation lost: %v", shadow)
	}
	// Outbox path is JSON: the payload must round-trip intact.
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back decision.Record
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Outcome != rec.Outcome {
		t.Fatalf("json round-trip broke outcome: %q vs %q", back.Outcome, rec.Outcome)
	}
}

func TestRecorder_RejectsUnknownQuestionAndEmptyFields(t *testing.T) {
	r, c := newTestRecorder()
	r.Emit(context.Background(), ShadowEvent{CorrelationID: "x", QuestionID: "nope", Incumbent: Incumbent{Key: "k", Method: "vir"}})
	r.Emit(context.Background(), ShadowEvent{CorrelationID: "x", QuestionID: "route_mode", Incumbent: Incumbent{Method: "vir"}})
	r.Emit(context.Background(), ShadowEvent{QuestionID: "route_mode", Incumbent: Incumbent{Key: "k", Method: "vir"}})
	if len(c.recs) != 0 {
		t.Fatalf("all three emits must drop, got %d records", len(c.recs))
	}
}

func TestRecorder_OutcomeFitsCapForAllQuestions(t *testing.T) {
	r, c := newTestRecorder()
	keys := map[string][]string{
		"route_mode":        {"direct_lookup", "chronological", "aggregate", "enumerative", "relational", "trend", "no_recall"},
		"correction_gate":   {"update_fact", "keep_as_is"},
		"forget_gate":       {"erase", "retain"},
		"tool_relevance":    {"needed", "not_needed"},
		"skill_intent":      {"none", "sales/feedback/email"},
		"mcp_server_choice": {"none", "github"},
	}
	for qid, ks := range keys {
		for _, k := range ks {
			r.Emit(context.Background(), ShadowEvent{
				CorrelationID: "c", QuestionID: qid,
				State:     map[string]any{"user_message": "m", "task": "m"},
				Incumbent: Incumbent{Key: k, Method: "vir"},
			})
		}
	}
	for _, rec := range c.recs {
		if len(rec.Outcome) > 64 {
			t.Fatalf("outcome exceeds 64-char cap: %q", rec.Outcome)
		}
		if err := rec.Validate(); err != nil {
			t.Fatalf("record invalid: %v", err)
		}
	}
}

func TestRecorder_Phase1PlanesUseJEVShadowCategory(t *testing.T) {
	r, c := newTestRecorder()
	r.Emit(context.Background(), ShadowEvent{
		CorrelationID: "c1", QuestionID: "skill_intent",
		State:     map[string]any{"user_message": "发邮件"},
		Incumbent: Incumbent{Key: "none", Method: "skillrouter"},
	})
	r.Emit(context.Background(), ShadowEvent{
		CorrelationID: "c2", QuestionID: "tool_relevance",
		State:     map[string]any{"task": "search docs", "tool_candidates": []string{"knowledge_search"}},
		Incumbent: Incumbent{Key: "needed", Method: "bm25"},
	})
	if len(c.recs) != 2 {
		t.Fatalf("want 2, got %d", len(c.recs))
	}
	for _, rec := range c.recs {
		if rec.Category != decision.CategoryJEVShadow {
			t.Fatalf("category = %q, want jev_shadow", rec.Category)
		}
		if rec.ActorKey != "system:jev_shadow" {
			t.Fatalf("actor = %q", rec.ActorKey)
		}
	}
}

func TestRecorder_ScenarioHintTruncatesCJK(t *testing.T) {
	long := strings.Repeat("长", 300)
	got := scenarioHint(map[string]any{"user_message": long})
	if n := len([]rune(got)); n != 120 {
		t.Fatalf("hint runes = %d, want 120", n)
	}
	if got := scenarioHint(map[string]any{}); got != "(no slot text)" {
		t.Fatalf("empty state hint = %q", got)
	}
}

func TestRecorder_JEVAnswerRecordedWhenWired(t *testing.T) {
	r, c := newTestRecorder()
	r.Emit(context.Background(), ShadowEvent{
		CorrelationID: "pair-1", QuestionID: "correction_gate",
		State:     map[string]any{"user_message": "我改主意了"},
		Incumbent: Incumbent{Key: "update_fact", Scores: map[string]float64{"correction": 0.9}, Threshold: 0.72, Method: "vir", Matched: true},
		JEV: &Answer{
			Probabilities: map[string]float64{"update_fact": 0.7, "keep_as_is": 0.3},
			Confidence:    0.7, Backend: "laya-multilingual", Epoch: "tev4-1024-r1",
		},
	})
	if len(c.recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(c.recs))
	}
	if !strings.Contains(c.recs[0].Reasoning, "jev=laya-multilingual") {
		t.Fatalf("reasoning must name the jev backend: %q", c.recs[0].Reasoning)
	}
	raw, _ := json.Marshal(c.recs[0].Metadata["shadow"].(map[string]any)["jev"])
	if !strings.Contains(string(raw), "tev4-1024-r1") {
		t.Fatalf("epoch must round-trip for P20 bucketing: %s", raw)
	}
}

func TestQuestionDef_TranscriptionMatchesContract(t *testing.T) {
	q, ok := QuestionDef("route_mode")
	if !ok || q.Plane != "read" || q.Type != "choice" {
		t.Fatalf("route_mode def drifted: %+v", q)
	}
	if len(q.Criteria) != 7 {
		t.Fatalf("route_mode options = %d, want 7 (questionsets.json)", len(q.Criteria))
	}
	if _, ok := q.Criteria["no_recall"]; !ok {
		t.Fatal("route_mode must carry the no_recall option (VIR blind-spot encoding)")
	}
	if q2, ok := QuestionDef("forget_gate"); !ok || len(q2.Criteria) != 2 {
		t.Fatalf("forget_gate def drifted: %+v ok=%v", q2, ok)
	}
	if _, ok := QuestionDef("consolidation_representation"); ok {
		t.Fatal("consolidation questions stay Phase-2; must not be registered yet")
	}
	if q, ok := QuestionDef("skill_intent"); !ok || q.Plane != "skill" {
		t.Fatalf("skill_intent Phase-1 def missing: %+v ok=%v", q, ok)
	}
	if q, ok := QuestionDef("tool_relevance"); !ok || len(q.Criteria) != 2 {
		t.Fatalf("tool_relevance Phase-1 def drifted: %+v ok=%v", q, ok)
	}
	if q, ok := QuestionDef("mcp_server_choice"); !ok || q.Plane != "mcp" {
		t.Fatalf("mcp_server_choice Phase-1 def missing: %+v ok=%v", q, ok)
	}
}
