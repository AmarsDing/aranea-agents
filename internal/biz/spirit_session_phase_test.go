package biz

import (
	"strings"
	"testing"
)

func TestResolveSpiritSessionPhase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		teams []Team
		want  SpiritSessionPhase
	}{
		{name: "empty is idle", want: SpiritPhaseIdle},
		{name: "deleted only is idle", teams: []Team{{ID: "t1", Status: TeamStatusCompleted, DeletedAt: "2026-08-22"}}, want: SpiritPhaseIdle},
		{name: "running is orchestrating", teams: []Team{{ID: "t1", Status: TeamStatusRunning}, {ID: "t2", Status: TeamStatusCompleted}}, want: SpiritPhaseOrchestrating},
		{name: "pending is orchestrating", teams: []Team{{ID: "t1", Status: TeamStatusPending}}, want: SpiritPhaseOrchestrating},
		{name: "running beats interrupted", teams: []Team{{ID: "t1", Status: TeamStatusRunning}, {ID: "t2", Status: TeamStatusInterrupted}}, want: SpiritPhaseOrchestrating},
		{name: "interrupted only", teams: []Team{{ID: "t1", Status: TeamStatusInterrupted}, {ID: "t2", Status: TeamStatusCompleted}}, want: SpiritPhaseInterrupted},
		{name: "all completed is ready", teams: []Team{{ID: "t1", Status: TeamStatusCompleted}, {ID: "t2", Status: TeamStatusCompleted}}, want: SpiritPhaseReady},
		{name: "failed cancelled archived is ready", teams: []Team{
			{ID: "t1", Status: TeamStatusFailed},
			{ID: "t2", Status: TeamStatusCancelled},
			{ID: "t3", Status: TeamStatusArchived},
		}, want: SpiritPhaseReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveSpiritSessionPhase(tt.teams); got != tt.want {
				t.Fatalf("phase = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestShouldForcePlanning(t *testing.T) {
	t.Parallel()
	if !ShouldForcePlanning(SpiritPhaseIdle, true, false) {
		t.Fatal("idle + complex must force planning")
	}
	if ShouldForcePlanning(SpiritPhaseReady, true, false) {
		t.Fatal("ready + same goal must not force planning")
	}
	if !ShouldForcePlanning(SpiritPhaseReady, true, true) {
		t.Fatal("ready + explicit new task must force planning")
	}
	if ShouldForcePlanning(SpiritPhaseOrchestrating, false, true) {
		t.Fatal("simple complexity never forces planning")
	}
	if ShouldForcePlanning(SpiritPhaseInterrupted, true, false) {
		t.Fatal("interrupted must not force a new DAG")
	}
}

func TestOrchestrationLooksLikeNewTask(t *testing.T) {
	t.Parallel()
	last := "组建几个团队，分析金鹏科技行情"
	if OrchestrationLooksLikeNewTask(last, last, "") {
		t.Fatal("repeat of T1 must not be a new task")
	}
	if OrchestrationLooksLikeNewTask("结果怎么样了", last, "") {
		t.Fatal("follow-up must not be a new task")
	}
	if !OrchestrationLooksLikeNewTask("请重新组建团队分析茅台", last, "") {
		t.Fatal("重新组建 must be a new task")
	}
	if !OrchestrationLooksLikeNewTask("分析贵州茅台的基本面", last, "") {
		t.Fatal("different entity analysis must be a new task")
	}
	if OrchestrationLooksLikeNewTask("分析金鹏科技现在怎么看", last, "") {
		t.Fatal("same entity follow-up analysis must stay on current DAG")
	}
}

func TestOrchestrationGoalShifted(t *testing.T) {
	t.Parallel()
	teams := []Team{{ID: "t1", DisplayName: "核实金鹏科技主体", Status: TeamStatusCompleted}}
	if OrchestrationGoalShifted("组建几个团队，分析金鹏科技行情", teams) {
		t.Fatal("same entity must not look shifted")
	}
	if !OrchestrationGoalShifted("分析贵州茅台行情", teams) {
		t.Fatal("茅台 vs 金鹏 must look shifted")
	}
	if OrchestrationGoalShifted("结果怎么样了", teams) {
		t.Fatal("follow-up without entity must not look shifted")
	}
}

func TestFormatOrchestrationBrief(t *testing.T) {
	t.Parallel()
	if FormatOrchestrationBrief(SpiritPhaseIdle, nil) != "" {
		t.Fatal("idle brief must be empty")
	}
	teams := []Team{
		{ID: "t1", DisplayName: "核实金鹏科技", Status: TeamStatusCompleted, DeliverablesOutput: `{"n1":{"summary":"主体已核实"}}`},
		{ID: "t2", DisplayName: "基本面", Status: TeamStatusCompleted},
	}
	got := FormatOrchestrationBrief(SpiritPhaseReady, teams)
	for _, want := range []string{"phase: ready", "t1", "get_team_deliverable", "plan_and_execute"} {
		if !strings.Contains(got, want) {
			t.Fatalf("brief missing %q:\n%s", want, got)
		}
	}
	if len([]rune(got)) > orchestrationBriefMaxRunes {
		t.Fatalf("brief too long: %d runes", len([]rune(got)))
	}
}

func TestLooksLikeDeferredSummaryPromise(t *testing.T) {
	t.Parallel()
	if !LooksLikeDeferredSummaryPromise("我先去调度团队，后台跑完再汇总。") {
		t.Fatal("want deferred-summary match")
	}
	if LooksLikeDeferredSummaryPromise("主体已核实，报告如下。") {
		t.Fatal("ordinary answer must not match")
	}
	if !strings.Contains(DeferredSummaryGuardCue, "后台跑完再汇总") {
		t.Fatal("guard cue must name the forbidden closeout")
	}
}

func TestPhasePromotedToolNames(t *testing.T) {
	t.Parallel()
	if names := PhasePromotedToolNames(SpiritPhaseIdle); len(names) != 0 {
		t.Fatalf("idle promoted = %v", names)
	}
	orch := PhasePromotedToolNames(SpiritPhaseOrchestrating)
	if len(orch) != 1 || orch[0] != "cancel_orchestration" {
		t.Fatalf("orchestrating = %v", orch)
	}
	ready := PhasePromotedToolNames(SpiritPhaseReady)
	if len(ready) != 2 {
		t.Fatalf("ready = %v", ready)
	}
}

func TestWantsNewOrchestration(t *testing.T) {
	t.Parallel()
	if WantsNewOrchestration("组建几个团队，分析金鹏科技行情") {
		t.Fatal("repeat ask must not want new")
	}
	if !WantsNewOrchestration("换标的分析茅台") {
		t.Fatal("换标的 must want new")
	}
}
