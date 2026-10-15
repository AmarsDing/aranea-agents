package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// shouldSkipIntentPass reports whether this turn should skip the extra intent
// LLM: DirectReply patterns (介绍自己 / 不要调用工具 / 请记住 / 现在几点),
// or QuickAssess==simple when the message is not an underspecified task
// (帮我做个应用 still needs the clarification gate).
//
// 第二返回值携带 QuickAssess 等级（未评估时为空串）：C2 方案 D 分级时限用
// 其区分 intent 收口策略——complex 全等至保险丝，moderate 走 rendezvous。
// 第三返回值是 skip 判定的机器可读理由（包B B4 路由语料）：direct_reply /
// underspecified_task / agent_skip_disabled / task_action_signal /
// planner_unavailable / planner_error / not_simple / confident_simple /
// low_confidence_simple。
//
// 包B（session-eval-20260825 B1）两道闸：
//  1. agent 维度闸：ag.Settings.IntentSkipEnabled=false（管理层 agent：3 GM +
//     部门主管 + spirit，SQL 置 0）时禁用 QuickAssess==simple skip——任务型
//     消息被误判 simple 导致 intent pass 跳过、组织路由失效（P-INTENT-SKIP）。
//     DirectReply 是确定性模式匹配（非误判源），不受此闸影响。
//  2. R4 低置信不 skip：QuickAssess score 贴近 0.3 阈值的边界 simple 说明
//     评分器对「简单」不确信——走完整 intent pass（宁重勿轻，RouteLLM
//     非对称代价：任务型误判为简单 ≫ 闲聊误跑 intent pass）。仅高置信
//     simple（score < intentSkipConfidentSimpleMaxScore）允许 skip。
//
// ADR-79-V V2（2026-08-26）第三道闸：任务信号排除。分类器输出只可用于
// 增加义务、不可用于免除义务——凡含任务动作词的轮次，即使评分器误判
// confident simple 也不得 skip（与 QuickAssess 内部 taskRequestWords
// 升级互为纵深：那里是第一道，这里兜底评分器漏判）。
func shouldSkipIntentPass(o *ChatOrchestrator, ctx context.Context, ag biz.Agent, input biz.TurnInput, content string) (bool, biz.ComplexityLevel, string) {
	if intent.SkipForDirectReply(content) {
		return true, "", "direct_reply"
	}
	if intent.LooksLikeUnderspecifiedTask(content) {
		return false, "", "underspecified_task"
	}
	if ag.Settings != nil && !ag.Settings.IntentSkipEnabled {
		return false, "", "agent_skip_disabled"
	}
	if biz.HasTaskActionSignal(content) {
		return false, "", "task_action_signal"
	}
	planner := o.team().TaskPlanner
	if planner == nil {
		return false, "", "planner_unavailable"
	}
	level, score, err := planner.QuickAssess(ctx, biz.PlanInput{
		UserMessage:     content,
		SpiritSessionID: input.SessionID,
	})
	if err != nil {
		return false, "", "planner_error"
	}
	if level != biz.ComplexitySimple {
		return false, level, "not_simple"
	}
	if score < intentSkipConfidentSimpleMaxScore {
		return true, level, "confident_simple"
	}
	return false, level, "low_confidence_simple"
}

// intentSkipConfidentSimpleMaxScore R4 低置信闸（包B B1）：QuickAssess 判
// simple 的阈值为 score<0.3，但 [0.2,0.3) 属贴线边界——评分器对「简单」
// 不确信，不允许 skip intent pass。0.2 = 阈值 0.3 内收 0.1 置信边距。
const intentSkipConfidentSimpleMaxScore = 0.2

// intentKindOf 安全取 artifact 的 IntentKind（nil → ""），包B B4 路由语料用。
func intentKindOf(art *intent.Artifact) string {
	if art == nil {
		return ""
	}
	return art.IntentKind
}

// intentRendezvousWindow C2 方案 D：moderate 非欠规格轮次在 BUILD 收口后
// 再等 intent 的窗口。Intent Pass 实测 0.5–2.3s，500ms 会合几乎必然
// raced_out（产物丢弃、2.3s 白烧）。对齐保险丝 2.5s，让中档有机会吃到
// refined goal / 澄清门。超时 cancel intent，按既有无意图路径降级。
const intentRendezvousWindow = 2500 * time.Millisecond

// awaitIntentArtifact C2 方案 D 分级收口：fullWait（欠规格/complex）全等至
// intentCtx 自带的 2.5s 保险丝；否则限 rendezvous 窗口。artifact 只经
// channel 传递（容量 1，goroutine 永不阻塞退出），主流程无共享变量竞争。
// outcome 取值：completed / fused（保险丝熔断）/ raced_out（窗口未命中）。
func awaitIntentArtifact(intentCh <-chan *intent.Artifact, intentCtx context.Context, intentCancel context.CancelFunc, fullWait bool) (*intent.Artifact, string) {
	defer intentCancel()
	if fullWait {
		select {
		case art := <-intentCh:
			return art, "completed"
		case <-intentCtx.Done():
			return nil, "fused"
		}
	}
	select {
	case art := <-intentCh:
		return art, "completed"
	case <-time.After(intentRendezvousWindow):
		return nil, "raced_out"
	}
}

// shouldRunProactiveRecall 判定本 turn 是否执行主动召回（P3-11）。
//
// 语音轮次（input.Voice != nil）跳过：真机实测语音轮次 recall hits 恒为 0
// （语音输入多为短口语句，实体提及稀少），而 recall 含 query embedding +
// 向量检索，耗时 0.3-3.3s；虽然 recall 与 BUILD/Intent Pass 同 errgroup
// 并行，eg.Wait() 以最慢 goroutine 收口，零产出召回会成为关键路径纯开销，
// 直接击穿 ≤2s 停口到首音预算（2026-08-11）。
//
// 文字 DirectReply / 短闲聊同样跳过：中文分词几乎总会抽出「实体」，不能
// 靠 extractMentionedEntities 判断；L3 硬注入仍覆盖工号召回。
// Stability:internal
func shouldRunProactiveRecall(input biz.TurnInput) bool {
	if input.Voice != nil {
		return false
	}
	if intent.SkipForDirectReply(input.Content) {
		return false
	}
	text := strings.TrimSpace(input.Content)
	if text == "" {
		return false
	}
	if utf8.RuneCountInString(text) <= 8 && !intent.LooksLikeUnderspecifiedTask(text) {
		return false
	}
	return true
}

func proactiveRecallSkipReason(input biz.TurnInput) string {
	if input.Voice != nil {
		return "voice_fast_path"
	}
	if intent.SkipForDirectReply(input.Content) {
		return "direct_reply"
	}
	return "simple_chitchat"
}

// runProactiveRecall triggers proactive memory recall at turn start to surface
// relevant memories based on the conversation context (P3-11). The returned
// hits are later merged with RecallComposite results by the MemoryInject
// before-model hook.
//
// Failures are non-fatal: only a warning is logged and nil is returned so the
// turn continues without proactive hits. The method is a no-op when memory is
// disabled, the composite recaller is not wired, or the recaller does not
// implement biz.ProactiveRecaller.
func (o *ChatOrchestrator) runProactiveRecall(ctx context.Context, sess biz.Session, ag biz.Agent, content string, emitter *event.TraceEmitter) []biz.CompositeRecallHit {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled || !policy.RecallL2 || !policy.InjectL3 {
		return nil
	}
	compositeRecaller := o.td().Persist.Memory.CompositeRecall
	if compositeRecaller == nil {
		return nil
	}
	proactiveRecaller, ok := compositeRecaller.(biz.ProactiveRecaller)
	if !ok || proactiveRecaller == nil {
		return nil
	}
	agentID := strings.TrimSpace(ag.ID)
	if agentID == "" {
		return nil
	}
	userID := strings.TrimSpace(chatagent.UserIDFromCtx(ctx))
	convCtx := biz.ProactiveRecallContext{
		UserStatement:     strutil.TruncateRunes(content, 200),
		MentionedEntities: extractMentionedEntities(content),
	}
	hits, err := proactiveRecaller.ProactiveRecall(ctx, agentID, userID, convCtx)
	if err != nil {
		o.lg().Warn("proactive recall failed, continuing without proactive hits",
			loggateway.StepID("chat.proactive_recall"),
			loggateway.SessionID(sess.ID),
			loggateway.Err(err))
		return nil
	}
	if len(hits) > 0 {
		emitter.LogDone("chat.proactive_recall", "主动召回完成",
			event.P("hit_count", len(hits)),
			event.P("agent_id", agentID))
	}
	return hits
}

// extractMentionedEntities extracts candidate entity keywords from the user
// message for proactive recall. This is a simple tokenisation (YAGNI: no NLP,
// no stemming) that splits on whitespace and common punctuation (ASCII + CJK),
// filters out very short tokens, and caps the result to avoid excessive queries.
func extractMentionedEntities(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	const (
		minTokenLen = 3
		maxEntities = 8
	)
	fields := strings.FieldsFunc(strings.ToLower(content), func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', ',', '.', '!', '?', ';', ':', '\'', '"':
			return true
		case '，', '。', '！', '？', '；', '：', '、':
			return true
		}
		return false
	})
	out := make([]string, 0, maxEntities)
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		if utf8.RuneCountInString(f) < minTokenLen || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) >= maxEntities {
			break
		}
	}
	return out
}

func (o *ChatOrchestrator) emitOrchDecisionRecord(ctx context.Context, sessionID, agentKey string, skipIntent bool, skipReason, assessLevel, intentOutcome string, gateErr error, gate GateDecision, flowPairs []event.Pair) {
	level := assessLevel
	reason := skipReason
	if gateErr == nil && gate.Level != "" {
		level = string(gate.Level)
		if gate.Reason != "" {
			reason = gate.Reason
		}
	}
	outcome := "assessed_" + level
	if outcome == "assessed_" {
		outcome = "assessed"
	}
	meta := map[string]any{
		"skip_intent":    skipIntent,
		"skip_reason":    skipReason,
		"assess_level":   assessLevel,
		"intent_outcome": intentOutcome,
		"session_id":     sessionID,
		"agent_key":      agentKey,
	}
	if gateErr != nil {
		meta["gate_error"] = gateErr.Error()
	} else {
		meta["gate_level"] = string(gate.Level)
		meta["gate_score"] = gate.Score
		meta["force_planning"] = gate.ForcePlanning
		meta["gate_reason"] = gate.Reason
	}

	event.EmitDecision(ctx, o.infraDeps.DecisionCollector, decision.Record{
		DecisionKey: uuid.NewString(),
		Category:    decision.CategoryPlannerOrchestration,
		Scenario:    "编排路由决策",
		Reasoning:   reason,
		Outcome:     outcome,
		ActorType:   decision.ActorSystem,
		ActorKey:    "system:chat_orchestrator",
		SourceRef:   decision.SourceRef{SessionID: sessionID},
		Metadata:    meta,
	}, "chat.orch.decision", "编排路由决策", flowPairs...)
}
