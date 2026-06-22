package biz

import "testing"

func TestDefToBuildConfig_PreservesGraphShape(t *testing.T) {
	def := &GraphDefinition{
		ID:          "g1",
		Name:        "demo",
		EntryPoint:  "start",
		FinishPoint: "end",
		Nodes: []NodeDef{
			{ID: "start", Type: "llm"},
			{ID: "agent", Type: "agent", AgentName: "helper"},
			{ID: "route", Type: "router"},
			{ID: "end", Type: "llm"},
		},
		Edges:            []EdgeDef{{From: "start", To: "agent"}, {From: "agent", To: "end"}},
		EnableCheckpoint: true,
		ExecutionEngine:  EngineDAG,
	}
	cfg := defToBuildConfig(def)
	if cfg.EntryPoint != "start" || cfg.FinishPoint != "end" {
		t.Fatalf("entry/finish: %+v", cfg)
	}
	if len(cfg.Nodes) != 4 || !cfg.EnableCheckpoint || cfg.ExecutionEngine != EngineDAG {
		t.Fatalf("nodes/checkpoint/engine: %+v", cfg)
	}
	if cfg.Nodes[1].AgentName != "helper" || cfg.Nodes[2].Type != "router" {
		t.Fatalf("agent/router nodes: %+v", cfg.Nodes[1:3])
	}
}
