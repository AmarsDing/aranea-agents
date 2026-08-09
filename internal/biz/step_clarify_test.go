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

func TestClarificationEnvelope_BuildClarifiedContext_FreeText(t *testing.T) {
	env := ClarificationEnvelope{
		Version: 1,
		Kind:    "clarification",
		Questions: []ClarificationQuestion{
			{Question: "平台？", Mode: ClarificationModeSingle, Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}},
		},
		Answers:  []ClarificationAnswer{{}}, // 自由回复路径：空作答 → 按推荐
		FreeText: "做成内部工具即可",
	}
	ctx := env.BuildClarifiedContext()
	if !containsAll(ctx, []string{"平台？", "Web"}) {
		t.Errorf("recommended fallback missing from context: %q", ctx)
	}
	if !containsAll(ctx, []string{"做成内部工具即可"}) {
		t.Errorf("free text missing from context: %q", ctx)
	}
}

func TestClarificationEnvelope_OriginalInputRoundTrip(t *testing.T) {
	raw := `{"version":1,"kind":"clarification","questions":[],"answers":null,"original_input":"帮我做个 CRM"}`
	var env ClarificationEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.OriginalInput != "帮我做个 CRM" {
		t.Errorf("OriginalInput = %q, want %q", env.OriginalInput, "帮我做个 CRM")
	}
	out, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !containsAll(string(out), []string{"original_input"}) {
		t.Errorf("marshaled envelope missing original_input: %s", out)
	}
}

func TestClarificationEnvelope_ResolutionRoundTrip(t *testing.T) {
	// 新信封：resolution 持久化
	env := ClarificationEnvelope{
		Version:    1,
		Kind:       "clarification",
		Resolution: ClarificationResolutionAutoDefault,
	}
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !containsAll(string(raw), []string{"resolution", "auto_default"}) {
		t.Errorf("marshaled envelope missing resolution: %s", raw)
	}
	var got ClarificationEnvelope
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Resolution != ClarificationResolutionAutoDefault {
		t.Errorf("Resolution = %q, want %q", got.Resolution, ClarificationResolutionAutoDefault)
	}
	// 旧信封（无 resolution 字段）：解析为零值，向后兼容
	var legacy ClarificationEnvelope
	if err := json.Unmarshal([]byte(`{"version":1,"kind":"clarification","questions":[],"answers":null}`), &legacy); err != nil {
		t.Fatalf("legacy unmarshal: %v", err)
	}
	if legacy.Resolution != "" {
		t.Errorf("legacy Resolution = %q, want empty", legacy.Resolution)
	}
}

func TestClarificationEnvelope_ApplyRecommendedAnswers(t *testing.T) {
	env := ClarificationEnvelope{
		Questions: []ClarificationQuestion{
			{Question: "平台？", Mode: ClarificationModeSingle, Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}},
			{Question: "交付物？", Mode: ClarificationModeMulti, Options: []string{"代码", "文档"}, Recommended: []string{"代码", "文档"}},
			{Question: "无推荐的问题", Mode: ClarificationModeSingle, Options: []string{"x"}}, // 无推荐 → 保持空作答
		},
	}
	env.ApplyRecommendedAnswers()
	if len(env.Answers) != 3 {
		t.Fatalf("answers len = %d, want 3", len(env.Answers))
	}
	if len(env.Answers[0].Selected) != 1 || env.Answers[0].Selected[0] != "Web" {
		t.Errorf("answers[0] = %+v, want selected [Web]", env.Answers[0])
	}
	if len(env.Answers[1].Selected) != 2 {
		t.Errorf("answers[1] = %+v, want 2 selected", env.Answers[1])
	}
	if len(env.Answers[2].Selected) != 0 {
		t.Errorf("answers[2] = %+v, want empty selected (no recommended)", env.Answers[2])
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
