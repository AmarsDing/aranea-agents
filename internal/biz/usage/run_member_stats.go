package usage

import (
	"context"
	"strings"
)

// run_member_stats.go — 79-runtime-governance R7：run 级成员维度 token 聚合。
// stats API 的 members 段（design §8.1：agent_key/prompt_tokens/cached_tokens/
// steps）需要按成员分列的 token 账单；数据面复用 model_token_usage_events
// genuine team_member 行（每行=一次模型调用），不落新表。

// RunMemberUsage 是一个成员在一个 run 内的用量聚合。
type RunMemberUsage struct {
	AgentKey string
	// PromptTok/CompletionTok/CachedTok 是该成员 genuine 调用行的求和
	// （attribution 非空的镜像行排除，防与 team_turn 总账双计——同
	// RunCacheHitRatio 回退分支语义）。
	PromptTok     int64
	CompletionTok int64
	CachedTok     int64
	// Calls 是 genuine 调用行数（= 模型调用次数，非 step 数；step 数由
	// team_run_steps 聚合，装配层按 agent_key 合流）。
	Calls int
}

// RunMemberUsageRepo 从用量事件面读取 run 级成员聚合。
//
// Stability:evolving
type RunMemberUsageRepo interface {
	// RunMemberUsageStats 聚合一个 run 的 genuine team_member 行
	// GROUP BY agent_key。runID 为空或无命中返回 nil, nil。
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
