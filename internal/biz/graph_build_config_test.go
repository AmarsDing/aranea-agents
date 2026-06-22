package biz

import (
	"encoding/json"
	"testing"
)

func TestBuildConfigFromGraphDefinition(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		cfg := BuildConfigFromGraphDefinition(nil)
		if cfg.EntryPoint != "" || len(cfg.Nodes) != 0 {
			t.Fatalf("expected zero value, got %+v", cfg)
		}
	})

	t.Run("full_mapping", func(t *testing.T) {
		def := &GraphDefinition{
			ID:               "g1",
			Name:             "demo",
			EntryPoint:       "start",
			FinishPoint:      "end",
			EnableCheckpoint: true,
			ExecutionEngine:  EngineDAG,
			Nodes:            []NodeDef{{ID: "start", Type: "llm"}, {ID: "end", Type: "llm"}},
			Edges:            []EdgeDef{{From: "start", To: "end"}},
			ConditionalEdges: []ConditionalEdgeDef{{From: "start", CondFuncRef: "route_fn", PathMap: map[string]string{"yes": "end"}}},
			StateFields:      []StateFieldDef{{Name: "count", Type: "int", Reducer: ReducerDefault}},
			InterruptBefore:  []string{"start"},
			InterruptAfter:   []string{"end"},
		}
		cfg := BuildConfigFromGraphDefinition(def)
		if cfg.EntryPoint != "start" || cfg.FinishPoint != "end" {
			t.Fatalf("entry/finish: %+v", cfg)
		}
		if !cfg.EnableCheckpoint || cfg.ExecutionEngine != EngineDAG {
			t.Fatalf("checkpoint/engine: %+v", cfg)
		}
		if len(cfg.Nodes) != 2 || len(cfg.Edges) != 1 {
			t.Fatalf("nodes/edges: %+v", cfg)
		}
		if len(cfg.ConditionalEdges) != 1 || cfg.ConditionalEdges[0].CondFuncRef != "route_fn" {
			t.Fatalf("cond_edges: %+v", cfg.ConditionalEdges)
		}
		if len(cfg.StateFields) != 1 || cfg.StateFields[0].Name != "count" {
			t.Fatalf("state_fields: %+v", cfg.StateFields)
		}
		if len(cfg.InterruptBefore) != 1 || cfg.InterruptBefore[0] != "start" {
			t.Fatalf("interrupt_before: %+v", cfg.InterruptBefore)
		}
		if len(cfg.InterruptAfter) != 1 || cfg.InterruptAfter[0] != "end" {
			t.Fatalf("interrupt_after: %+v", cfg.InterruptAfter)
		}
	})
}

func TestGraphDefinitionFromBuildConfig(t *testing.T) {
	t.Run("name_fallback_to_id", func(t *testing.T) {
		cfg := GraphBuildConfig{
			EntryPoint:  "start",
			FinishPoint: "end",
			Nodes:       []NodeDef{{ID: "start", Type: "llm"}},
		}
		def := GraphDefinitionFromBuildConfig(cfg, "g1", "")
		if def.Name != "g1" {
			t.Fatalf("name=%q want g1", def.Name)
		}
	})

	t.Run("preserves_name", func(t *testing.T) {
		cfg := GraphBuildConfig{EntryPoint: "s", FinishPoint: "e"}
		def := GraphDefinitionFromBuildConfig(cfg, "g1", "my-graph")
		if def.Name != "my-graph" {
			t.Fatalf("name=%q want my-graph", def.Name)
		}
	})

	t.Run("defensive_copy", func(t *testing.T) {
		cfg := GraphBuildConfig{
			Nodes:           []NodeDef{{ID: "n1"}},
			Edges:           []EdgeDef{{From: "a", To: "b"}},
			InterruptBefore: []string{"n1"},
		}
		def := GraphDefinitionFromBuildConfig(cfg, "g1", "test")
		cfg.Nodes[0].ID = "mutated"
		if def.Nodes[0].ID == "mutated" {
			t.Fatal("expected defensive copy, but slice was shared")
		}
	})
}

func TestCompactNodesForVersion(t *testing.T) {
	nodes := []NodeDef{
		{
			ID:               "n1",
			Type:             "llm",
			Description:      "some desc",
			Instruction:      "do this",
			InputMapperJSON:  `{"x":1}`,
			OutputMapperJSON: `{"y":2}`,
		},
	}
	out := compactNodesForVersion(nodes)
	if out[0].Description != "" {
		t.Errorf("description should be cleared")
	}
	if out[0].Instruction != "" {
		t.Errorf("instruction should be cleared")
	}
	if out[0].InputMapperJSON != "" {
		t.Errorf("input_mapper should be cleared")
	}
	if out[0].OutputMapperJSON != "" {
		t.Errorf("output_mapper should be cleared")
	}
	if out[0].ID != "n1" || out[0].Type != "llm" {
		t.Errorf("id/type should be preserved: %+v", out[0])
	}
	if nodes[0].Description != "some desc" {
		t.Errorf("original slice should not be mutated")
	}
}

func TestReadUserTemplateMeta(t *testing.T) {
	t.Run("nil_def", func(t *testing.T) {
		if m := ReadUserTemplateMeta(nil); m != nil {
			t.Fatalf("expected nil, got %+v", m)
		}
	})

	t.Run("nil_metadata", func(t *testing.T) {
		if m := ReadUserTemplateMeta(&GraphDefinition{}); m != nil {
			t.Fatalf("expected nil, got %+v", m)
		}
	})

	t.Run("missing_key", func(t *testing.T) {
		if m := ReadUserTemplateMeta(&GraphDefinition{Metadata: map[string]any{}}); m != nil {
			t.Fatalf("expected nil, got %+v", m)
		}
	})

	t.Run("valid_meta", func(t *testing.T) {
		meta := UserTemplateMeta{
			TemplateID:  "user:g1",
			Name:        "My Template",
			Category:    "custom",
			Description: "A test template",
		}
		def := &GraphDefinition{Metadata: map[string]any{GraphMetadataUserTemplateKey: meta}}
		got := ReadUserTemplateMeta(def)
		if got == nil || got.TemplateID != "user:g1" {
			t.Fatalf("got=%+v", got)
		}
		if got.Name != "My Template" || got.Category != "custom" {
			t.Fatalf("name/category: %+v", got)
		}
	})

	t.Run("empty_template_id", func(t *testing.T) {
		meta := UserTemplateMeta{Name: "No ID"}
		def := &GraphDefinition{Metadata: map[string]any{GraphMetadataUserTemplateKey: meta}}
		if m := ReadUserTemplateMeta(def); m != nil {
			t.Fatalf("expected nil for empty template_id, got %+v", m)
		}
	})
}

func TestWriteUserTemplateMeta(t *testing.T) {
	t.Run("nil_def", func(t *testing.T) {
		WriteUserTemplateMeta(nil, UserTemplateMeta{TemplateID: "t1"})
	})

	t.Run("nil_metadata_creates_map", func(t *testing.T) {
		def := &GraphDefinition{}
		WriteUserTemplateMeta(def, UserTemplateMeta{TemplateID: "user:g1", Name: "Test"})
		if def.Metadata == nil {
			t.Fatal("metadata should be created")
		}
		got := ReadUserTemplateMeta(def)
		if got == nil || got.TemplateID != "user:g1" {
			t.Fatalf("got=%+v", got)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		def := &GraphDefinition{Metadata: map[string]any{}}
		meta := UserTemplateMeta{
			TemplateID:  "user:g2",
			Name:        "Roundtrip",
			Category:    "analytics",
			Description: "Roundtrip test",
		}
		WriteUserTemplateMeta(def, meta)
		got := ReadUserTemplateMeta(def)
		if got == nil {
			t.Fatal("expected non-nil")
		}
		if *got != meta {
			t.Fatalf("roundtrip mismatch: got=%+v want=%+v", *got, meta)
		}
	})
}

func TestGraphValidationResult(t *testing.T) {
	t.Run("nil_HasErrors", func(t *testing.T) {
		var r *GraphValidationResult
		if r.HasErrors() {
			t.Error("nil result should not have errors")
		}
	})

	t.Run("nil_HasWarnings", func(t *testing.T) {
		var r *GraphValidationResult
		if r.HasWarnings() {
			t.Error("nil result should not have warnings")
		}
	})

	t.Run("empty_errors", func(t *testing.T) {
		r := &GraphValidationResult{}
		if r.HasErrors() {
			t.Error("empty result should not have errors")
		}
	})

	t.Run("with_errors", func(t *testing.T) {
		r := &GraphValidationResult{
			Errors: []GraphValidationIssue{{Code: "no_entry_point", Message: "missing"}},
		}
		if !r.HasErrors() {
			t.Error("should have errors")
		}
		if r.HasWarnings() {
			t.Error("should not have warnings")
		}
	})

	t.Run("with_warnings", func(t *testing.T) {
		r := &GraphValidationResult{
			Warnings: []GraphValidationIssue{{Code: "orphan_node", Message: "dangling"}},
		}
		if r.HasErrors() {
			t.Error("should not have errors")
		}
		if !r.HasWarnings() {
			t.Error("should have warnings")
		}
	})
}

func TestGraphStepSnapshotToJSON(t *testing.T) {
	steps := []GraphStepSnapshot{
		{NodeID: "n1", StepIndex: 0, Status: "completed"},
		{NodeID: "n2", StepIndex: 1, Status: "failed", Error: "timeout"},
	}
	raw := graphStepSnapshotToJSON(steps)
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("invalid json: %s", raw)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(parsed))
	}
	if parsed[0]["node_id"] != "n1" || parsed[0]["status"] != "completed" {
		t.Fatalf("step0=%+v", parsed[0])
	}
	if parsed[1]["error"] != "timeout" {
		t.Fatalf("step1 error=%v", parsed[1]["error"])
	}
}
