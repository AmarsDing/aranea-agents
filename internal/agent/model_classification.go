package agent

// D6 L-1 密级路由门：ClassificationAwareModelSelector（E 阶段 W2b D6，
// 需求 §7.3 验收 #1/#5）。会话水位线 >= model_policy 本地阈值 → 候选裁到
// channel_class=local；水位线解析/读库失败 → 保守 nil 保持 base 路由。
//
// 门是进程级默认（atomic.Value，wire 装配期注册一次，selector 每调用读取）：
// agent 构建被缓存且跨 workspace 共享，建构期钉死门会像 PluginCostGuard
// 建构期钉死 tracker 一样串桶（N-B2 先例）——必须每调用解析。

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// ClassificationGate D6 L-1 密级路由门：每调用解析「会话水位线 vs 本地
// 阈值」，命中即强制 local 通道。全部依赖缺失 = 门停用（selector 恒 nil）。
type ClassificationGate struct {
	// CeilingFor 读会话水位线（未定级/无行 → 0）。nil = 门未装配。
	CeilingFor func(ctx context.Context, sessionID string) (int, error)
	// Policy 模型策略热副本用例（local_threshold 来源，TTL 5s 进程内缓存）。
	// nil = 门未装配。
	Policy *biz.SecopsModelPolicyUsecase
}

// classificationDefaultGate 进程级默认密级门（零值 = 停用）。
var classificationDefaultGate atomic.Value // stores ClassificationGate

// SetDefaultClassificationGate 注册进程级默认密级门。wire 装配期由
// provideSecopsModelPolicyUsecase 副作用调用一次（app 启动即生效；
// 先例：app.go BeforeStart 的 SetTeamCompletionChecker 副作用装配）。
// 重复调用覆盖（幂等语义：最后一次注册胜出）。
func SetDefaultClassificationGate(g ClassificationGate) {
	classificationDefaultGate.Store(g)
}

// DefaultClassificationGate 读进程级默认门（未注册/类型不符 → 零值门）。
func DefaultClassificationGate() ClassificationGate {
	if v := classificationDefaultGate.Load(); v != nil {
		if g, ok := v.(ClassificationGate); ok {
			return g
		}
	}
	return ClassificationGate{}
}

// enabled 门装配判定：两个依赖都到位才算启用，缺一即整体停用
//（半个门无法做出可信的强制本地判定）。
func (g ClassificationGate) enabled() bool {
	return g.CeilingFor != nil && g.Policy != nil
}

// sessionIDFromInvocation 取 invocation 的会话 ID（trpc-agent-go v1.11.2
// session.Session.ID）。无会话/空 ID → ""（无水位线可判 → 保持 base）。
func sessionIDFromInvocation(inv *trpcagent.Invocation) string {
	if inv == nil || inv.Session == nil {
		return ""
	}
	return strings.TrimSpace(inv.Session.ID)
}

// ClassificationAwareModelSelector 返回 D6 L-1 密级路由 selector。
//
//	baseProv/baseMod：agent base 模型对（建构建候选时排除自身；团队
//	    RunOption 注入点拿不到 member base 时传空串——最多白重建一次
//	    同模型，无害）。
//	catalog：模型目录（List 取候选；nil → 门停用态处理）。
//
// 保守 nil 语义（验收 §7.3 #5）：门未装配 / 目录 nil / 无会话 ID /
// 水位线读失败 / 策略读失败 / 水位线低于阈值 / base 已是 local ——
// 全部返回 (nil, nil) 保持 base 路由。
//
// 唯一 fail-closed：已确认「水位线 >= 阈值」但目录里没有任何可用的
// local 候选 —— 此时回 cloud 是密级违规，返回 error 让运行响亮失败
//（宁可任务失败，不可 L3+ 内容落云通道）。
func ClassificationAwareModelSelector(
	baseProv, baseMod string,
	catalog biz.TeamModelCatalog,
	rt *provider.RoundTrip,
	lg loggateway.Logger,
) trpcagent.ModelSelector {
	baseProv = strings.TrimSpace(baseProv)
	baseMod = strings.TrimSpace(baseMod)
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return func(ctx context.Context, inv *trpcagent.Invocation) (trpcmodel.Model, error) {
		gate := DefaultClassificationGate()
		if !gate.enabled() || catalog == nil {
			return nil, nil
		}
		sessionID := sessionIDFromInvocation(inv)
		if sessionID == "" {
			return nil, nil
		}
		ceiling, err := gate.CeilingFor(ctx, sessionID)
		if err != nil {
			metrics.ClassificationRouteTotal.WithLabelValues("keep_base_ceiling_error").Inc()
			lg.Warn("密级路由：水位线读取失败，保持 base",
				loggateway.StepID("agent.classification.ceiling_fail"),
				loggateway.Str("session_id", sessionID), loggateway.Err(err))
			return nil, nil
		}
		policy, err := gate.Policy.GetPolicy(ctx)
		if err != nil || policy == nil {
			metrics.ClassificationRouteTotal.WithLabelValues("keep_base_policy_error").Inc()
			lg.Warn("密级路由：策略读取失败，保持 base",
				loggateway.StepID("agent.classification.policy_fail"),
				loggateway.Str("session_id", sessionID), loggateway.Err(err))
			return nil, nil
		}
		if !biz.CeilingForcesLocal(ceiling, policy.LocalThreshold) {
			return nil, nil
		}

		// 命中强制本地：列目录裁 local 候选。
		models, err := catalog.List(ctx)
		if err != nil {
			metrics.ClassificationRouteTotal.WithLabelValues("keep_base_catalog_error").Inc()
			lg.Warn("密级路由：目录读取失败，保持 base",
				loggateway.StepID("agent.classification.catalog_fail"),
				loggateway.Str("session_id", sessionID), loggateway.Err(err))
			return nil, nil
		}
		localCandidates := make([]biz.ProviderModel, 0, len(models))
		baseAlreadyLocal := false
		for _, m := range models {
			if m.Provider == baseProv && m.Model == baseMod && biz.ProviderChannelForcesLocal(m.ChannelClass) {
				baseAlreadyLocal = true
			}
			if m.Enabled && biz.ProviderChannelForcesLocal(m.ChannelClass) {
				localCandidates = append(localCandidates, m)
			}
		}
		if baseAlreadyLocal {
			return nil, nil
		}
		picked, ok := CheapestCapableModel(localCandidates, baseMod)
		if !ok {
			// 无定价的本地候选也兜底可用：取第一个启用 local（保守顺序=目录序）。
			for _, m := range localCandidates {
				if m.Model != baseMod {
					picked, ok = m, true
					break
				}
			}
		}
		if !ok {
			metrics.ClassificationRouteTotal.WithLabelValues("failed_closed_no_local").Inc()
			lg.Error("密级路由：水位线已达本地阈值但目录无 local 候选，fail-closed",
				loggateway.StepID("agent.classification.no_local"),
				loggateway.Str("session_id", sessionID),
				loggateway.Int("ceiling", ceiling),
				loggateway.Int("local_threshold", policy.LocalThreshold))
			return nil, fmt.Errorf("classification forces local but no local channel model available (session=%s ceiling=%d threshold=%d)",
				sessionID, ceiling, policy.LocalThreshold)
		}
		m, err := provider.TRPCModelForProviderModel(ctx, catalog, rt, picked.Provider, picked.Model, lg)
		if err != nil {
			// 构建失败不允许回 cloud（同 fail-closed 语义），但原因不同指标值。
			metrics.ClassificationRouteTotal.WithLabelValues("failed_closed_build_error").Inc()
			lg.Error("密级路由：本地模型构建失败，fail-closed",
				loggateway.StepID("agent.classification.build_fail"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("provider", picked.Provider),
				loggateway.Str("model", picked.Model), loggateway.Err(err))
			return nil, fmt.Errorf("classification local model build %s/%s: %w", picked.Provider, picked.Model, err)
		}
		metrics.ClassificationRouteTotal.WithLabelValues("forced_local").Inc()
		lg.Info("密级路由：强制本地通道",
			loggateway.StepID("agent.classification.forced_local"), loggateway.Phase("done"),
			loggateway.Str("session_id", sessionID),
			loggateway.Int("ceiling", ceiling),
			loggateway.Int("local_threshold", policy.LocalThreshold),
			loggateway.Str("provider", picked.Provider),
			loggateway.Str("model", picked.Model))
		return m, nil
	}
}
