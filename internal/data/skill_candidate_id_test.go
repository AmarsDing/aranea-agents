package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestRuntimeCandidateSkillID_HealthMetricsReadable 覆盖 B1（2026-08-14）：
// RuntimeCandidate.SkillID 必须是平台 ID（skill_<unixnano>），因为
// skill_invocation.skill_id 列按平台 ID 落库；GetHealthMetrics 只有按 ID
// 查询才能匹配到非零数据，否则永远 0 行、排序特性静默失效。
func TestRuntimeCandidateSkillID_HealthMetricsReadable(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	h := NewSkillIntelligenceRepo(d, d.lg)
	ctx := context.Background()

	sk, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "CandidateID", Slug: "candidate-id", Body: "# body",
	})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	// 候选查询只覆盖 enabled+published/active，需先发布再启用。
	if _, err := r.PublishSkill(ctx, sk.ID, "pass"); err != nil {
		t.Fatalf("publish skill: %v", err)
	}
	if _, err := r.UpdateSkillEnabled(ctx, sk.ID, true); err != nil {
		t.Fatalf("enable skill: %v", err)
	}

	// 1) 候选必须带出 SkillID。
	candidates, err := r.ListEnabledPublishedSkillCandidates(ctx)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	var found *biz.SkillRuntimeCandidate
	for i := range candidates {
		if candidates[i].Slug == "candidate-id" {
			found = &candidates[i]
			break
		}
	}
	if found == nil {
		t.Fatal("candidate not found")
	}
	if found.SkillID == "" {
		t.Error("RuntimeCandidate.SkillID is empty")
	}
	if found.SkillID != sk.ID {
		t.Errorf("RuntimeCandidate.SkillID = %q, want %q", found.SkillID, sk.ID)
	}

	// 2) 记录一条运行时调用（source=runtime，显式写入 outcome+status）。
	if err := r.RecordSkillInvocation(ctx, biz.SkillInvocationWrite{
		SkillID:    sk.ID,
		AgentID:    "agent-c",
		Outcome:    "success",
		Status:     "completed",
		DurationMS: 42,
		Source:     biz.SkillInvocationSourceRuntime,
	}); err != nil {
		t.Fatalf("record invocation: %v", err)
	}

	// 3) 用平台 ID 查询健康指标必须命中非零数据。
	metrics, err := h.GetHealthMetrics(ctx, sk.ID, time.Now().UTC().Add(-7*24*time.Hour))
	if err != nil {
		t.Fatalf("GetHealthMetrics: %v", err)
	}
	if metrics.InvocationCount == 0 {
		t.Error("GetHealthMetrics.InvocationCount = 0, expected > 0 (skill_id must be platform ID)")
	}
	if metrics.SuccessCount == 0 {
		t.Error("GetHealthMetrics.SuccessCount = 0, expected > 0")
	}
	if metrics.AvgDurationMS == 0 {
		t.Error("GetHealthMetrics.AvgDurationMS = 0, expected > 0")
	}
}
