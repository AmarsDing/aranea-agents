package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestCommitRoute_S06PlanTeam(t *testing.T) {
	msg := "让数字内容媒体公司市场部出一版 Q3 推广文案框架，含三个渠道。"
	d := CommitRoute(biz.ComplexityModerate, true, 0.4, "评估完成：中等任务，强制走规划路径", msg)
	if d.Lane != biz.RouteLanePlanTeam {
		t.Fatalf("S06 lane = %s, want plan_team", d.Lane)
	}
	if d.Mode == "" || d.Mode == "direct" {
		t.Fatalf("S06 committed mode must be team-forming, got %q", d.Mode)
	}
	if !d.TeamEvidence {
		t.Fatal("S06 must carry team evidence")
	}
}

func TestCommitRoute_S09UnforcedUnspecified(t *testing.T) {
	msg := "我们来规划 Q3 的内容运营方案。先搭一个整体框架，包含渠道、节奏、预算三大块，每块简要说明。"
	d := CommitRoute(biz.ComplexityModerate, false, 0.4, "评估完成：中等任务（无组队证据，不强制规划）", msg)
	if d.Specified() {
		t.Fatalf("S09 must stay unspecified so Plan() evidence gate still applies, got %+v", d)
	}
}

func TestCommitRoute_ComplexNoEvidencePlanSolo(t *testing.T) {
	d := CommitRoute(biz.ComplexityComplex, true, 0.8, "评估完成：复杂任务，强制走规划路径", "写一份竞品分析报告")
	if d.Lane != biz.RouteLanePlanSolo {
		t.Fatalf("complex without team evidence = %s, want plan_solo", d.Lane)
	}
	if d.Mode != "" {
		t.Fatalf("plan_solo must not inject a team-forming mode, got %q", d.Mode)
	}
}

func TestCommitRoute_S01DirectUnspecified(t *testing.T) {
	d := CommitRoute(biz.ComplexitySimple, false, 0.1, "评估完成：简单任务", "你好")
	if d.Specified() {
		t.Fatalf("S01 must not commit a lane, got %+v", d)
	}
}
