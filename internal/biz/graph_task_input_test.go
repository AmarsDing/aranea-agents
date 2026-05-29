package biz

import "testing"

func TestGraphTaskInputFromNode(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		role, mode, strategy, input := GraphTaskInputFromNode(NodeDef{ID: "t1", Type: "task"})
		if role != "" {
			t.Errorf("role=%q want empty", role)
		}
		if mode != "static" {
			t.Errorf("mode=%q want static", mode)
		}
		if strategy != "" {
			t.Errorf("strategy=%q want empty", strategy)
		}
		if input != "t1" {
			t.Errorf("input=%q want t1", input)
		}
	})

	t.Run("all_fields", func(t *testing.T) {
		node := NodeDef{
			ID:                 "review-1",
			Type:               "review",
			RequiredRole:       "  admin  ",
			AssignmentMode:     "  round_robin  ",
			AssignmentStrategy: "  least_busy  ",
			Description:        "  Review the output  ",
		}
		role, mode, strategy, input := GraphTaskInputFromNode(node)
		if role != "admin" {
			t.Errorf("role=%q want admin", role)
		}
		if mode != "round_robin" {
			t.Errorf("mode=%q want round_robin", mode)
		}
		if strategy != "least_busy" {
			t.Errorf("strategy=%q want least_busy", strategy)
		}
		if input != "Review the output" {
			t.Errorf("input=%q want trimmed description", input)
		}
	})

	t.Run("empty_mode_falls_back_to_static", func(t *testing.T) {
		_, mode, _, _ := GraphTaskInputFromNode(NodeDef{ID: "n1", AssignmentMode: "   "})
		if mode != "static" {
			t.Errorf("mode=%q want static", mode)
		}
	})

	t.Run("description_fallback_to_id", func(t *testing.T) {
		_, _, _, input := GraphTaskInputFromNode(NodeDef{ID: "n1", Description: "   "})
		if input != "n1" {
			t.Errorf("input=%q want n1", input)
		}
	})
}

func TestNodeDefFromConfig(t *testing.T) {
	cfg := GraphBuildConfig{
		Nodes: []NodeDef{
			{ID: "start", Type: "llm"},
			{ID: "end", Type: "llm"},
		},
	}

	t.Run("found", func(t *testing.T) {
		n := nodeDefFromConfig(cfg, "start")
		if n == nil || n.ID != "start" {
			t.Fatalf("n=%+v", n)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		n := nodeDefFromConfig(cfg, "missing")
		if n != nil {
			t.Fatalf("expected nil, got %+v", n)
		}
	})

	t.Run("empty_config", func(t *testing.T) {
		n := nodeDefFromConfig(GraphBuildConfig{}, "any")
		if n != nil {
			t.Fatalf("expected nil, got %+v", n)
		}
	})
}
