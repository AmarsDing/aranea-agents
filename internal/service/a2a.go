package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/a2a/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	a2aInvokeTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aranea_a2a_invoke_total",
		Help: "Total A2A invoke calls.",
	}, []string{"caller", "callee", "status"})

	a2aInvokeDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aranea_a2a_invoke_duration_seconds",
		Help:    "Duration of A2A invoke calls.",
		Buckets: prometheus.DefBuckets,
	})
)

// A2AService implements kratos a2a.v1.
type A2AService struct {
	v1.UnimplementedA2AServiceServer
	uc *biz.A2AUsecase
}

// NewA2AService constructs an A2AService.
func NewA2AService(uc *biz.A2AUsecase) *A2AService {
	return &A2AService{uc: uc}
}

// Discover returns A2A-enabled agents, optionally filtered by workspace/capability.
func (s *A2AService) Discover(ctx context.Context, req *v1.DiscoverRequest) (*v1.DiscoverResponse, error) {
	cards, err := s.uc.Discover(ctx, req.GetWorkspace(), req.GetCapability())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.A2AAgentCard, 0, len(cards))
	for _, c := range cards {
		out = append(out, toProtoA2ACard(c))
	}
	return &v1.DiscoverResponse{Agents: out}, nil
}

// Invoke dispatches a capability call to the target agent.
// The actual agent execution is expected to be wired at the application layer;
// this service records the invocation and audit entries.
func (s *A2AService) Invoke(ctx context.Context, req *v1.A2AInvokeRequest) (*v1.A2AInvokeResponse, error) {
	calleeID := strings.TrimSpace(req.GetCalleeAgentId())
	capability := strings.TrimSpace(req.GetCapability())
	if calleeID == "" {
		return nil, kerrors.BadRequest("A2A", "callee_agent_id is required")
	}
	if capability == "" {
		return nil, kerrors.BadRequest("A2A", "capability is required")
	}

	// Verify the callee has A2A enabled.
	card, err := s.uc.GetAgentCard(ctx, calleeID)
	if err != nil || !card.Enabled {
		a2aInvokeTotal.WithLabelValues("", calleeID, "forbidden").Inc()
		return nil, kerrors.Forbidden("A2A", "agent "+calleeID+" is not A2A-enabled")
	}

	timer := prometheus.NewTimer(a2aInvokeDuration)
	inv, err := s.uc.StartInvocation(ctx, biz.A2AInvocation{
		CalleeAgentID:   calleeID,
		Capability:      capability,
		PayloadJSON:     req.GetPayloadJson(),
		CallerSessionID: req.GetCallerSessionId(),
		TimeoutSeconds:  int(req.GetTimeoutSeconds()),
	})
	if err != nil {
		return nil, err
	}

	// Mark as pending; the actual execution is async / handled externally.
	timer.ObserveDuration()
	a2aInvokeTotal.WithLabelValues("", calleeID, "pending").Inc()

	_ = s.uc.AppendAudit(ctx, biz.A2AAuditEntry{
		InvokeID:      inv.ID,
		CalleeAgentID: calleeID,
		Capability:    capability,
		Status:        "pending",
	})

	return &v1.A2AInvokeResponse{
		InvokeId: inv.ID,
		Status:   "pending",
	}, nil
}

// UpdateAgentCard sets or updates the A2A capabilities of an agent.
func (s *A2AService) UpdateAgentCard(ctx context.Context, req *v1.UpdateAgentCardRequest) (*v1.A2AAgentCard, error) {
	if strings.TrimSpace(req.GetAgentId()) == "" {
		return nil, kerrors.BadRequest("A2A", "agent_id is required")
	}
	caps := make([]biz.A2ACapability, 0, len(req.GetCapabilities()))
	for _, c := range req.GetCapabilities() {
		caps = append(caps, biz.A2ACapability{
			Name:             c.GetName(),
			Description:      c.GetDescription(),
			InputSchemaJSON:  c.GetInputSchemaJson(),
			OutputSchemaJSON: c.GetOutputSchemaJson(),
		})
	}
	card, err := s.uc.UpdateAgentCard(ctx, biz.A2AAgentCard{
		AgentID:      req.GetAgentId(),
		Enabled:      req.GetEnabled(),
		Capabilities: caps,
	})
	if err != nil {
		return nil, err
	}
	return toProtoA2ACard(card), nil
}

// GetAgentCard returns the A2A card for one agent.
func (s *A2AService) GetAgentCard(ctx context.Context, req *v1.GetAgentCardRequest) (*v1.A2AAgentCard, error) {
	card, err := s.uc.GetAgentCard(ctx, req.GetAgentId())
	if err != nil {
		return nil, err
	}
	return toProtoA2ACard(card), nil
}

// ListAudit returns the A2A audit log.
func (s *A2AService) ListAudit(ctx context.Context, req *v1.ListAuditRequest) (*v1.ListAuditResponse, error) {
	entries, total, err := s.uc.ListAudit(ctx, req.GetCallerAgentId(), req.GetCalleeAgentId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.A2AAuditEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, &v1.A2AAuditEntry{
			Id:            e.ID,
			InvokeId:      e.InvokeID,
			CallerAgentId: e.CallerAgentID,
			CalleeAgentId: e.CalleeAgentID,
			Capability:    e.Capability,
			Status:        e.Status,
			DurationMs:    int32(e.DurationMs),
			Workspace:     e.Workspace,
			CreatedAt:     e.CreatedAt,
		})
	}
	return &v1.ListAuditResponse{Items: out, Total: int32(total)}, nil
}

// --- proto conversion helpers ---

func toProtoA2ACard(c biz.A2AAgentCard) *v1.A2AAgentCard {
	caps := make([]*v1.A2ACapability, 0, len(c.Capabilities))
	for _, cap := range c.Capabilities {
		caps = append(caps, &v1.A2ACapability{
			Name:             cap.Name,
			Description:      cap.Description,
			InputSchemaJson:  cap.InputSchemaJSON,
			OutputSchemaJson: cap.OutputSchemaJSON,
		})
	}
	return &v1.A2AAgentCard{
		AgentId:      c.AgentID,
		DisplayName:  c.DisplayName,
		Workspace:    c.Workspace,
		Enabled:      c.Enabled,
		Capabilities: caps,
		UpdatedAt:    c.UpdatedAt,
	}
}
