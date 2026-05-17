package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// A2ACapability describes one callable capability on an agent.
type A2ACapability struct {
	Name              string
	Description       string
	InputSchemaJSON   string
	OutputSchemaJSON  string
}

// A2AAgentCard is the public A2A profile of an agent.
type A2AAgentCard struct {
	AgentID      string
	DisplayName  string
	Workspace    string
	Enabled      bool
	Capabilities []A2ACapability
	UpdatedAt    string
}

// A2AInvocation records one call from one agent to another.
type A2AInvocation struct {
	ID              string
	CallerAgentID   string
	CalleeAgentID   string
	CallerSessionID string
	Capability      string
	PayloadJSON     string
	Status          string // pending | running | success | error | timeout
	ResultJSON      string
	ErrorMessage    string
	DurationMs      int
	TimeoutSeconds  int
}

// A2AAuditEntry is one row in the audit log.
type A2AAuditEntry struct {
	ID            string
	InvokeID      string
	CallerAgentID string
	CalleeAgentID string
	Capability    string
	Status        string
	DurationMs    int
	Workspace     string
	CreatedAt     string
}

// A2ARepo is the persistence interface for A2A operations.
type A2ARepo interface {
	UpsertAgentCard(ctx context.Context, card A2AAgentCard) (A2AAgentCard, error)
	GetAgentCard(ctx context.Context, agentID string) (A2AAgentCard, error)
	ListEnabledCards(ctx context.Context, workspace, capability string) ([]A2AAgentCard, error)

	CreateInvocation(ctx context.Context, inv A2AInvocation) (A2AInvocation, error)
	UpdateInvocation(ctx context.Context, inv A2AInvocation) error

	InsertAudit(ctx context.Context, entry A2AAuditEntry) error
	ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]A2AAuditEntry, int, error)
}

// A2AUsecase implements A2A card management and invocation logic.
type A2AUsecase struct {
	repo A2ARepo
}

// NewA2AUsecase constructs an A2AUsecase.
func NewA2AUsecase(repo A2ARepo) *A2AUsecase {
	return &A2AUsecase{repo: repo}
}

func newA2AID() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "a2a-fallback"
	}
	return "a2a-" + hex.EncodeToString(buf)
}

// UpdateAgentCard sets or updates the A2A card for an agent.
func (u *A2AUsecase) UpdateAgentCard(ctx context.Context, card A2AAgentCard) (A2AAgentCard, error) {
	if strings.TrimSpace(card.AgentID) == "" {
		return A2AAgentCard{}, errors.BadRequest("A2A", "agent_id is required")
	}
	return u.repo.UpsertAgentCard(ctx, card)
}

// GetAgentCard returns the A2A card for one agent.
func (u *A2AUsecase) GetAgentCard(ctx context.Context, agentID string) (A2AAgentCard, error) {
	if strings.TrimSpace(agentID) == "" {
		return A2AAgentCard{}, errors.BadRequest("A2A", "agent_id is required")
	}
	return u.repo.GetAgentCard(ctx, agentID)
}

// Discover returns A2A-enabled agents in a workspace, optionally filtered by capability.
func (u *A2AUsecase) Discover(ctx context.Context, workspace, capability string) ([]A2AAgentCard, error) {
	return u.repo.ListEnabledCards(ctx, workspace, capability)
}

// StartInvocation records a new invocation and returns it in status=pending.
func (u *A2AUsecase) StartInvocation(ctx context.Context, inv A2AInvocation) (A2AInvocation, error) {
	inv.CallerAgentID = strings.TrimSpace(inv.CallerAgentID)
	inv.CalleeAgentID = strings.TrimSpace(inv.CalleeAgentID)
	inv.Capability = strings.TrimSpace(inv.Capability)
	if inv.CalleeAgentID == "" {
		return A2AInvocation{}, errors.BadRequest("A2A", "callee_agent_id is required")
	}
	if inv.Capability == "" {
		return A2AInvocation{}, errors.BadRequest("A2A", "capability is required")
	}
	if inv.PayloadJSON == "" {
		inv.PayloadJSON = "{}"
	}
	if inv.ID == "" {
		inv.ID = newA2AID()
	}
	if inv.Status == "" {
		inv.Status = "pending"
	}
	if inv.TimeoutSeconds <= 0 {
		inv.TimeoutSeconds = 30
	}
	return u.repo.CreateInvocation(ctx, inv)
}

// FinishInvocation updates an invocation with the result.
func (u *A2AUsecase) FinishInvocation(ctx context.Context, inv A2AInvocation) error {
	return u.repo.UpdateInvocation(ctx, inv)
}

// AppendAudit writes one audit log record.
func (u *A2AUsecase) AppendAudit(ctx context.Context, entry A2AAuditEntry) error {
	if entry.ID == "" {
		entry.ID = newA2AID()
	}
	return u.repo.InsertAudit(ctx, entry)
}

// ListAudit returns audit log records.
func (u *A2AUsecase) ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]A2AAuditEntry, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return u.repo.ListAudit(ctx, callerID, calleeID, limit, offset)
}
