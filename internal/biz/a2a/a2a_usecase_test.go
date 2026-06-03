package a2a

import (
	"context"
	"errors"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type mockUsecaseRepo struct {
	upsertAgentCardFn         func(context.Context, AgentCard) (AgentCard, error)
	getAgentCardFn            func(context.Context, string) (AgentCard, error)
	listEnabledCardsFn        func(context.Context, string, string) ([]AgentCard, error)
	createInvocationFn        func(context.Context, Invocation) (Invocation, error)
	updateInvocationFn        func(context.Context, Invocation) error
	insertAuditFn             func(context.Context, AuditEntry) error
	listAuditFn               func(context.Context, string, string, int, int) ([]AuditEntry, int, error)
	mapEndpointEnabledFn      func(context.Context, []string) (map[string]bool, error)
	createRemoteAgentFn       func(context.Context, RemoteAgent) (RemoteAgent, error)
	listRemoteAgentsFn        func(context.Context, string) ([]RemoteAgent, error)
	deleteRemoteAgentFn       func(context.Context, string) error
	getRemoteAgentFn          func(context.Context, string) (RemoteAgent, error)
	discoverRemoteCardFn      func(context.Context, RemoteCardDiscoverInput) (AgentCard, error)
	updateRemoteAgentHealthFn func(context.Context, string, bool, string) error
}

func (m *mockUsecaseRepo) UpsertAgentCard(ctx context.Context, card AgentCard) (AgentCard, error) {
	if m.upsertAgentCardFn != nil {
		return m.upsertAgentCardFn(ctx, card)
	}
	return card, nil
}

func (m *mockUsecaseRepo) GetAgentCard(ctx context.Context, id string) (AgentCard, error) {
	if m.getAgentCardFn != nil {
		return m.getAgentCardFn(ctx, id)
	}
	return AgentCard{}, nil
}

func (m *mockUsecaseRepo) ListEnabledCards(ctx context.Context, workspace, capability string) ([]AgentCard, error) {
	if m.listEnabledCardsFn != nil {
		return m.listEnabledCardsFn(ctx, workspace, capability)
	}
	return nil, nil
}

func (m *mockUsecaseRepo) CreateInvocation(ctx context.Context, inv Invocation) (Invocation, error) {
	if m.createInvocationFn != nil {
		return m.createInvocationFn(ctx, inv)
	}
	return inv, nil
}

func (m *mockUsecaseRepo) UpdateInvocation(ctx context.Context, inv Invocation) error {
	if m.updateInvocationFn != nil {
		return m.updateInvocationFn(ctx, inv)
	}
	return nil
}

func (m *mockUsecaseRepo) InsertAudit(ctx context.Context, entry AuditEntry) error {
	if m.insertAuditFn != nil {
		return m.insertAuditFn(ctx, entry)
	}
	return nil
}

func (m *mockUsecaseRepo) ListAudit(ctx context.Context, callerID, calleeID string, limit, offset int) ([]AuditEntry, int, error) {
	if m.listAuditFn != nil {
		return m.listAuditFn(ctx, callerID, calleeID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockUsecaseRepo) MapEndpointEnabled(ctx context.Context, agentIDs []string) (map[string]bool, error) {
	if m.mapEndpointEnabledFn != nil {
		return m.mapEndpointEnabledFn(ctx, agentIDs)
	}
	return map[string]bool{}, nil
}

func (m *mockUsecaseRepo) CreateRemoteAgent(ctx context.Context, agent RemoteAgent) (RemoteAgent, error) {
	if m.createRemoteAgentFn != nil {
		return m.createRemoteAgentFn(ctx, agent)
	}
	return agent, nil
}

func (m *mockUsecaseRepo) ListRemoteAgents(ctx context.Context, workspace string) ([]RemoteAgent, error) {
	if m.listRemoteAgentsFn != nil {
		return m.listRemoteAgentsFn(ctx, workspace)
	}
	return nil, nil
}

func (m *mockUsecaseRepo) DeleteRemoteAgent(ctx context.Context, id string) error {
	if m.deleteRemoteAgentFn != nil {
		return m.deleteRemoteAgentFn(ctx, id)
	}
	return nil
}

func (m *mockUsecaseRepo) GetRemoteAgent(ctx context.Context, id string) (RemoteAgent, error) {
	if m.getRemoteAgentFn != nil {
		return m.getRemoteAgentFn(ctx, id)
	}
	return RemoteAgent{}, nil
}

func (m *mockUsecaseRepo) DiscoverRemoteCard(ctx context.Context, in RemoteCardDiscoverInput) (AgentCard, error) {
	if m.discoverRemoteCardFn != nil {
		return m.discoverRemoteCardFn(ctx, in)
	}
	return AgentCard{}, nil
}

func (m *mockUsecaseRepo) UpdateRemoteAgentHealth(ctx context.Context, id string, ok bool, errMsg string) error {
	if m.updateRemoteAgentHealthFn != nil {
		return m.updateRemoteAgentHealthFn(ctx, id, ok, errMsg)
	}
	return nil
}

func TestDiscover(t *testing.T) {
	tests := []struct {
		name       string
		workspace  string
		capability string
		setup      func(*mockUsecaseRepo)
		wantErr    bool
		check      func(t *testing.T, cards []AgentCard)
	}{
		{
			name:       "returns_local_and_remote_cards",
			workspace:  "ws-1",
			capability: "",
			setup: func(r *mockUsecaseRepo) {
				r.listEnabledCardsFn = func(_ context.Context, _, _ string) ([]AgentCard, error) {
					return []AgentCard{
						{AgentID: "local-1", DisplayName: "Local Agent 1", Enabled: true},
						{AgentID: "local-2", DisplayName: "Local Agent 2", Enabled: true},
					}, nil
				}
				r.listRemoteAgentsFn = func(_ context.Context, _ string) ([]RemoteAgent, error) {
					return []RemoteAgent{
						{
							ID:          "remote-1",
							Workspace:   "ws-1",
							DisplayName: "Remote Agent 1",
							Enabled:     true,
							RemoteURL:   "https://remote.example.com",
							DiscoveredCard: AgentCard{
								AgentID: "remote-agent-1",
								Capabilities: []Capability{
									{Name: "chat"},
									{Name: "search"},
								},
							},
						},
					}, nil
				}
			},
			check: func(t *testing.T, cards []AgentCard) {
				if len(cards) != 3 {
					t.Fatalf("len(cards) = %d, want 3", len(cards))
				}
				localCount := 0
				remoteCount := 0
				for _, c := range cards {
					if c.Source == SourceLocal {
						localCount++
					}
					if c.Source == SourceRemote {
						remoteCount++
					}
				}
				if localCount != 2 {
					t.Errorf("local cards = %d, want 2", localCount)
				}
				if remoteCount != 1 {
					t.Errorf("remote cards = %d, want 1", remoteCount)
				}
			},
		},
		{
			name:       "capability_filter_works",
			workspace:  "ws-1",
			capability: "search",
			setup: func(r *mockUsecaseRepo) {
				r.listEnabledCardsFn = func(_ context.Context, _, _ string) ([]AgentCard, error) {
					return []AgentCard{
						{AgentID: "local-1", DisplayName: "Local Agent 1", Enabled: true},
					}, nil
				}
				r.listRemoteAgentsFn = func(_ context.Context, _ string) ([]RemoteAgent, error) {
					return []RemoteAgent{
						{
							ID:          "remote-1",
							Workspace:   "ws-1",
							DisplayName: "Remote With Search",
							Enabled:     true,
							RemoteURL:   "https://remote.example.com",
							DiscoveredCard: AgentCard{
								AgentID: "remote-agent-1",
								Capabilities: []Capability{
									{Name: "chat"},
									{Name: "search"},
								},
							},
						},
						{
							ID:          "remote-2",
							Workspace:   "ws-1",
							DisplayName: "Remote Without Search",
							Enabled:     true,
							RemoteURL:   "https://remote2.example.com",
							DiscoveredCard: AgentCard{
								AgentID: "remote-agent-2",
								Capabilities: []Capability{
									{Name: "chat"},
								},
							},
						},
					}, nil
				}
			},
			check: func(t *testing.T, cards []AgentCard) {
				remoteWithSearch := 0
				for _, c := range cards {
					if c.Source == SourceRemote {
						hasSearch := false
						for _, cap := range c.Capabilities {
							if cap.Name == "search" {
								hasSearch = true
							}
						}
						if hasSearch {
							remoteWithSearch++
						} else {
							t.Error("remote card without 'search' capability should not be included")
						}
					}
				}
				if remoteWithSearch != 1 {
					t.Errorf("remote cards with search = %d, want 1", remoteWithSearch)
				}
			},
		},
		{
			name:       "empty_result",
			workspace:  "ws-empty",
			capability: "",
			setup: func(r *mockUsecaseRepo) {
				r.listEnabledCardsFn = func(_ context.Context, _, _ string) ([]AgentCard, error) {
					return nil, nil
				}
				r.listRemoteAgentsFn = func(_ context.Context, _ string) ([]RemoteAgent, error) {
					return nil, nil
				}
			},
			check: func(t *testing.T, cards []AgentCard) {
				if len(cards) != 0 {
					t.Errorf("len(cards) = %d, want 0", len(cards))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			tt.setup(repo)
			uc := NewUsecase(repo, repo, repo, repo)
			cards, err := uc.Discover(context.Background(), tt.workspace, tt.capability)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Discover() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Discover() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cards)
			}
		})
	}
}

func TestGatewayDiscover(t *testing.T) {
	tests := []struct {
		name          string
		input         GatewayDiscoverInput
		publicBaseURL string
		setup         func(*mockUsecaseRepo)
		wantErr       bool
		check         func(t *testing.T, entries []GatewayEntry)
	}{
		{
			name: "aggregates_local_and_remote_entries",
			input: GatewayDiscoverInput{
				Workspace:   "ws-1",
				CheckHealth: false,
			},
			publicBaseURL: "https://gateway.example.com",
			setup: func(r *mockUsecaseRepo) {
				r.listEnabledCardsFn = func(_ context.Context, _, _ string) ([]AgentCard, error) {
					return []AgentCard{
						{AgentID: "local-1", DisplayName: "Local Agent 1", Enabled: true},
					}, nil
				}
				r.mapEndpointEnabledFn = func(_ context.Context, _ []string) (map[string]bool, error) {
					return map[string]bool{"local-1": true}, nil
				}
				r.listRemoteAgentsFn = func(_ context.Context, _ string) ([]RemoteAgent, error) {
					return []RemoteAgent{
						{
							ID:          "remote-1",
							Workspace:   "ws-1",
							DisplayName: "Remote Agent 1",
							Enabled:     true,
							RemoteURL:   "https://remote.example.com",
							DiscoveredCard: AgentCard{
								AgentID: "remote-agent-1",
							},
						},
					}, nil
				}
			},
			check: func(t *testing.T, entries []GatewayEntry) {
				if len(entries) != 2 {
					t.Fatalf("len(entries) = %d, want 2", len(entries))
				}
				localFound := false
				remoteFound := false
				for _, e := range entries {
					if e.Source == SourceLocal {
						localFound = true
						if e.EndpointURL != "https://gateway.example.com/local-1" {
							t.Errorf("local EndpointURL = %q, want %q", e.EndpointURL, "https://gateway.example.com/local-1")
						}
						if !e.Healthy {
							t.Error("local entry should be healthy")
						}
					}
					if e.Source == SourceRemote {
						remoteFound = true
						if e.RemoteURL != "https://remote.example.com" {
							t.Errorf("remote RemoteURL = %q, want %q", e.RemoteURL, "https://remote.example.com")
						}
					}
				}
				if !localFound {
					t.Error("no local entry found")
				}
				if !remoteFound {
					t.Error("no remote entry found")
				}
			},
		},
		{
			name: "empty_workspace",
			input: GatewayDiscoverInput{
				Workspace:   "ws-empty",
				CheckHealth: false,
			},
			publicBaseURL: "https://gateway.example.com",
			setup: func(r *mockUsecaseRepo) {
				r.listEnabledCardsFn = func(_ context.Context, _, _ string) ([]AgentCard, error) {
					return nil, nil
				}
				r.listRemoteAgentsFn = func(_ context.Context, _ string) ([]RemoteAgent, error) {
					return nil, nil
				}
			},
			check: func(t *testing.T, entries []GatewayEntry) {
				if len(entries) != 0 {
					t.Errorf("len(entries) = %d, want 0", len(entries))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			tt.setup(repo)
			uc := NewUsecase(repo, repo, repo, repo)
			entries, err := uc.GatewayDiscover(context.Background(), tt.input, tt.publicBaseURL)
			if tt.wantErr {
				if err == nil {
					t.Fatal("GatewayDiscover() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("GatewayDiscover() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, entries)
			}
		})
	}
}

func TestUpdateAgentCard(t *testing.T) {
	tests := []struct {
		name       string
		card       AgentCard
		setup      func(*mockUsecaseRepo)
		wantErr    bool
		wantReason string
		wantCode   int32
		check      func(t *testing.T, card AgentCard)
	}{
		{
			name: "valid_update",
			card: AgentCard{
				AgentID:     "agent-1",
				DisplayName: "Updated Agent",
				Enabled:     true,
				Capabilities: []Capability{
					{Name: "chat"},
				},
			},
			setup: func(r *mockUsecaseRepo) {
				r.upsertAgentCardFn = func(_ context.Context, card AgentCard) (AgentCard, error) {
					return card, nil
				}
			},
			check: func(t *testing.T, card AgentCard) {
				if card.AgentID != "agent-1" {
					t.Errorf("AgentID = %q, want %q", card.AgentID, "agent-1")
				}
				if card.DisplayName != "Updated Agent" {
					t.Errorf("DisplayName = %q, want %q", card.DisplayName, "Updated Agent")
				}
			},
		},
		{
			name: "empty_agent_id_returns_bad_request",
			card: AgentCard{
				AgentID:     "",
				DisplayName: "No ID Agent",
			},
			wantErr:    true,
			wantReason: "A2A",
			wantCode:   400,
		},
		{
			name: "repo_error_propagation",
			card: AgentCard{
				AgentID:     "agent-1",
				DisplayName: "Agent",
			},
			setup: func(r *mockUsecaseRepo) {
				r.upsertAgentCardFn = func(_ context.Context, _ AgentCard) (AgentCard, error) {
					return AgentCard{}, errors.New("db write failed")
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo, repo, repo, repo)
			card, err := uc.UpdateAgentCard(context.Background(), tt.card)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UpdateAgentCard() expected error, got nil")
				}
				if tt.wantReason != "" {
					se := kerrors.FromError(err)
					if se == nil {
						t.Fatalf("expected kratos error, got %T", err)
					}
					if se.Reason != tt.wantReason {
						t.Errorf("reason = %q, want %q", se.Reason, tt.wantReason)
					}
					if se.Code != tt.wantCode {
						t.Errorf("code = %d, want %d", se.Code, tt.wantCode)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateAgentCard() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, card)
			}
		})
	}
}

func TestMapEndpointEnabled(t *testing.T) {
	tests := []struct {
		name    string
		ids     []string
		setup   func(*mockUsecaseRepo)
		wantErr bool
		check   func(t *testing.T, m map[string]bool)
	}{
		{
			name: "returns_map_of_agent_id_to_enabled",
			ids:  []string{"agent-1", "agent-2", "agent-3"},
			setup: func(r *mockUsecaseRepo) {
				r.mapEndpointEnabledFn = func(_ context.Context, _ []string) (map[string]bool, error) {
					return map[string]bool{
						"agent-1": true,
						"agent-2": false,
						"agent-3": true,
					}, nil
				}
			},
			check: func(t *testing.T, m map[string]bool) {
				if len(m) != 3 {
					t.Fatalf("len(m) = %d, want 3", len(m))
				}
				if !m["agent-1"] {
					t.Error("agent-1 should be enabled")
				}
				if m["agent-2"] {
					t.Error("agent-2 should be disabled")
				}
				if !m["agent-3"] {
					t.Error("agent-3 should be enabled")
				}
			},
		},
		{
			name: "empty_input",
			ids:  []string{},
			setup: func(r *mockUsecaseRepo) {
				r.mapEndpointEnabledFn = func(_ context.Context, _ []string) (map[string]bool, error) {
					return map[string]bool{}, nil
				}
			},
			check: func(t *testing.T, m map[string]bool) {
				if len(m) != 0 {
					t.Errorf("len(m) = %d, want 0", len(m))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			tt.setup(repo)
			uc := NewUsecase(repo, repo, repo, repo)
			m, err := uc.MapEndpointEnabled(context.Background(), tt.ids)
			if tt.wantErr {
				if err == nil {
					t.Fatal("MapEndpointEnabled() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("MapEndpointEnabled() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, m)
			}
		})
	}
}
