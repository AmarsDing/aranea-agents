package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// resolveConfirmReply maps the ConfirmActivity request to the reply token
// sent through the await channel. A structured reply token (grant-scoped
// approve/deny) takes precedence over the legacy approved flag so the
// runtime confirmation gate can record the grant scope.
func resolveConfirmReply(reqApproved bool, reqReply string) (token string, approved bool, err error) {
	reqReply = strings.TrimSpace(reqReply)
	if reqReply == "" {
		if reqApproved {
			return "approved", true, nil
		}
		return "rejected", false, nil
	}
	outcome, structured := serviceawaitreply.ParseToolConfirmOutcome(reqReply)
	if !structured {
		return "", false, apierror.BadRequest(apierror.DomainChat, "unknown confirm reply token")
	}
	return reqReply, outcome.Approved(), nil
}

// ConfirmActivity handles user approval/rejection of a tool-blocked confirm Step.
// It loads from steps_v2, validates kind=confirm + status=tool_blocked,
// updates status via StepV2Writer, publishes system.notice, and resumes the
// awaiting run via the await channel.
func (s *ChatService) ConfirmActivity(ctx context.Context, req *chatv1.ConfirmActivityRequest) (*chatv1.ConfirmActivityResponse, error) {
	if s == nil || s.orch == nil {
		return nil, apierror.Internal(apierror.DomainChat, "service unavailable")
	}

	sessionID := strings.TrimSpace(req.GetSessionId())
	activityID := strings.TrimSpace(req.GetActivityId())
	if sessionID == "" || activityID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id and activity_id are required")
	}

	if biz.IsPlaybookConfirmActivityID(activityID) || (s.planExec != nil && s.planExec.HasPlaybookStageConfirm(sessionID, activityID)) {
		return s.confirmPlaybookStage(ctx, sessionID, activityID, req)
	}

	stepReader := s.orch.stepReader()
	stepWriter := s.orch.stepWriter()
	if stepReader == nil || stepWriter == nil {
		return nil, apierror.Internal(apierror.DomainChat, "step store unavailable")
	}
	step, err := stepReader.GetStep(ctx, activityID)
	if err != nil {
		return nil, err
	}

	if step.ToolName == biz.ToolPlaybookConfirmBefore || biz.IsPlaybookConfirmActivityID(step.ID) {
		return s.confirmPlaybookStage(ctx, sessionID, activityID, req)
	}

	if step.Kind != biz.StepKindConfirm {
		return nil, apierror.BadRequest(apierror.DomainChat, "expected confirm kind, got %s", step.Kind)
	}
	if step.Status != biz.StepStatusToolBlocked {
		return nil, apierror.BadRequest(apierror.DomainChat, "activity is not in tool_blocked state (current: %s)", step.Status)
	}
	if step.SessionID != sessionID {
		return nil, apierror.BadRequest(apierror.DomainChat, "activity does not belong to session %s", sessionID)
	}

	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized(apierror.DomainChat, "user authentication required for activity confirmation")
	}

	sessions := s.orch.td().Sessions
	if sessions != nil {
		session, err := sessions.Get(ctx, sessionID)
		if err != nil {
			return nil, apierror.NotFound(apierror.DomainChat, "session not found")
		}
		// 与 chat_plan_confirm.go / chat_clarify.go / assertSessionAccess 同语义：
		// 空 UserID 的会话（dev bypass/渠道入口创建，如飞书）放行，仅拒绝跨用户访问。
		// 严格相等会让渠道会话的工具确认卡在 Web 控制台被 403。
		if session.UserID != "" && session.UserID != userID {
			s.lg.Warn("confirm activity ownership denied",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("activity_id", activityID),
				loggateway.Str("user_id", userID),
				loggateway.Str("owner_id", session.UserID),
			)
			return nil, apierror.Forbidden(apierror.DomainChat, "only the session owner can confirm activities")
		}
	} else {
		return nil, apierror.Internal(apierror.DomainChat, "session store unavailable, cannot verify ownership")
	}

	replyMsg, approved, err := resolveConfirmReply(req.GetApproved(), req.GetReply())
	if err != nil {
		return nil, err
	}

	accepted, newStatus, err := s.confirmToolGate(ctx, step, replyMsg, approved)
	if err != nil {
		return nil, err
	}

	return &chatv1.ConfirmActivityResponse{
		Accepted: accepted,
		Status:   newStatus,
	}, nil
}

// ConfirmToolGateForCard 实现 biz.TurnControlGateway：渠道卡片回调的工具确认入口。
// 归属校验已由渠道 peer 绑定（resolveCardActionSessionID）完成，此处不复查 ctxuser。
func (s *ChatService) ConfirmToolGateForCard(ctx context.Context, sessionID, stepID, replyToken string) (bool, string) {
	if s == nil || s.orch == nil {
		return false, "服务不可用"
	}
	if biz.IsPlaybookConfirmActivityID(stepID) || (s.planExec != nil && s.planExec.HasPlaybookStageConfirm(sessionID, stepID)) {
		_, approved, err := resolveConfirmReply(true, replyToken)
		if err != nil {
			return false, err.Error()
		}
		if err := s.applyPlaybookStageDecision(ctx, sessionID, stepID, approved); err != nil {
			return false, err.Error()
		}
		if approved {
			return true, "已确认剧本阶段"
		}
		return true, "已拒绝剧本阶段"
	}
	stepReader := s.orch.stepReader()
	if stepReader == nil {
		return false, "服务不可用"
	}
	step, err := stepReader.GetStep(ctx, strings.TrimSpace(stepID))
	if err != nil {
		return false, "确认不存在或已删除"
	}
	if step.SessionID != strings.TrimSpace(sessionID) {
		return false, "确认不属于当前会话"
	}
	if step.ToolName == biz.ToolPlaybookConfirmBefore || biz.IsPlaybookConfirmActivityID(step.ID) {
		_, approved, err := resolveConfirmReply(true, replyToken)
		if err != nil {
			return false, err.Error()
		}
		if err := s.applyPlaybookStageDecision(ctx, sessionID, stepID, approved); err != nil {
			return false, err.Error()
		}
		if approved {
			return true, "已确认剧本阶段"
		}
		return true, "已拒绝剧本阶段"
	}
	if step.Kind != biz.StepKindConfirm || step.Status != biz.StepStatusToolBlocked {
		return false, "该确认已被处理或已超时"
	}
	replyMsg, approved, err := resolveConfirmReply(true, replyToken)
	if err != nil {
		return false, "未知的确认操作"
	}
	accepted, _, err := s.confirmToolGate(ctx, step, replyMsg, approved)
	if err != nil {
		s.lg.Warn("channel card confirm failed",
			loggateway.Str("session_id", sessionID),
			loggateway.Str("step_id", stepID),
			loggateway.Err(err),
		)
		return false, "确认处理失败，请稍后重试"
	}
	if !accepted {
		return false, "确认未生效（运行可能已结束）"
	}
	if approved {
		return true, "已批准执行"
	}
	return true, "已拒绝执行"
}

// confirmToolGate 是工具确认的状态机核心：状态转换、落库、事件发布、续跑投递。
// 调用方负责鉴权（RPC 路径：ctxuser + 归属；卡片区路径：peer 绑定）。
func (s *ChatService) confirmToolGate(ctx context.Context, step biz.Step, replyMsg string, approved bool) (accepted bool, newStatus string, err error) {
	stepWriter := s.orch.stepWriter()
	if stepWriter == nil {
		return false, "", apierror.Internal(apierror.DomainChat, "step store unavailable")
	}
	if IsExternalCodingConfirm(step) {
		if s.bridge == nil {
			return false, "", apierror.Internal(apierror.DomainChat, "coding bridge approval not wired")
		}
		if err := s.bridge.ConfirmBridgePermissionFromStep(ctx, step, replyMsg, approved); err != nil {
			return false, "", err
		}
	}

	transitionEvent := biz.ActivityTransitionDone
	if !approved {
		transitionEvent = biz.ActivityTransitionCancel
	}
	nextStatus, err := biz.TransitionActivityStatus(biz.ActivityStatus(step.Status), transitionEvent)
	if err != nil {
		return false, "", apierror.BadRequest(apierror.DomainChat,
			"illegal activity transition from %s via %s: %v",
			step.Status, transitionEvent, err)
	}
	step.Status = biz.StepStatus(nextStatus)
	now := time.Now().UTC()
	step.CompletedAt = &now
	if _, err := stepWriter.UpdateStep(ctx, step); err != nil {
		return false, "", err
	}

	if bus := s.orch.td().Pipeline.EventBus; bus != nil {
		decision := "approved"
		noticeType := "tool_confirm_approved"
		if !approved {
			decision = "rejected"
			noticeType = "tool_confirm_rejected"
		}
		meta := map[string]any{
			"activity_id": step.ID,
			"step_id":     step.ID,
			"decision":    decision,
			"status":      string(step.Status),
			"kind":        string(step.Kind),
		}
		bus.Publish(ctx, biz.NewSystemNoticeEvent(step.SessionID, noticeType, "", meta))
	}

	runID := ""
	if _, requestID, active := s.orch.ActiveRunner(step.SessionID); active {
		runID = requestID
	}
	// Unified delivery: the live channel receives the machine token (parsed by
	// the confirmation gate); if the channel is gone (process restart), the
	// run is resumed with a semantic natural-language statement of the user's
	// decision so the LLM receives it as meaningful context (P3 fix —
	// previously the decision was silently dropped on the restart path).
	outcome, err := s.orch.submitAwaitReply(ctx, step.SessionID, awaitReply{
		runID:         runID,
		token:         replyMsg,
		resumeContent: buildConfirmResumeContent(step, approved),
	})
	if err != nil {
		return false, "", err
	}

	return outcome != awaitReplyRejected, string(nextStatus), nil
}

func (s *ChatService) confirmPlaybookStage(ctx context.Context, sessionID, stepID string, req *chatv1.ConfirmActivityRequest) (*chatv1.ConfirmActivityResponse, error) {
	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized(apierror.DomainChat, "user authentication required for activity confirmation")
	}
	sessions := s.orch.td().Sessions
	if sessions == nil {
		return nil, apierror.Internal(apierror.DomainChat, "session store unavailable, cannot verify ownership")
	}
	session, err := sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, apierror.NotFound(apierror.DomainChat, "session not found")
	}
	if session.UserID != "" && session.UserID != userID {
		return nil, apierror.Forbidden(apierror.DomainChat, "only the session owner can confirm activities")
	}
	_, approved, err := resolveConfirmReply(req.GetApproved(), req.GetReply())
	if err != nil {
		return nil, err
	}
	if err := s.applyPlaybookStageDecision(ctx, sessionID, stepID, approved); err != nil {
		return nil, err
	}
	status := "confirmed"
	if !approved {
		status = "rejected"
	}
	return &chatv1.ConfirmActivityResponse{Accepted: true, Status: status}, nil
}

func (s *ChatService) applyPlaybookStageDecision(ctx context.Context, sessionID, activityID string, approved bool) error {
	sessionID = strings.TrimSpace(sessionID)
	activityID = strings.TrimSpace(activityID)
	if s == nil || s.orch == nil {
		return apierror.Internal(apierror.DomainChat, "service unavailable")
	}
	writer := s.orch.stepWriter()
	if writer == nil {
		return apierror.Internal(apierror.DomainChat, "step store unavailable")
	}

	existing, hasExisting := s.loadPlaybookConfirmStep(ctx, sessionID, activityID)
	if hasExisting {
		if existing.Kind != biz.StepKindConfirm {
			return apierror.BadRequest(apierror.DomainChat, "expected confirm kind, got %s", existing.Kind)
		}
		if playbookConfirmDecisionMatches(existing.Status, approved) {
			s.signalPlaybookConfirm(sessionID, activityID, approved)
			return nil
		}
		ev := biz.ActivityTransitionDone
		if !approved {
			ev = biz.ActivityTransitionCancel
		}
		if _, err := biz.TransitionActivityStatus(biz.ActivityStatus(existing.Status), ev); err != nil {
			return apierror.BadRequest(apierror.DomainChat,
				"illegal playbook confirm transition from %s via %s: %v",
				existing.Status, ev, err)
		}
	} else if s.planExec == nil || !s.planExec.HasPlaybookStageConfirm(sessionID, activityID) {
		return apierror.NotFound(apierror.DomainChat, "playbook confirm not found")
	}

	if err := s.persistPlaybookConfirmCard(ctx, sessionID, activityID, approved); err != nil {
		s.lg.Warn("persist playbook confirm card failed",
			loggateway.Str("session_id", sessionID),
			loggateway.Str("activity_id", activityID),
			loggateway.Err(err))
		return err
	}
	s.signalPlaybookConfirm(sessionID, activityID, approved)
	return nil
}

func (s *ChatService) signalPlaybookConfirm(sessionID, activityID string, approved bool) {
	if s == nil || s.planExec == nil {
		return
	}
	s.planExec.NotePlaybookConfirmDecision(sessionID, activityID, approved)
	_ = s.planExec.ResolvePlaybookStageConfirm(sessionID, activityID, approved)
}

func playbookConfirmDecisionMatches(status biz.StepStatus, approved bool) bool {
	if approved {
		return status == biz.StepStatusCompleted
	}
	return status == biz.StepStatusCancelled
}

func (s *ChatService) loadPlaybookConfirmStep(ctx context.Context, sessionID, activityID string) (biz.Step, bool) {
	if s == nil || s.orch == nil {
		return biz.Step{}, false
	}
	reader := s.orch.stepReader()
	if reader == nil {
		return biz.Step{}, false
	}
	existing, err := reader.GetStep(ctx, activityID)
	if err != nil || existing.ID == "" {
		return biz.Step{}, false
	}
	if existing.SessionID != "" && existing.SessionID != sessionID {
		return biz.Step{}, false
	}
	return existing, true
}

func (s *ChatService) persistPlaybookConfirmCard(ctx context.Context, sessionID, activityID string, approved bool) error {
	if s == nil || s.orch == nil {
		return apierror.Internal(apierror.DomainChat, "service unavailable")
	}
	writer := s.orch.stepWriter()
	if writer == nil {
		return apierror.Internal(apierror.DomainChat, "step store unavailable")
	}
	now := time.Now().UTC()
	step := biz.Step{
		ID:              activityID,
		SessionID:       sessionID,
		SpiritSessionID: sessionID,
		Kind:            biz.StepKindConfirm,
		ToolName:        biz.ToolPlaybookConfirmBefore,
		Status:          biz.StepStatusCompleted,
		CompletedAt:     &now,
		Version:         2,
	}
	if !approved {
		step.Status = biz.StepStatusCancelled
	}
	if existing, ok := s.loadPlaybookConfirmStep(ctx, sessionID, activityID); ok {
		step = existing
		step.CompletedAt = &now
		step.Version++
		if approved {
			step.Status = biz.StepStatusCompleted
		} else {
			step.Status = biz.StepStatusCancelled
		}
	}
	_, err := writer.UpsertStep(ctx, step)
	return err
}

// buildConfirmResumeContent renders the user's tool-confirmation decision as
// a self-contained natural-language statement. It is used as the turn content
// when the awaiting run is resumed after a process restart, so the LLM
// receives the decision as meaningful context instead of a bare machine token.
func buildConfirmResumeContent(step biz.Step, approved bool) string {
	toolName := strings.TrimSpace(step.ToolName)
	if toolName == "" {
		toolName = "未知工具"
	}
	if approved {
		return fmt.Sprintf("【工具确认】用户已批准执行工具 %q。请继续完成此前的任务。", toolName)
	}
	return fmt.Sprintf("【工具确认】用户已拒绝执行工具 %q。这是用户的明确决定，禁止重试相同或等价的操作；请向用户说明该操作已取消，并询问接下来如何处理。", toolName)
}
