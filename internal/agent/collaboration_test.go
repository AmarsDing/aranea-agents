package agent

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestCollaborationSketch_ParallelUsesSpiritSynthesis(t *testing.T) {
	got := collaborationSketch(&biz.TaskPlan{SubTasks: []biz.SubTask{
		{ID: "st1", Name: "文案", DomainPath: "创作/文案"},
		{ID: "st2", Name: "视觉", DomainPath: "设计/视觉"},
	}})
	if got.SlotCount != 2 || got.EdgeCount != 0 || got.Unifier != unifierSpiritOnly {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Summary, "创作/文案") || !strings.Contains(got.Summary, "设计/视觉") {
		t.Fatalf("summary %q", got.Summary)
	}
}

func TestCollaborationSketch_DAGUsesBriefHandoff(t *testing.T) {
	got := collaborationSketch(&biz.TaskPlan{SubTasks: []biz.SubTask{
		{ID: "st_a", DomainPath: "软件/后端"},
		{ID: "st_b", DomainPath: "软件/前端", DependsOn: []string{"st_a"}},
	}})
	if got.EdgeCount != 1 || got.Unifier != unifierBriefAndSpirit {
		t.Fatalf("%+v", got)
	}
}
