package biz

import (
	"context"
	"strings"
	"time"
)

const (
	A2ASourceLocal  = "local"
	A2ASourceRemote = "remote"
)

// A2AGatewayEntry is a federated discover row for the A2A gateway catalog.
type A2AGatewayEntry struct {
	Card        A2AAgentCard
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

// GatewayDiscover aggregates local enabled endpoints and remote registry entries.
func (u *A2AUsecase) GatewayDiscover(ctx context.Context, in GatewayDiscoverInput, publicBaseURL string) ([]A2AGatewayEntry, error) {
	if u == nil || u.repo == nil {
		return nil, nil
	}
	publicBaseURL = strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	local, err := u.repo.ListEnabledCards(ctx, in.Workspace, in.Capability)
	if err != nil {
		return nil, err
	}
	endpointEnabled, _ := u.repo.MapEndpointEnabled(ctx, agentIDsFromCards(local))

	out := make([]A2AGatewayEntry, 0, len(local)+8)
	for _, card := range local {
		entry := A2AGatewayEntry{
			Card:   card,
			Source: A2ASourceLocal,
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
		out = append(out, A2AGatewayEntry{
			Card:       card,
			Source:     A2ASourceRemote,
			RegistryID: r.ID,
			RemoteURL:  r.RemoteURL,
			Healthy:    healthy,
		})
	}
	return out, nil
}

func agentIDsFromCards(cards []A2AAgentCard) []string {
	return AgentIDsFromCards(cards)
}

// AgentIDsFromCards extracts non-empty agent ids from cards.
func AgentIDsFromCards(cards []A2AAgentCard) []string {
	ids := make([]string, 0, len(cards))
	for _, c := range cards {
		if id := strings.TrimSpace(c.AgentID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}
