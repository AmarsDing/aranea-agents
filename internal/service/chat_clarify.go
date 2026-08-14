package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// errClarifyOriginalInputMissing 信封缺少 original_input，无法在 cache 缺失
// （重启 / 其他副本）时重建续跑输入。
var errClarifyOriginalInputMissing = errors.New("clarification envelope missing original_input")

// pendingClarification holds the state needed to resume a turn after the user
// submits clarification answers. Process-local cache only; persist via Step envelope.
type pendingClarification struct {
	Input     biz.TurnInput
	StepID    string
	TaskID    string
	CreatedAt time.Time
	// Artifact 是触发澄清门前的 Intent Pass 产物。续跑（卡片提交/自由回复）
	// 时经 ctx 复用，避免为重写后的输入重跑 Intent Pass LLM。
	Artifact *intent.Artifact
}

// clarificationPendingCache is a process-local hot cache of in-flight
// clarification resume state. The persisted clarify Step envelope is the
// source of truth; this cache must not be required after restart or across
// replicas (P0-3 admin multi-instance).
type clarificationPendingCache struct {
	m sync.Map // sessionID → pendingClarification
}

func (c *clarificationPendingCache) Load(sessionID string) (pendingClarification, bool) {
	if c == nil {
		return pendingClarification{}, false
	}
	v, ok := c.m.Load(sessionID)
	if !ok {
		return pendingClarification{}, false
	}
	pc, ok := v.(pendingClarification)
	return pc, ok
}

func (c *clarificationPendingCache) Store(sessionID string, pc pendingClarification) {
	if c == nil {
		return
	}
	c.m.Store(sessionID, pc)
}

func (c *clarificationPendingCache) Delete(sessionID string) {
	if c == nil {
		return
	}
	c.m.Delete(sessionID)
}

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

	// Convert proto answers to biz answers.
	answers := make([]biz.ClarificationAnswer, len(req.GetAnswers()))
	for i, a := range req.GetAnswers() {
		answers[i] = biz.ClarificationAnswer{
			Selected: a.GetSelected(),
			Other:    strings.TrimSpace(a.GetOther()),
		}
	}

	clarifiedContext, err := s.submitClarification(ctx, step, answers)
	if err != nil {
		return nil, err
	}

	return &chatv1.SubmitClarificationResponse{
		Accepted:         true,
		Status:           string(biz.StepStatusCompleted),
		ClarifiedContext: clarifiedContext,
	}, nil
}

// SubmitClarificationForCard 实现 biz.TurnControlGateway：渠道卡片回调的澄清提交入口。
// 归属校验已由渠道 peer 绑定（resolveCardActionSessionID）完成，此处不复查 ctxuser。
func (s *ChatService) SubmitClarificationForCard(ctx context.Context, sessionID, stepID string, answers []biz.ClarificationAnswer) (string, error) {
	if s == nil || s.orch == nil {
		return "", apierror.Internal(apierror.DomainChat, "service unavailable")
	}
	stepReader := s.orch.stepReader()
	if stepReader == nil {
		return "", apierror.Internal(apierror.DomainChat, "step store unavailable")
	}
	step, err := stepReader.GetStep(ctx, strings.TrimSpace(stepID))
	if err != nil {
		return "", err
	}
	if step.SessionID != strings.TrimSpace(sessionID) {
		return "", apierror.BadRequest(apierror.DomainChat, "step does not belong to session %s", sessionID)
	}
	if step.Kind != biz.StepKindClarify || step.Status != biz.StepStatusAwaitingInput {
		return "", apierror.Conflict(apierror.DomainChat, "clarification already submitted or expired (current status: %s)", step.Status)
	}
	if _, err := s.submitClarification(ctx, step, answers); err != nil {
		return "", err
	}
	return "已提交澄清回答", nil
}

// submitClarification 是澄清提交的状态机核心：信封作答、落库、事件发布、续跑。
// 调用方负责鉴权（RPC 路径：ctxuser + 归属；卡片路径：peer 绑定）。
// 返回注入 LLM 的澄清上下文。
func (s *ChatService) submitClarification(ctx context.Context, step biz.Step, answers []biz.ClarificationAnswer) (string, error) {
	stepWriter := s.orch.stepWriter()
	if stepWriter == nil {
		return "", apierror.Internal(apierror.DomainChat, "step store unavailable")
	}
	sessionID := step.SessionID

	// Parse the clarification envelope from step content.
	var envelope biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
		return "", apierror.Internal(apierror.DomainChat, "failed to parse clarification envelope: %v", err)
	}
	envelope.Answers = answers

	// Cache miss（重启 / 其他副本）时必须能从信封重建续跑输入。
	// 在 CAS 完成 step 之前失败，避免卡片已收口却无法续跑。
	if _, ok := s.orch.pendingClarifications.Load(sessionID); !ok {
		if strings.TrimSpace(envelope.OriginalInput) == "" {
			return "", apierror.FailedPrecondition(apierror.DomainChat,
				"clarification envelope missing original_input; cannot resume after restart")
		}
	}

	// Build clarified context for turn resumption.
	clarifiedContext := envelope.BuildClarifiedContext()

	// Update step: awaiting_input → completed, write back answers.
	now := time.Now().UTC()
	step.Status = biz.StepStatusCompleted
	step.CompletedAt = &now
	updatedContent, err := json.Marshal(envelope)
	if err != nil {
		return "", apierror.Internal(apierror.DomainChat, "failed to marshal clarification envelope: %v", err)
	}
	step.Content = string(updatedContent)

	if _, err := stepWriter.UpdateStep(ctx, step); err != nil {
		return "", err
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
	// envelope.OriginalInput 用于 cache 缺失时从信封重建续跑输入。
	// envelope.IntentArtifactJSON 用于重启后恢复意图产物，续跑复用而不重跑 Intent Pass。
	// step.AuthorAgentKey 补齐重建 TurnInput 的 AgentKey（信封未单列该字段）。
	// 异步执行：agent turn 可能持续数分钟，HTTP 必须立即返回；续跑过程通过 WS 事件呈现。
	fallbackArt := parseIntentArtifactJSON(envelope.IntentArtifactJSON)
	safego.Go(context.WithoutCancel(ctx), "chat.clarify.resume", func() {
		if resumeErr := s.orch.resumeTurnWithClarification(context.Background(), sessionID, step.TaskID, clarifiedContext, envelope.OriginalInput, step.AuthorAgentKey, fallbackArt); resumeErr != nil {
			s.lg.Warn("failed to resume turn with clarification",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("step_id", step.ID),
				loggateway.Err(resumeErr),
			)
			// Non-fatal: the step is already marked completed, so the user can
			// continue by sending a new message.
		}
	})

	return clarifiedContext, nil
}

// resumeTurnWithClarification resumes a paused turn after the user submits
// clarification answers. It resolves the original input (in-memory pending
// state, or lazily rebuilt from envelope.OriginalInput after a restart),
// injects the clarified context, and executes the turn.
// fallbackArt 是信封中持久化的意图产物（服务重启 pending 丢失时使用）；
// 解析出的产物经 ctx 传入续跑 turn，复用而不重跑 Intent Pass。
func (o *ChatOrchestrator) resumeTurnWithClarification(ctx context.Context, sessionID, taskID, clarifiedContext, originalInput, agentKey string, fallbackArt *intent.Artifact) error {
	input, intentArt, err := o.resolveResumeInput(sessionID, taskID, originalInput, agentKey, fallbackArt)
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
		loggateway.Bool("intent_artifact_reused", intentArt != nil),
	)

	// Execute the turn with the clarified input.
	// Use a background context to avoid cancellation from the HTTP request.
	turnCtx := context.Background()
	if intentArt != nil {
		turnCtx = intent.WithArtifact(turnCtx, intentArt)
	}
	_, err = o.Execute(turnCtx, input)
	return err
}

// resolveResumeInput 解析续跑所需的原始输入与意图产物：优先进程内 cache，
// 缺失（重启 / 其他副本）则从信封 OriginalInput 惰性重建最小 TurnInput，
// AgentKey 取 clarify Step.AuthorAgentKey，意图产物取信封 fallback。
func (o *ChatOrchestrator) resolveResumeInput(sessionID, taskID, originalInput, agentKey string, fallbackArt *intent.Artifact) (biz.TurnInput, *intent.Artifact, error) {
	if pc, ok := o.pendingClarifications.Load(sessionID); ok {
		if pc.TaskID != taskID {
			return biz.TurnInput{}, nil, apierror.BadRequest(apierror.DomainChat, "task ID mismatch: expected %s, got %s", pc.TaskID, taskID)
		}
		o.pendingClarifications.Delete(sessionID)
		return pc.Input, pc.Artifact, nil
	}
	if strings.TrimSpace(originalInput) == "" {
		return biz.TurnInput{}, nil, apierror.FailedPrecondition(apierror.DomainChat,
			"clarification envelope missing original_input; cannot resume after restart")
	}
	return biz.TurnInput{
		SessionID: sessionID,
		Content:   originalInput,
		AgentKey:  agentKey,
	}, fallbackArt, nil
}

// parseIntentArtifactJSON 反序列化信封中持久化的意图产物；空串或解析失败返回 nil
// （产物复用是性能优化，缺失时续跑退化为重跑 Intent Pass，不影响正确性）。
func parseIntentArtifactJSON(raw string) *intent.Artifact {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var art intent.Artifact
	if err := json.Unmarshal([]byte(raw), &art); err != nil {
		return nil
	}
	return &art
}

// resolveClarificationFreeText 检查会话是否处于澄清等待态；如是，将消息视为
// 自由回复。进程内 cache 优先；缺失时从持久化 clarify Step 信封重建。
// 单测与无会话快照的调用走本入口（cache miss 时直接 ListSteps）。
func (o *ChatOrchestrator) resolveClarificationFreeText(ctx context.Context, input biz.TurnInput) (biz.TurnInput, *intent.Artifact) {
	return o.resolveClarificationFreeTextHint(ctx, input, nil)
}

// resolveClarificationFreeTextHint 与 resolveClarificationFreeText 相同，但可
// 传入已加载的会话快照：非 awaiting_confirmation(reason=clarification) 时跳过
// ListSteps，供 runNativeAgentTurnBody 在 Sessions.Get 之后零额外查询重建。
func (o *ChatOrchestrator) resolveClarificationFreeTextHint(ctx context.Context, input biz.TurnInput, sess *biz.Session) (biz.TurnInput, *intent.Artifact) {
	sessionID := input.SessionID
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return input, nil
	}

	pc, ok := o.loadPendingClarification(ctx, sessionID, sess)
	if !ok {
		return input, nil
	}

	reader := o.stepReader()
	writer := o.stepWriter()
	if reader == nil || writer == nil {
		return input, nil
	}

	// 加载澄清 step，必须仍处于 awaiting_input
	step, err := reader.GetStep(ctx, pc.StepID)
	if err != nil || step.Status != biz.StepStatusAwaitingInput {
		o.pendingClarifications.Delete(sessionID)
		return input, nil
	}

	var envelope biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
		o.lg().Warn("failed to unmarshal clarification envelope", loggateway.Err(err))
		return input, nil
	}
	envelope.FreeText = content
	envelope.Answers = make([]biz.ClarificationAnswer, len(envelope.Questions))
	for i, q := range envelope.Questions {
		if len(q.Recommended) > 0 {
			envelope.Answers[i].Selected = q.Recommended
		}
	}

	now := time.Now().UTC()
	step.Status = biz.StepStatusCompleted
	step.CompletedAt = &now
	updatedContent, err := json.Marshal(envelope)
	if err != nil {
		o.lg().Warn("failed to marshal clarification envelope", loggateway.Err(err))
		return input, nil
	}
	step.Content = string(updatedContent)
	if _, err := writer.UpdateStep(ctx, step); err != nil {
		o.lg().Warn("failed to update clarification step", loggateway.Err(err))
		return input, nil
	}

	if o.v2Seq != nil {
		o.v2Seq.Publish(ctx, biz.NewStepUpdatedEvent(step))
	}

	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")
	o.pendingClarifications.Delete(sessionID)

	o.lg().Info("clarification resolved via free text",
		loggateway.SessionID(sessionID),
		loggateway.Str("step_id", pc.StepID),
		loggateway.Str("task_id", pc.TaskID),
	)

	resolved := pc.Input
	resolved.Content = envelope.BuildClarifiedContext() + "\n\n原始需求：" + pc.Input.Content
	resolved.ParentTaskID = pc.TaskID
	return resolved, pc.Artifact
}

func sessionAwaitingClarification(sess biz.Session) bool {
	return sess.Status == string(sessstatus.SessionStatusAwaitingConfirmation) &&
		sess.StatusReason == string(sessstatus.StatusReasonClarification)
}

// loadPendingClarification 优先进程内 cache；miss 时从持久化 clarify Step 重建。
// sess 非 nil 且并非澄清等待态时跳过 ListSteps（热路径零额外查询）。
func (o *ChatOrchestrator) loadPendingClarification(ctx context.Context, sessionID string, sess *biz.Session) (pendingClarification, bool) {
	if pc, ok := o.pendingClarifications.Load(sessionID); ok {
		return pc, true
	}
	if sess != nil && !sessionAwaitingClarification(*sess) {
		return pendingClarification{}, false
	}
	return o.rebuildPendingFromSteps(ctx, sessionID)
}

func (o *ChatOrchestrator) rebuildPendingFromSteps(ctx context.Context, sessionID string) (pendingClarification, bool) {
	reader := o.stepReader()
	if reader == nil {
		return pendingClarification{}, false
	}
	steps, err := reader.ListStepsBySession(ctx, sessionID)
	if err != nil {
		o.lg().Warn("list steps for clarification rebuild failed",
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
		return pendingClarification{}, false
	}
	step, ok := latestAwaitingClarifyStep(steps)
	if !ok {
		return pendingClarification{}, false
	}
	pc, err := pendingFromClarifyStep(step)
	if err != nil {
		o.lg().Warn("rebuild pending clarification from envelope failed",
			loggateway.SessionID(sessionID),
			loggateway.Str("step_id", step.ID),
			loggateway.Err(err))
		return pendingClarification{}, false
	}
	return pc, true
}

func latestAwaitingClarifyStep(steps []biz.Step) (biz.Step, bool) {
	var best biz.Step
	found := false
	for _, s := range steps {
		if s.Kind != biz.StepKindClarify || s.Status != biz.StepStatusAwaitingInput {
			continue
		}
		if !found || s.StartedAt.After(best.StartedAt) {
			best = s
			found = true
		}
	}
	return best, found
}

func pendingFromClarifyStep(step biz.Step) (pendingClarification, error) {
	var envelope biz.ClarificationEnvelope
	if err := json.Unmarshal([]byte(step.Content), &envelope); err != nil {
		return pendingClarification{}, err
	}
	if strings.TrimSpace(envelope.OriginalInput) == "" {
		return pendingClarification{}, errClarifyOriginalInputMissing
	}
	return pendingClarification{
		Input: biz.TurnInput{
			SessionID: step.SessionID,
			Content:   envelope.OriginalInput,
			AgentKey:  step.AuthorAgentKey,
		},
		StepID:    step.ID,
		TaskID:    step.TaskID,
		CreatedAt: step.StartedAt,
		Artifact:  parseIntentArtifactJSON(envelope.IntentArtifactJSON),
	}, nil
}
