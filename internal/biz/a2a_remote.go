package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// A2ARemoteAgent registers an external A2A service in a workspace catalog.
type A2ARemoteAgent struct {
	ID             string
	Workspace      string
	DisplayName    string
	RemoteURL      string
	AgentCardURL   string
	AuthType       string
	AuthConfigJSON string
	Enabled        bool
	DiscoveredCard A2AAgentCard
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

// RegisterRemoteAgent validates input, discovers the remote card, and persists.
func (u *A2AUsecase) RegisterRemoteAgent(ctx context.Context, in RegisterRemoteAgentInput) (A2ARemoteAgent, error) {
	if u == nil || u.repo == nil {
		return A2ARemoteAgent{}, errors.InternalServer("A2A", "a2a repo not configured")
	}
	remoteURL := strings.TrimSpace(in.RemoteURL)
	if remoteURL == "" {
		return A2ARemoteAgent{}, errors.BadRequest("A2A", "remote_url is required")
	}
	card, err := u.repo.DiscoverRemoteCard(ctx, RemoteCardDiscoverInput{
		RemoteURL:      remoteURL,
		AuthType:       in.AuthType,
		AuthConfigJSON: in.AuthConfigJSON,
	})
	if err != nil {
		return A2ARemoteAgent{}, err
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
	return u.repo.CreateRemoteAgent(ctx, A2ARemoteAgent{
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
func (u *A2AUsecase) ListRemoteAgents(ctx context.Context, workspace string) ([]A2ARemoteAgent, error) {
	if u == nil || u.repo == nil {
		return nil, errors.InternalServer("A2A", "a2a repo not configured")
	}
	return u.repo.ListRemoteAgents(ctx, strings.TrimSpace(workspace))
}

// DeleteRemoteAgent removes a remote registry entry.
func (u *A2AUsecase) DeleteRemoteAgent(ctx context.Context, id string) error {
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
func (u *A2AUsecase) DiscoverRemoteAgent(ctx context.Context, in RemoteCardDiscoverInput) (A2AAgentCard, error) {
	if u == nil || u.repo == nil {
		return A2AAgentCard{}, errors.InternalServer("A2A", "a2a repo not configured")
	}
	if strings.TrimSpace(in.RemoteURL) == "" {
		return A2AAgentCard{}, errors.BadRequest("A2A", "remote_url is required")
	}
	return u.repo.DiscoverRemoteCard(ctx, in)
}
