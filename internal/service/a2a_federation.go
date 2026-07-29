package service

import (
	"context"
	"time"

	v1 "aranea-agents/api/kratos/a2a/v1"
	a2abiz "aranea-agents/internal/biz/a2a"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// FederationService implements kratos a2a.v1 FederationService: the transport
// adapter over FederationUsecase (org registry, trust, policies, directory,
// governed invocation, audit query). Governance itself lives in the biz
// layer; this type only maps proto <-> domain and enforces admin auth on
// mutating/invoking RPCs (EP-A2A-02, same as A2AService). It holds no logger:
// every failure is either returned as an apierror (logged by middleware) or
// already logged best-effort in the biz layer.
type FederationService struct {
	v1.UnimplementedFederationServiceServer
	uc *a2abiz.FederationUsecase
}

func NewFederationService(uc *a2abiz.FederationUsecase) *FederationService {
	return &FederationService{uc: uc}
}

// RegisterFederationOrg creates or updates an org (upsert by domain).
func (s *FederationService) RegisterFederationOrg(ctx context.Context, req *v1.RegisterFederationOrgRequest) (*v1.FederationOrg, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	org, err := s.uc.RegisterOrg(ctx, a2abiz.FederationOrg{
		Name:           req.GetName(),
		Domain:         req.GetDomain(),
		PublicBaseURL:  req.GetPublicBaseUrl(),
		TrustLevel:     req.GetTrustLevel(),
		AuthType:       req.GetAuthType(),
		AuthConfigJSON: req.GetAuthConfigJson(),
	})
	if err != nil {
		return nil, err
	}
	return toProtoFederationOrg(org), nil
}

// ListFederationOrgs lists all registered orgs.
func (s *FederationService) ListFederationOrgs(ctx context.Context, _ *v1.ListFederationOrgsRequest) (*v1.ListFederationOrgsResponse, error) {
	orgs, err := s.uc.ListOrgs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*v1.FederationOrg, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, toProtoFederationOrg(o))
	}
	return &v1.ListFederationOrgsResponse{Items: out}, nil
}

// DeleteFederationOrg removes an org; remote agents are disassociated only.
func (s *FederationService) DeleteFederationOrg(ctx context.Context, req *v1.DeleteFederationOrgRequest) (*emptypb.Empty, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.DeleteOrg(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// SetFederationTrustLevel sets an org's trust level and returns the updated org.
func (s *FederationService) SetFederationTrustLevel(ctx context.Context, req *v1.SetFederationTrustLevelRequest) (*v1.FederationOrg, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.SetTrustLevel(ctx, req.GetId(), req.GetTrustLevel()); err != nil {
		return nil, err
	}
	org, err := s.uc.GetOrg(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoFederationOrg(org), nil
}

// SyncFederationOrgCards manually pulls the org's remote agent cards into the
// directory cache; per-agent failures are skipped, synced reports successes.
func (s *FederationService) SyncFederationOrgCards(ctx context.Context, req *v1.SyncFederationOrgCardsRequest) (*v1.SyncFederationOrgCardsResponse, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	synced, err := s.uc.SyncOrgCards(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.SyncFederationOrgCardsResponse{Synced: int32(synced)}, nil
}

// UpsertFederationPolicy configures the call policy for one org pair.
func (s *FederationService) UpsertFederationPolicy(ctx context.Context, req *v1.UpsertFederationPolicyRequest) (*v1.FederationPolicy, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	p, err := s.uc.UpsertPolicy(ctx, a2abiz.FederationPolicy{
		CallerOrgID: req.GetCallerOrgId(),
		CalleeOrgID: req.GetCalleeOrgId(),
		Action:      req.GetAction(),
		MaxPerMin:   int(req.GetMaxPerMin()),
		DailyQuota:  int(req.GetDailyQuota()),
	})
	if err != nil {
		return nil, err
	}
	return toProtoFederationPolicy(p), nil
}

// DiscoverFederationAgents searches the federation directory (cached cards).
func (s *FederationService) DiscoverFederationAgents(ctx context.Context, req *v1.DiscoverFederationAgentsRequest) (*v1.DiscoverFederationAgentsResponse, error) {
	entries, err := s.uc.ListFederationAgents(ctx, req.GetCapability(), req.GetOrgId())
	if err != nil {
		return nil, err
	}
	out := make([]*v1.FederationAgentEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, &v1.FederationAgentEntry{
			Org:         toProtoFederationOrg(e.Org),
			RemoteAgent: toProtoRemoteAgent(e.RemoteAgent),
			Card:        toProtoA2ACard(e.Card),
		})
	}
	return &v1.DiscoverFederationAgentsResponse{Items: out}, nil
}

// InvokeFederatedAgent invokes a remote agent through the governance chain.
// Governance denials surface as gRPC errors; remote invocation outcomes
// (including failures) surface in the response status.
func (s *FederationService) InvokeFederatedAgent(ctx context.Context, req *v1.InvokeFederatedAgentRequest) (*v1.InvokeFederatedAgentResponse, error) {
	if err := requireA2AAdmin(ctx); err != nil {
		return nil, err
	}
	result, err := s.uc.InvokeFederated(ctx, a2abiz.FederatedInvokeInput{
		OrgID:         req.GetOrgId(),
		AgentID:       req.GetAgentId(),
		Capability:    req.GetCapability(),
		PayloadJSON:   req.GetPayloadJson(),
		TimeoutSec:    int(req.GetTimeoutSeconds()),
		Workspace:     req.GetWorkspace(),
		CallerAgentID: req.GetCallerAgentId(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.InvokeFederatedAgentResponse{
		AuditId:      result.AuditID,
		Status:       result.Status,
		ResultJson:   result.ResultJSON,
		ErrorMessage: result.ErrorMessage,
		LatencyMs:    result.LatencyMs,
	}, nil
}

// QueryFederationAuditLogs queries audit entries with filters + pagination.
func (s *FederationService) QueryFederationAuditLogs(ctx context.Context, req *v1.QueryFederationAuditLogsRequest) (*v1.QueryFederationAuditLogsResponse, error) {
	logs, total, err := s.uc.QueryAuditLogs(ctx, a2abiz.FederationAuditFilter{
		CallerOrgID: req.GetCallerOrgId(),
		CalleeOrgID: req.GetCalleeOrgId(),
		Decision:    req.GetDecision(),
		Status:      req.GetStatus(),
		Limit:       int(req.GetLimit()),
		Offset:      int(req.GetOffset()),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*v1.FederationAuditEntry, 0, len(logs))
	for _, l := range logs {
		out = append(out, toProtoFederationAuditEntry(l))
	}
	return &v1.QueryFederationAuditLogsResponse{Items: out, Total: int32(total)}, nil
}

// --- proto conversion helpers ---

// fedRFC3339 formats a timestamp; zero times map to "" rather than the
// RFC3339 zero value.
func fedRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func toProtoFederationOrg(o a2abiz.FederationOrg) *v1.FederationOrg {
	return &v1.FederationOrg{
		Id:            o.ID,
		Name:          o.Name,
		Domain:        o.Domain,
		PublicBaseUrl: o.PublicBaseURL,
		TrustLevel:    o.TrustLevel,
		AuthType:      o.AuthType,
		// auth_config_json is write-only (design F.8); only its presence is reported.
		AuthConfigSet: o.AuthConfigJSON != "",
		Status:        o.Status,
		JoinedAt:      fedRFC3339(o.JoinedAt),
		UpdatedAt:     fedRFC3339(o.UpdatedAt),
	}
}

func toProtoFederationPolicy(p a2abiz.FederationPolicy) *v1.FederationPolicy {
	return &v1.FederationPolicy{
		Id:          p.ID,
		CallerOrgId: p.CallerOrgID,
		CalleeOrgId: p.CalleeOrgID,
		Action:      p.Action,
		MaxPerMin:   int32(p.MaxPerMin),
		DailyQuota:  int32(p.DailyQuota),
		CreatedAt:   fedRFC3339(p.CreatedAt),
		UpdatedAt:   fedRFC3339(p.UpdatedAt),
	}
}

func toProtoFederationAuditEntry(l a2abiz.FederationAuditLog) *v1.FederationAuditEntry {
	return &v1.FederationAuditEntry{
		Id:            l.ID,
		Direction:     l.Direction,
		CallerOrgId:   l.CallerOrgID,
		CalleeOrgId:   l.CalleeOrgID,
		CallerAgentId: l.CallerAgentID,
		CalleeAgentId: l.CalleeAgentID,
		Capability:    l.Capability,
		Decision:      l.Decision,
		Status:        l.Status,
		LatencyMs:     l.LatencyMs,
		ErrorMessage:  l.ErrorMessage,
		CreatedAt:     fedRFC3339(l.CreatedAt),
	}
}
