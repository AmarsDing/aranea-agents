package agent

import (
	"context"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// P1-2 策略 resolver 化（Cordis B8/J3，report-05 P1-2）。
//
// 背景：工具策略参数（当前 = ToolsExecutionTimeoutSec）原在构建期固化进回调
// 闭包，任何策略调整都经 SettingsJSON 进指纹 → 全量重建（3-7s）。本 resolver
// 照搬 hook.Resolver 的生产验证模式（cache + Reload + 查询），把策略读取从
// 构建期下沉为每调用 O(1) map 查：
//
//   - 策略字段从构建指纹剥离（cache.go policyStrippedSettings），其变更不再
//     改变指纹；UpdateAgentToolPolicy 通道对「仅策略字段变化」跳过 invalidate，
//     仅 Set 本 resolver → 新调用立即生效，agent 零重建。
//   - 向后兼容：resolver miss（未初始化/未 Reload/无该 agent 行）时回退
//     构建期快照值，行为与改造前完全等价——wire 未接线的部署形态零回归。
//
// 范围说明：本 wave 仅 resolver 化 ToolsExecutionTimeoutSec（报告验收字段）。
// retry/熔断器同属候选，但其参数嵌进工具包装/注册表构建（非纯回调闭包），
// resolver 化需各自评估，不盲目扩大（report-05 开放问题跟踪）。

// resolverManagedPolicyFields 是已从构建指纹剥离、由 resolver 接管的
// AgentRuntimeSettings 字段名集合。policyStrippedSettings 与
// TestPolicyStrippedFields_Guard 共同保证：新增 resolver 化字段必须同步
// 两处，否则守卫测试红（对应 report-05 P0-2 风险①的同型漂移防护）。
var resolverManagedPolicyFields = []string{"ToolsExecutionTimeoutSec"}

// PolicyResolver 是工具策略的运行时快照表（进程级单例）。
type PolicyResolver struct {
	mu sync.RWMutex
	// timeoutSec[agentID] = ToolsExecutionTimeoutSec 原始值（0 = 跟随默认，
	// 规范化在查询出口统一处理，与 buildToolExecutionTimeout 语义一致）。
	timeoutSec map[string]int
	repo       biz.AgentRuntimeSettingsRepo
	lg         loggateway.Logger
}

var globalPolicyResolver = &PolicyResolver{lg: loggateway.NewNoop()}

// InitPolicyResolver 注入数据源并 best-effort 首轮 Reload（启动接线一次）。
// 重复调用仅替换数据源（幂等）；Reload 失败不致命——查询回退构建期值。
func InitPolicyResolver(repo biz.AgentRuntimeSettingsRepo, lg loggateway.Logger) *PolicyResolver {
	r := globalPolicyResolver
	r.mu.Lock()
	r.repo = repo
	if lg != nil {
		r.lg = lg
	}
	r.mu.Unlock()
	if err := r.Reload(context.Background()); err != nil {
		r.lg.Warn("policy resolver 首轮加载失败，回退构建期策略值",
			loggateway.StepID("agent.policy_resolver.reload_failed"), loggateway.Err(err))
	}
	return r
}

// Reload 全量刷新策略快照（启动/批量变更后调用）。
func (r *PolicyResolver) Reload(ctx context.Context) error {
	r.mu.RLock()
	repo := r.repo
	r.mu.RUnlock()
	if repo == nil {
		return nil
	}
	all, err := repo.ListAgentRuntimeSettings(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]int, len(all))
	for id, s := range all {
		m[id] = s.ToolsExecutionTimeoutSec
	}
	r.mu.Lock()
	r.timeoutSec = m
	r.mu.Unlock()
	return nil
}

// SetToolExecutionTimeout 增量更新单 agent 的 timeout 策略（service 层
// 「仅策略字段变化」路径调用；空 agentID 忽略）。
func SetToolExecutionTimeout(agentID string, sec int) {
	if agentID == "" {
		return
	}
	r := globalPolicyResolver
	r.mu.Lock()
	if r.timeoutSec == nil {
		r.timeoutSec = map[string]int{}
	}
	r.timeoutSec[agentID] = sec
	r.mu.Unlock()
}

// toolExecutionTimeoutFor 是回调链每调用的查询入口：resolver 命中（含显式
// 配置的 0=默认）用 resolver 值，miss 回退构建期快照 buildTimeSec。
// 出口统一规范化（≤0 → defaultToolExecutionTimeout），与
// buildToolExecutionTimeout 的默认兜底语义一致。
func toolExecutionTimeoutFor(agentID string, buildTimeSec int) time.Duration {
	r := globalPolicyResolver
	r.mu.RLock()
	sec, ok := r.timeoutSec[agentID]
	r.mu.RUnlock()
	if !ok {
		sec = buildTimeSec
	}
	if sec <= 0 {
		return defaultToolExecutionTimeout
	}
	return time.Duration(sec) * time.Second
}

// policyStrippedSettings 返回 Settings 的浅拷贝，resolver 化字段清零——
// 用于 BuildCacheKey 指纹计算：这些字段的变更不再改变指纹（由 resolver
// 运行时接管），其余字段仍全量进指纹（保守默认，未映射字段变更照旧触发
// 全量重建）。nil 入参原样返回。
func policyStrippedSettings(s *biz.AgentRuntimeSettings) *biz.AgentRuntimeSettings {
	if s == nil {
		return nil
	}
	cp := *s
	cp.ToolsExecutionTimeoutSec = 0
	return &cp
}
