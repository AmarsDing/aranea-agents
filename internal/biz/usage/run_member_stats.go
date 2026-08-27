package usage

import (
	"context"
	"strings"
)

// run_member_stats.go — 79-runtime-governance R7：run 级成员维度 token 聚合。
// stats API 的 members 段（design §8.1：agent_key/prompt_tokens/cached_tokens/
// steps）需要按成员分列的 token 账单；数据面复用 model_token_usage_events
// team_member 行，不落新表。

// RunMemberUsage 是一个成员在一个 run 内的用量聚合。
type RunMemberUsage struct {
	AgentKey string
	// PromptTok/CompletionTok/CachedTok 是该成员 team_member 行的求和。
	// 口径（2026-08-27 最终裁定）：**纳入** attribution 标记行——graph
	// runtime 下带 token 的 member 行全带 member_level_stream（成员 run
	// 总量）/stream_anchor_remainder（归 anchor）/run_level_anchor_fallback
	// （与二者互斥）标记，过滤会使 members 段恒空；标记行互斥且 team_turn
	// 总账行按 usage_kind 排除，无双计。与 RunCacheHitRatio 回退分支的
	// 旧口径（attribution='' 过滤）属不同服务场景，勿对齐（该分支仅覆盖
	// 无 team_turn 行的失败/熔断 run，维持 P2-1 语义）。
	PromptTok     int64
	CompletionTok int64
	CachedTok     int64
	// Calls 是成员用量行数（非 step 数；step 数由 team_run_steps 聚合，
	// 装配层按 agent_key 合流）。
	Calls int
}

// RunMemberUsageRepo 从用量事件面读取 run 级成员聚合。
//
// Stability:evolving
type RunMemberUsageRepo interface {
	// RunMemberUsageStats 聚合一个 run 的 team_member 行 GROUP BY
	// agent_key（含 attribution 标记行，口径见 RunMemberUsage）。
	// runID 为空或无命中返回 nil, nil。
	RunMemberUsageStats(ctx context.Context, runID string) ([]RunMemberUsage, error)
}

// RunMemberUsageStats 服务 R7 stats API 读路径。窄能力 type-assertion 解析，
// repo 无该能力时返回 nil（装配层按「无成员用量」降级，steps 仍由
// team_run_steps 供应）。
func (u *Usecase) RunMemberUsageStats(ctx context.Context, runID string) ([]RunMemberUsage, error) {
	if u == nil || strings.TrimSpace(runID) == "" {
		return nil, nil
	}
	repo, ok := u.repo.(RunMemberUsageRepo)
	if !ok {
		return nil, nil
	}
	return repo.RunMemberUsageStats(ctx, runID)
}
