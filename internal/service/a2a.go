package service

import (
	"context"
	"strings"
	"time"

	v1 "aranea-agents/api/kratos/a2a/v1"
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/internal/biz"
	a2atrpc "aranea-agents/internal/a2a/trpc"
	"aranea-agents/pkg/auth"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
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
	uc          *biz.A2AUsecase
	runner      a2apkg.AgentTurnRunner
	agents      biz.AgentRepository
	endpoints          *a2atrpc.EndpointRegistry
	publicBaseStore    *a2apkg.PublicBaseURLStore
}

// NewA2AService constructs an A2AService.
func NewA2AService(uc *biz.A2AUsecase, runner a2apkg.AgentTurnRunner, agents biz.AgentRepository, endpoints *a2atrpc.EndpointRegistry, publicBaseStore *a2apkg.PublicBaseURLStore) *A2AService {
	return &A2AService{
		uc:              uc,
		runner:          runner,
		agents:          agents,
		endpoints:       endpoints,
		publicBaseStore: publicBaseStore,
	}
}

func (s *A2AService) effectivePublicBase() (url, source string) {
	if s.publicBaseStore != nil {
		r := s.publicBaseStore.Get()
		return r.URL, r.Source
	}
	if s.endpoints != nil {
		return s.endpoints.BaseURL(), a2apkg.PublicBaseSourceDerived
	}
	return "", ""
}

// Discover returns A2A-enabled agents, optionally filtered by workspace/capability.
func (s *A2AService) Discover(ctx context.Context, req *v1.DiscoverRequest) (*v1.DiscoverResponse, error) {
	cards, err := s.uc.Discover(ctx, req.GetWorkspace(), req.GetCapability())
	if err != nil {
		return nil, err
	}
	endpointEnabled, _ := s.uc.MapEndpointEnabled(ctx, biz.AgentIDsFromCards(cards))
	publicBase, _ := s.effectivePublicBase()
	out := make([]*v1.A2AAgentCard, 0, len(cards))
	for _, c := range cards {
		protoCard := toProtoA2ACard(c)
		if c.Source == biz.A2ASourceLocal && endpointEnabled[c.AgentID] && publicBase != "" {
			protoCard.EndpointUrl = publicBase + "/" + c.AgentID
		}
		out = append(out, protoCard)
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

	target, err := a2apkg.ResolveInvokeTarget(ctx, s.uc, calleeID)
	if err != nil {
		a2aInvokeTotal.WithLabelValues("", calleeID, "forbidden").Inc()
		return nil, err
	}
	var card biz.A2AAgentCard
	switch target.Kind {
	case a2apkg.InvokeTargetLocal:
		card = target.Local
		if err := a2apkg.CheckCalleeCard(card, nil, capability); err != nil {
			a2aInvokeTotal.WithLabelValues("", calleeID, "forbidden").Inc()
			return nil, err
		}
	case a2apkg.InvokeTargetRemote:
		card = target.Remote.DiscoveredCard
		if card.AgentID == "" {
			card.AgentID = target.Remote.ID
		}
		if card.Workspace == "" {
			card.Workspace = target.Remote.Workspace
		}
		if err := a2apkg.CheckCalleeCard(card, nil, capability); err != nil {
			a2aInvokeTotal.WithLabelValues("", calleeID, "forbidden").Inc()
			return nil, err
		}
	default:
		a2aInvokeTotal.WithLabelValues("", calleeID, "forbidden").Inc()
		return nil, kerrors.InternalServer("A2A", "unknown invoke target")
	}
	if err := a2apkg.ValidateAdminInvokeWorkspace(ctx, req.GetWorkspace(), card); err != nil {
		a2aInvokeTotal.WithLabelValues("", calleeID, "forbidden").Inc()
		return nil, err
	}

	callerKey := strings.TrimSpace(req.GetWorkspace())
	if callerKey == "" {
		callerKey = "admin"
	}
	if !biz.DefaultA2AInvokeLimiter().Allow(callerKey, calleeID) {
		a2aInvokeTotal.WithLabelValues(callerKey, calleeID, "rate_limited").Inc()
		return nil, kerrors.New(429, "A2A", "invoke rate limit exceeded")
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

	invoker := a2apkg.NewInvoker(s.runner, s.uc, s.agents)

	timer := prometheus.NewTimer(a2aInvokeDuration)
	start := time.Now()
	result, runErr := invoker(ctx, calleeID, capability, req.GetPayloadJson(), timeoutSec)
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
	displayName := ""
	if s.agents != nil {
		if ag, err := s.agents.GetAgentByID(ctx, req.GetAgentId()); err == nil {
			displayName = strings.TrimSpace(ag.DisplayName)
			if ag.Settings != nil {
				workspace = strings.TrimSpace(ag.Settings.GetIdentity().Workspace)
			}
			if workspace == "" {
				workspace = displayName
			}
		}
	}
	card, err := s.uc.UpdateAgentCard(ctx, biz.A2AAgentCard{
		AgentID:      req.GetAgentId(),
		DisplayName:  displayName,
		Workspace:    workspace,
		Enabled:      req.GetEnabled(),
		Capabilities: caps,
	})
	if err != nil {
		return nil, err
	}
	if s.endpoints != nil {
		s.endpoints.Invalidate(req.GetAgentId())
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

// RegisterRemoteAgent adds an external agent to the workspace registry.
func (s *A2AService) RegisterRemoteAgent(ctx context.Context, req *v1.RegisterRemoteAgentRequest) (*v1.A2ARemoteAgent, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = req.GetEnabled()
	}
	item, err := s.uc.RegisterRemoteAgent(ctx, biz.RegisterRemoteAgentInput{
		Workspace:      req.GetWorkspace(),
		RemoteURL:      req.GetRemoteUrl(),
		AgentCardURL:   req.GetAgentCardUrl(),
		DisplayName:    req.GetDisplayName(),
		AuthType:       req.GetAuthType(),
		AuthConfigJSON: req.GetAuthConfigJson(),
		Enabled:        enabled,
	})
	if err != nil {
		return nil, err
	}
	return toProtoRemoteAgent(item), nil
}

// ListRemoteAgents returns registry entries for a workspace.
func (s *A2AService) ListRemoteAgents(ctx context.Context, req *v1.ListRemoteAgentsRequest) (*v1.ListRemoteAgentsResponse, error) {
	items, err := s.uc.ListRemoteAgents(ctx, req.GetWorkspace())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.A2ARemoteAgent, 0, len(items))
	for _, item := range items {
		out = append(out, toProtoRemoteAgent(item))
	}
	return &v1.ListRemoteAgentsResponse{Items: out}, nil
}

// DeleteRemoteAgent removes a remote registry entry.
func (s *A2AService) DeleteRemoteAgent(ctx context.Context, req *v1.DeleteRemoteAgentRequest) (*emptypb.Empty, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.DeleteRemoteAgent(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DiscoverRemoteAgent fetches AgentCard metadata from a URL without persisting.
func (s *A2AService) DiscoverRemoteAgent(ctx context.Context, req *v1.DiscoverRemoteAgentRequest) (*v1.A2AAgentCard, error) {
	card, err := s.uc.DiscoverRemoteAgent(ctx, biz.RemoteCardDiscoverInput{
		RemoteURL:      req.GetRemoteUrl(),
		AuthType:       req.GetAuthType(),
		AuthConfigJSON: req.GetAuthConfigJson(),
	})
	if err != nil {
		return nil, err
	}
	return toProtoA2ACard(card), nil
}

// GatewayDiscover returns federated local endpoints and remote registry entries.
func (s *A2AService) GatewayDiscover(ctx context.Context, req *v1.GatewayDiscoverRequest) (*v1.GatewayDiscoverResponse, error) {
	publicBase, _ := s.effectivePublicBase()
	items, err := s.uc.GatewayDiscover(ctx, biz.GatewayDiscoverInput{
		Workspace:   req.GetWorkspace(),
		Capability:  req.GetCapability(),
		CheckHealth: req.GetCheckHealth(),
	}, publicBase)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.A2AGatewayEntry, 0, len(items))
	for _, item := range items {
		out = append(out, &v1.A2AGatewayEntry{
			Card:        toProtoA2ACard(item.Card),
			Source:      item.Source,
			RegistryId:  item.RegistryID,
			EndpointUrl: item.EndpointURL,
			RemoteUrl:   item.RemoteURL,
			Healthy:     item.Healthy,
		})
	}
	return &v1.GatewayDiscoverResponse{Items: out}, nil
}

// GetA2AConfig returns read-only runtime settings for admin UI.
func (s *A2AService) GetA2AConfig(context.Context, *emptypb.Empty) (*v1.A2ARuntimeConfig, error) {
	url, source := s.effectivePublicBase()
	return &v1.A2ARuntimeConfig{
		PublicBaseUrl:       url,
		PublicBaseUrlSource: source,
	}, nil
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
		Source:       c.Source,
		EndpointUrl:  c.EndpointURL,
		RemoteUrl:    c.RemoteURL,
	}
}

func toProtoRemoteAgent(r biz.A2ARemoteAgent) *v1.A2ARemoteAgent {
	return &v1.A2ARemoteAgent{
		Id:             r.ID,
		Workspace:      r.Workspace,
		DisplayName:    r.DisplayName,
		RemoteUrl:      r.RemoteURL,
		AgentCardUrl:   r.AgentCardURL,
		AuthType:       r.AuthType,
		Enabled:        r.Enabled,
		DiscoveredCard: toProtoA2ACard(r.DiscoveredCard),
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}
