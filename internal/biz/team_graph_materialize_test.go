package biz

import "testing"

func TestMaterializeTeamGraphDefinitionBasic(t *testing.T) {
	team := Team{ID: "team-1", DisplayName: "评审团队"}
	cfg := GraphBuildConfig{
		Nodes: []NodeDef{
			{ID: "start", Type: "start"},
			{ID: "member-1", Type: "agent", AgentName: "agent-a"},
			{ID: "end", Type: "end"},
		},
		Edges: []EdgeDef{
			{From: "start", To: "member-1"},
			{From: "member-1", To: "end"},
		},
		StateFields: []StateFieldDef{{Name: "messages", Type: "[]message", Reducer: ReducerAppend}},
		EntryPoint:  "start",
		FinishPoint: "end",
	}
	def := MaterializeTeamGraphDefinition(team, cfg, nil, DefinitionGraphSourcePreset)
	if def == nil {
		t.Fatal("def is nil")
	}
	if def.TeamID != "team-1" {
		t.Fatalf("team_id=%q", def.TeamID)
	}
	if def.Name != "评审团队" {
		t.Fatalf("name=%q", def.Name)
	}
	if len(def.Nodes) != 3 || len(def.Edges) != 2 {
		t.Fatalf("nodes=%d edges=%d", len(def.Nodes), len(def.Edges))
	}
	if def.EntryPoint != "start" || def.FinishPoint != "end" {
		t.Fatalf("entry=%q finish=%q", def.EntryPoint, def.FinishPoint)
	}
	if got, _ := def.Metadata[GraphMetadataTeamOwnedKey].(bool); !got {
		t.Fatalf("team_owned marker missing: %v", def.Metadata)
	}
	if got, _ := def.Metadata[GraphMetadataTeamSourceKey].(string); got != DefinitionGraphSourcePreset {
		t.Fatalf("team_source=%q", got)
	}
}

func TestMaterializeTeamGraphDefinitionLayoutPreserved(t *testing.T) {
	existing := &GraphDefinition{
		ID:      "graph-1",
		Version: 3,
		Metadata: map[string]any{
			GraphMetadataLayoutKey: map[string]any{
				"member-1": map[string]any{"x": 120.0, "y": 40.0},
				"removed":  map[string]any{"x": 1.0, "y": 2.0},
			},
			"custom_meta": "keep",
		},
		Description: "原有描述",
	}
	team := Team{ID: "team-1", DisplayName: "评审团队"}
	cfg := GraphBuildConfig{
		Nodes: []NodeDef{
			{ID: "member-1", Type: "agent"},
			{ID: "member-2", Type: "agent"},
		},
	}
	def := MaterializeTeamGraphDefinition(team, cfg, existing, DefinitionGraphSourcePreset)
	if def.ID != "graph-1" || def.Version != 3 {
		t.Fatalf("id=%q version=%d", def.ID, def.Version)
	}
	if def.Description != "原有描述" {
		t.Fatalf("description=%q", def.Description)
	}
	if def.Metadata["custom_meta"] != "keep" {
		t.Fatalf("custom_meta lost: %v", def.Metadata)
	}
	layout, _ := def.Metadata[GraphMetadataLayoutKey].(map[string]any)
	if layout == nil {
		t.Fatalf("layout missing: %v", def.Metadata)
	}
	pos, _ := layout["member-1"].(map[string]any)
	if pos == nil || pos["x"] != 120.0 || pos["y"] != 40.0 {
		t.Fatalf("member-1 pos not preserved: %v", layout["member-1"])
	}
	if _, ok := layout["removed"]; ok {
		t.Fatalf("stale node kept: %v", layout)
	}
	if _, ok := layout["member-2"]; ok {
		t.Fatalf("new node should have no saved position (auto-layout on open): %v", layout)
	}
}

func TestMaterializeTeamGraphDefinitionCheckpointPassthrough(t *testing.T) {
	cfg := GraphBuildConfig{
		Nodes:            []NodeDef{{ID: "start", Type: "start"}},
		EnableCheckpoint: true,
		ExecutionEngine:  EngineDAG,
	}
	def := MaterializeTeamGraphDefinition(Team{ID: "t", DisplayName: "n"}, cfg, nil, DefinitionGraphSourceCustom)
	if !def.EnableCheckpoint {
		t.Fatal("enable_checkpoint not passed through")
	}
	if def.ExecutionEngine != EngineDAG {
		t.Fatalf("engine=%q", def.ExecutionEngine)
	}
	if got, _ := def.Metadata[GraphMetadataTeamSourceKey].(string); got != DefinitionGraphSourceCustom {
		t.Fatalf("team_source=%q", got)
	}
}
