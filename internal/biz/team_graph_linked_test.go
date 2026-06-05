package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
	"aranea-agents/pkg/loggateway"
)

func TestLinkedGraph_AdaptiveMode(t *testing.T) {
	lg := loggateway.NewNoop()

	def := team.Definition{
		Mode: biz.TeamModeAdaptive,
		Members: []team.MemberDef{
			{AgentID: "agent-1", Role: biz.RoleWorker, SortOrder: 1},
			{AgentID: "agent-2", Role: biz.RoleSynthesizer, SortOrder: 2},
		},
	}

	cfg, _, err := team.CompileToGraphBuildConfig(def, func(id string) string { return "key-" + id }, lg)
	if err != nil {
		t.Fatalf("CompileToGraphBuildConfig failed: %v", err)
	}

	if cfg.EntryPoint == "" {
		t.Error("expected non-empty entry point for adaptive mode")
	}
	if cfg.FinishPoint == "" {
		t.Error("expected non-empty finish point for adaptive mode")
	}
	if len(cfg.Nodes) == 0 {
		t.Error("expected non-empty nodes for adaptive mode")
	}

	// Verify adaptive mode produces transfer edges (handoff pattern)
	transferCount := 0
	for _, e := range cfg.Edges {
		if e.Kind == biz.EdgeKindTransfer {
			transferCount++
		}
	}
	if transferCount == 0 {
		t.Error("expected at least one transfer edge for adaptive mode")
	}
}

func TestLinkedGraph_SequentialMode(t *testing.T) {
	lg := loggateway.NewNoop()

	def := team.Definition{
		Mode: biz.TeamModeSequential,
		Members: []team.MemberDef{
			{AgentID: "a1", Role: biz.RoleWorker, SortOrder: 1},
			{AgentID: "a2", Role: biz.RoleWorker, SortOrder: 2},
		},
	}

	cfg, _, err := team.CompileToGraphBuildConfig(def, nil, lg)
	if err != nil {
		t.Fatalf("CompileToGraphBuildConfig failed: %v", err)
	}

	if len(cfg.Nodes) == 0 {
		t.Error("expected non-empty nodes for sequential mode")
	}
	if cfg.EntryPoint == "" || cfg.FinishPoint == "" {
		t.Error("expected non-empty entry/finish points")
	}
}

func TestLinkedGraph_CoordinatorMode(t *testing.T) {
	lg := loggateway.NewNoop()

	def := team.Definition{
		Mode: biz.TeamModeCoordinator,
		Members: []team.MemberDef{
			{AgentID: "lead", Role: biz.RoleCoordinator, SortOrder: 1},
			{AgentID: "w1", SortOrder: 2},
			{AgentID: "w2", SortOrder: 3},
		},
	}

	cfg, _, err := team.CompileToGraphBuildConfig(def, nil, lg)
	if err != nil {
		t.Fatalf("CompileToGraphBuildConfig failed: %v", err)
	}

	if len(cfg.Nodes) == 0 {
		t.Error("expected non-empty nodes for coordinator mode")
	}

	// Verify dispatch edges exist for coordinator mode
	dispatchCount := 0
	for _, e := range cfg.Edges {
		if e.Kind == biz.EdgeKindDispatch {
			dispatchCount++
		}
	}
	if dispatchCount == 0 {
		t.Error("expected at least one dispatch edge for coordinator mode")
	}
}
