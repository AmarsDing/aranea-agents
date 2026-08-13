package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// 路由命中率精确匹配语义（A1 修复）：
// RouteHitRate(X) = X 被加载的去重轮次 / X 被路由的去重轮次（按 activation_id 去重）。
// 旧口径（该 Skill 自身记录中 routed_slugs 非空 / loaded_slug 非空）把「路由/加载了
// 任意 Skill」都计入，导致命中率虚高（本测试场景下旧口径算出 100%）。

func TestGetSkillHealth_RouteHitRatePreciseMatching(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	h := NewSkillHealthRepo(d)
	ctx := context.Background()

	skX, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "Health X", Slug: "health-x", Body: "# X body",
	})
	if err != nil {
		t.Fatalf("create skill X: %v", err)
	}
	skY, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "Health Y", Slug: "health-y", Body: "# Y body",
	})
	if err != nil {
		t.Fatalf("create skill Y: %v", err)
	}

	rows := []biz.SkillInvocationWrite{
		// act-a：X 被路由且被加载（两行同 activation，模拟同轮多次工具调用，须去重）。
		{SkillID: skX.ID, ActivationID: "act-a", RoutedSlugs: []string{"health-x", "health-y"}, LoadedSlug: "health-x"},
		{SkillID: skX.ID, ActivationID: "act-a", RoutedSlugs: []string{"health-x", "health-y"}, LoadedSlug: "health-x"},
		// act-b：X 被路由但加载的是 Y。
		{SkillID: skY.ID, ActivationID: "act-b", RoutedSlugs: []string{"health-x", "health-y"}, LoadedSlug: "health-y"},
		// act-c：X 被路由，加载的是 Y。
		{SkillID: skY.ID, ActivationID: "act-c", RoutedSlugs: []string{"health-x"}, LoadedSlug: "health-y"},
	}
	for _, w := range rows {
		w.AgentID = "agent-h"
		w.Outcome = "success"
		w.DurationMS = 10
		if err := r.RecordSkillInvocation(ctx, w); err != nil {
			t.Fatalf("RecordSkillInvocation: %v", err)
		}
	}

	now := time.Now()
	detail, err := h.GetSkillHealth(ctx, skX.ID, now.Add(-7*24*time.Hour), now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("GetSkillHealth: %v", err)
	}

	// X 被路由的去重轮次：act-a / act-b / act-c = 3。
	if detail.RoutedCount30d != 3 {
		t.Errorf("RoutedCount30d = %d, want 3", detail.RoutedCount30d)
	}
	// X 被加载的去重轮次：act-a = 1（同 activation 的两行不得重复计数）。
	if detail.LoadedCount30d != 1 {
		t.Errorf("LoadedCount30d = %d, want 1", detail.LoadedCount30d)
	}
	wantRate := 1.0 / 3.0
	if diff := detail.RouteHitRate30d - wantRate; diff > 0.001 || diff < -0.001 {
		t.Errorf("RouteHitRate30d = %v, want %v", detail.RouteHitRate30d, wantRate)
	}
	if detail.RoutedCount7d != 3 || detail.LoadedCount7d != 1 {
		t.Errorf("7d counts = %d/%d, want 3/1", detail.RoutedCount7d, detail.LoadedCount7d)
	}
	// X 自身的调用记录（成功率/延迟口径）仍只统计 skill_id = X 的行。
	if detail.TotalInvocations30d != 2 {
		t.Errorf("TotalInvocations30d = %d, want 2", detail.TotalInvocations30d)
	}
}

// 无任何路由数据时计数为 0，供前端区分「无数据」与「0% 命中率」。
func TestGetSkillHealth_NoRouteData(t *testing.T) {
	d := newTestDataPG(t)
	r := NewSkillRepo(d)
	h := NewSkillHealthRepo(d)
	ctx := context.Background()

	sk, err := r.CreateSkillWithVersion(ctx, biz.SkillCreateInput{
		Name: "Health Z", Slug: "health-z", Body: "# Z body",
	})
	if err != nil {
		t.Fatalf("create skill: %v", err)
	}
	// 一条无路由信息的调用记录（旧数据 / 未走路由的运行时调用）。
	if err := r.RecordSkillInvocation(ctx, biz.SkillInvocationWrite{
		SkillID: sk.ID, AgentID: "agent-h", ActivationID: "act-z", Outcome: "success",
	}); err != nil {
		t.Fatalf("RecordSkillInvocation: %v", err)
	}

	now := time.Now()
	detail, err := h.GetSkillHealth(ctx, sk.ID, now.Add(-7*24*time.Hour), now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("GetSkillHealth: %v", err)
	}
	if detail.RoutedCount30d != 0 || detail.LoadedCount30d != 0 {
		t.Errorf("counts = %d/%d, want 0/0", detail.RoutedCount30d, detail.LoadedCount30d)
	}
	if detail.RouteHitRate30d != 0 {
		t.Errorf("RouteHitRate30d = %v, want 0", detail.RouteHitRate30d)
	}
	if detail.TotalInvocations30d != 1 {
		t.Errorf("TotalInvocations30d = %d, want 1", detail.TotalInvocations30d)
	}
}
