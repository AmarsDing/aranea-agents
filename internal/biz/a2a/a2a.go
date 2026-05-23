// Package a2a implements agent-to-agent card management, invocation, and gateway workflows.
package a2a

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/errors"
)

// Capability describes one callable capability on an agent.
type Capability struct {
	Name             string
	Description      string
	InputSchemaJSON  string
	OutputSchemaJSON string
}

// AgentCard is the public A2A profile of an agent.
type AgentCard struct {
	AgentID      string
	DisplayName  string
	Workspace    string
	Enabled      bool
	Capabilities []Capability
	UpdatedAt    string
	Source       string
	EndpointURL  string
	RemoteURL    string
}

// Invocation records one call from one agent to another.
type Invocation struct {
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

// AuditEntry is one row in the audit log.
type AuditEntry struct {
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

// RemoteAgent registers an external A2A service in a workspace catalog.
type RemoteAgent struct {
	ID             string
	Workspace      string
	DisplayName    string
	RemoteURL      string
	AgentCardURL   string
	AuthType       string
	AuthConfigJSON string
	Enabled        bool
	DiscoveredCard AgentCard
	LastHealthAt   string
	LastHealthOK   bool
	LastHealthError string
	CreatedAt      string
	UpdatedAt      string
}

// RegisterRemoteAgentInput is the create payload for remote registry entries.
type RegisterRemoteAgentInput struct {
	Workspace      string
	RemoteURL      string
	AgentCardURL   string
	DisplayName    string
	AuthType       string
	AuthConfigJSON string
	Enabled        bool
}

// RemoteCardDiscoverInput fetches a remote AgentCard without persisting.
type RemoteCardDiscoverInput struct {
	RemoteURL      string
	AuthType       string
	AuthConfigJSON string
}

// GatewayEntry is a federated discover row for the A2A gateway catalog.
type GatewayEntry struct {
	Card        AgentCard
	Source      string
	RegistryID  string
	EndpointURL string
	RemoteURL   string
	Healthy     bool
}

// GatewayDiscoverInput filters gateway catalog rows.
type GatewayDiscoverInput struct {
	Workspace   string
	Capability  string
	CheckHealth bool
}

// Repo is the persistence interface for A2A operations.
type Repo interface {
	UpsertAgentCard(ctx context.Context, card AgentCard) (AgentCard, error)
	GetAgentCard(ctx context.Context, agentID string) (AgentCard, error)
	ListEnabledCards(ctx context.Context, workspace, capability string) ([]AgentCard, error)

	CreateInvocation(ctx context.Context, inv Invocation) (Invocation, error)
	UpdateInvocation(ctx context.Context, inv Invocation) error

	InsertAudit(ctx context.Context, entry AuditEntry) error
	ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]AuditEntry, int, error)

	MapEndpointEnabled(ctx context.Context, agentIDs []string) (map[string]bool, error)

	CreateRemoteAgent(ctx context.Context, agent RemoteAgent) (RemoteAgent, error)
	ListRemoteAgents(ctx context.Context, workspace string) ([]RemoteAgent, error)
	DeleteRemoteAgent(ctx context.Context, id string) error
	GetRemoteAgent(ctx context.Context, id string) (RemoteAgent, error)
	DiscoverRemoteCard(ctx context.Context, in RemoteCardDiscoverInput) (AgentCard, error)
	UpdateRemoteAgentHealth(ctx context.Context, id string, ok bool, errMsg string) error
}

// Source constants.
const (
	SourceLocal  = "local"
	SourceRemote = "remote"
)

// Usecase implements A2A card management and invocation logic.
type Usecase struct {
	repo Repo
}

// NewUsecase constructs an A2AUsecase.
func NewUsecase(repo Repo) *Usecase {
	return &Usecase{repo: repo}
}

func newA2AID() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "a2a-fallback"
	}
	return "a2a-" + hex.EncodeToString(buf)
}

// UpdateAgentCard sets or updates the A2A card for an agent.
func (u *Usecase) UpdateAgentCard(ctx context.Context, card AgentCard) (AgentCard, error) {
	if strings.TrimSpace(card.AgentID) == "" {
		return AgentCard{}, errors.BadRequest("A2A", "agent_id is required")
	}
	return u.repo.UpsertAgentCard(ctx, card)
}

// GetAgentCard returns the A2A card for one agent.
func (u *Usecase) GetAgentCard(ctx context.Context, agentID string) (AgentCard, error) {
	if strings.TrimSpace(agentID) == "" {
		return AgentCard{}, errors.BadRequest("A2A", "agent_id is required")
	}
	return u.repo.GetAgentCard(ctx, agentID)
}

// Discover returns A2A-enabled agents in a workspace, optionally filtered by capability.
func (u *Usecase) Discover(ctx context.Context, workspace, capability string) ([]AgentCard, error) {
	local, err := u.repo.ListEnabledCards(ctx, workspace, capability)
	if err != nil {
		return nil, err
	}
	remote, err := u.repo.ListRemoteAgents(ctx, workspace)
	if err != nil {
		return nil, err
	}
	out := make([]AgentCard, 0, len(local)+len(remote))
	for _, c := range local {
		c.Source = SourceLocal
		out = append(out, c)
	}
	for _, r := range remote {
		if !r.Enabled {
			continue
		}
		card := r.DiscoveredCard
		if card.AgentID == "" {
			card.AgentID = r.ID
		}
		if card.DisplayName == "" {
			card.DisplayName = r.DisplayName
		}
		if card.Workspace == "" {
			card.Workspace = r.Workspace
		}
		card.Enabled = true
		card.Source = SourceRemote
		card.RemoteURL = r.RemoteURL
		if capability != "" {
			found := false
			for _, c := range card.Capabilities {
				if c.Name == capability {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		out = append(out, card)
	}
	return out, nil
}

// GetRemoteAgent returns one remote registry entry by id.
func (u *Usecase) GetRemoteAgent(ctx context.Context, id string) (RemoteAgent, error) {
	if u == nil || u.repo == nil {
		return RemoteAgent{}, errors.InternalServer("A2A", "a2a repo not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return RemoteAgent{}, errors.BadRequest("A2A", "id is required")
	}
	agent, err := u.repo.GetRemoteAgent(ctx, id)
	if err != nil {
		return RemoteAgent{}, err
	}
	return agent, nil
}

// StartInvocation records a new invocation and returns it in status=pending.
func (u *Usecase) StartInvocation(ctx context.Context, inv Invocation) (Invocation, error) {
	inv.CallerAgentID = strings.TrimSpace(inv.CallerAgentID)
	inv.CalleeAgentID = strings.TrimSpace(inv.CalleeAgentID)
	inv.Capability = strings.TrimSpace(inv.Capability)
	if inv.CalleeAgentID == "" {
		return Invocation{}, errors.BadRequest("A2A", "callee_agent_id is required")
	}
	if inv.Capability == "" {
		return Invocation{}, errors.BadRequest("A2A", "capability is required")
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
func (u *Usecase) FinishInvocation(ctx context.Context, inv Invocation) error {
	return u.repo.UpdateInvocation(ctx, inv)
}

// AppendAudit writes one audit log record.
func (u *Usecase) AppendAudit(ctx context.Context, entry AuditEntry) error {
	if entry.ID == "" {
		entry.ID = newA2AID()
	}
	return u.repo.InsertAudit(ctx, entry)
}

// ListAudit returns audit log records.
func (u *Usecase) ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]AuditEntry, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return u.repo.ListAudit(ctx, callerID, calleeID, limit, offset)
}

// MapEndpointEnabled batch-loads a2a_agent_cards.enabled for catalog agent ids.
func (u *Usecase) MapEndpointEnabled(ctx context.Context, agentIDs []string) (map[string]bool, error) {
	if u == nil || u.repo == nil {
		return map[string]bool{}, nil
	}
	return u.repo.MapEndpointEnabled(ctx, agentIDs)
}

// RegisterRemoteAgent validates input, discovers the remote card, and persists.
func (u *Usecase) RegisterRemoteAgent(ctx context.Context, in RegisterRemoteAgentInput) (RemoteAgent, error) {
	if u == nil || u.repo == nil {
		return RemoteAgent{}, errors.InternalServer("A2A", "a2a repo not configured")
	}
	remoteURL := strings.TrimSpace(in.RemoteURL)
	if remoteURL == "" {
		return RemoteAgent{}, errors.BadRequest("A2A", "remote_url is required")
	}
	card, err := u.repo.DiscoverRemoteCard(ctx, RemoteCardDiscoverInput{
		RemoteURL:      remoteURL,
		AuthType:       in.AuthType,
		AuthConfigJSON: in.AuthConfigJSON,
	})
	if err != nil {
		return RemoteAgent{}, err
	}
	display := strings.TrimSpace(in.DisplayName)
	if display == "" {
		display = card.DisplayName
	}
	ws := strings.TrimSpace(in.Workspace)
	if ws == "" {
		ws = strings.TrimSpace(card.Workspace)
	}
	agentCardURL := strings.TrimSpace(in.AgentCardURL)
	if agentCardURL == "" {
		agentCardURL = remoteURL
	}
	return u.repo.CreateRemoteAgent(ctx, RemoteAgent{
		Workspace:      ws,
		DisplayName:    display,
		RemoteURL:      remoteURL,
		AgentCardURL:   agentCardURL,
		AuthType:       strings.TrimSpace(in.AuthType),
		AuthConfigJSON: strings.TrimSpace(in.AuthConfigJSON),
		Enabled:        in.Enabled,
		DiscoveredCard: card,
	})
}

// ListRemoteAgents returns registry entries for a workspace.
func (u *Usecase) ListRemoteAgents(ctx context.Context, workspace string) ([]RemoteAgent, error) {
	if u == nil || u.repo == nil {
		return nil, errors.InternalServer("A2A", "a2a repo not configured")
	}
	return u.repo.ListRemoteAgents(ctx, strings.TrimSpace(workspace))
}

// DeleteRemoteAgent removes a remote registry entry.
func (u *Usecase) DeleteRemoteAgent(ctx context.Context, id string) error {
	if u == nil || u.repo == nil {
		return errors.InternalServer("A2A", "a2a repo not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.BadRequest("A2A", "id is required")
	}
	return u.repo.DeleteRemoteAgent(ctx, id)
}

// DiscoverRemoteAgent fetches AgentCard metadata from a remote URL.
func (u *Usecase) DiscoverRemoteAgent(ctx context.Context, in RemoteCardDiscoverInput) (AgentCard, error) {
	if u == nil || u.repo == nil {
		return AgentCard{}, errors.InternalServer("A2A", "a2a repo not configured")
	}
	if strings.TrimSpace(in.RemoteURL) == "" {
		return AgentCard{}, errors.BadRequest("A2A", "remote_url is required")
	}
	return u.repo.DiscoverRemoteCard(ctx, in)
}

// PersistRemoteHealth stores the latest gateway health probe result for a remote registry entry.
func (u *Usecase) PersistRemoteHealth(ctx context.Context, id string, ok bool, errMsg string) error {
	if u == nil || u.repo == nil {
		return errors.InternalServer("A2A", "a2a repo not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.BadRequest("A2A", "id is required")
	}
	return u.repo.UpdateRemoteAgentHealth(ctx, id, ok, errMsg)
}

// GatewayDiscover aggregates local enabled endpoints and remote registry entries.
func (u *Usecase) GatewayDiscover(ctx context.Context, in GatewayDiscoverInput, publicBaseURL string) ([]GatewayEntry, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	local, err := u.repo.ListEnabledCards(ctx, in.Workspace, in.Capability)
	if err != nil {
		return nil, err
	}
	endpointEnabled, _ := u.repo.MapEndpointEnabled(ctx, AgentIDsFromCards(local))

	out := make([]GatewayEntry, 0, len(local)+8)
	for _, card := range local {
		entry := GatewayEntry{
			Card:    card,
			Source:  SourceLocal,
			Healthy: true,
		}
		if endpointEnabled[card.AgentID] && publicBaseURL != "" {
			entry.EndpointURL = publicBaseURL + "/" + card.AgentID
		}
		out = append(out, entry)
	}

	remote, err := u.repo.ListRemoteAgents(ctx, in.Workspace)
	if err != nil {
		return out, nil
	}
	for _, r := range remote {
		if !r.Enabled {
			continue
		}
		card := r.DiscoveredCard
		if card.AgentID == "" {
			card.AgentID = r.ID
		}
		if card.DisplayName == "" {
			card.DisplayName = r.DisplayName
		}
		if card.Workspace == "" {
			card.Workspace = r.Workspace
		}
		card.Enabled = true
		if in.Capability != "" {
			found := false
			for _, c := range card.Capabilities {
				if c.Name == in.Capability {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		healthy := true
		if in.CheckHealth {
			healthCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			_, err := u.repo.DiscoverRemoteCard(healthCtx, RemoteCardDiscoverInput{
				RemoteURL:      r.RemoteURL,
				AuthType:       r.AuthType,
				AuthConfigJSON: r.AuthConfigJSON,
			})
			cancel()
			healthy = err == nil
		}
		out = append(out, GatewayEntry{
			Card:       card,
			Source:     SourceRemote,
			RegistryID: r.ID,
			RemoteURL:  r.RemoteURL,
			Healthy:    healthy,
		})
	}
	return out, nil
}

// AgentIDsFromCards extracts non-empty agent ids from cards.
func AgentIDsFromCards(cards []AgentCard) []string {
	ids := make([]string, 0, len(cards))
	for _, c := range cards {
		if id := strings.TrimSpace(c.AgentID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
