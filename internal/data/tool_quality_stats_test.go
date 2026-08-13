package data

import (
	"context"
	"math"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestToolQualityStatsRepo_AggregatesArgsQuality 验证按工具聚合的调用质量
// 统计：总数/成功/失败/修复数/无效参数数/成功率/参数一次合法率/平均耗时。
// 数据来源 tool_invocations.metadata_json 的 args_repaired / args_invalid。
func TestToolQualityStatsRepo_AggregatesArgsQuality(t *testing.T) {
	ctx := context.Background()
	repo, d := newToolTestRepo(t)
	now := time.Now().UTC().Truncate(time.Second)
	mk := func(toolKey, status string, repaired, invalid bool, dur int) biz.ToolInvocationWrite {
		return biz.ToolInvocationWrite{
			ToolKey:      toolKey,
			Status:       status,
			DurationMS:   dur,
			StartedAt:    now.Format(time.RFC3339),
			EndedAt:      now.Format(time.RFC3339),
			ArgsRepaired: repaired,
			ArgsInvalid:  invalid,
		}
	}
	seeds := []biz.ToolInvocationWrite{
		mk("tool_a", "success", false, false, 100),
		mk("tool_a", "success", true, false, 200),
		mk("tool_a", "failed", false, false, 300),
		mk("tool_b", "failed", false, true, 50),
	}
	for _, w := range seeds {
		if err := repo.RecordToolInvocation(ctx, w); err != nil {
			t.Fatalf("seed %s: %v", w.ToolKey, err)
		}
	}

	stats, err := NewToolQualityStatsRepo(d).GetToolQualityStats(ctx, "", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("GetToolQualityStats: %v", err)
	}
	byTool := make(map[string]biz.ToolQualityStat, len(stats))
	for _, s := range stats {
		byTool[s.ToolKey] = s
	}

	a, ok := byTool["tool_a"]
	if !ok {
		t.Fatalf("tool_a missing in %+v", stats)
	}
	if a.Count != 3 || a.SuccessCount != 2 || a.FailureCount != 1 || a.RepairedCount != 1 || a.InvalidCount != 0 {
		t.Fatalf("tool_a counts wrong: %+v", a)
	}
	if math.Abs(a.SuccessRate-2.0/3.0) > 1e-9 {
		t.Fatalf("tool_a SuccessRate = %v, want 2/3", a.SuccessRate)
	}
	// 参数一次合法率 = 1 - (repaired+invalid)/count = 1 - 1/3
	if math.Abs(a.ArgsFirstPassRate-2.0/3.0) > 1e-9 {
		t.Fatalf("tool_a ArgsFirstPassRate = %v, want 2/3", a.ArgsFirstPassRate)
	}
	if a.AvgDurationMs != 200 {
		t.Fatalf("tool_a AvgDurationMs = %v, want 200", a.AvgDurationMs)
	}

	b, ok := byTool["tool_b"]
	if !ok {
		t.Fatalf("tool_b missing in %+v", stats)
	}
	if b.InvalidCount != 1 || b.ArgsFirstPassRate != 0 {
		t.Fatalf("tool_b wrong: %+v", b)
	}

	// agent 过滤：未知 agent 必须为空
	empty, err := NewToolQualityStatsRepo(d).GetToolQualityStats(ctx, "agent-nope", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("filtered GetToolQualityStats: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("unknown agent must yield no rows, got %+v", empty)
	}
}
