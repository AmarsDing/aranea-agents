package biz

import (
	"strings"
	"testing"
)

// ─── P2-4 中期 project-state JSON（Ensemble QSP 有界记忆对齐）─────────────────
//
// 结构化项目状态：活跃请求 / 最近变更 / 里程碑 / 决策摘要。
// 滚动更新（各字段封顶），按切片预算注入，替代长任务的对话历史全量拼接。

func TestTeamProjectState_RollChangeCapsAndTruncates(t *testing.T) {
	var ps TeamProjectState
	long := strings.Repeat("长", 200)
	for i := 0; i < 15; i++ {
		ps.RollChange("member-a", long)
	}
	if len(ps.RecentChanges) != ProjectStateMaxRecent {
		t.Fatalf("recent = %d, want cap %d", len(ps.RecentChanges), ProjectStateMaxRecent)
	}
	if got := len([]rune(ps.RecentChanges[0].Summary)); got > projectStateChangeSummaryRunes {
		t.Fatalf("summary runes = %d, want ≤ %d", got, projectStateChangeSummaryRunes)
	}
}

func TestTeamProjectState_ActiveRequestsAndMilestonesCapped(t *testing.T) {
	var ps TeamProjectState
	reqs := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		reqs = append(reqs, strings.Repeat("r", 3))
	}
	ps.SetActiveRequests(reqs)
	if len(ps.ActiveRequests) != ProjectStateMaxActive {
		t.Fatalf("active = %d, want cap %d", len(ps.ActiveRequests), ProjectStateMaxActive)
	}
	for i := 0; i < 20; i++ {
		ps.RecordMilestone("m")
	}
	if len(ps.Milestones) != ProjectStateMaxMilestones {
		t.Fatalf("milestones = %d, want cap %d", len(ps.Milestones), ProjectStateMaxMilestones)
	}
	ps.SetDecisionDigest(strings.Repeat("决", 2000))
	if got := len([]rune(ps.DecisionDigest)); got > ProjectStateMaxDigestRunes {
		t.Fatalf("digest runes = %d, want ≤ %d", got, ProjectStateMaxDigestRunes)
	}
}

func TestTeamProjectState_RenderSliceWithinBudget(t *testing.T) {
	var ps TeamProjectState
	ps.SetActiveRequests([]string{"req-1", "req-2"})
	for i := 0; i < ProjectStateMaxRecent; i++ {
		ps.RollChange("m", strings.Repeat("变更", 60))
	}
	for i := 0; i < ProjectStateMaxMilestones; i++ {
		ps.RecordMilestone(strings.Repeat("里程碑", 30))
	}
	ps.SetDecisionDigest(strings.Repeat("决策", 300))

	const budget = 400
	got := ps.RenderSlice(budget)
	if n := len([]rune(got)); n > budget {
		t.Fatalf("slice runes = %d, want ≤ budget %d", n, budget)
	}
	// 高优先级字段必须存活：活跃请求在任何裁剪下都不丢。
	if !strings.Contains(got, "req-1") {
		t.Fatalf("active requests must survive slicing, got %q", got)
	}
}

func TestTeamProjectState_RenderSliceEmpty(t *testing.T) {
	var ps TeamProjectState
	if got := ps.RenderSlice(400); got != "" {
		t.Fatalf("empty state must render empty slice, got %q", got)
	}
}

func TestTeamProjectState_MapRoundTrip(t *testing.T) {
	var ps TeamProjectState
	ps.RollChange("m1", "完成了方案设计")
	ps.RecordMilestone("v1 发布")
	ps.SetActiveRequests([]string{"需求评审"})
	ps.SetDecisionDigest("采用事件溯源")

	m := ps.ToMap()
	back := TeamProjectStateFromMap(m)
	if len(back.RecentChanges) != 1 || back.RecentChanges[0].Actor != "m1" || back.RecentChanges[0].Summary != "完成了方案设计" {
		t.Fatalf("recent round trip failed: %+v", back.RecentChanges)
	}
	if len(back.Milestones) != 1 || back.Milestones[0] != "v1 发布" {
		t.Fatalf("milestones round trip failed: %+v", back.Milestones)
	}
	if len(back.ActiveRequests) != 1 || back.ActiveRequests[0] != "需求评审" {
		t.Fatalf("active round trip failed: %+v", back.ActiveRequests)
	}
	if back.DecisionDigest != "采用事件溯源" {
		t.Fatalf("digest round trip failed: %q", back.DecisionDigest)
	}
	// nil/垃圾输入必须零值可用。
	if got := TeamProjectStateFromMap(nil); len(got.RecentChanges) != 0 {
		t.Fatalf("nil map must yield zero state, got %+v", got)
	}
}
