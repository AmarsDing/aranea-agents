package service

import (
	"context"
	"strings"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// ListPlans returns all TaskPlans for a spirit session, newest first (T3.2).
//
// Authorization: only the session owner may list plans. Anonymous
// (default_user) requests are rejected to prevent unauthenticated access.
func (s *ChatService) ListPlans(ctx context.Context, req *chatv1.ListPlansRequest) (*chatv1.ListPlansResponse, error) {
	if s == nil || s.orch == nil {
		return nil, apierror.Internal("CHAT", "service unavailable")
	}

	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest("CHAT", "session_id is required")
	}

	planner := s.orch.team().TaskPlanner
	if planner == nil {
		return nil, apierror.Internal("CHAT", "task planner unavailable")
	}

	// Authorization: reject anonymous users.
	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized("CHAT", "user authentication required for plan listing")
	}

	// Authorization: only the session owner may list plans.
	sessions := s.orch.td().Sessions
	if sessions != nil {
		session, err := sessions.Get(ctx, sessionID)
		if err != nil {
			return nil, apierror.NotFound("CHAT", "session not found")
		}
		if session.UserID != userID {
			s.lg.Warn("list plans ownership denied",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("user_id", userID),
				loggateway.Str("owner_id", session.UserID),
			)
			return nil, apierror.Forbidden("CHAT", "only the session owner can list plans")
		}
	} else {
		s.lg.Warn("list plans skipped ownership check: sessions unavailable",
			loggateway.Str("session_id", sessionID),
		)
	}

	plans, err := planner.ListPlans(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	resp := &chatv1.ListPlansResponse{}
	for _, p := range plans {
		resp.Plans = append(resp.Plans, toTaskPlanSummary(p))
	}
	return resp, nil
}

// GetPlan returns a single TaskPlan by ID with full details (T3.2).
//
// Authorization: only the session owner may view plan details. Anonymous
// (default_user) requests are rejected to prevent unauthenticated access.
func (s *ChatService) GetPlan(ctx context.Context, req *chatv1.GetPlanRequest) (*chatv1.GetPlanResponse, error) {
	if s == nil || s.orch == nil {
		return nil, apierror.Internal("CHAT", "service unavailable")
	}

	planID := strings.TrimSpace(req.GetPlanId())
	sessionID := strings.TrimSpace(req.GetSessionId())
	if planID == "" || sessionID == "" {
		return nil, apierror.BadRequest("CHAT", "plan_id and session_id are required")
	}

	planner := s.orch.team().TaskPlanner
	if planner == nil {
		return nil, apierror.Internal("CHAT", "task planner unavailable")
	}

	// Authorization: reject anonymous users.
	userID := ctxuser.FromContext(ctx)
	if userID == ctxuser.DefaultUserID {
		return nil, apierror.Unauthorized("CHAT", "user authentication required for plan details")
	}

	// Authorization: only the session owner may view plan details.
	sessions := s.orch.td().Sessions
	if sessions != nil {
		session, err := sessions.Get(ctx, sessionID)
		if err != nil {
			return nil, apierror.NotFound("CHAT", "session not found")
		}
		if session.UserID != userID {
			s.lg.Warn("get plan ownership denied",
				loggateway.Str("session_id", sessionID),
				loggateway.Str("plan_id", planID),
				loggateway.Str("user_id", userID),
				loggateway.Str("owner_id", session.UserID),
			)
			return nil, apierror.Forbidden("CHAT", "only the session owner can view plan details")
		}
	} else {
		s.lg.Warn("get plan skipped ownership check: sessions unavailable",
			loggateway.Str("session_id", sessionID),
			loggateway.Str("plan_id", planID),
		)
	}

	plan, err := planner.GetPlan(ctx, planID)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, apierror.NotFound("CHAT", "plan not found")
	}
	if plan.SpiritSessionID != sessionID {
		return nil, apierror.BadRequest("CHAT", "plan does not belong to session %s", sessionID)
	}

	return &chatv1.GetPlanResponse{Plan: toTaskPlanDetail(plan)}, nil
}

// toTaskPlanSummary converts a biz.TaskPlan to a proto TaskPlanSummary.
func toTaskPlanSummary(p *biz.TaskPlan) *chatv1.TaskPlanSummary {
	if p == nil {
		return nil
	}
	return &chatv1.TaskPlanSummary{
		PlanId:          p.ID,
		SessionId:       p.SpiritSessionID,
		TraceId:         p.TraceID,
		UserMessage:     p.UserMessage,
		ComplexityLevel: string(p.ComplexityLevel),
		ComplexityScore: p.ComplexityScore,
		Strategy:        string(p.Strategy),
		Status:          string(p.Status),
		SubtaskCount:    int32(len(p.SubTasks)),
		CreatedAt:       p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// toTaskPlanDetail converts a biz.TaskPlan to a proto TaskPlanDetail.
func toTaskPlanDetail(p *biz.TaskPlan) *chatv1.TaskPlanDetail {
	if p == nil {
		return nil
	}
	detail := &chatv1.TaskPlanDetail{
		PlanId:            p.ID,
		SessionId:         p.SpiritSessionID,
		TraceId:           p.TraceID,
		UserMessage:       p.UserMessage,
		IntentArtifactJson: p.IntentArtifactJSON,
		ComplexityLevel:   string(p.ComplexityLevel),
		ComplexityScore:   p.ComplexityScore,
		Dimensions: &chatv1.DimensionScores{
			Semantic:   p.Dimensions.Semantic,
			Structural: p.Dimensions.Structural,
			Domain:     p.Dimensions.Domain,
			Tool:       p.Dimensions.Tool,
			Context:    p.Dimensions.Context,
			Historical: p.Dimensions.Historical,
		},
		DecomposeReason: p.DecomposeReason,
		Strategy:        string(p.Strategy),
		StrategyReason:  p.StrategyReason,
		TopologyHint:    string(p.TopologyHint),
		Status:          string(p.Status),
		CreatedAt:       p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:       p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	for _, st := range p.SubTasks {
		detail.SubTasks = append(detail.SubTasks, toProtoSubTask(st))
	}

	if p.TaskDAG != nil {
		detail.TaskDag = toProtoPlanTaskDAG(p.TaskDAG)
	}

	if p.MemoryHit != nil {
		detail.MemoryHit = &chatv1.MemoryHit{
			CacheId:       p.MemoryHit.CacheID,
			DqScore:       p.MemoryHit.DQScore,
			TopologyUsed:  p.MemoryHit.TopologyUsed,
			AgentKeysUsed: p.MemoryHit.AgentKeysUsed,
		}
	}

	return detail
}

// toProtoSubTask converts a biz.SubTask to a proto SubTask.
func toProtoSubTask(st biz.SubTask) *chatv1.SubTask {
	return &chatv1.SubTask{
		Id:                   st.ID,
		Name:                 st.Name,
		Description:          st.Description,
		DependsOn:            st.DependsOn,
		RequiredCapabilities: st.RequiredCapabilities,
		Priority:             int32(st.Priority),
		EstimatedComplexity:  st.EstimatedComplexity,
	}
}

// toProtoPlanTaskDAG converts a biz.PlanTaskDAG to a proto PlanTaskDAG.
func toProtoPlanTaskDAG(dag *biz.PlanTaskDAG) *chatv1.PlanTaskDAG {
	if dag == nil {
		return nil
	}
	protoDag := &chatv1.PlanTaskDAG{
		RootIds: dag.RootIDs,
		LeafIds: dag.LeafIDs,
	}
	for _, n := range dag.Nodes {
		protoDag.Nodes = append(protoDag.Nodes, toProtoSubTask(n))
	}
	return protoDag
}
