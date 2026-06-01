package monitor_test

import (
	"testing"

	"aranea-agents/internal/biz/monitor"
)

func TestSpanKindFromStep(t *testing.T) {
	tests := []struct {
		name   string
		stepID string
		domain string
		want   string
	}{
		{"empty_step", "", "", ""},
		{"chat_turn", "chat.turn", "", "root"},
		{"turn", "turn", "", "root"},
		{"llm_prefix", "llm.call", "", "llm"},
		{"model_prefix", "model.complete", "", "llm"},
		{"completion_prefix", "completion.generate", "", "llm"},
		{"tool_prefix", "tool.search", "", "tool"},
		{"function_prefix", "function.execute", "", "tool"},
		{"tool_suffix", "my.tool", "", "tool"},
		{"function_suffix", "my.function", "", "tool"},
		{"retrieve_prefix", "retrieve.docs", "", "memory"},
		{"memory_prefix", "memory.store", "", "memory"},
		{"recall_prefix", "recall.context", "", "memory"},
		{"graph_prefix", "graph.run", "", "graph"},
		{"node_prefix", "node.step", "", "graph"},
		{"node_suffix", "my.node", "", "graph"},
		{"hitl_prefix", "hitl.confirm", "", "hitl"},
		{"human_prefix", "human.review", "", "hitl"},
		{"confirm_prefix", "confirm.action", "", "hitl"},
		{"team_prefix", "team.delegate", "", "team"},
		{"subteam_prefix", "subteam.run", "", "team"},
		{"unknown_step", "unknown.step", "", "step"},
		{"case_insensitive", "LLM.Call", "", "llm"},
		{"case_insensitive_tool", "Tool.Search", "", "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.SpanKindFromStep(tt.stepID, tt.domain)
			if got != tt.want {
				t.Errorf("SpanKindFromStep(%q, %q) = %q, want %q", tt.stepID, tt.domain, got, tt.want)
			}
		})
	}
}

func TestMetaStr(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		key  string
		want string
	}{
		{"nil_map", nil, "key", ""},
		{"missing_key", map[string]any{"other": "val"}, "key", ""},
		{"nil_value", map[string]any{"key": nil}, "key", ""},
		{"string_value", map[string]any{"key": "hello"}, "key", "hello"},
		{"int_value", map[string]any{"key": 42}, "key", "42"},
		{"float_value", map[string]any{"key": 3.14}, "key", "3.14"},
		{"bool_value", map[string]any{"key": true}, "key", "true"},
		{"trimmed_value", map[string]any{"key": "  hello  "}, "key", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.MetaStr(tt.m, tt.key)
			if got != tt.want {
				t.Errorf("MetaStr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCoalesceStr(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{"both_nonempty", "first", "second", "first"},
		{"a_empty", "", "second", "second"},
		{"a_whitespace", "  ", "second", "second"},
		{"both_empty", "", "", ""},
		{"both_whitespace", "  ", "  ", "  "},
		{"a_nonempty_b_empty", "first", "", "first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monitor.CoalesceStr(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CoalesceStr(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
