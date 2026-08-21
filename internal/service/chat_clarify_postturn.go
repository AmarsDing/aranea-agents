// chat_clarify_postturn.go — P2 后置澄清转换（2026-08-21）。
//
// 场景（17:03 会话实证）：assistant 以纯文本阻断性提问收尾（「我需要您补充：
// 股票代码或交易所…？」），Intent Pass 未标 needs_clarification，澄清门不触发，
// turn 直接结束——用户面对纯文本干等、无结构化确认面板。
//
// 本文件在 PERSIST 之后、POST-PROCESS 之前对「回复含 ?/？ 问号」的成功 turn 做
// 轻量 LLM 判定；判定为阻断性提问时把问题升级为结构化 ClarifyBlock 卡片挂起，
// 卡片/恢复链路与澄清门完全同构（orphan clarify step + pendingClarification +
// session→awaiting_confirmation），用户卡片作答或直接自由回复均可续跑。
package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// maybeSuspendTurnForClarification 检测 turn 回复是否为阻断性纯文本提问；
// 是则发布 awaiting_input 澄清卡片并把 session 翻转为 awaiting_confirmation
// （running→awaiting_confirmation 为 FSM 合法转移；调用方须保证本函数在
// postProcessTurn 落 completed 之前调用）。返回 true 表示已挂起。
//
// Stability:internal
func (o *ChatOrchestrator) maybeSuspendTurnForClarification(
	ctx context.Context,
	ag biz.Agent,
	input biz.TurnInput,
	prov, mod, replyText string,
	emitter *event.TraceEmitter,
) bool {
	sessionID := strings.TrimSpace(input.SessionID)

	// ── 守卫条件（任一命中即不转换）──
	// 续跑 turn（ParentTaskID 非空）不转换，防澄清循环——与澄清门一致。
	if input.ParentTaskID != "" {
		return false
	}
	// 精灵总结 turn / 评测合成 turn 不挂起等待用户。
	if input.Synthesis || input.EntryConfig.EntryPoint == biz.EntryPointEvaluation {
		return false
	}
	if ag.Settings == nil || !ag.Settings.ClarificationEnabled {
		return false
	}
	replyText = strings.TrimSpace(replyText)
	if replyText == "" || sessionID == "" {
		return false
	}
	// 启发式预筛：仅含 ?/？ 的回复才值得付出一次旁路 LLM 判定
	// （含问号即放行，问句不要求在结尾；误放的礼貌收尾由判定器排除）。
	if !intent.LooksLikeTrailingQuestion(replyText) {
		return false
	}
	// F4 取消竞态：用户已取消的 turn 不再弹澄清卡（与 postProcessTurn 对齐）。
	if o.runWasCancelled(ctx, sessionID, nil) {
		return false
	}
	stepReader := o.stepReader()
	stepWriter := o.stepWriter()
	if stepReader == nil || stepWriter == nil {
		return false
	}
	taskID := string(chatagent.RootTaskActivityIDFromCtx(ctx))
	if taskID == "" {
		return false
	}

	// ── 轻量 LLM 判定：阻断性提问 vs 礼貌性收尾 ──
	// 复用 Intent Pass 的轻量模型覆盖（ARANEA_INTENT_PASS_MODEL），压旁路延迟。
	judgeProv, judgeMod := resolveIntentPassProviderModel(ctx, o.td().ReadDeps.LLM, prov, mod, o.lg())
	verdict := intent.JudgeBlockingQuestion(ctx, o.td().ReadDeps.LLM, o.td().LLMHTTP, judgeProv, judgeMod, replyText)
	if verdict.PromptTok > 0 || verdict.CompletionTok > 0 {
		o.turnMetrics().RecordAuxUsage(ctx, biz.AuxLLMUsageInput{
			Kind:          biz.UsageKindAuxIntent,
			SessionID:     sessionID,
			AgentID:       ag.ID,
			AgentKey:      ag.AgentKey,
			Provider:      judgeProv,
			Model:         judgeMod,
			Status:        "success",
			PromptTok:     verdict.PromptTok,
			CompletionTok: verdict.CompletionTok,
			UsageSource:   biz.UsageSourceResponse,
			Latency:       verdict.Duration,
		})
	}
	if !verdict.Blocking {
		if verdict.Outcome != "completed" {
			// 判定失败是增强失效而非错误：保持纯文本回复原样，仅留痕。
			o.lg().Warn("阻断性提问判定失败，保持纯文本回复",
				loggateway.StepID("chat.clarify_postturn"),
				loggateway.SessionID(sessionID),
				loggateway.Str("outcome", verdict.Outcome))
		}
		return false
	}

	// ── 升级纯文本提问为结构化澄清卡片 ──
	question := verdict.Question
	if question == "" {
		question = trailingQuestionFallback(replyText)
	}
	return o.suspendTurnWithClarificationQuestion(ctx, ag, input, taskID, question, emitter)
}

// suspendTurnWithClarificationQuestion 发布 awaiting_input 澄清卡片并挂起
// session（与澄清门卡片同构：orphan clarify step + pendingClarification +
// awaiting_confirmation）。调用方负责完成全部守卫与判定。
// Stability:internal
func (o *ChatOrchestrator) suspendTurnWithClarificationQuestion(
	ctx context.Context,
	ag biz.Agent,
	input biz.TurnInput,
	taskID, question string,
	emitter *event.TraceEmitter,
) bool {
	sessionID := strings.TrimSpace(input.SessionID)
	envelope := biz.ClarificationEnvelope{
		Version: 1,
		Kind:    biz.ClarificationEnvelopeKind,
		Questions: []biz.ClarificationQuestion{{
			Question: question,
			Mode:     biz.ClarificationModeSingle,
			// 自由文本问答：无预置选项/推荐，用户经卡片「其他」输入或自由回复作答。
			Options:     nil,
			Recommended: nil,
		}},
		OriginalInput: input.Content,
	}
	// 意图产物不落信封：turn 已执行完毕，续跑对「澄清上下文+原始需求」重跑
	// Intent Pass 语义更准确（产物复用是优化而非正确性依赖）。
	contentJSON, err := json.Marshal(envelope)
	if err != nil {
		o.lg().Warn("后置澄清信封序列化失败", loggateway.StepID("chat.clarify_postturn"), loggateway.Err(err))
		return false
	}

	now := time.Now().UTC()
	// step ID 与澄清门（taskID+"-clarify"）区分：自动默认路径可能已占该 ID。
	stepID := taskID + "-clarify-post"
	step := biz.Step{
		ID:              stepID,
		TurnID:          "", // orphan step——与澄清门卡片同构，前端按 TaskID 挂载
		TaskID:          taskID,
		SessionID:       sessionID,
		SpiritSessionID: sessionID,
		Kind:            biz.StepKindClarify,
		AuthorAgentKey:  ag.AgentKey,
		Seq:             nextTaskStepSeq(ctx, o.stepReader(), taskID),
		Version:         1,
		Content:         string(contentJSON),
		Status:          biz.StepStatusAwaitingInput,
		StartedAt:       now,
	}
	if _, err := o.stepWriter().CreateStep(ctx, step); err != nil {
		o.lg().Warn("后置澄清 CreateStep 失败",
			loggateway.StepID("chat.clarify_postturn"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return false
	}
	// 只发 step.created：task 由本 turn 投影早已存在，重发 task.created 无意义。
	if o.v2Seq != nil {
		o.v2Seq.Publish(ctx, biz.NewStepCreatedEvent(step))
	}

	// Session → awaiting_confirmation（reason=clarification）。本函数先于
	// postProcessTurn 的 completed 翻转执行，running→awaiting_confirmation 合法。
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusAwaitingConfirmation, sessstatus.StatusReasonClarification)

	// 进程内热路径恢复状态（信封为跨进程真相源）。
	o.pendingClarifications.Store(sessionID, pendingClarification{
		Input:     input,
		StepID:    stepID,
		TaskID:    taskID,
		CreatedAt: now,
	})

	o.lg().Info("纯文本阻断性提问已升级为澄清卡片，turn 挂起等待用户作答",
		loggateway.StepID("chat.clarify_postturn"),
		loggateway.SessionID(sessionID),
		loggateway.Str("task_id", taskID),
		loggateway.Str("step_id", stepID),
		loggateway.Str("question", question))
	if emitter != nil {
		emitter.LogDone("chat.clarify_postturn", "阻断性提问已转为结构化澄清卡片", event.P("step_id", stepID))
	}
	return true
}

// nextTaskStepSeq 取任务现有步骤的最大 Seq+1（orphan 澄清卡排在回复步骤之后）；
// reader 缺失/查询失败降级为 1（Seq 仅影响展示排序，不影响正确性）。
func nextTaskStepSeq(ctx context.Context, reader biz.StepV2Reader, taskID string) int64 {
	if reader == nil {
		return 1
	}
	steps, err := reader.ListStepsByTask(ctx, taskID)
	if err != nil || len(steps) == 0 {
		return 1
	}
	var maxSeq int64
	for _, s := range steps {
		if s.Seq > maxSeq {
			maxSeq = s.Seq
		}
	}
	return maxSeq + 1
}

// trailingQuestionFallback 在判定器未回写 question 时从回复尾部提取问题文本：
// 短回复取全文，长回复取末段（最后一个空行之后的内容）。
func trailingQuestionFallback(reply string) string {
	const shortLimit = 200
	if len([]rune(reply)) <= shortLimit {
		return reply
	}
	paragraphs := strings.Split(reply, "\n\n")
	last := strings.TrimSpace(paragraphs[len(paragraphs)-1])
	if last == "" {
		return reply
	}
	return last
}
