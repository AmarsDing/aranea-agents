package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/metrics"
	"aranea-agents/internal/tools/deferred"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// knowledgeIntentTools 是知识意图命中时按序尝试激活的检索工具（P2-④）。
// knowledge_search 是主检索；knowledge_reflect 支撑跨库/质量反思。两者均在
// 目录中才激活，缺席不视为错误（agent 可能只开了其中一个）。
var knowledgeIntentTools = []string{
	biz.ToolKeyKnowledgeSearch,
	biz.ToolKeyKnowledgeReflect,
}

// newKnowledgeIntentPromoteBeforeHook 在「显式知识库意图」轮次把检索工具从
// 延迟目录激活到工具面（session-eval-20260829-r2 R4-Q7 / P2-④ 根修）。
//
// 背景：Spirit profile 核心常驻集不含 knowledge_search（token 治理，闲聊轮
// 不占 Request.Tools），知识 cue 又明确告知「不要 tool_search 猎取」——
// 用户点名知识库时检索路径被双向堵死，S03 全程零检索参数化作答。
//
// 机制与组队激活同构（orchestration_phase_hooks）：词法命中
// biz.LooksLikeKnowledgeQuery → DeferredToolManager.Activate 写 session
// state（temp:deferred:activated），本 turn 起 ToolFilter 放行，模型首轮
// 即见完整 schema，无 tool_load 往返。激活是 session 级粘性——知识会话的
// 后续追问（「这个 SOP 的第 3 步呢」）无需重复命中词法。
//
// 与 newIntentToolHintPromoteBeforeHook 的关系：那边消费 aux_intent 的 LLM
// tool_hints（概率性、每轮 1.3-1.5k tokens）；本 hook 是确定性词法路由，
// 零 LLM 开销。两 hook 可同轮各自激活不同工具，Activate 幂等。
func newKnowledgeIntentPromoteBeforeHook(ag biz.Agent, dm *deferred.DeferredToolManager, lg loggateway.Logger) callbacks.Callback {
	if dm == nil {
		return nil
	}
	// 关工具的 agent 无目录可激活（eff 门禁在装配期已把知识工具挡在目录外，
	// 此处早退省每轮一次词法扫描）。
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return callbacks.NewBeforeAgentHook(3, func(ctx context.Context, args *trpcagent.BeforeAgentArgs) (*trpcagent.BeforeAgentResult, error) {
		msg := ""
		if args != nil && args.Invocation != nil {
			msg = args.Invocation.Message.Content
		}
		if msg == "" {
			if inv, ok := trpcagent.InvocationFromContext(ctx); ok && inv != nil {
				msg = inv.Message.Content
			}
		}
		if !biz.LooksLikeKnowledgeQuery(msg) {
			return &trpcagent.BeforeAgentResult{Context: ctx}, nil
		}
		activated := 0
		for _, name := range knowledgeIntentTools {
			resolved, ok := dm.ResolveName(name)
			if !ok {
				resolved = name
			}
			if !dm.IsInCatalog(resolved) {
				continue
			}
			if dm.IsActivated(ctx, resolved) {
				continue
			}
			if _, err := dm.Activate(ctx, resolved); err != nil {
				lg.Warn("知识意图工具激活失败",
					loggateway.StepID("agent.knowledge_intent.promote"),
					loggateway.Str("tool", resolved),
					loggateway.Err(err),
				)
				metrics.DeferredToolActivationTotal.WithLabelValues(resolved, "knowledge_intent_error").Inc()
				continue
			}
			metrics.DeferredToolActivationTotal.WithLabelValues(resolved, "knowledge_intent").Inc()
			activated++
		}
		if activated > 0 {
			lg.Info("知识意图命中，检索工具已激活",
				loggateway.StepID("agent.knowledge_intent.promote"),
				loggateway.Int("activated", activated),
			)
		}
		return &trpcagent.BeforeAgentResult{Context: ctx}, nil
	})
}

// knowledgeSearchOnFace 报告本轮知识检索工具是否已在工具面上：静态 profile
// （coding/research/full 常驻）或本会话已动态激活（知识意图/手动 tool_load）。
// 供知识 cue 决定引导文案口径（可否点名 knowledge_search）。
func knowledgeSearchOnFace(ctx context.Context, ag biz.Agent, dm *deferred.DeferredToolManager) bool {
	if agentHasKnowledgeSearch(ag) {
		return true
	}
	if dm == nil {
		return false
	}
	for _, name := range knowledgeIntentTools {
		resolved, ok := dm.ResolveName(name)
		if !ok {
			resolved = name
		}
		if dm.IsInCatalog(resolved) && dm.IsActivated(ctx, resolved) {
			return true
		}
	}
	return false
}
