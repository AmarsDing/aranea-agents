package decision

import (
	"context"
	"strings"
)

// gate_stats.go — 79-runtime-governance R7：run 级系统闸聚合格（design §8.1）。
// 数据面全部落在既有 decision_records（category=system_guard，
// source_ref.run_id 归属），不落新表；本文件只是读侧聚合契约。

// RunGateStats 是一个 team run 的系统闸事件聚合。
type RunGateStats struct {
	// LoopGuardBlocks 是 loop_guard_blocked 记录条数。
	LoopGuardBlocks int
	// BudgetTripped 报告该 run 是否触发过 token_budget_tripped。
	BudgetTripped bool
	// NoProgressTripped 报告该 run 是否触发过 no_progress_tripped。
	NoProgressTripped bool
	// PruneCount 是被剪枝 tool result 总条数（tool_result_pruned 记录的
	// observed_value 求和——每条记录代表一次剪枝动作，observed_value 是该次
	// 剪掉的 result 条数）。
	PruneCount int
	// PruneBytes 是被剪枝原文总字节数（metadata.prune_bytes 求和）。
	PruneBytes int64
	// CompactCount 是终审压缩触发次数（context_compacted 记录条数）。
	CompactCount int
	// ParamRuleDenies 是工具参数门禁 deny 记录条数（param_rule_deny，
	// 79-runtime-governance R9；2026-08-27 二轮审查 M1 补入契约）。
	ParamRuleDenies int
}

// RunGateStatsRepo 是 run 级闸统计的窄读接口（同 RunCacheHitRatioRepo 的
// type-assertion 解析模式）：composite repo 不动，无该能力的实现返回零值。
//
// Stability:evolving
type RunGateStatsRepo interface {
	// RunGateStats 聚合一个 run 的 system_guard 记录。runID 为空或未命中
	// 任何记录时返回零值, nil。
	RunGateStats(ctx context.Context, runID string) (RunGateStats, error)
}

// RunGateStats 服务 R7 stats API 读路径。
func (u *QueryUsecase) RunGateStats(ctx context.Context, runID string) (RunGateStats, error) {
	if u == nil || u.repo == nil || strings.TrimSpace(runID) == "" {
		return RunGateStats{}, nil
	}
	repo, ok := u.repo.(RunGateStatsRepo)
	if !ok {
		return RunGateStats{}, nil
	}
	return repo.RunGateStats(ctx, runID)
}
