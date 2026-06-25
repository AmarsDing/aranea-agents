package service

import (
	"context"
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
	if plan.Status != biz.PlanStatusDraft {
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

	return &chatv1.ConfirmPlanResponse{
		Accepted:     true,
		Status:       "confirmed",
		PlanId:       confirmed.ID,
		Strategy:     string(confirmed.Strategy),
		SubtaskCount: int32(len(confirmed.SubTasks)),
	}, nil
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

// publishPlanEvent publishes a plan_confirmed or plan_rejected event envelope
// so the frontend can refresh UI state. Failures are logged but not returned,
// since event delivery is best-effort for informational events (AS-EVT-01).
func (s *ChatService) publishPlanEvent(ctx context.Context, sessionID, planID, decision, reason string) {
	bus := s.orch.td().Pipeline.ActivityBus
	if bus == nil {
		return
	}
	bus.Publish(ctx, biz.ActivityEvent{
		Event: biz.ActivityEventCreated,
		Activity: biz.Activity{
			ID:        uuid.NewString(),
			Kind:      biz.ActivityKindPlan,
			SessionID: sessionID,
			Timestamp: time.Now().UTC(),
			Meta: map[string]any{
				"plan_id":  planID,
				"decision": decision,
				"reason":   reason,
				"status":   decision,
			},
		},
		Domain: biz.ActivityDomainChat,
	})
}
