package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// SubmitClarification handles user submission of clarification answers.
// It loads the clarify step, validates kind=clarify + status=awaiting_input,
// updates the step with answers, publishes system.notice, and resumes the turn.
func (s *ChatService) SubmitClarification(ctx context.Context, req *chatv1.SubmitClarificationRequest) (*chatv1.SubmitClarificationResponse, error) {
	if s == nil || s.orch == nil {
		return nil, apierror.Internal(apierror.DomainChat, "service unavailable")
	}

	sessionID := strings.TrimSpace(req.GetSessionId())
	stepID := strings.TrimSpace(req.GetStepId())
	if sessionID == "" || stepID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id and step_id are required")
	}

	stepReader := s.orch.stepReader()
	stepWriter := s.orch.stepWriter()
	if stepReader == nil || stepWriter == nil {
		return nil, apierror.Internal(apierror.DomainChat, "step store unavailable")
	}

	step, err := stepReader.GetStep(ctx, stepID)
	if err != nil {
		return nil, err
	}

	if step.Kind != biz.StepKindClarify {
		return nil, apierror.BadRequest(apierror.DomainChat, "expected clarify kind, got %s", step.Kind)
	}
	if step.Status != biz.StepStatusAwaitingInput {
		return nil, apierror.Conflict(apierror.DomainChat, "clarification already submitted (current status: %s)", step.Status)
	}
	if step.SessionID != sessionID {
		return nil, apierror.BadRequest(apierror.DomainChat, "step does not belong to session %s", sessionID)
	}

	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized(apierror.DomainChat, "user authentication required for clarification")
	}

	sessions := s.orch.td().Sessions
	if sessions != nil {
		session, err := sessions.Get(ctx, sessionID)
		if err != nil {
			return nil, apierror.NotFound(apierror.DomainChat, "session not found")
		}
		// 与 assertSessionAccess 同语义：空 UserID 的会话（dev bypass/渠道入口
		// 创建）放行，仅拒绝跨用户访问。严格相等会让 dev 模式会话提交澄清被 403。
		if session.UserID != "" && session.UserID != userID {
			s.lg.Warn("submit clarification ownership denied",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("step_id", stepID),
				loggateway.Str("user_id", userID),
				loggateway.Str("owner_id", session.UserID),
			)
			return nil, apierror.Forbidden(apierror.DomainChat, "only the session owner can submit clarification")
		}
	} else {
		return nil, apierror.Internal(apierror.DomainChat, "session store unavailable, cannot verify ownership")
	}

	// Parse the clarification envelope from step content.
	var envelope biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
		return nil, apierror.Internal(apierror.DomainChat, "failed to parse clarification envelope: %v", err)
	}

	// Convert proto answers to biz answers.
	answers := make([]biz.ClarificationAnswer, len(req.GetAnswers()))
	for i, a := range req.GetAnswers() {
		answers[i] = biz.ClarificationAnswer{
			Selected: a.GetSelected(),
			Other:    strings.TrimSpace(a.GetOther()),
		}
	}
	envelope.Answers = answers

	// Build clarified context for turn resumption.
	clarifiedContext := envelope.BuildClarifiedContext()

	// Update step: awaiting_input → completed, write back answers.
	now := time.Now().UTC()
	step.Status = biz.StepStatusCompleted
	step.CompletedAt = &now
	updatedContent, err := json.Marshal(envelope)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainChat, "failed to marshal clarification envelope: %v", err)
	}
	step.Content = string(updatedContent)

	if _, err := stepWriter.UpdateStep(ctx, step); err != nil {
		return nil, err
	}

	// Publish step updated event.
	if s.orch.v2Seq != nil {
		s.orch.v2Seq.Publish(ctx, biz.NewStepUpdatedEvent(step))
	}

	// Publish system.notice for clarification submission.
	if bus := s.orch.td().Pipeline.EventBus; bus != nil {
		meta := map[string]any{
			"step_id":  step.ID,
			"task_id":  step.TaskID,
			"status":   string(step.Status),
			"kind":     string(step.Kind),
			"answered": len(answers),
			"total":    len(envelope.Questions),
		}
		bus.Publish(ctx, biz.NewSystemNoticeEvent(step.SessionID, "clarification_submitted", "", meta))
	}

	// Transition session back to running.
	s.orch.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")

	// Resume the turn with clarified context.
	// The clarified context is injected as a user-perspective message.
	// envelope.OriginalInput 用于服务重启后 pendingClarifications 丢失时惰性重建续跑输入。
	// 异步执行：agent turn 可能持续数分钟，HTTP 必须立即返回；续跑过程通过 WS 事件呈现。
	safego.Go(context.WithoutCancel(ctx), "chat.clarify.resume", func() {
		if resumeErr := s.orch.resumeTurnWithClarification(context.Background(), sessionID, step.TaskID, clarifiedContext, envelope.OriginalInput); resumeErr != nil {
			s.lg.Warn("failed to resume turn with clarification",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("step_id", stepID),
				loggateway.Err(resumeErr),
			)
			// Non-fatal: the step is already marked completed, so the user can
			// continue by sending a new message.
		}
	})

	return &chatv1.SubmitClarificationResponse{
		Accepted:         true,
		Status:           string(step.Status),
		ClarifiedContext: clarifiedContext,
	}, nil
}

// resumeTurnWithClarification resumes a paused turn after the user submits
// clarification answers. It resolves the original input (in-memory pending
// state, or lazily rebuilt from envelope.OriginalInput after a restart),
// injects the clarified context, and executes the turn.
func (o *ChatOrchestrator) resumeTurnWithClarification(ctx context.Context, sessionID, taskID, clarifiedContext, originalInput string) error {
	input, err := o.resolveResumeInput(sessionID, taskID, originalInput)
	if err != nil {
		return err
	}

	// 续跑 turn 挂接到澄清门创建的 Task：同一任务卡片下展示澄清+执行，
	// 且 runClarificationGate 见 ParentTaskID 非空会跳过，防止澄清循环。
	input.ParentTaskID = taskID

	// Inject the clarified context into the original input.
	// The clarified context is prepended to the original content as a user-perspective message.
	input.Content = clarifiedContext + "\n\n原始需求：" + input.Content

	o.lg().Info("resuming turn with clarification",
		loggateway.SessionID(sessionID),
		loggateway.Str("task_id", taskID),
		loggateway.Int("clarified_context_len", len(clarifiedContext)),
	)

	// Execute the turn with the clarified input.
	// Use a background context to avoid cancellation from the HTTP request.
	turnCtx := context.Background()
	_, err = o.Execute(turnCtx, input)
	return err
}

// resolveResumeInput 解析续跑所需的原始输入：优先内存 pendingClarifications，
// 缺失（服务重启）则从信封 OriginalInput 惰性重建最小 TurnInput。
func (o *ChatOrchestrator) resolveResumeInput(sessionID, taskID, originalInput string) (biz.TurnInput, error) {
	// 1. 尝试从内存 pendingClarifications 加载
	if v, ok := o.pendingClarifications.Load(sessionID); ok {
		pc := v.(pendingClarification)
		if pc.TaskID != taskID {
			return biz.TurnInput{}, apierror.BadRequest(apierror.DomainChat, "task ID mismatch: expected %s, got %s", pc.TaskID, taskID)
		}
		o.pendingClarifications.Delete(sessionID)
		return pc.Input, nil
	}
	// 2. 内存态缺失，从信封 OriginalInput 惰性重建
	if originalInput != "" {
		return biz.TurnInput{
			SessionID: sessionID,
			Content:   originalInput,
		}, nil
	}
	return biz.TurnInput{}, apierror.NotFound(apierror.DomainChat, "no pending clarification for session %s", sessionID)
}

// resolveClarificationFreeText 检查会话是否处于澄清等待态；如是，将消息视为
// 自由回复：按推荐填充空作答、回写 free_text、完成澄清 step、恢复会话运行，
// 并返回重写后的输入（澄清上下文 + 原始需求，基于 pending 存储的原输入）。
// 非澄清等待态或处理失败时原样透传。
func (o *ChatOrchestrator) resolveClarificationFreeText(ctx context.Context, input biz.TurnInput) biz.TurnInput {
	sessionID := input.SessionID
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return input // 空消息直接透传
	}

	// 内存 pending 是澄清等待态的唯一判据（gate 触发时才写入）
	v, ok := o.pendingClarifications.Load(sessionID)
	if !ok {
		return input
	}
	pc := v.(pendingClarification)

	// 加载澄清 step，必须仍处于 awaiting_input
	step, err := o.stepReader().GetStep(ctx, pc.StepID)
	if err != nil || step.Status != biz.StepStatusAwaitingInput {
		// 失效 pending（已通过卡片提交 / step 不存在）：清除并透传
		o.pendingClarifications.Delete(sessionID)
		return input
	}

	// 解析信封并填充自由回复
	var envelope biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
		o.lg().Warn("failed to unmarshal clarification envelope", loggateway.Err(err))
		return input
	}
	envelope.FreeText = content
	// 空作答按推荐处理
	envelope.Answers = make([]biz.ClarificationAnswer, len(envelope.Questions))
	for i, q := range envelope.Questions {
		if len(q.Recommended) > 0 {
			envelope.Answers[i].Selected = q.Recommended
		}
	}

	// 更新 step 为 completed
	now := time.Now().UTC()
	step.Status = biz.StepStatusCompleted
	step.CompletedAt = &now
	updatedContent, err := json.Marshal(envelope)
	if err != nil {
		o.lg().Warn("failed to marshal clarification envelope", loggateway.Err(err))
		return input
	}
	step.Content = string(updatedContent)
	if _, err := o.stepWriter().UpdateStep(ctx, step); err != nil {
		o.lg().Warn("failed to update clarification step", loggateway.Err(err))
		return input
	}

	// 发布 StepUpdated 事件
	if o.v2Seq != nil {
		o.v2Seq.Publish(ctx, biz.NewStepUpdatedEvent(step))
	}

	// 恢复会话状态并清除 pending
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")
	o.pendingClarifications.Delete(sessionID)

	o.lg().Info("clarification resolved via free text",
		loggateway.SessionID(sessionID),
		loggateway.Str("step_id", pc.StepID),
		loggateway.Str("task_id", pc.TaskID),
	)

	// 重写输入：澄清上下文 + 原始需求（基于 pending 存储的原输入，保留 AgentKey 等字段）；
	// ParentTaskID 挂接 gate 创建的 Task（同一任务卡片，且防止澄清门循环）。
	resolved := pc.Input
	resolved.Content = envelope.BuildClarifiedContext() + "\n\n原始需求：" + pc.Input.Content
	resolved.ParentTaskID = pc.TaskID
	return resolved
}
