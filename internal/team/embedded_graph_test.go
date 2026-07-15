package team

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestCompileToGraphBuildConfig_embeddedGraph(t *testing.T) {
	raw := `{
		"mode":"sequential",
		"members":[{"agent_id":"a1","sort_order":1,"name":"A"},{"agent_id":"a2","sort_order":2,"name":"B"}],
		"graph":{
			"version":1,
			"layout":"linear",
			"nodes":[
				{"id":"start","type":"start","label":"开始"},
				{"id":"member-1","type":"agent","label":"A","agent_id":"a1","role":"worker"},
				{"id":"member-2","type":"agent","label":"B","agent_id":"a2","role":"worker"},
				{"id":"end","type":"end","label":"结束"}
			],
			"edges":[
				{"id":"s1","source":"start","target":"member-1"},
				{"id":"12","source":"member-1","target":"member-2"},
				{"id":"2e","source":"member-2","target":"end"}
			]
		}
	}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := CompileToGraphBuildConfigFromJSON(def, raw, func(id string) string { return "key-" + id }, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EntryPoint != "member-1" || cfg.FinishPoint != "member-2" {
		t.Fatalf("entry/finish=%q/%q", cfg.EntryPoint, cfg.FinishPoint)
	}
	if len(cfg.Edges) != 1 || cfg.Edges[0].From != "member-1" || cfg.Edges[0].To != "member-2" {
		t.Fatalf("edges=%+v", cfg.Edges)
	}
	if cfg.Nodes[0].AgentName != "key-a1" {
		t.Fatalf("agent=%q", cfg.Nodes[0].AgentName)
	}
}

func TestBuildCompileSnapshot_embeddedGraph(t *testing.T) {
	raw := `{"mode":"sequential","members":[{"agent_id":"a1","sort_order":1}],"graph":{"nodes":[{"id":"member-1","type":"agent","agent_id":"a1"}],"edges":[]}}`
	def, _ := ParseDefinition(raw)
	snap := BuildCompileSnapshot(def, raw, nil, loggateway.NewNoop())
	if !snap.Valid || snap.EntryPoint != "member-1" {
		t.Fatalf("snap=%+v", snap)
	}
}

func TestCompileToGraphBuildConfig_embeddedTaskNode(t *testing.T) {
	raw := `{
		"mode":"sequential",
		"members":[],
		"graph":{
			"nodes":[
				{"id":"start","type":"start"},
				{"id":"review-1","type":"review","label":"人工审核","reviewer_agent":"critic","review_rules":"approve"},
				{"id":"end","type":"end"}
			],
			"edges":[
				{"source":"start","target":"review-1"},
				{"source":"review-1","target":"end"}
			]
		}
	}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, taskMeta, err := CompileToGraphBuildConfigFromJSON(def, raw, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Nodes) != 1 || cfg.Nodes[0].Type != "review" {
		t.Fatalf("nodes=%+v", cfg.Nodes)
	}
	if !cfg.Nodes[0].InterruptAfter {
		t.Fatalf("review node=%+v", cfg.Nodes[0])
	}
	if m, ok := taskMeta[cfg.Nodes[0].ID]; !ok || m.ReviewerAgent != "critic" {
		t.Fatalf("review taskMeta=%+v", taskMeta)
	}
}

func TestCompileToGraphBuildConfig_embeddedSubgraph(t *testing.T) {
	loader := stubGraphLoader{configs: map[string]biz.GraphBuildConfig{
		"g-nested": {
			Nodes:      []biz.NodeDef{{ID: "inner", Type: "agent", AgentName: "inner-agent"}},
			Edges:      []biz.EdgeDef{},
			EntryPoint: "inner", FinishPoint: "inner",
		},
	}}
	raw := `{
		"mode":"sequential",
		"members":[],
		"graph":{
			"nodes":[
				{"id":"start","type":"start"},
				{"id":"sub-1","type":"subgraph","subgraph_id":"g-nested"},
				{"id":"end","type":"end"}
			],
			"edges":[
				{"source":"start","target":"sub-1"},
				{"source":"sub-1","target":"end"}
			]
		}
	}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, _, err := compileToGraphBuildConfigWithLoader(context.Background(), def, raw, nil, loader, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Subgraphs) != 1 || cfg.Subgraphs[0].GraphID != "g-nested" {
		t.Fatalf("subgraphs=%+v", cfg.Subgraphs)
	}
	if cfg.EntryPoint != "sub-1" || cfg.FinishPoint != "sub-1" {
		t.Fatalf("entry/finish=%q/%q", cfg.EntryPoint, cfg.FinishPoint)
	}
}

func TestLoadEmbeddedSubgraphConfig_cycle(t *testing.T) {
	loader := stubGraphLoader{configs: map[string]biz.GraphBuildConfig{
		"a": {Subgraphs: []biz.SubgraphDef{{ID: "s", GraphID: "b"}}},
		"b": {Subgraphs: []biz.SubgraphDef{{ID: "s", GraphID: "a"}}},
	}}
	_, err := loadEmbeddedSubgraphConfig(context.Background(), loader, "a", map[string]struct{}{})
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

type stubGraphLoader struct {
	configs map[string]biz.GraphBuildConfig
}

func (s stubGraphLoader) LoadGraphBuildConfig(_ context.Context, graphID string) (biz.GraphBuildConfig, error) {
	cfg, ok := s.configs[graphID]
	if !ok {
		return biz.GraphBuildConfig{}, fmt.Errorf("not found")
	}
	return cfg, nil
}

func TestCompileToGraphBuildConfig_embeddedParallelJoin(t *testing.T) {
	raw := `{
		"mode":"parallel",
		"synthesizer_agent_id":"a3",
		"members":[
			{"agent_id":"a1","sort_order":10,"name":"W1","role":"worker"},
			{"agent_id":"a2","sort_order":20,"name":"W2","role":"worker"},
			{"agent_id":"a3","sort_order":30,"name":"Synth","role":"synthesizer"}
		],
		"failure_policy":{"parallel_fail":"continue"},
		"graph":{
			"nodes":[
				{"id":"start","type":"start"},
				{"id":"member-10","type":"agent","agent_id":"a1"},
				{"id":"member-20","type":"agent","agent_id":"a2"},
				{"id":"member-30","type":"agent","agent_id":"a3","role":"synthesizer"},
				{"id":"join","type":"join","label":"join"},
				{"id":"end","type":"end"}
			],
			"edges":[
				{"source":"start","target":"member-10"},
				{"source":"start","target":"member-20"},
				{"source":"member-10","target":"join"},
				{"source":"member-20","target":"join"},
				{"source":"join","target":"member-30"},
				{"source":"member-30","target":"end"}
			]
		}
	}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, branchIDs, err := compileToGraphBuildConfigWithLoader(context.Background(), def, raw, nil, stubGraphLoader{}, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FinishPoint != "member-30" {
		t.Fatalf("finish=%q want member-30", cfg.FinishPoint)
	}
	if len(branchIDs) != 2 {
		t.Fatalf("branchIDs=%v", branchIDs)
	}
	runtimeCt, err := CompileToGraphRuntimeConfigFromJSON(context.Background(), def, raw, nil, nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	skipCount := 0
	for _, n := range runtimeCt.Nodes {
		if n.FailureAction == biz.FailureOnFailureSkip {
			skipCount++
		}
	}
	if skipCount < 2 {
		t.Fatalf("expected 2 skip-on-failure branches, nodes=%+v", runtimeCt.Nodes)
	}
}

// TestCompileToGraphBuildConfig_unknownEdgeRejected (C-22) verifies that edges
// referencing unknown nodes are rejected instead of silently dropped.
func TestCompileToGraphBuildConfig_unknownEdgeRejected(t *testing.T) {
	raw := `{
		"mode":"sequential",
		"members":[{"agent_id":"a1","sort_order":1}],
		"graph":{
			"nodes":[
				{"id":"start","type":"start"},
				{"id":"member-1","type":"agent","agent_id":"a1"},
				{"id":"end","type":"end"}
			],
			"edges":[
				{"source":"start","target":"member-1"},
				{"source":"member-1","target":"ghost-node"},
				{"source":"member-1","target":"end"}
			]
		}
	}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = CompileToGraphBuildConfigFromJSON(def, raw, nil, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for edge referencing unknown node 'ghost-node', got nil (fail-open)")
	}
}

// TestCompileToGraphBuildConfig_multiNodeNoEntryRejected (C-22) verifies that
// a multi-node graph without start/end decorators is rejected (no guessing).
func TestCompileToGraphBuildConfig_multiNodeNoEntryRejected(t *testing.T) {
	raw := `{
		"mode":"sequential",
		"members":[{"agent_id":"a1","sort_order":1},{"agent_id":"a2","sort_order":2}],
		"graph":{
			"nodes":[
				{"id":"member-1","type":"agent","agent_id":"a1"},
				{"id":"member-2","type":"agent","agent_id":"a2"}
			],
			"edges":[
				{"source":"member-1","target":"member-2"}
			]
		}
	}`
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = CompileToGraphBuildConfigFromJSON(def, raw, nil, loggateway.NewNoop())
	if err == nil {
		t.Fatal("expected error for multi-node graph without start/end decorators, got nil (entry/finish guessing)")
	}
}

// TestCompileToGraphBuildConfig_singleNodeNoEntryOK (C-22) verifies that a
// single-node graph without start/end still works (implicit entry/finish).
func TestCompileToGraphBuildConfig_singleNodeNoEntryOK(t *testing.T) {
	raw := `{"mode":"sequential","members":[{"agent_id":"a1","sort_order":1}],"graph":{"nodes":[{"id":"member-1","type":"agent","agent_id":"a1"}],"edges":[]}}`
	def, _ := ParseDefinition(raw)
	cfg, _, err := CompileToGraphBuildConfigFromJSON(def, raw, func(id string) string { return "key-" + id }, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("expected success for single-node graph without start/end, got error: %v", err)
	}
	if cfg.EntryPoint != "member-1" || cfg.FinishPoint != "member-1" {
		t.Fatalf("entry/finish=%q/%q, want member-1/member-1", cfg.EntryPoint, cfg.FinishPoint)
	}
}
