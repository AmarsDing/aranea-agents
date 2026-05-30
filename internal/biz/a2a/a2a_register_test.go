package a2a

import (
	"context"
	"errors"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestRegisterRemoteAgent(t *testing.T) {
	tests := []struct {
		name       string
		input      RegisterRemoteAgentInput
		setup      func(*mockUsecaseRepo)
		wantErr    bool
		wantReason string
		wantCode   int32
		check      func(t *testing.T, agent RemoteAgent)
	}{
		{
			name: "valid_registration",
			input: RegisterRemoteAgentInput{
				Workspace:      "ws-1",
				RemoteURL:      "https://remote.example.com",
				AgentCardURL:   "https://remote.example.com/card",
				DisplayName:    "My Remote Agent",
				AuthType:       "bearer",
				AuthConfigJSON: `{"token":"abc"}`,
				Enabled:        true,
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{
						AgentID:     "remote-agent-1",
						DisplayName: "Discovered Agent",
						Workspace:   "ws-discovered",
						Capabilities: []Capability{
							{Name: "chat"},
						},
					}, nil
				}
				r.createRemoteAgentFn = func(_ context.Context, agent RemoteAgent) (RemoteAgent, error) {
					return agent, nil
				}
			},
			check: func(t *testing.T, agent RemoteAgent) {
				if agent.Workspace != "ws-1" {
					t.Errorf("Workspace = %q, want %q", agent.Workspace, "ws-1")
				}
				if agent.DisplayName != "My Remote Agent" {
					t.Errorf("DisplayName = %q, want %q", agent.DisplayName, "My Remote Agent")
				}
				if agent.RemoteURL != "https://remote.example.com" {
					t.Errorf("RemoteURL = %q, want %q", agent.RemoteURL, "https://remote.example.com")
				}
				if agent.AgentCardURL != "https://remote.example.com/card" {
					t.Errorf("AgentCardURL = %q, want %q", agent.AgentCardURL, "https://remote.example.com/card")
				}
				if agent.AuthType != "bearer" {
					t.Errorf("AuthType = %q, want %q", agent.AuthType, "bearer")
				}
				if !agent.Enabled {
					t.Error("Enabled = false, want true")
				}
				if agent.DiscoveredCard.AgentID != "remote-agent-1" {
					t.Errorf("DiscoveredCard.AgentID = %q, want %q", agent.DiscoveredCard.AgentID, "remote-agent-1")
				}
			},
		},
		{
			name: "empty_remote_url_returns_error",
			input: RegisterRemoteAgentInput{
				Workspace:   "ws-1",
				RemoteURL:   "",
				DisplayName: "Agent",
			},
			wantErr:    true,
			wantReason: "A2A",
			wantCode:   400,
		},
		{
			name: "whitespace_remote_url_returns_error",
			input: RegisterRemoteAgentInput{
				RemoteURL: "   ",
			},
			wantErr:    true,
			wantReason: "A2A",
			wantCode:   400,
		},
		{
			name: "empty_display_name_falls_back_to_card",
			input: RegisterRemoteAgentInput{
				Workspace: "ws-1",
				RemoteURL: "https://remote.example.com",
				Enabled:   true,
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{
						AgentID:     "remote-agent-1",
						DisplayName: "Fallback Name",
						Workspace:   "ws-fallback",
					}, nil
				}
				r.createRemoteAgentFn = func(_ context.Context, agent RemoteAgent) (RemoteAgent, error) {
					return agent, nil
				}
			},
			check: func(t *testing.T, agent RemoteAgent) {
				if agent.DisplayName != "Fallback Name" {
					t.Errorf("DisplayName = %q, want fallback %q", agent.DisplayName, "Fallback Name")
				}
			},
		},
		{
			name: "empty_workspace_falls_back_to_card",
			input: RegisterRemoteAgentInput{
				RemoteURL: "https://remote.example.com",
				Enabled:   true,
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{
						AgentID:     "remote-agent-1",
						DisplayName: "Agent",
						Workspace:   "ws-from-card",
					}, nil
				}
				r.createRemoteAgentFn = func(_ context.Context, agent RemoteAgent) (RemoteAgent, error) {
					return agent, nil
				}
			},
			check: func(t *testing.T, agent RemoteAgent) {
				if agent.Workspace != "ws-from-card" {
					t.Errorf("Workspace = %q, want fallback %q", agent.Workspace, "ws-from-card")
				}
			},
		},
		{
			name: "empty_agent_card_url_falls_back_to_remote_url",
			input: RegisterRemoteAgentInput{
				Workspace:   "ws-1",
				RemoteURL:   "https://remote.example.com",
				DisplayName: "Agent",
				Enabled:     true,
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{AgentID: "remote-agent-1"}, nil
				}
				r.createRemoteAgentFn = func(_ context.Context, agent RemoteAgent) (RemoteAgent, error) {
					return agent, nil
				}
			},
			check: func(t *testing.T, agent RemoteAgent) {
				if agent.AgentCardURL != "https://remote.example.com" {
					t.Errorf("AgentCardURL = %q, want fallback %q", agent.AgentCardURL, "https://remote.example.com")
				}
			},
		},
		{
			name: "discover_remote_card_error_propagation",
			input: RegisterRemoteAgentInput{
				Workspace:   "ws-1",
				RemoteURL:   "https://remote.example.com",
				DisplayName: "Agent",
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{}, errors.New("connection refused")
				}
			},
			wantErr: true,
		},
		{
			name: "create_remote_agent_repo_error",
			input: RegisterRemoteAgentInput{
				Workspace:   "ws-1",
				RemoteURL:   "https://remote.example.com",
				DisplayName: "Agent",
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{AgentID: "remote-agent-1"}, nil
				}
				r.createRemoteAgentFn = func(_ context.Context, _ RemoteAgent) (RemoteAgent, error) {
					return RemoteAgent{}, errors.New("db write failed")
				}
			},
			wantErr: true,
		},
		{
			name: "nil_usecase_returns_internal_server",
			input: RegisterRemoteAgentInput{
				RemoteURL: "https://remote.example.com",
			},
			setup: func(r *mockUsecaseRepo) {},
			check: func(t *testing.T, _ RemoteAgent) {
				var uc *Usecase
				_, err := uc.RegisterRemoteAgent(context.Background(), RegisterRemoteAgentInput{
					RemoteURL: "https://remote.example.com",
				})
				if err == nil {
					t.Fatal("nil Usecase expected error, got nil")
				}
				se := kerrors.FromError(err)
				if se.Reason != "A2A" {
					t.Errorf("reason = %q, want %q", se.Reason, "A2A")
				}
				if se.Code != 500 {
					t.Errorf("code = %d, want %d", se.Code, 500)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo)
			agent, err := uc.RegisterRemoteAgent(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("RegisterRemoteAgent() expected error, got nil")
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
				t.Fatalf("RegisterRemoteAgent() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, agent)
			}
		})
	}
}

func TestDiscoverRemoteAgent(t *testing.T) {
	tests := []struct {
		name       string
		input      RemoteCardDiscoverInput
		setup      func(*mockUsecaseRepo)
		wantErr    bool
		wantReason string
		wantCode   int32
		check      func(t *testing.T, card AgentCard)
	}{
		{
			name: "valid_discovery",
			input: RemoteCardDiscoverInput{
				RemoteURL:      "https://remote.example.com/.well-known/agent.json",
				AuthType:       "bearer",
				AuthConfigJSON: `{"token":"xyz"}`,
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, in RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{
						AgentID:     "discovered-1",
						DisplayName: "Discovered Agent",
						Workspace:   "ws-remote",
						Capabilities: []Capability{
							{Name: "chat"},
							{Name: "search"},
						},
					}, nil
				}
			},
			check: func(t *testing.T, card AgentCard) {
				if card.AgentID != "discovered-1" {
					t.Errorf("AgentID = %q, want %q", card.AgentID, "discovered-1")
				}
				if card.DisplayName != "Discovered Agent" {
					t.Errorf("DisplayName = %q, want %q", card.DisplayName, "Discovered Agent")
				}
				if len(card.Capabilities) != 2 {
					t.Errorf("len(Capabilities) = %d, want 2", len(card.Capabilities))
				}
			},
		},
		{
			name: "empty_remote_url_returns_error",
			input: RemoteCardDiscoverInput{
				RemoteURL: "",
			},
			wantErr:    true,
			wantReason: "A2A",
			wantCode:   400,
		},
		{
			name: "whitespace_remote_url_returns_error",
			input: RemoteCardDiscoverInput{
				RemoteURL: "   ",
			},
			wantErr:    true,
			wantReason: "A2A",
			wantCode:   400,
		},
		{
			name: "http_error_propagation",
			input: RemoteCardDiscoverInput{
				RemoteURL: "https://unreachable.example.com",
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{}, errors.New("connection refused")
				}
			},
			wantErr: true,
		},
		{
			name: "kratos_error_propagation",
			input: RemoteCardDiscoverInput{
				RemoteURL: "https://forbidden.example.com",
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{}, kerrors.Forbidden("A2A", "access denied")
				}
			},
			wantErr:    true,
			wantReason: "A2A",
			wantCode:   403,
		},
		{
			name: "nil_usecase_returns_internal_server",
			input: RemoteCardDiscoverInput{
				RemoteURL: "https://remote.example.com",
			},
			check: func(t *testing.T, _ AgentCard) {
				var uc *Usecase
				_, err := uc.DiscoverRemoteAgent(context.Background(), RemoteCardDiscoverInput{
					RemoteURL: "https://remote.example.com",
				})
				if err == nil {
					t.Fatal("nil Usecase expected error, got nil")
				}
				se := kerrors.FromError(err)
				if se.Reason != "A2A" {
					t.Errorf("reason = %q, want %q", se.Reason, "A2A")
				}
				if se.Code != 500 {
					t.Errorf("code = %d, want %d", se.Code, 500)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo)
			card, err := uc.DiscoverRemoteAgent(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("DiscoverRemoteAgent() expected error, got nil")
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
				t.Fatalf("DiscoverRemoteAgent() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, card)
			}
		})
	}
}

func TestUpdateAgentCard_EdgeCases(t *testing.T) {
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
			name: "whitespace_only_agent_id_returns_error",
			card: AgentCard{
				AgentID:     "   ",
				DisplayName: "Whitespace ID Agent",
			},
			wantErr:    true,
			wantReason: "A2A",
			wantCode:   400,
		},
		{
			name: "update_with_capabilities_preserved",
			card: AgentCard{
				AgentID:     "agent-1",
				DisplayName: "Agent With Caps",
				Enabled:     true,
				Capabilities: []Capability{
					{Name: "chat", Description: "Chat capability"},
					{Name: "search", Description: "Search capability", InputSchemaJSON: `{"type":"object"}`},
				},
			},
			setup: func(r *mockUsecaseRepo) {
				r.upsertAgentCardFn = func(_ context.Context, card AgentCard) (AgentCard, error) {
					return card, nil
				}
			},
			check: func(t *testing.T, card AgentCard) {
				if len(card.Capabilities) != 2 {
					t.Fatalf("len(Capabilities) = %d, want 2", len(card.Capabilities))
				}
				if card.Capabilities[0].Name != "chat" {
					t.Errorf("Capabilities[0].Name = %q, want %q", card.Capabilities[0].Name, "chat")
				}
				if card.Capabilities[1].InputSchemaJSON != `{"type":"object"}` {
					t.Errorf("Capabilities[1].InputSchemaJSON = %q, want %q", card.Capabilities[1].InputSchemaJSON, `{"type":"object"}`)
				}
			},
		},
		{
			name: "update_with_empty_capabilities",
			card: AgentCard{
				AgentID:     "agent-2",
				DisplayName: "No Caps Agent",
				Enabled:     true,
			},
			setup: func(r *mockUsecaseRepo) {
				r.upsertAgentCardFn = func(_ context.Context, card AgentCard) (AgentCard, error) {
					return card, nil
				}
			},
			check: func(t *testing.T, card AgentCard) {
				if card.Capabilities != nil {
					t.Errorf("Capabilities should be nil, got %v", card.Capabilities)
				}
			},
		},
		{
			name: "repo_upsert_error",
			card: AgentCard{
				AgentID:     "agent-1",
				DisplayName: "Agent",
			},
			setup: func(r *mockUsecaseRepo) {
				r.upsertAgentCardFn = func(_ context.Context, _ AgentCard) (AgentCard, error) {
					return AgentCard{}, kerrors.InternalServer("A2A", "db error")
				}
			},
			wantErr:    true,
			wantReason: "A2A",
			wantCode:   500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockUsecaseRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(repo)
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
