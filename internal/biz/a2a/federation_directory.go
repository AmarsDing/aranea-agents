package a2a

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Narrow ports for the federation directory/sync components, defined at the
// point of use (BI2). All three are satisfied by the existing data-layer
// a2aRepo; RemoteAgentCardWriter is the only new method (T10) and is kept out
// of RemoteAgentRepo to avoid widening an already-oversized interface.
// Stability:evolving
type RemoteAgentLister interface {
	ListRemoteAgents(ctx context.Context, workspace string) ([]RemoteAgent, error)
}

// Stability:evolving
type RemoteCardDiscoverer interface {
	DiscoverRemoteCard(ctx context.Context, input RemoteCardDiscoverInput) (AgentCard, error)
}

// Stability:evolving
type RemoteAgentCardWriter interface {
	UpdateRemoteAgentCard(ctx context.Context, id string, card AgentCard) error
}

// FederationAgentEntry is one directory row: the org plus its registered
// remote agent and cached agent card (design F.5 Directory).
type FederationAgentEntry struct {
	Org         FederationOrg
	RemoteAgent RemoteAgent
	Card        AgentCard
}

// Directory aggregates the federation catalog from cached data only
// (FED-NFR4: < 500ms, no live pull): orgs filtered to trusted/neutral +
// active, remote agents grouped by OrgID, optional capability/org filters.
type Directory struct {
	orgs    FederationOrgRepo
	remotes RemoteAgentLister
}

// NewDirectory constructs a Directory.
func NewDirectory(orgs FederationOrgRepo, remotes RemoteAgentLister) *Directory {
	return &Directory{orgs: orgs, remotes: remotes}
}

// ListFederationAgents returns the federated directory. capability filters by
// card capability name; orgID filters to one org (an excluded org — untrusted
// or suspended — yields an empty result).
func (d *Directory) ListFederationAgents(ctx context.Context, capability, orgID string) ([]FederationAgentEntry, error) {
	orgs, err := d.orgs.ListOrgs(ctx)
	if err != nil {
		return nil, err
	}
	eligible := make(map[string]FederationOrg, len(orgs))
	for _, o := range orgs {
		if orgID != "" && o.ID != orgID {
			continue
		}
		if o.TrustLevel == TrustLevelUntrusted || o.Status != OrgStatusActive {
			continue
		}
		eligible[o.ID] = o
	}
	if len(eligible) == 0 {
		return nil, nil
	}
	remotes, err := d.remotes.ListRemoteAgents(ctx, "")
	if err != nil {
		return nil, err
	}
	out := make([]FederationAgentEntry, 0, len(remotes))
	for _, r := range remotes {
		org, ok := eligible[r.OrgID]
		if !ok || !r.Enabled {
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
		if capability != "" && !cardHasCapability(card, capability) {
			continue
		}
		out = append(out, FederationAgentEntry{Org: org, RemoteAgent: r, Card: card})
	}
	return out, nil
}

func cardHasCapability(card AgentCard, capability string) bool {
	for _, c := range card.Capabilities {
		if c.Name == capability {
			return true
		}
	}
	return false
}

// AgentCardSync refreshes cached agent cards of one org's remote agents
// (design F.5): manual trigger this iteration; per-agent failures are logged
// and skipped without aborting the run.
type AgentCardSync struct {
	remotes    RemoteAgentLister
	discoverer RemoteCardDiscoverer
	writer     RemoteAgentCardWriter
	lg         loggateway.Logger
}

// NewAgentCardSync constructs an AgentCardSync.
func NewAgentCardSync(remotes RemoteAgentLister, discoverer RemoteCardDiscoverer, writer RemoteAgentCardWriter, lg loggateway.Logger) *AgentCardSync {
	return &AgentCardSync{remotes: remotes, discoverer: discoverer, writer: writer, lg: lg}
}

// SyncOrgCards re-discovers the cards of all enabled remote agents belonging
// to orgID and persists them. Returns the number of successfully synced
// agents; individual failures are Warn-logged and skipped (FED-F7).
func (s *AgentCardSync) SyncOrgCards(ctx context.Context, orgID string) (int, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return 0, apierror.BadRequest(apierror.DomainA2AFed, "org_id is required")
	}
	remotes, err := s.remotes.ListRemoteAgents(ctx, "")
	if err != nil {
		return 0, err
	}
	synced := 0
	for _, r := range remotes {
		if r.OrgID != orgID || !r.Enabled {
			continue
		}
		card, err := s.discoverer.DiscoverRemoteCard(ctx, RemoteCardDiscoverInput{
			RemoteURL:      r.RemoteURL,
			AuthType:       r.AuthType,
			AuthConfigJSON: r.AuthConfigJSON,
		})
		if err != nil {
			s.warn("federation card sync: discover failed; skipping agent", r.ID, err)
			continue
		}
		if err := s.writer.UpdateRemoteAgentCard(ctx, r.ID, card); err != nil {
			s.warn("federation card sync: persist failed; skipping agent", r.ID, err)
			continue
		}
		synced++
	}
	return synced, nil
}

func (s *AgentCardSync) warn(msg, agentID string, err error) {
	if s.lg == nil {
		return
	}
	s.lg.Warn(msg,
		loggateway.StepID("a2a.fed.card_sync"),
		loggateway.Str("remote_agent_id", agentID),
		loggateway.Err(err),
	)
}
