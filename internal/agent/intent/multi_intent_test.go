package intent

import "testing"

// TestParseArtifactJSON_SubIntents 多意图解析（2026-08-28 方案①）：
// 正常解析 / 坏条目容错 / 单条置 nil / 上限截断 / 坏类型剥离重试。
func TestParseArtifactJSON_SubIntents(t *testing.T) {
	t.Run("two sub intents parsed", func(t *testing.T) {
		text := `{"refined_goal":"查 X 运行数据并发邮件给运维","intent_kind":"research","tool_hints":["metrics_query"],"sub_intents":[{"goal":"查 X 运行数据","intent_kind":"research","tool_hints":["metrics_query"]},{"goal":"整理成邮件发给运维","intent_kind":"task","tool_hints":["send_email"]}]}`
		art, _ := parseArtifactJSON(text)
		if art == nil {
			t.Fatal("expected non-nil Artifact")
		}
		if len(art.SubIntents) != 2 {
			t.Fatalf("SubIntents len = %d, want 2", len(art.SubIntents))
		}
		if art.SubIntents[1].Goal != "整理成邮件发给运维" || art.SubIntents[1].IntentKind != "task" {
			t.Errorf("sub[1] = %+v", art.SubIntents[1])
		}
	})

	t.Run("entry with empty goal dropped", func(t *testing.T) {
		text := `{"refined_goal":"A 然后 B","intent_kind":"task","sub_intents":[{"goal":"","intent_kind":"task"},{"goal":"做 A","intent_kind":"task"},{"goal":"做 B","intent_kind":"task"}]}`
		art, _ := parseArtifactJSON(text)
		if art == nil {
			t.Fatal("expected non-nil Artifact")
		}
		if len(art.SubIntents) != 2 {
			t.Fatalf("SubIntents len = %d, want 2 (empty-goal entry dropped)", len(art.SubIntents))
		}
	})

	t.Run("single sub intent treated as non-composite", func(t *testing.T) {
		text := `{"refined_goal":"只做一件事","intent_kind":"task","sub_intents":[{"goal":"做那件事","intent_kind":"task"}]}`
		art, _ := parseArtifactJSON(text)
		if art == nil {
			t.Fatal("expected non-nil Artifact")
		}
		if art.SubIntents != nil {
			t.Errorf("SubIntents = %v, want nil (single entry is not composite)", art.SubIntents)
		}
	})

	t.Run("over-limit truncated to MaxSubIntents", func(t *testing.T) {
		text := `{"refined_goal":"A B C D E F","intent_kind":"task","sub_intents":[
			{"goal":"a","intent_kind":"task"},{"goal":"b","intent_kind":"task"},
			{"goal":"c","intent_kind":"task"},{"goal":"d","intent_kind":"task"},
			{"goal":"e","intent_kind":"task"},{"goal":"f","intent_kind":"task"}]}`
		art, _ := parseArtifactJSON(text)
		if art == nil {
			t.Fatal("expected non-nil Artifact")
		}
		if len(art.SubIntents) != MaxSubIntents {
			t.Fatalf("SubIntents len = %d, want %d (truncated)", len(art.SubIntents), MaxSubIntents)
		}
	})

	t.Run("bad sub_intents type stripped, main artifact survives", func(t *testing.T) {
		// goal 写成数字导致整体 unmarshal 失败 → 剥离 sub_intents 重试，主字段保留。
		text := `{"refined_goal":"查数据发邮件","intent_kind":"research","tool_hints":["metrics_query"],"sub_intents":[{"goal":123}]}`
		art, _ := parseArtifactJSON(text)
		if art == nil {
			t.Fatal("expected non-nil Artifact (bad sub_intents must not kill main artifact)")
		}
		if art.RefinedGoal != "查数据发邮件" || art.IntentKind != "research" {
			t.Errorf("main fields lost: %+v", art)
		}
		if art.SubIntents != nil {
			t.Errorf("SubIntents = %v, want nil after strip-retry", art.SubIntents)
		}
	})

	t.Run("no sub_intents key unchanged (regression)", func(t *testing.T) {
		text := `{"refined_goal":"build a landing page","intent_kind":"task"}`
		art, _ := parseArtifactJSON(text)
		if art == nil {
			t.Fatal("expected non-nil Artifact")
		}
		if art.SubIntents != nil {
			t.Errorf("SubIntents = %v, want nil", art.SubIntents)
		}
	})
}

// TestAllToolHints 顶层与子意图 hints 并集（去重、保序、空白剔除、nil 安全）。
func TestAllToolHints(t *testing.T) {
	var nilArt *Artifact
	if got := nilArt.AllToolHints(); got != nil {
		t.Errorf("nil artifact AllToolHints = %v, want nil", got)
	}

	art := &Artifact{
		ToolHints: []string{"metrics_query", " search_content "},
		SubIntents: []SubIntent{
			{Goal: "查", IntentKind: "research", ToolHints: []string{"metrics_query", "knowledge_search"}},
			{Goal: "发", IntentKind: "task", ToolHints: []string{"send_email", ""}},
		},
	}
	got := art.AllToolHints()
	want := []string{"metrics_query", "search_content", "knowledge_search", "send_email"}
	if len(got) != len(want) {
		t.Fatalf("AllToolHints = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AllToolHints = %v, want %v", got, want)
		}
	}

	// 单意图（无子意图）：与直接读 ToolHints 等价（回归语义）。
	single := &Artifact{ToolHints: []string{"a", "b"}}
	if got := single.AllToolHints(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("single-intent AllToolHints = %v, want [a b]", got)
	}
}

// TestBuildIntentPassPayload_SubIntentsCount 监控字段（复合意图占比口径）。
func TestBuildIntentPassPayload_SubIntentsCount(t *testing.T) {
	r := RunResult{
		Outcome: "completed",
		Artifact: &Artifact{
			RefinedGoal: "A 然后 B",
			IntentKind:  "task",
			SubIntents: []SubIntent{
				{Goal: "a", IntentKind: "task"},
				{Goal: "b", IntentKind: "task"},
			},
		},
	}
	p := BuildIntentPassPayload(r, RunMeta{})
	if got, ok := p["sub_intents_count"].(int); !ok || got != 2 {
		t.Errorf("sub_intents_count = %v, want 2", p["sub_intents_count"])
	}

	// 单意图：字段存在且为 0（前端/监控可区分「跑了 pass 但单意图」与「没跑」）。
	r0 := RunResult{Outcome: "completed", Artifact: &Artifact{RefinedGoal: "x", IntentKind: "task"}}
	p0 := BuildIntentPassPayload(r0, RunMeta{})
	if got, ok := p0["sub_intents_count"].(int); !ok || got != 0 {
		t.Errorf("single-intent sub_intents_count = %v, want 0", p0["sub_intents_count"])
	}
}
