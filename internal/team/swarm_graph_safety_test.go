package team

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestApplyCrossRequestEntryOverride(t *testing.T) {
	cfg := biz.GraphBuildConfig{
		EntryPoint: "member-1",
		Nodes: []biz.NodeDef{
			{ID: "member-1", Type: biz.NodeTypeAgent, AgentName: "alpha"},
			{ID: "member-2", Type: biz.NodeTypeAgent, AgentName: "beta"},
		},
	}
	def := Definition{Swarm: &SwarmConfigDef{CrossRequestTransfer: true}}

	got := applyCrossRequestEntryOverride(cfg, def, "beta")
	if got.EntryPoint != "member-2" {
		t.Fatalf("want member-2, got %s", got.EntryPoint)
	}

	def.Swarm.CrossRequestTransfer = false
	got = applyCrossRequestEntryOverride(cfg, def, "beta")
	if got.EntryPoint != "member-1" {
		t.Fatalf("disabled should keep entry, got %s", got.EntryPoint)
	}
}

func TestReadSwarmActiveAgent(t *testing.T) {
	sess := biz.Session{MetadataJSON: `{"swarm_active_agent":"beta","other":1}`}
	if got := readSwarmActiveAgent(sess); got != "beta" {
		t.Fatalf("want beta, got %q", got)
	}
	if got := readSwarmActiveAgent(biz.Session{}); got != "" {
		t.Fatalf("empty meta => empty, got %q", got)
	}
}

func TestApplySwarmGraphConfig(t *testing.T) {
	cfg := biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "member-1", Type: biz.NodeTypeAgent},
			{ID: "tool-1", Type: biz.NodeTypeTool},
		},
	}
	def := Definition{Swarm: &SwarmConfigDef{
		MaxHandoffs:             5,
		RepetitiveHandoffWindow: 4,
		RepetitiveHandoffMinUnique: 2,
		NodeTimeoutSeconds:      30,
		CrossRequestTransfer:    true,
	}}
	got := applySwarmGraphConfig(cfg, def)
	if got.SwarmSafety == nil || got.SwarmSafety.MaxHandoffs != 5 {
		t.Fatalf("SwarmSafety not set: %+v", got.SwarmSafety)
	}
	if got.Nodes[0].TimeoutSeconds != 30 || !got.Nodes[0].IsolatedMessages {
		t.Fatalf("agent node not updated: %+v", got.Nodes[0])
	}
	if got.Nodes[1].TimeoutSeconds != 30 {
		t.Fatalf("tool timeout not set: %+v", got.Nodes[1])
	}
}
