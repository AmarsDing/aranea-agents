package skillruntime

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// Overview 预算渲染器（B 批次）：超预算截断 + 剩余计数提示 + 确定性输出
// （同输入必同字节，保持 prompt 缓存前缀稳定）。

func TestRenderSkillOverviewBudgeted_UnderBudget(t *testing.T) {
	sums := []trpcskill.Summary{
		{Name: "alpha", Description: "first"},
		{Name: "beta", Description: "second"},
	}
	out := RenderSkillOverviewBudgeted(sums, 4000)
	if !strings.Contains(out, "Available skills:") || !strings.Contains(out, "- alpha: first") || !strings.Contains(out, "- beta: second") {
		t.Fatalf("unexpected output: %q", out)
	}
	if strings.Contains(out, "more skills") {
		t.Fatalf("under budget must not emit truncation note: %q", out)
	}
}

func TestRenderSkillOverviewBudgeted_Truncates(t *testing.T) {
	var sums []trpcskill.Summary
	for i := 0; i < 50; i++ {
		sums = append(sums, trpcskill.Summary{Name: fmt.Sprintf("skill-%02d", i), Description: strings.Repeat("d", 30)})
	}
	out := RenderSkillOverviewBudgeted(sums, 200)
	if !strings.Contains(out, "more skills available") {
		t.Fatalf("expected truncation note: %q", out)
	}
	if strings.Contains(out, "skill-49:") {
		t.Fatalf("over-budget skills must be omitted: %q", out)
	}
	// 确定性：同输入必同字节（prompt 缓存前缀稳定的前提）。
	if got := RenderSkillOverviewBudgeted(sums, 200); got != out {
		t.Fatal("renderer must be deterministic for identical input")
	}
	// 无截断时预算内全量（maxChars<=0 = 不限）。
	full := RenderSkillOverviewBudgeted(sums, 0)
	if !strings.Contains(full, "skill-49:") {
		t.Fatal("maxChars=0 must render all skills")
	}
}

func TestRenderSkillOverviewBudgeted_NoteCountsOmitted(t *testing.T) {
	var sums []trpcskill.Summary
	for i := 0; i < 10; i++ {
		sums = append(sums, trpcskill.Summary{Name: fmt.Sprintf("s%d", i), Description: "x"})
	}
	out := RenderSkillOverviewBudgeted(sums, 20) // 只容得下前几行
	if !strings.Contains(out, "more skills available") {
		t.Fatalf("expected note: %q", out)
	}
	// 提示中的 N 必须等于实际被省略的数量。
	var omitted int
	if _, err := fmt.Sscanf(out[strings.LastIndex(out, "(")+1:], "%d more", &omitted); err != nil {
		t.Fatalf("parse omitted count from %q: %v", out, err)
	}
	shown := strings.Count(out, "\n- s")
	if shown+omitted != len(sums) {
		t.Fatalf("shown %d + omitted %d != total %d", shown, omitted, len(sums))
	}
	_ = utf8.RuneCountInString // keep import when assertions change
}

func TestRunOptionWithOverviewBudget_DefaultInstallsRenderer(t *testing.T) {
	opt := RunOptionWithOverviewBudget(&mockRuntime{json: "{}"})
	ro := &trpcagent.RunOptions{}
	opt(ro)
	if ro.AvailableSkillsRenderer == nil {
		t.Fatal("default policy must install budgeted renderer")
	}
}

func TestRunOptionWithOverviewBudget_ExplicitZeroUnlimited(t *testing.T) {
	opt := RunOptionWithOverviewBudget(&mockRuntime{json: `{"overview_max_chars":0}`})
	ro := &trpcagent.RunOptions{}
	opt(ro)
	if ro.AvailableSkillsRenderer != nil {
		t.Fatal("overview_max_chars=0 must disable budget (unlimited)")
	}
}

func TestRunOptionWithOverviewBudget_RendererUsesPolicyChars(t *testing.T) {
	opt := RunOptionWithOverviewBudget(&mockRuntime{json: `{"overview_max_chars":40}`})
	ro := &trpcagent.RunOptions{}
	opt(ro)
	out := ro.AvailableSkillsRenderer(context.Background(), trpcagent.AvailableSkillsRenderRequest{
		Summaries: []trpcskill.Summary{
			{Name: "a", Description: strings.Repeat("x", 50)},
			{Name: "b", Description: "short"},
		},
	})
	if strings.Contains(out, "- b: short") {
		t.Fatalf("40 rune budget must omit second skill: %q", out)
	}
}
