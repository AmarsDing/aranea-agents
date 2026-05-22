package team

import (
	"testing"
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
	cfg, err := CompileToGraphBuildConfigFromJSON(def, raw, func(id string) string { return "key-" + id })
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
	snap := BuildCompileSnapshot(def, raw, nil)
	if !snap.Valid || snap.EntryPoint != "member-1" {
		t.Fatalf("snap=%+v", snap)
	}
}
