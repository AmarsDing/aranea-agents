package biz

import (
	"encoding/json"
	"testing"
)

func TestStepKindClarify_Constant(t *testing.T) {
	if StepKindClarify != StepKind("clarify") {
		t.Errorf("StepKindClarify = %q, want %q", StepKindClarify, "clarify")
	}
}

func TestStepStatusAwaitingInput_Constant(t *testing.T) {
	if StepStatusAwaitingInput != StepStatus("awaiting_input") {
		t.Errorf("StepStatusAwaitingInput = %q, want %q", StepStatusAwaitingInput, "awaiting_input")
	}
}

func TestClarificationEnvelope_RoundTrip(t *testing.T) {
	env := ClarificationEnvelope{
		Version: 1,
		Kind:    "clarification",
		Questions: []ClarificationQuestion{
			{
				Question:    "目标平台是什么？",
				Mode:        ClarificationModeSingle,
				Options:     []string{"Web", "iOS", "Android"},
				Recommended: []string{"Web"},
			},
			{
				Question:    "需要哪些交付物？",
				Mode:        ClarificationModeMulti,
				Options:     []string{"代码", "文档", "测试"},
				Recommended: []string{"代码", "文档"},
			},
		},
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ClarificationEnvelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Kind != "clarification" || got.Version != 1 {
		t.Errorf("envelope header = %+v", got)
	}
	if len(got.Questions) != 2 {
		t.Fatalf("questions len = %d, want 2", len(got.Questions))
	}
	q0 := got.Questions[0]
	if q0.Mode != ClarificationModeSingle || len(q0.Options) != 3 || len(q0.Recommended) != 1 {
		t.Errorf("q0 = %+v", q0)
	}
	if got.Answers != nil {
		t.Errorf("answers should be nil before submit, got %+v", got.Answers)
	}
}

func TestClarificationEnvelope_WithAnswers(t *testing.T) {
	raw := `{"version":1,"kind":"clarification","questions":[{"question":"Q","mode":"single","options":["a"],"recommended":["a"]}],"answers":[{"selected":["a"],"other":""}]}`
	var env ClarificationEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Answers) != 1 || env.Answers[0].Selected[0] != "a" {
		t.Errorf("answers = %+v", env.Answers)
	}
}

func TestClarificationEnvelope_BuildClarifiedContext(t *testing.T) {
	env := ClarificationEnvelope{
		Version: 1,
		Kind:    "clarification",
		Questions: []ClarificationQuestion{
			{Question: "平台？", Mode: ClarificationModeSingle, Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}},
			{Question: "风格？", Mode: ClarificationModeSingle, Options: []string{"简约", "华丽"}, Recommended: []string{"简约"}},
		},
		Answers: []ClarificationAnswer{
			{Selected: []string{"iOS"}},
			{}, // 未作答 → 按推荐
		},
	}
	ctx := env.BuildClarifiedContext()
	if ctx == "" {
		t.Fatal("context should not be empty")
	}
	// 已答：包含用户选择
	if !containsAll(ctx, []string{"平台？", "iOS"}) {
		t.Errorf("answered question missing from context: %q", ctx)
	}
	// 未答：包含推荐项
	if !containsAll(ctx, []string{"风格？", "简约"}) {
		t.Errorf("recommended fallback missing from context: %q", ctx)
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
