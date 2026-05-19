package service

import (
	"context"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/a2a/v1"
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/auth"

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
	uc     *biz.A2AUsecase
	runner a2apkg.AgentTurnRunner
	agents biz.AgentRepository
}

// NewA2AService constructs an A2AService.
func NewA2AService(uc *biz.A2AUsecase, runner a2apkg.AgentTurnRunner, agents biz.AgentRepository) *A2AService {
	return &A2AService{uc: uc, runner: runner, agents: agents}
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

// Invoke dispatches a capability call to the target agent (EP-A2A-01).
// EP-A2A-02: requires authenticated admin principal.
func (s *A2AService) Invoke(ctx context.Context, req *v1.A2AInvokeRequest) (*v1.A2AInvokeResponse, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	calleeID := strings.TrimSpace(req.GetCalleeAgentId())
	capability := strings.TrimSpace(req.GetCapability())
	if calleeID == "" {
		return nil, kerrors.BadRequest("A2A", "callee_agent_id is required")
	}
	if capability == "" {
		return nil, kerrors.BadRequest("A2A", "capability is required")
	}

	card, err := s.uc.GetAgentCard(ctx, calleeID)
	if err != nil || !card.Enabled {
		a2aInvokeTotal.WithLabelValues("", calleeID, "forbidden").Inc()
		return nil, kerrors.Forbidden("A2A", "agent "+calleeID+" is not A2A-enabled")
	}
	foundCap := false
	for _, c := range card.Capabilities {
		if c.Name == capability {
			foundCap = true
			break
		}
	}
	if !foundCap {
		a2aInvokeTotal.WithLabelValues("", calleeID, "forbidden").Inc()
		return nil, kerrors.BadRequest("A2A", "capability "+capability+" is not advertised by agent "+calleeID)
	}

	timeoutSec := int(req.GetTimeoutSeconds())
	if timeoutSec <= 0 {
		timeoutSec = 30
	}

	inv, err := s.uc.StartInvocation(ctx, biz.A2AInvocation{
		CalleeAgentID:   calleeID,
		Capability:      capability,
		PayloadJSON:     req.GetPayloadJson(),
		CallerSessionID: req.GetCallerSessionId(),
		TimeoutSeconds:  timeoutSec,
	})
	if err != nil {
		return nil, err
	}

	if s.runner == nil {
		a2aInvokeTotal.WithLabelValues("", calleeID, "error").Inc()
		return nil, kerrors.InternalServer("A2A", "agent turn runner not configured")
	}

	input := a2apkg.PayloadToInput(req.GetPayloadJson(), capability)
	timer := prometheus.NewTimer(a2aInvokeDuration)
	start := time.Now()
	result, runErr := s.runner.RunAgentTurn(ctx, calleeID, input, timeoutSec)
	durationMs := int(time.Since(start).Milliseconds())
	timer.ObserveDuration()

	status := "success"
	var errMsg string
	if runErr != nil {
		status = "error"
		errMsg = runErr.Error()
		a2aInvokeTotal.WithLabelValues("", calleeID, "error").Inc()
	} else {
		a2aInvokeTotal.WithLabelValues("", calleeID, "success").Inc()
	}

	inv.Status = status
	inv.ResultJSON = result
	inv.ErrorMessage = errMsg
	inv.DurationMs = durationMs
	_ = s.uc.FinishInvocation(ctx, inv)
	_ = s.uc.AppendAudit(ctx, biz.A2AAuditEntry{
		InvokeID:      inv.ID,
		CalleeAgentID: calleeID,
		Capability:    capability,
		Status:        status,
		DurationMs:    durationMs,
		Workspace:     card.Workspace,
	})

	return &v1.A2AInvokeResponse{
		InvokeId:     inv.ID,
		Status:       status,
		ResultJson:   result,
		ErrorMessage: errMsg,
		DurationMs:   int32(durationMs),
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
	workspace := ""
	if s.agents != nil {
		if ag, err := s.agents.GetAgentByID(ctx, req.GetAgentId()); err == nil {
			if ag.Settings != nil {
				workspace = strings.TrimSpace(ag.Settings.GetIdentity().Workspace)
			}
			if workspace == "" {
				workspace = strings.TrimSpace(ag.DisplayName)
			}
		}
	}
	card, err := s.uc.UpdateAgentCard(ctx, biz.A2AAgentCard{
		AgentID:      req.GetAgentId(),
		Workspace:    workspace,
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

// requireA2AAdmin enforces EP-A2A-02: HTTP Invoke requires authenticated admin.
func requireA2AAdmin(ctx context.Context) error {
	a, ok := auth.FromContext(ctx)
	if !ok || a == nil {
		return kerrors.Unauthorized("A2A", "authentication required")
	}
	if !a.HasAdminAccess() {
		return kerrors.Forbidden("A2A", "admin access required for invoke")
	}
	return nil
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
