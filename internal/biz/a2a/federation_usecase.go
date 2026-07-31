package a2a

import (
	"context"
	"errors"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// RemoteInvokeExecutor executes a governed remote A2A invocation. Implemented
// in internal/a2a (federation_invoke.go) by adapting InvokeRemoteRegistry so
// retry/SSRF/auth behavior is inherited unchanged (design F.10).
// Stability:evolving
type RemoteInvokeExecutor interface {
	InvokeRemote(ctx context.Context, remote RemoteAgent, capability, payloadJSON string, timeoutSec int) (string, error)
}

// FederationGovernance bundles the four chain components that always act
// together in InvokeFederated (design F.5/F.6): trust → policy → quota →
// audit. Bundling keeps FederationUsecase's dependency count within AS-COG-01.
type FederationGovernance struct {
	Trust  *TrustManager
	Policy *PolicyEngine
	Quota  *QuotaChecker
	Audit  *AuditLogger
}

// FederatedInvokeInput is the payload of one outbound cross-org invocation.
type FederatedInvokeInput struct {
	OrgID         string
	AgentID       string
	Capability    string
	PayloadJSON   string
	TimeoutSec    int
	Workspace     string
	CallerAgentID string // optional, audit attribution only
}

// FederatedInvokeResult is the outcome of one governed invocation. AuditID
// correlates the response with the persisted federation audit entry. Status
// is one of FederationCallStatusSuccess/Error/Timeout; invoke failures are
// reported here (not as Go errors) so the caller keeps the audit correlation
// — mirroring A2A Invoke semantics.
type FederatedInvokeResult struct {
	AuditID      string
	Status       string
	ResultJSON   string
	ErrorMessage string
	LatencyMs    int64
}

// Audit query pagination bounds (FED-F8).
const (
	defaultAuditQueryLimit = 50
	maxAuditQueryLimit     = 200
)

// FlowLogPair is a key-value pair for flow log extras. Defined locally
// because biz/a2a cannot import internal/biz (import cycle: biz → biz/a2a).
type FlowLogPair struct {
	Key   string
	Value any
}

// FlowLogWriter is the biz-side port for user-visible flow logs (流程日志),
// mirroring biz.FlowLogWriter. Implemented in the service/wire layer via
// event.TraceEmitter; a2a.* step IDs map to TraceDomainA2A there. Nil-safe:
// callers must nil-check before use (tests may pass nil).
// Stability:evolving
type FlowLogWriter interface {
	LogFlowStart(ctx context.Context, sessionID, stepID, message string, pairs ...FlowLogPair)
	LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...FlowLogPair)
	LogFlowError(ctx context.Context, sessionID, stepID, message string, pairs ...FlowLogPair)
}

// FederationUsecase orchestrates federation org/policy/audit management and
// the governed outbound invocation chain (design F.6).
type FederationUsecase struct {
	orgs      FederationOrgRepo
	gov       *FederationGovernance
	directory *Directory
	cardSync  *AgentCardSync
	remotes   RemoteAgentLister
	executor  RemoteInvokeExecutor
	flowLog   FlowLogWriter
}

// NewFederationUsecase constructs a FederationUsecase. flowLog may be nil
// (tests); flow log emission is then skipped.
func NewFederationUsecase(orgs FederationOrgRepo, gov *FederationGovernance, directory *Directory, cardSync *AgentCardSync, remotes RemoteAgentLister, executor RemoteInvokeExecutor, flowLog FlowLogWriter) *FederationUsecase {
	return &FederationUsecase{
		orgs:      orgs,
		gov:       gov,
		directory: directory,
		cardSync:  cardSync,
		remotes:   remotes,
		executor:  executor,
		flowLog:   flowLog,
	}
}

// --- Org management ---

// RegisterOrg validates and upserts an organization (by domain).
func (u *FederationUsecase) RegisterOrg(ctx context.Context, org FederationOrg) (FederationOrg, error) {
	if strings.TrimSpace(org.Name) == "" || strings.TrimSpace(org.Domain) == "" {
		return FederationOrg{}, apierror.BadRequest(apierror.DomainA2AFed, "name and domain are required")
	}
	return u.orgs.UpsertOrg(ctx, org)
}

// ListOrgs returns all registered organizations.
func (u *FederationUsecase) ListOrgs(ctx context.Context) ([]FederationOrg, error) {
	return u.orgs.ListOrgs(ctx)
}

// GetOrg returns one organization by ID.
func (u *FederationUsecase) GetOrg(ctx context.Context, id string) (FederationOrg, error) {
	if strings.TrimSpace(id) == "" {
		return FederationOrg{}, apierror.BadRequest(apierror.DomainA2AFed, "id is required")
	}
	return u.orgs.GetOrg(ctx, id)
}

// DeleteOrg removes an organization. Remote agents are disassociated (org_id
// cleared), not deleted (design F.7) — handled transactionally in the repo.
func (u *FederationUsecase) DeleteOrg(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest(apierror.DomainA2AFed, "id is required")
	}
	return u.orgs.DeleteOrg(ctx, id)
}

// SetTrustLevel validates and updates an organization's trust level.
func (u *FederationUsecase) SetTrustLevel(ctx context.Context, id, trustLevel string) error {
	switch trustLevel {
	case TrustLevelTrusted, TrustLevelNeutral, TrustLevelUntrusted:
	default:
		return apierror.BadRequest(apierror.DomainA2AFed, "invalid trust_level: %s", trustLevel)
	}
	return u.orgs.UpdateOrgTrust(ctx, id, trustLevel)
}

// --- Policy management ---

// UpsertPolicy validates, persists and cache-refreshes an org-pair policy.
func (u *FederationUsecase) UpsertPolicy(ctx context.Context, p FederationPolicy) (FederationPolicy, error) {
	if strings.TrimSpace(p.CallerOrgID) == "" || strings.TrimSpace(p.CalleeOrgID) == "" {
		return FederationPolicy{}, apierror.BadRequest(apierror.DomainA2AFed, "caller_org_id and callee_org_id are required")
	}
	switch p.Action {
	case PolicyActionAllow, PolicyActionDeny, PolicyActionApproval:
	default:
		return FederationPolicy{}, apierror.BadRequest(apierror.DomainA2AFed, "invalid action: %s", p.Action)
	}
	if p.MaxPerMin < 0 || p.DailyQuota < 0 {
		return FederationPolicy{}, apierror.BadRequest(apierror.DomainA2AFed, "max_per_min and daily_quota must be >= 0")
	}
	return u.gov.Policy.UpsertPolicy(ctx, p)
}

// ListPolicies returns the cached policy snapshot.
func (u *FederationUsecase) ListPolicies() []FederationPolicy {
	return u.gov.Policy.ListPolicies()
}

// DeletePolicy removes a policy by ID and invalidates its cache entry.
func (u *FederationUsecase) DeletePolicy(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest(apierror.DomainA2AFed, "id is required")
	}
	return u.gov.Policy.DeletePolicy(ctx, id)
}

// --- Directory / sync / audit query ---

// ListFederationAgents returns the cached federation directory (design F.5).
func (u *FederationUsecase) ListFederationAgents(ctx context.Context, capability, orgID string) ([]FederationAgentEntry, error) {
	return u.directory.ListFederationAgents(ctx, capability, orgID)
}

// SyncOrgCards re-discovers agent cards of one org's remote agents (FED-F7).
func (u *FederationUsecase) SyncOrgCards(ctx context.Context, orgID string) (int, error) {
	return u.cardSync.SyncOrgCards(ctx, orgID)
}

// QueryAuditLogs queries federation audit logs with bounded pagination.
func (u *FederationUsecase) QueryAuditLogs(ctx context.Context, filter FederationAuditFilter) ([]FederationAuditLog, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = defaultAuditQueryLimit
	}
	if filter.Limit > maxAuditQueryLimit {
		filter.Limit = maxAuditQueryLimit
	}
	return u.gov.Audit.ListAudits(ctx, filter)
}

// --- InvokeFederated governance chain (design F.6) ---

// InvokeFederated runs the full governance chain for one outbound call:
// validate → org lookup → status → trust → policy → quota → allowed audit
// (fail-closed) → target resolution → remote invoke → result audit.
//
// Governance/validation failures are returned as Go errors (mapped to 4xx/5xx
// by the transport). Remote invocation failures are reported in the result
// (Status=error|timeout) with a nil error, so the caller keeps the audit
// correlation — mirroring A2A Invoke semantics.
func (u *FederationUsecase) InvokeFederated(ctx context.Context, in FederatedInvokeInput) (FederatedInvokeResult, error) {
	var result FederatedInvokeResult
	if strings.TrimSpace(in.OrgID) == "" || strings.TrimSpace(in.AgentID) == "" || strings.TrimSpace(in.Capability) == "" {
		return result, apierror.BadRequest(apierror.DomainA2AFed, "org_id, agent_id and capability are required")
	}
	u.flowStart(ctx, "a2a.invoke.start", "A2A 联邦调用开始",
		FlowLogPair{Key: "org_id", Value: in.OrgID},
		FlowLogPair{Key: "agent_id", Value: in.AgentID},
		FlowLogPair{Key: "capability", Value: in.Capability})
	org, err := u.orgs.GetOrg(ctx, in.OrgID)
	if err != nil {
		u.flowError(ctx, "a2a.invoke.governance", "A2A 治理链检查失败",
			FlowLogPair{Key: "gate", Value: "org_lookup"},
			FlowLogPair{Key: "org_id", Value: in.OrgID},
			FlowLogPair{Key: "error", Value: err.Error()})
		return result, err
	}
	if org.Status != OrgStatusActive {
		u.flowError(ctx, "a2a.invoke.governance", "A2A 治理链拒绝：组织已停用",
			FlowLogPair{Key: "gate", Value: "org_status"},
			FlowLogPair{Key: "org_id", Value: org.ID})
		return result, apierror.Forbidden(apierror.DomainA2AFed, "federation org %s is suspended", in.OrgID)
	}
	auditBase := FederationAuditLog{
		CallerOrgID:   FederationLocalOrgID,
		CalleeOrgID:   org.ID,
		CallerAgentID: strings.TrimSpace(in.CallerAgentID),
		CalleeAgentID: in.AgentID,
		Capability:    in.Capability,
	}
	if !u.gov.Trust.Check(org.TrustLevel) {
		u.recordDenied(ctx, auditBase, DecisionDeniedTrust)
		u.flowError(ctx, "a2a.invoke.governance", "A2A 治理链拒绝：信任级别不足",
			FlowLogPair{Key: "gate", Value: "trust"},
			FlowLogPair{Key: "org_id", Value: org.ID},
			FlowLogPair{Key: "trust_level", Value: org.TrustLevel})
		return result, apierror.Forbidden(apierror.DomainA2AFed, "federation org %s is untrusted", in.OrgID)
	}
	if p, found := u.gov.Policy.Evaluate(FederationLocalOrgID, org.ID); found && u.gov.Policy.IsDenyAction(p.Action) {
		u.recordDenied(ctx, auditBase, DecisionDeniedPolicy)
		u.flowError(ctx, "a2a.invoke.governance", "A2A 治理链拒绝：策略拒绝",
			FlowLogPair{Key: "gate", Value: "policy"},
			FlowLogPair{Key: "org_id", Value: org.ID},
			FlowLogPair{Key: "policy_id", Value: p.ID},
			FlowLogPair{Key: "action", Value: p.Action})
		return result, apierror.Forbidden(apierror.DomainA2AFed, "federation policy denies calls from %s to %s", FederationLocalOrgID, org.ID)
	}
	if err := u.gov.Quota.Check(ctx, FederationLocalOrgID, org.ID); err != nil {
		if apierror.IsCode(err, apierror.CodeRateLimit) {
			u.recordDenied(ctx, auditBase, DecisionDeniedQuota)
		}
		u.flowError(ctx, "a2a.invoke.governance", "A2A 治理链拒绝：配额超限",
			FlowLogPair{Key: "gate", Value: "quota"},
			FlowLogPair{Key: "org_id", Value: org.ID},
			FlowLogPair{Key: "error", Value: err.Error()})
		return result, err
	}
	u.flowDone(ctx, "a2a.invoke.governance", "A2A 治理链检查通过",
		FlowLogPair{Key: "org_id", Value: org.ID},
		FlowLogPair{Key: "gates", Value: "trust,policy,quota"})
	allowed, err := u.gov.Audit.RecordAllowed(ctx, auditBase)
	if err != nil {
		u.flowError(ctx, "a2a.invoke.done", "A2A 审计落库失败",
			FlowLogPair{Key: "org_id", Value: org.ID},
			FlowLogPair{Key: "error", Value: err.Error()})
		return result, err
	}
	result.AuditID = allowed.ID
	remote, err := u.resolveFederatedTarget(ctx, in.Workspace, org.ID, in.AgentID)
	if err != nil {
		u.flowError(ctx, "a2a.invoke.remote", "A2A 远端目标解析失败",
			FlowLogPair{Key: "org_id", Value: org.ID},
			FlowLogPair{Key: "agent_id", Value: in.AgentID},
			FlowLogPair{Key: "error", Value: err.Error()})
		return result, err
	}
	start := time.Now()
	out, invokeErr := u.executor.InvokeRemote(ctx, remote, in.Capability, in.PayloadJSON, in.TimeoutSec)
	result.LatencyMs = time.Since(start).Milliseconds()
	if invokeErr != nil {
		result.Status = FederationCallStatusError
		if errors.Is(invokeErr, context.DeadlineExceeded) {
			result.Status = FederationCallStatusTimeout
		}
		result.ErrorMessage = invokeErr.Error()
		u.gov.Audit.RecordResult(ctx, allowed.ID, result.Status, result.LatencyMs, invokeErr.Error())
		u.flowError(ctx, "a2a.invoke.remote", "A2A 远端调用失败",
			FlowLogPair{Key: "org_id", Value: org.ID},
			FlowLogPair{Key: "agent_id", Value: remote.ID},
			FlowLogPair{Key: "remote_url", Value: remote.RemoteURL},
			FlowLogPair{Key: "latency_ms", Value: result.LatencyMs},
			FlowLogPair{Key: "status", Value: result.Status},
			FlowLogPair{Key: "error", Value: invokeErr.Error()})
		u.flowError(ctx, "a2a.invoke.done", "A2A 调用完成（失败）",
			FlowLogPair{Key: "audit_id", Value: allowed.ID},
			FlowLogPair{Key: "status", Value: result.Status},
			FlowLogPair{Key: "latency_ms", Value: result.LatencyMs})
		return result, nil
	}
	result.Status = FederationCallStatusSuccess
	result.ResultJSON = out
	u.gov.Audit.RecordResult(ctx, allowed.ID, result.Status, result.LatencyMs, "")
	u.flowDone(ctx, "a2a.invoke.remote", "A2A 远端调用完成",
		FlowLogPair{Key: "org_id", Value: org.ID},
		FlowLogPair{Key: "agent_id", Value: remote.ID},
		FlowLogPair{Key: "remote_url", Value: remote.RemoteURL},
		FlowLogPair{Key: "latency_ms", Value: result.LatencyMs})
	u.flowDone(ctx, "a2a.invoke.done", "A2A 调用完成",
		FlowLogPair{Key: "audit_id", Value: allowed.ID},
		FlowLogPair{Key: "status", Value: result.Status},
		FlowLogPair{Key: "latency_ms", Value: result.LatencyMs})
	return result, nil
}

// flowStart/flowDone/flowError emit user-visible flow logs via the injected
// port; nil-safe (no session scope for outbound federation calls).
func (u *FederationUsecase) flowStart(ctx context.Context, stepID, message string, pairs ...FlowLogPair) {
	if u == nil || u.flowLog == nil {
		return
	}
	u.flowLog.LogFlowStart(ctx, "", stepID, message, pairs...)
}

func (u *FederationUsecase) flowDone(ctx context.Context, stepID, message string, pairs ...FlowLogPair) {
	if u == nil || u.flowLog == nil {
		return
	}
	u.flowLog.LogFlowDone(ctx, "", stepID, message, pairs...)
}

func (u *FederationUsecase) flowError(ctx context.Context, stepID, message string, pairs ...FlowLogPair) {
	if u == nil || u.flowLog == nil {
		return
	}
	u.flowLog.LogFlowError(ctx, "", stepID, message, pairs...)
}

// resolveFederatedTarget finds the callee remote agent by filtering the
// caller workspace's remote agents in memory (T12; callee_resolve.go main
// path untouched per design).
func (u *FederationUsecase) resolveFederatedTarget(ctx context.Context, workspace, orgID, agentID string) (RemoteAgent, error) {
	remotes, err := u.remotes.ListRemoteAgents(ctx, workspace)
	if err != nil {
		return RemoteAgent{}, err
	}
	for _, r := range remotes {
		if r.OrgID == orgID && r.ID == agentID {
			return r, nil
		}
	}
	return RemoteAgent{}, apierror.NotFound(apierror.DomainA2AFed, "federation agent %s not registered under org %s in workspace", agentID, orgID)
}

func (u *FederationUsecase) recordDenied(ctx context.Context, base FederationAuditLog, decision string) {
	base.Decision = decision
	u.gov.Audit.RecordDenied(ctx, base)
}
