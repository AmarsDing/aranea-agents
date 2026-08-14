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
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// recentIntentHistory 加载近 N 条对话消息供意图识别解析指代/省略（P1 抗过度澄清）。
// 加载失败降级为 nil（不阻断 turn）；过滤非 user/assistant 角色与空内容，
// 并剔除与当前输入同文的条目（用户消息先于 intent pass 落库的重入场景）。
// Stability:internal
func (o *ChatOrchestrator) recentIntentHistory(ctx context.Context, sessionID, currentContent string) []intent.HistoryMessage {
	lister := o.td().MsgHistory
	if lister == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	// 多取 1 条：为与当前输入同文的条目预留去重空间。
	msgs, err := lister.ListMessagesRecent(ctx, sessionID, intent.MaxIntentHistoryMessages+1)
	if err != nil {
		o.lg().Warn("意图识别历史加载失败，降级为无历史注入",
			loggateway.StepID("chat.intent.history"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return nil
	}
	current := strings.TrimSpace(currentContent)
	out := make([]intent.HistoryMessage, 0, len(msgs))
	for _, m := range msgs {
		role := strings.TrimSpace(m.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(m.ContentMarkdown)
		if content == "" || content == current {
			continue
		}
		out = append(out, intent.HistoryMessage{Role: role, Content: content})
	}
	if len(out) > intent.MaxIntentHistoryMessages {
		out = out[len(out)-intent.MaxIntentHistoryMessages:]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ClarificationGate 判定结果。
type ClarificationGateDecision struct {
	// Triggered 为 true 表示已发布澄清卡片并挂起 turn。
	Triggered bool
	// StepID 是发布的澄清 Step ID（仅 Triggered=true 时有效）。
	StepID string
	// AutoResolved 为 true 表示全部澄清问题按推荐默认自动作答（假设式前进），
	// turn 不挂起：已落一张 completed 澄清审计卡，ResolvedInput 注入了澄清上下文，
	// Artifact 已剥离澄清残留（避免下游 LLM 依据 needs_clarification 重问）。
	AutoResolved  bool
	ResolvedInput biz.TurnInput
	Artifact      *intent.Artifact
}

// runClarificationGate 在 Intent Pass 之后、PrePlanning 之前执行澄清门判定。
//
// 触发条件（同时满足）：
//  1. Agent 设置 ClarificationEnabled=true（默认 true）
//  2. intent.Artifact.NeedsClarification() 返回 true（RiskFlags 含 needs_clarification 且 Clarifications 非空）
//
// 触发后行为：
//  1. UpsertTask 幂等建任务（Task 先于 Run 存在）
//  2. 发布 StepCreatedEvent：Kind=clarify，Status=awaiting_input，Content=澄清问题 JSON 信封
//  3. Session → awaiting_confirmation（reason=clarification）
//  4. 存储 pendingClarification 供同进程热路径恢复（信封为跨进程真相源）
//  5. turn 挂起（RunTurn 返回空回复，不报错）
//
// Stability:internal
func (o *ChatOrchestrator) runClarificationGate(
	ctx context.Context,
	sessionID string,
	intentArt *intent.Artifact,
	ag biz.Agent,
	input biz.TurnInput,
) (ClarificationGateDecision, error) {
	// 续跑 turn（澄清提交/自由回复后 ParentTaskID 非空）不再触发澄清门，防止澄清循环。
	if input.ParentTaskID != "" {
		return ClarificationGateDecision{}, nil
	}

	// 条件 1：Agent 设置检查
	if ag.Settings == nil || !ag.Settings.ClarificationEnabled {
		return ClarificationGateDecision{}, nil
	}

	// 条件 2：Intent Artifact 检查
	if intentArt == nil || !intentArt.NeedsClarification() {
		return ClarificationGateDecision{}, nil
	}

	questions := intentArt.ClarificationQuestions()
	if len(questions) == 0 {
		return ClarificationGateDecision{}, nil
	}

	// 假设式前进（2026-08-09）：全部问题携带推荐默认且无高风险标记时，
	// 按推荐自动作答、落 completed 审计卡，turn 注入澄清上下文继续执行，
	// 不再挂起打扰用户。任一问题无推荐或命中高风险标记（touches_auth/
	// migrations/sensitive_data/compliance/destructive/irreversible）时，
	// 仍走下方挂起弹卡路径。
	if !intentArt.HasHighRiskFlag() && biz.ClarificationAllRecommended(questions) {
		return o.autoResolveClarification(ctx, sessionID, intentArt, ag, input, questions)
	}

	lg := o.lg().With(
		loggateway.SessionID(sessionID),
		loggateway.StepID("chat.clarification_gate"),
	)

	// 1. UpsertTask 幂等建任务（Task 先于 Run 存在）
	// 复用 turn 入口预生成的 RootTaskActivityID（chat_orchestrator_turn.go
	// 在 BUILD/IntentPass 并行后注入 ctx），保证 ctx 链与落库 Task ID 一致；
	// ctx 缺失时兜底新 UUID。
	taskID := string(chatagent.RootTaskActivityIDFromCtx(ctx))
	if taskID == "" {
		taskID = string(chatagent.RootTaskActivityID(uuid.NewString()))
	}
	now := time.Now().UTC()
	task := biz.Task{
		ID:          taskID,
		SessionID:   sessionID,
		UserMessage: input.Content, // 展示用：澄清卡片所属任务显示原始需求
		Status:      biz.TaskStatusRunning,
		Seq:         1, // 单任务 session 的 seq=1
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := o.taskV2Writer().UpsertTask(ctx, task); err != nil {
		lg.Warn("澄清门 UpsertTask 失败", loggateway.Err(err))
		return ClarificationGateDecision{}, err
	}

	// 2. 构建澄清信封并发布 StepCreatedEvent
	envelope := biz.ClarificationEnvelope{
		Version:       1,
		Kind:          "clarification",
		Questions:     questions,
		Answers:       nil,           // 发布时无答案
		OriginalInput: input.Content, // 持久化原始输入，重启/多副本后可重建续跑
	}
	// 持久化意图产物：进程内 cache 丢失后续跑从信封恢复，
	// 避免重跑 Intent Pass LLM。序列化失败不阻断澄清（产物复用是优化而非正确性依赖）。
	if intentArt != nil {
		if artJSON, err := json.Marshal(intentArt); err == nil {
			envelope.IntentArtifactJSON = string(artJSON)
		}
	}
	contentJSON, err := json.Marshal(envelope)
	if err != nil {
		lg.Warn("澄清信封序列化失败", loggateway.Err(err))
		return ClarificationGateDecision{}, err
	}

	stepID := taskID + "-clarify"
	step := biz.Step{
		ID:              stepID,
		TurnID:          "", // orphan step（Turn 尚未创建）
		TaskID:          taskID,
		SessionID:       sessionID,
		SpiritSessionID: sessionID,
		Kind:            biz.StepKindClarify,
		AuthorAgentKey:  ag.AgentKey,
		Seq:             1,
		Version:         1,
		Content:         string(contentJSON),
		Status:          biz.StepStatusAwaitingInput,
		StartedAt:       now,
	}
	if _, err := o.stepWriter().CreateStep(ctx, step); err != nil {
		lg.Warn("澄清门 CreateStep 失败", loggateway.Err(err))
		return ClarificationGateDecision{}, err
	}

	// 发布 WS 事件：task.created 必须先于 step.created——前端 TaskCard 需先
	// 有 Task 才能挂载 orphan clarify step（getTaskOrphanSteps 按 TaskID 过滤），
	// 否则澄清卡片永不渲染。UpsertTask 幂等（VersionLT guard），sequencer
	// 异步落库 task.created 与上方直写不冲突。
	if o.v2Seq != nil {
		o.v2Seq.Publish(ctx, biz.NewTaskCreatedEvent(task))
		o.v2Seq.Publish(ctx, biz.NewStepCreatedEvent(step))
	}

	// 3. Session → awaiting_confirmation（reason=clarification）
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusAwaitingConfirmation, sessstatus.StatusReasonClarification)

	// 4. 存储 pendingClarification 供同进程热路径恢复（含 Intent 产物，续跑复用）
	o.pendingClarifications.Store(sessionID, pendingClarification{
		Input:     input,
		StepID:    stepID,
		TaskID:    taskID,
		CreatedAt: now,
		Artifact:  intentArt,
	})

	lg.Info("澄清门已触发，turn 挂起等待用户作答",
		loggateway.Str("task_id", taskID),
		loggateway.Str("step_id", stepID),
		loggateway.Int("question_count", len(questions)))

	return ClarificationGateDecision{
		Triggered: true,
		StepID:    stepID,
	}, nil
}

// autoResolveClarification 假设式前进路径：全部澄清问题按推荐默认自动作答。
// 与挂起路径的差异：step 直接以 completed 落库（resolution=auto_default，审计透明），
// 不迁移会话状态、不登记 pendingClarification，turn 携带澄清上下文继续执行。
// Stability:internal
func (o *ChatOrchestrator) autoResolveClarification(
	ctx context.Context,
	sessionID string,
	intentArt *intent.Artifact,
	ag biz.Agent,
	input biz.TurnInput,
	questions []biz.ClarificationQuestion,
) (ClarificationGateDecision, error) {
	lg := o.lg().With(
		loggateway.SessionID(sessionID),
		loggateway.StepID("chat.clarification_gate"),
	)

	taskID := string(chatagent.RootTaskActivityIDFromCtx(ctx))
	if taskID == "" {
		taskID = string(chatagent.RootTaskActivityID(uuid.NewString()))
	}
	now := time.Now().UTC()
	task := biz.Task{
		ID:          taskID,
		SessionID:   sessionID,
		UserMessage: input.Content,
		Status:      biz.TaskStatusRunning,
		Seq:         1,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := o.taskV2Writer().UpsertTask(ctx, task); err != nil {
		lg.Warn("澄清自动作答 UpsertTask 失败", loggateway.Err(err))
		return ClarificationGateDecision{}, err
	}

	envelope := biz.ClarificationEnvelope{
		Version:       1,
		Kind:          "clarification",
		Questions:     questions,
		OriginalInput: input.Content,
		Resolution:    biz.ClarificationResolutionAutoDefault,
	}
	envelope.ApplyRecommendedAnswers()
	if artJSON, err := json.Marshal(intentArt); err == nil {
		envelope.IntentArtifactJSON = string(artJSON)
	}
	contentJSON, err := json.Marshal(envelope)
	if err != nil {
		lg.Warn("澄清自动作答信封序列化失败", loggateway.Err(err))
		return ClarificationGateDecision{}, err
	}

	stepID := taskID + "-clarify"
	step := biz.Step{
		ID:              stepID,
		TurnID:          "",
		TaskID:          taskID,
		SessionID:       sessionID,
		SpiritSessionID: sessionID,
		Kind:            biz.StepKindClarify,
		AuthorAgentKey:  ag.AgentKey,
		Seq:             1,
		Version:         1,
		Content:         string(contentJSON),
		Status:          biz.StepStatusCompleted,
		StartedAt:       now,
		CompletedAt:     &now,
	}
	if _, err := o.stepWriter().CreateStep(ctx, step); err != nil {
		lg.Warn("澄清自动作答 CreateStep 失败", loggateway.Err(err))
		return ClarificationGateDecision{}, err
	}

	if o.v2Seq != nil {
		o.v2Seq.Publish(ctx, biz.NewTaskCreatedEvent(task))
		o.v2Seq.Publish(ctx, biz.NewStepCreatedEvent(step))
	}

	// 重写输入：澄清上下文 + 原始需求；产物剥离澄清残留后由调用方重新注入。
	resolved := input
	resolved.Content = envelope.BuildClarifiedContext() + "\n\n原始需求：" + input.Content

	lg.Info("澄清问题均含推荐默认，按推荐假设继续执行",
		loggateway.Str("task_id", taskID),
		loggateway.Str("step_id", stepID),
		loggateway.Int("question_count", len(questions)))

	return ClarificationGateDecision{
		StepID:        stepID,
		AutoResolved:  true,
		ResolvedInput: resolved,
		Artifact:      intentArt.CloneWithoutClarification(),
	}, nil
}
