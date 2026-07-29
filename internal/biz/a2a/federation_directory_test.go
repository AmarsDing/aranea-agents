package a2a

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type mockFederationOrgRepo struct {
	listFn   func(context.Context) ([]FederationOrg, error)
	getFn    func(context.Context, string) (FederationOrg, error)
	upsertFn func(context.Context, FederationOrg) (FederationOrg, error)
	trustFn  func(context.Context, string, string) error
	delFn    func(context.Context, string) error
}

func (m *mockFederationOrgRepo) ListOrgs(ctx context.Context) ([]FederationOrg, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return nil, nil
}

func (m *mockFederationOrgRepo) GetOrg(ctx context.Context, id string) (FederationOrg, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return FederationOrg{}, nil
}

func (m *mockFederationOrgRepo) UpsertOrg(ctx context.Context, org FederationOrg) (FederationOrg, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, org)
	}
	return org, nil
}

func (m *mockFederationOrgRepo) UpdateOrgTrust(ctx context.Context, id, trustLevel string) error {
	if m.trustFn != nil {
		return m.trustFn(ctx, id, trustLevel)
	}
	return nil
}

func (m *mockFederationOrgRepo) DeleteOrg(ctx context.Context, id string) error {
	if m.delFn != nil {
		return m.delFn(ctx, id)
	}
	return nil
}

type mockRemoteAgentLister struct {
	listFn func(context.Context, string) ([]RemoteAgent, error)
}

func (m *mockRemoteAgentLister) ListRemoteAgents(ctx context.Context, workspace string) ([]RemoteAgent, error) {
	if m.listFn != nil {
		return m.listFn(ctx, workspace)
	}
	return nil, nil
}

type mockRemoteCardDiscoverer struct {
	discoverFn func(context.Context, RemoteCardDiscoverInput) (AgentCard, error)
}

func (m *mockRemoteCardDiscoverer) DiscoverRemoteCard(ctx context.Context, in RemoteCardDiscoverInput) (AgentCard, error) {
	if m.discoverFn != nil {
		return m.discoverFn(ctx, in)
	}
	return AgentCard{}, nil
}

type mockRemoteAgentCardWriter struct {
	updateFn func(context.Context, string, AgentCard) error
	updated  map[string]AgentCard
}

func (m *mockRemoteAgentCardWriter) UpdateRemoteAgentCard(ctx context.Context, id string, card AgentCard) error {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, card)
	}
	if m.updated == nil {
		m.updated = make(map[string]AgentCard)
	}
	m.updated[id] = card
	return nil
}

func federationOrgsFixture() []FederationOrg {
	return []FederationOrg{
		{ID: "org-a", Name: "Org A", TrustLevel: TrustLevelTrusted, Status: OrgStatusActive},
		{ID: "org-b", Name: "Org B", TrustLevel: TrustLevelNeutral, Status: OrgStatusActive},
		{ID: "org-c", Name: "Org C", TrustLevel: TrustLevelUntrusted, Status: OrgStatusActive},
		{ID: "org-d", Name: "Org D", TrustLevel: TrustLevelTrusted, Status: OrgStatusSuspended},
	}
}

func federationRemotesFixture() []RemoteAgent {
	return []RemoteAgent{
		{
			ID: "ra-1", Workspace: "ws-1", DisplayName: "Agent A1", RemoteURL: "https://a1.example.com",
			Enabled: true, OrgID: "org-a",
			DiscoveredCard: AgentCard{Capabilities: []Capability{{Name: "chat"}, {Name: "search"}}},
		},
		{
			ID: "ra-2", Workspace: "ws-1", DisplayName: "Agent A2", RemoteURL: "https://a2.example.com",
			Enabled: false, OrgID: "org-a", // disabled: excluded from directory
			DiscoveredCard: AgentCard{Capabilities: []Capability{{Name: "chat"}}},
		},
		{
			ID: "ra-3", Workspace: "ws-2", DisplayName: "Agent B1", RemoteURL: "https://b1.example.com",
			Enabled: true, OrgID: "org-b",
			DiscoveredCard: AgentCard{Capabilities: []Capability{{Name: "search"}}},
		},
		{
			ID: "ra-4", Workspace: "ws-1", DisplayName: "Agent C1", RemoteURL: "https://c1.example.com",
			Enabled: true, OrgID: "org-c", // untrusted org: excluded
			DiscoveredCard: AgentCard{Capabilities: []Capability{{Name: "chat"}}},
		},
		{
			ID: "ra-5", Workspace: "ws-1", DisplayName: "Agent D1", RemoteURL: "https://d1.example.com",
			Enabled: true, OrgID: "org-d", // suspended org: excluded
			DiscoveredCard: AgentCard{Capabilities: []Capability{{Name: "chat"}}},
		},
		{
			ID: "ra-6", Workspace: "ws-1", DisplayName: "Agent Solo", RemoteURL: "https://solo.example.com",
			Enabled: true, OrgID: "", // workspace-level, non-federated: excluded
			DiscoveredCard: AgentCard{Capabilities: []Capability{{Name: "chat"}}},
		},
	}
}

func newDirectoryFixture() *Directory {
	orgs := &mockFederationOrgRepo{
		listFn: func(context.Context) ([]FederationOrg, error) { return federationOrgsFixture(), nil },
	}
	remotes := &mockRemoteAgentLister{
		listFn: func(context.Context, string) ([]RemoteAgent, error) { return federationRemotesFixture(), nil },
	}
	return NewDirectory(orgs, remotes)
}

func TestDirectory_ListFederationAgents(t *testing.T) {
	d := newDirectoryFixture()
	entries, err := d.ListFederationAgents(context.Background(), "", "")
	if err != nil {
		t.Fatalf("ListFederationAgents() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (org-a/ra-1 + org-b/ra-3)", len(entries))
	}
	byID := make(map[string]FederationAgentEntry, len(entries))
	for _, e := range entries {
		byID[e.RemoteAgent.ID] = e
	}
	a1, ok := byID["ra-1"]
	if !ok {
		t.Fatal("ra-1 missing from directory")
	}
	if a1.Org.ID != "org-a" {
		t.Errorf("ra-1 org = %q, want org-a", a1.Org.ID)
	}
	if a1.Card.AgentID != "ra-1" {
		t.Errorf("ra-1 card AgentID = %q, want fallback to remote agent id", a1.Card.AgentID)
	}
	if a1.Card.DisplayName != "Agent A1" {
		t.Errorf("ra-1 card DisplayName = %q, want fallback to remote display name", a1.Card.DisplayName)
	}
	if a1.Card.Source != SourceRemote || a1.Card.RemoteURL != "https://a1.example.com" {
		t.Errorf("ra-1 card source/url = (%q, %q), want (remote, https://a1.example.com)", a1.Card.Source, a1.Card.RemoteURL)
	}
	if !a1.Card.Enabled {
		t.Error("ra-1 card Enabled = false, want true")
	}
	if _, ok := byID["ra-3"]; !ok {
		t.Error("ra-3 missing from directory")
	}
}

func TestDirectory_CapabilityFilter(t *testing.T) {
	d := newDirectoryFixture()
	entries, err := d.ListFederationAgents(context.Background(), "search", "")
	if err != nil {
		t.Fatalf("ListFederationAgents() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (ra-1 chat+search, ra-3 search)", len(entries))
	}
	entries, err = d.ListFederationAgents(context.Background(), "chat", "")
	if err != nil {
		t.Fatalf("ListFederationAgents() error: %v", err)
	}
	if len(entries) != 1 || entries[0].RemoteAgent.ID != "ra-1" {
		t.Fatalf("chat filter = %+v, want only ra-1", entries)
	}
}

func TestDirectory_OrgFilter(t *testing.T) {
	d := newDirectoryFixture()
	entries, err := d.ListFederationAgents(context.Background(), "", "org-b")
	if err != nil {
		t.Fatalf("ListFederationAgents() error: %v", err)
	}
	if len(entries) != 1 || entries[0].RemoteAgent.ID != "ra-3" {
		t.Fatalf("org-b filter = %+v, want only ra-3", entries)
	}
	// Filtering by an untrusted org yields nothing (excluded before grouping).
	entries, err = d.ListFederationAgents(context.Background(), "", "org-c")
	if err != nil {
		t.Fatalf("ListFederationAgents() error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("org-c (untrusted) entries = %d, want 0", len(entries))
	}
}

func TestDirectory_RepoErrorsPropagate(t *testing.T) {
	orgs := &mockFederationOrgRepo{
		listFn: func(context.Context) ([]FederationOrg, error) { return nil, errors.New("db down") },
	}
	d := NewDirectory(orgs, &mockRemoteAgentLister{})
	if _, err := d.ListFederationAgents(context.Background(), "", ""); err == nil {
		t.Fatal("ListFederationAgents() expected ListOrgs error, got nil")
	}

	orgsOK := &mockFederationOrgRepo{
		listFn: func(context.Context) ([]FederationOrg, error) { return federationOrgsFixture(), nil },
	}
	remotesErr := &mockRemoteAgentLister{
		listFn: func(context.Context, string) ([]RemoteAgent, error) { return nil, errors.New("db down") },
	}
	d = NewDirectory(orgsOK, remotesErr)
	if _, err := d.ListFederationAgents(context.Background(), "", ""); err == nil {
		t.Fatal("ListFederationAgents() expected ListRemoteAgents error, got nil")
	}
}

// --- AgentCardSync ---

func newSyncFixture() (*AgentCardSync, *mockRemoteAgentCardWriter) {
	remotes := &mockRemoteAgentLister{
		listFn: func(context.Context, string) ([]RemoteAgent, error) { return federationRemotesFixture(), nil },
	}
	discoverer := &mockRemoteCardDiscoverer{
		discoverFn: func(_ context.Context, in RemoteCardDiscoverInput) (AgentCard, error) {
			return AgentCard{
				AgentID:      "discovered-" + in.RemoteURL,
				Capabilities: []Capability{{Name: "chat"}},
			}, nil
		},
	}
	writer := &mockRemoteAgentCardWriter{}
	return NewAgentCardSync(remotes, discoverer, writer, loggateway.NewNoop()), writer
}

func TestAgentCardSync_SyncOrgCards(t *testing.T) {
	s, writer := newSyncFixture()
	// org-a has ra-1 (enabled) + ra-2 (disabled, skipped).
	n, err := s.SyncOrgCards(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("SyncOrgCards() error: %v", err)
	}
	if n != 1 {
		t.Errorf("synced = %d, want 1", n)
	}
	if _, ok := writer.updated["ra-1"]; !ok {
		t.Error("ra-1 card not updated")
	}
	if _, ok := writer.updated["ra-2"]; ok {
		t.Error("ra-2 (disabled) updated, want skipped")
	}
}

func TestAgentCardSync_EmptyOrgID(t *testing.T) {
	s, _ := newSyncFixture()
	_, err := s.SyncOrgCards(context.Background(), "  ")
	if err == nil {
		t.Fatal("SyncOrgCards() expected error for empty orgID, got nil")
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Errorf("error = %v, want code %v", err, apierror.CodeBadRequest)
	}
}

func TestAgentCardSync_PartialFailureSkips(t *testing.T) {
	remotes := &mockRemoteAgentLister{
		listFn: func(context.Context, string) ([]RemoteAgent, error) {
			return []RemoteAgent{
				{ID: "ra-ok", Enabled: true, OrgID: "org-a", RemoteURL: "https://ok.example.com"},
				{ID: "ra-bad-discover", Enabled: true, OrgID: "org-a", RemoteURL: "https://bad.example.com"},
				{ID: "ra-bad-write", Enabled: true, OrgID: "org-a", RemoteURL: "https://badwrite.example.com"},
			}, nil
		},
	}
	discoverer := &mockRemoteCardDiscoverer{
		discoverFn: func(_ context.Context, in RemoteCardDiscoverInput) (AgentCard, error) {
			if in.RemoteURL == "https://bad.example.com" {
				return AgentCard{}, errors.New("unreachable")
			}
			return AgentCard{AgentID: "card"}, nil
		},
	}
	writer := &mockRemoteAgentCardWriter{
		updateFn: func(_ context.Context, id string, card AgentCard) error {
			if id == "ra-bad-write" {
				return errors.New("db down")
			}
			return nil
		},
	}
	s := NewAgentCardSync(remotes, discoverer, writer, loggateway.NewNoop())
	n, err := s.SyncOrgCards(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("SyncOrgCards() error: %v, want nil (partial failures skipped)", err)
	}
	if n != 1 {
		t.Errorf("synced = %d, want 1 (only ra-ok)", n)
	}
}

func TestAgentCardSync_ListErrorPropagates(t *testing.T) {
	remotes := &mockRemoteAgentLister{
		listFn: func(context.Context, string) ([]RemoteAgent, error) { return nil, errors.New("db down") },
	}
	s := NewAgentCardSync(remotes, &mockRemoteCardDiscoverer{}, &mockRemoteAgentCardWriter{}, loggateway.NewNoop())
	if _, err := s.SyncOrgCards(context.Background(), "org-a"); err == nil {
		t.Fatal("SyncOrgCards() expected ListRemoteAgents error, got nil")
	}
}
