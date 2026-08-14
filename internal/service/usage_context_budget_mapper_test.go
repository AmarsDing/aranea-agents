package service

import (
	"testing"

	"aranea-agents/internal/biz/usage"
)

// P2-1：biz ContextBudgetStats → proto 映射（整体构成/per-agent/per-day/top tools）。
func TestToProtoContextBudgetStats(t *testing.T) {
	in := usage.ContextBudgetStats{
		ContextBudgetComposition: usage.ContextBudgetComposition{
			Samples:          6,
			AvgEstTotalInput: 2666.5,
			AvgToolsCount:    10.5,
			CategoryAvgEstTokens: map[string]float64{
				"tools_schema": 1583.33,
				"history":      166.67,
			},
		},
		Agents: []usage.ContextBudgetAgentStats{
			{
				AgentID:  "a1",
				AgentKey: "agent-a",
				ContextBudgetComposition: usage.ContextBudgetComposition{
					Samples:              3,
					AvgEstTotalInput:     2333.33,
					CategoryAvgEstTokens: map[string]float64{"tools_schema": 1166.67},
				},
			},
		},
		Trends: []usage.ContextBudgetTrendPoint{
			{
				DateKey: "2026-08-12",
				ContextBudgetComposition: usage.ContextBudgetComposition{
					Samples:              2,
					AvgEstTotalInput:     2000,
					CategoryAvgEstTokens: map[string]float64{"tools_schema": 1000},
				},
			},
		},
		TopTools: []usage.ContextBudgetToolStat{
			{ToolName: "big_tool", Appearances: 5, AvgEstTokens: 800, MaxEstTokens: 1200},
		},
	}

	out := toProtoContextBudgetStats(in)
	if out.GetOverall().GetSamples() != 6 || out.GetOverall().GetAvgEstTotalInput() != 2666.5 {
		t.Errorf("overall = %+v, want samples 6 avg 2666.5", out.GetOverall())
	}
	if got := out.GetOverall().GetCategoryAvgEstTokens()["tools_schema"]; got != 1583.33 {
		t.Errorf("overall tools_schema = %v, want 1583.33", got)
	}
	if len(out.GetAgents()) != 1 || out.GetAgents()[0].GetAgentKey() != "agent-a" {
		t.Fatalf("agents = %+v, want one agent-a row", out.GetAgents())
	}
	if out.GetAgents()[0].GetComposition().GetSamples() != 3 {
		t.Errorf("agent composition samples = %d, want 3", out.GetAgents()[0].GetComposition().GetSamples())
	}
	if len(out.GetTrends()) != 1 || out.GetTrends()[0].GetDateKey() != "2026-08-12" {
		t.Fatalf("trends = %+v, want one 2026-08-12 row", out.GetTrends())
	}
	if out.GetTrends()[0].GetComposition().GetCategoryAvgEstTokens()["tools_schema"] != 1000 {
		t.Errorf("trend tools_schema = %v, want 1000",
			out.GetTrends()[0].GetComposition().GetCategoryAvgEstTokens()["tools_schema"])
	}
	if len(out.GetTopTools()) != 1 || out.GetTopTools()[0].GetToolName() != "big_tool" {
		t.Fatalf("top_tools = %+v, want one big_tool row", out.GetTopTools())
	}
	if out.GetTopTools()[0].GetMaxEstTokens() != 1200 {
		t.Errorf("top tool max = %v, want 1200", out.GetTopTools()[0].GetMaxEstTokens())
	}
}

func TestToProtoContextBudgetStats_Empty(t *testing.T) {
	out := toProtoContextBudgetStats(usage.ContextBudgetStats{})
	if out == nil || out.GetOverall() == nil {
		t.Fatalf("empty stats must still produce non-nil overall, got %+v", out)
	}
	if len(out.GetAgents()) != 0 || len(out.GetTrends()) != 0 || len(out.GetTopTools()) != 0 {
		t.Errorf("empty stats lists must be empty, got %+v", out)
	}
}
