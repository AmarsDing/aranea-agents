package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	v1 "aranea-agents/api/kratos/sandbox/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/sandbox"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// SandboxService is the M82 sandbox admin API (design §5.3): a thin mapping
// over biz.SandboxAdminPort. Read-only in P0 (ADR-82-3: no execution
// capability over HTTP); ForceKill lands with P2-3.
type SandboxService struct {
	v1.UnimplementedSandboxServiceServer

	port biz.SandboxAdminPort
	lg   loggateway.Logger
}

// NewSandboxService creates the service. port is always non-nil in production
// (Manager is constructed even when disabled → empty list / zero metrics).
func NewSandboxService(port biz.SandboxAdminPort, lg loggateway.Logger) *SandboxService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SandboxService{port: port, lg: lg}
}

// ListSandboxes returns all live sandboxes (warm-pool ready + leased).
func (s *SandboxService) ListSandboxes(ctx context.Context, _ *v1.ListSandboxesRequest) (*v1.ListSandboxesResponse, error) {
	views, err := s.port.AdminSandboxList(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.ListSandboxesResponse{Items: make([]*v1.Sandbox, 0, len(views))}
	for _, sv := range views {
		out.Items = append(out.Items, &v1.Sandbox{
			Id:         sv.ID,
			Profile:    sv.Profile,
			AgentKey:   sv.AgentKey,
			SessionId:  sv.SessionID,
			RunId:      sv.RunID,
			State:      sv.State,
			CreatedAt:  sv.CreatedAt,
			Deadline:   sv.Deadline,
			LastExecAt: sv.LastExecAt,
			ExecCount:  sv.ExecCount,
		})
	}
	return out, nil
}

// ForceKillSandbox destroys a live sandbox on operator request (P2-3). The
// kill reason is mandatory (FR-33 audit); the operator identity is taken from
// the JWT claims, never from the request body.
func (s *SandboxService) ForceKillSandbox(ctx context.Context, req *v1.ForceKillSandboxRequest) (*v1.ForceKillSandboxResponse, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, apierror.BadRequest(apierror.DomainSandbox, "sandbox id required")
	}
	reason := strings.TrimSpace(req.GetReason())
	if reason == "" {
		return nil, apierror.BadRequest(apierror.DomainSandbox, "kill reason required (audit FR-33)")
	}
	operator := "anonymous"
	if claims, ok := auth.FromContext(ctx); ok && claims != nil {
		operator = strconv.FormatInt(claims.UserID, 10)
	}
	if err := s.port.AdminSandboxForceKill(ctx, id, reason, operator); err != nil {
		switch {
		case errors.Is(err, sandbox.ErrNotFound):
			return nil, apierror.NotFound(apierror.DomainSandbox, "sandbox %s not found", id)
		case errors.Is(err, sandbox.ErrDisabled):
			return nil, apierror.BadRequest(apierror.DomainSandbox, "sandbox subsystem disabled")
		default:
			return nil, err
		}
	}
	return &v1.ForceKillSandboxResponse{Id: id}, nil
}

// GetSandboxMetrics returns pool water levels and the counter snapshot.
func (s *SandboxService) GetSandboxMetrics(ctx context.Context, _ *v1.GetSandboxMetricsRequest) (*v1.SandboxMetrics, error) {
	m, err := s.port.AdminSandboxMetrics(ctx)
	if err != nil {
		return nil, err
	}
	out := &v1.SandboxMetrics{
		Profiles:     make([]*v1.SandboxProfileMetrics, 0, len(m.Profiles)),
		GlobalActive: int32(m.GlobalActive),
		AcquireWarm:  m.AcquireWarm,
		AcquireCold:  m.AcquireCold,
		AcquireFail:  m.AcquireFail,
		ExecOk:       m.ExecOK,
		ExecError:    m.ExecError,
		ExecTimeout:  m.ExecTimeout,
		Destroy:      m.Destroy,
		QuotaReject:  m.QuotaReject,
	}
	for _, p := range m.Profiles {
		out.Profiles = append(out.Profiles, &v1.SandboxProfileMetrics{
			Profile: p.Profile,
			Ready:   int32(p.Ready),
			Active:  int32(p.Active),
		})
	}
	return out, nil
}
