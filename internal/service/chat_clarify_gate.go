package service

import (
	"context"
	"encoding/json"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// ClarificationGate 判定结果。
type ClarificationGateDecision struct {
	// Triggered 为 true 表示已发布澄清卡片并挂起 turn。
	Triggered bool
	// StepID 是发布的澄清 Step ID（仅 Triggered=true 时有效）。
	StepID string
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
//  4. 存储 pendingClarification 供后续恢复
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
		OriginalInput: input.Content, // 持久化原始输入，服务重启后可惰性重建续跑
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

	// 4. 存储 pendingClarification 供后续恢复
	o.pendingClarifications.Store(sessionID, pendingClarification{
		Input:     input,
		StepID:    stepID,
		TaskID:    taskID,
		CreatedAt: now,
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
