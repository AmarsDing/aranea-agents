package a2a

import (
	"context"
	"time"
)

// Trust levels for federated organizations.
const (
	TrustLevelTrusted   = "trusted"
	TrustLevelNeutral   = "neutral"
	TrustLevelUntrusted = "untrusted"
)

// Policy actions. PolicyActionApproval is reserved; treated as deny this iteration.
const (
	PolicyActionAllow    = "allow"
	PolicyActionDeny     = "deny"
	PolicyActionApproval = "approval"
)

// Audit decisions.
const (
	DecisionAllowed      = "allowed"
	DecisionDeniedTrust  = "denied_trust"
	DecisionDeniedPolicy = "denied_policy"
	DecisionDeniedQuota  = "denied_quota"
)

// Audit directions. Only outbound is persisted this iteration.
const (
	AuditDirectionOutbound = "outbound"
	AuditDirectionInbound  = "inbound"
)

// Audit call statuses (result of an allowed invocation).
const (
	FederationCallStatusPending = "pending"
	FederationCallStatusSuccess = "success"
	FederationCallStatusError   = "error"
	FederationCallStatusTimeout = "timeout"
)

// Organization statuses.
const (
	OrgStatusActive    = "active"
	OrgStatusSuspended = "suspended"
)

// FederationLocalOrgID identifies this organization as the outbound caller.
// Policies and audits with caller_org_id = "local" refer to this organization;
// the frontend renders it as 「本组织」.
const FederationLocalOrgID = "local"

// FederationOrg is a registered organization in the A2A federation network.
type FederationOrg struct {
	ID             string
	Name           string
	Domain         string // unique; upsert key
	PublicBaseURL  string
	TrustLevel     string
	AuthType       string // reuses AuthType* constants
	AuthConfigJSON string
	Status         string
	JoinedAt       time.Time
	UpdatedAt      time.Time
}

// FederationPolicy controls outbound calls from one org pair.
type FederationPolicy struct {
	ID          string
	CallerOrgID string // FederationLocalOrgID = this organization (outbound policy)
	CalleeOrgID string
	Action      string
	MaxPerMin   int // per-minute invocation cap (Limiter sliding-window semantics); 0 = unlimited
	DailyQuota  int // daily invocation cap (count of decision=allowed); 0 = unlimited
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// FederationAuditLog records one cross-organization invocation decision + result.
type FederationAuditLog struct {
	ID            string
	Direction     string
	CallerOrgID   string
	CalleeOrgID   string
	CallerAgentID string
	CalleeAgentID string
	Capability    string
	Decision      string
	Status        string // pending | success | error | timeout
	LatencyMs     int64
	ErrorMessage  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// FederationAuditFilter scopes audit queries (audit panel filters + pagination).
type FederationAuditFilter struct {
	CallerOrgID string
	CalleeOrgID string
	Decision    string
	Status      string
	Limit       int
	Offset      int
}

// FederationOrgRepo persists federated organizations.
// Stability:evolving
type FederationOrgRepo interface {
	UpsertOrg(ctx context.Context, org FederationOrg) (FederationOrg, error) // upsert by domain
	GetOrg(ctx context.Context, id string) (FederationOrg, error)
	ListOrgs(ctx context.Context) ([]FederationOrg, error)
	UpdateOrgTrust(ctx context.Context, id, trustLevel string) error
	DeleteOrg(ctx context.Context, id string) error
}

// FederationPolicyRepo persists org-pair call policies.
// Stability:evolving
type FederationPolicyRepo interface {
	UpsertPolicy(ctx context.Context, p FederationPolicy) (FederationPolicy, error)
	GetPolicy(ctx context.Context, callerOrgID, calleeOrgID string) (FederationPolicy, error)
	ListPolicies(ctx context.Context) ([]FederationPolicy, error)
	DeletePolicy(ctx context.Context, id string) error
}

// FederationAuditRepo persists federation audit logs.
// Stability:evolving
type FederationAuditRepo interface {
	CreateAudit(ctx context.Context, log FederationAuditLog) (FederationAuditLog, error)
	UpdateAuditResult(ctx context.Context, id, status string, latencyMs int64, errMsg string) error
	ListAudits(ctx context.Context, filter FederationAuditFilter) ([]FederationAuditLog, int, error)
	CountCallsSince(ctx context.Context, callerOrgID, calleeOrgID string, since time.Time) (int, error)
}
