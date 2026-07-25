package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// ConfirmPlan handles user confirmation or rejection of a draft TaskPlan created
// by the pre-planning hard gate (P1-2).
//
// When approved=true:
//   - The plan transitions draft → confirmed via TaskPlanner.ConfirmPlan.
//   - An optional strategy_override may be applied.
//   - A plan_confirmed event is published so the frontend can refresh UI state.
//
// When approved=false:
//   - The plan is left in draft status (it is effectively abandoned). Future
//     turns will create a new plan if the user re-issues the same task.
//   - A plan_rejected event is published.
//
// Authorization: only the session owner may confirm/reject a plan. Anonymous
// (default_user) requests are rejected to prevent unauthenticated plan
// manipulation.
func (s *ChatService) ConfirmPlan(ctx context.Context, req *chatv1.ConfirmPlanRequest) (*chatv1.ConfirmPlanResponse, error) {
	if s == nil || s.orch == nil {
		return nil, apierror.Internal(apierror.DomainChat, "service unavailable")
	}

	planID := strings.TrimSpace(req.GetPlanId())
	sessionID := strings.TrimSpace(req.GetSessionId())
	if planID == "" || sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "plan_id and session_id are required")
	}

	planner := s.orch.team().TaskPlanner
	if planner == nil {
		return nil, apierror.Internal(apierror.DomainChat, "task planner unavailable")
	}

	// Load the plan first to validate session ownership before mutating state.
	plan, err := planner.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, apierror.NotFound(apierror.DomainChat, "plan not found")
	}
	if plan.SpiritSessionID != sessionID {
		return nil, apierror.BadRequest(apierror.DomainChat, "plan does not belong to session %s", sessionID)
	}
	if plan.Status != biz.TaskPlanStatusDraft {
		return nil, apierror.BadRequest(apierror.DomainChat, "plan is not in draft status (current: %s)", plan.Status)
	}

	// Authorization: reject anonymous users.
	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized(apierror.DomainChat, "user authentication required for plan confirmation")
	}

	// Authorization: only the session owner may confirm/reject a plan.
	sessions := s.orch.td().Sessions
	if sessions != nil {
		session, err := sessions.Get(ctx, sessionID)
		if err != nil {
			return nil, apierror.NotFound(apierror.DomainChat, "session not found")
		}
		if session.UserID != userID {
			s.lg.Warn("confirm plan ownership denied",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("plan_id", planID),
				loggateway.Str("user_id", userID),
				loggateway.Str("owner_id", session.UserID),
			)
			return nil, apierror.Forbidden(apierror.DomainChat, "only the session owner can confirm plans")
		}
	} else {
		return nil, apierror.Internal(apierror.DomainChat, "session store unavailable, cannot verify ownership")
	}

	// Rejection path: leave the plan in draft status and publish an event.
	// We do not transition the plan to a terminal state because the state
	// machine (validPlanTransitions) does not define a "rejected" state; the
	// draft plan simply becomes orphaned and a new turn will create a fresh
	// plan if the user re-issues the task.
	if !req.GetApproved() {
		s.lg.Info("TaskPlan rejected by user",
			loggateway.Str("plan_id", planID),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("reason", req.GetReason()),
		)
		s.publishPlanEvent(ctx, sessionID, planID, "rejected", req.GetReason())
		// P1: the rejection decision must become session context so the next
		// turn sees it and does not blindly regenerate the same plan.
		s.orch.injectPlanDecisionContext(ctx, sessionID, planID,
			buildPlanDecisionContext(plan, false, req.GetReason(), ""))
		return &chatv1.ConfirmPlanResponse{
			Accepted:     true,
			Status:       "rejected",
			PlanId:       planID,
			Strategy:     string(plan.Strategy),
			SubtaskCount: int32(len(plan.SubTasks)),
		}, nil
	}

	// Approval path: apply optional strategy override and confirm the plan.
	adjustments := biz.PlanAdjustments{
		Reason: req.GetReason(),
	}
	if override := strings.TrimSpace(req.GetStrategyOverride()); override != "" {
		if !isValidStrategy(override) {
			return nil, apierror.BadRequest(apierror.DomainChat, "invalid strategy_override: %s", override)
		}
		adjustments.StrategyOverride = override
	}

	confirmed, err := planner.ConfirmPlan(ctx, planID, adjustments)
	if err != nil {
		return nil, err
	}
	if confirmed == nil {
		return nil, apierror.Internal(apierror.DomainChat, "plan confirmation returned nil plan")
	}

	s.lg.Info("TaskPlan confirmed by user",
		loggateway.Str("plan_id", planID),
		loggateway.Str("session_id", sessionID),
		loggateway.Str("strategy", string(confirmed.Strategy)),
		loggateway.Int("subtask_count", len(confirmed.SubTasks)),
	)

	s.publishPlanEvent(ctx, sessionID, confirmed.ID, "confirmed", req.GetReason())

	// P1: the confirmation decision must become session context so subsequent
	// turns see what was approved (and why) as part of the LLM history.
	s.orch.injectPlanDecisionContext(ctx, sessionID, confirmed.ID,
		buildPlanDecisionContext(confirmed, true, req.GetReason(), req.GetStrategyOverride()))

	return &chatv1.ConfirmPlanResponse{
		Accepted:     true,
		Status:       "confirmed",
		PlanId:       confirmed.ID,
		Strategy:     string(confirmed.Strategy),
		SubtaskCount: int32(len(confirmed.SubTasks)),
	}, nil
}

// buildPlanDecisionContext renders the user's plan decision as a self-contained
// natural-language statement for LLM context injection. It follows the same
// semantics as the tool-confirmation resume content (buildConfirmResumeContent):
// the decision is stated explicitly, optional user reasoning is preserved
// verbatim, and the LLM receives actionable guidance (execute without re-asking
// on approval; never silently re-plan on rejection).
func buildPlanDecisionContext(plan *biz.TaskPlan, approved bool, reason, strategyOverride string) string {
	strategy := strings.TrimSpace(string(plan.Strategy))
	if strategy == "" {
		strategy = "未指定"
	}
	subtasks := len(plan.SubTasks)
	reason = strings.TrimSuffix(strings.TrimSpace(reason), "。")
	strategyOverride = strings.TrimSpace(strategyOverride)

	var b strings.Builder
	if approved {
		fmt.Fprintf(&b, "【计划确认】用户已批准执行计划（策略：%s，共 %d 个子任务）。", strategy, subtasks)
		if strategyOverride != "" {
			fmt.Fprintf(&b, "用户指定执行策略：%s。", strategyOverride)
		}
		if reason != "" {
			fmt.Fprintf(&b, "用户补充说明：%s。", reason)
		}
		b.WriteString("请按已确认的计划执行，无需再次征求确认。")
		return b.String()
	}
	fmt.Fprintf(&b, "【计划确认】用户拒绝了执行计划（策略：%s，共 %d 个子任务）。", strategy, subtasks)
	if reason != "" {
		fmt.Fprintf(&b, "用户理由：%s。", reason)
	} else {
		b.WriteString("用户未说明理由。")
	}
	b.WriteString("这是用户的明确决定：禁止自动重新生成相同或等价的计划；请先与用户沟通调整方向，再根据用户意图制定新计划。")
	return b.String()
}

// injectPlanDecisionContext makes a plan confirmation/rejection part of the
// session context (P1): a live trpc session event enters subsequent turns' LLM
// history (primary mechanism). It also attempts a system-role message append
// for audit — currently a no-op in production because NoopMessageWriter is
// wired (messages table removed; ActivityProjector owns persistence), but the
// call accumulates session metrics and remains correct if a real writer is
// restored. Best-effort: failures are logged, never returned — plan
// confirmation must not fail because context injection failed.
func (o *ChatOrchestrator) injectPlanDecisionContext(ctx context.Context, sessionID, planID, text string) {
	if sessRT := o.sessionRuntime(); sessRT != nil {
		if err := sessRT.AppendUserContextEvent(ctx, o.resolveUserID(ctx, sessionID), sessionID, "plan-confirm:"+planID, text); err != nil {
			o.lg().Warn("inject plan decision into session events failed",
				loggateway.StepID("chat.plan_confirm.inject"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("plan_id", planID),
				loggateway.Err(err))
		}
	}
	if sessions := o.td().Sessions; sessions != nil {
		msg := biz.ChatMessage{
			ID:              uuid.NewString(),
			SessionID:       sessionID,
			Role:            "system",
			ContentMarkdown: text,
			Status:          "ok",
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		}
		if err := sessions.AppendChatMessage(ctx, sessionID, msg, false); err != nil {
			o.lg().Warn("persist plan decision message failed",
				loggateway.StepID("chat.plan_confirm.inject"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("plan_id", planID),
				loggateway.Err(err))
		}
	}
}

// isValidStrategy checks whether a string is a valid OrchestrationStrategy.
func isValidStrategy(s string) bool {
	switch biz.OrchestrationStrategy(s) {
	case biz.StrategyDirect,
		biz.StrategySingleAgent,
		biz.StrategyParallel,
		biz.StrategyDAG,
		biz.StrategyCoordinator:
		return true
	}
	return false
}

// publishPlanEvent publishes a plan_confirmed or plan_rejected system.notice.
// Failures are logged but not returned (AS-EVT-01 informational).
func (s *ChatService) publishPlanEvent(ctx context.Context, sessionID, planID, decision, reason string) {
	bus := s.orch.td().Pipeline.EventBus
	if bus == nil {
		return
	}
	noticeType := "plan_confirmed"
	message := "计划已确认"
	if decision == "rejected" {
		noticeType = "plan_rejected"
		message = "计划已拒绝"
	}
	if reason != "" {
		message = message + ": " + reason
	}
	bus.Publish(ctx, biz.NewSystemNoticeEvent(sessionID, noticeType, message, map[string]any{
		"plan_id":  planID,
		"decision": decision,
		"reason":   reason,
		"status":   decision,
		"source":   "plan-confirm",
	}))
}
