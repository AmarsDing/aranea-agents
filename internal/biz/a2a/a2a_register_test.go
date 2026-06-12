package a2a

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
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
				se, ok := apierror.From(err)
				if !ok {
					t.Fatalf("expected apierror.Error, got %T", err)
				}
				if se.Domain != "A2A" {
					t.Errorf("domain = %q, want %q", se.Domain, "A2A")
				}
				if se.Code != apierror.CodeInternal {
					t.Errorf("code = %s, want INTERNAL", se.Code)
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
			uc := NewUsecase(repo, repo, repo, repo, nil)
			agent, err := uc.RegisterRemoteAgent(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("RegisterRemoteAgent() expected error, got nil")
				}
				if tt.wantReason != "" {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if se.Domain != tt.wantReason {
						t.Errorf("domain = %q, want %q", se.Domain, tt.wantReason)
					}
					if se.Code != codeFromInt(tt.wantCode) {
						t.Errorf("code = %s, want %d", se.Code, tt.wantCode)
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
					return AgentCard{}, apierror.Forbidden("A2A", "access denied")
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
				se, ok := apierror.From(err)
				if !ok {
					t.Fatalf("expected apierror.Error, got %T", err)
				}
				if se.Domain != "A2A" {
					t.Errorf("domain = %q, want %q", se.Domain, "A2A")
				}
				if se.Code != apierror.CodeInternal {
					t.Errorf("code = %s, want INTERNAL", se.Code)
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
			uc := NewUsecase(repo, repo, repo, repo, nil)
			card, err := uc.DiscoverRemoteAgent(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("DiscoverRemoteAgent() expected error, got nil")
				}
				if tt.wantReason != "" {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if se.Domain != tt.wantReason {
						t.Errorf("domain = %q, want %q", se.Domain, tt.wantReason)
					}
					if se.Code != codeFromInt(tt.wantCode) {
						t.Errorf("code = %s, want %d", se.Code, tt.wantCode)
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
					return AgentCard{}, apierror.Internal("A2A", "db error")
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
			uc := NewUsecase(repo, repo, repo, repo, nil)
			card, err := uc.UpdateAgentCard(context.Background(), tt.card)
			if tt.wantErr {
				if err == nil {
					t.Fatal("UpdateAgentCard() expected error, got nil")
				}
				if tt.wantReason != "" {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if se.Domain != tt.wantReason {
						t.Errorf("domain = %q, want %q", se.Domain, tt.wantReason)
					}
					if se.Code != codeFromInt(tt.wantCode) {
						t.Errorf("code = %s, want %d", se.Code, tt.wantCode)
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

func TestValidateAuthConfig(t *testing.T) {
	tests := []struct {
		name           string
		authType       string
		authConfigJSON string
		wantErr        bool
		wantCode       int32
	}{
		{
			name:     "empty_auth_type_defaults_to_none",
			authType: "",
		},
		{
			name:     "none_auth_type_valid",
			authType: "none",
		},
		{
			name:           "none_auth_type_ignores_config",
			authType:       "none",
			authConfigJSON: `{"token":"abc"}`,
		},
		{
			name:           "bearer_valid",
			authType:       "bearer",
			authConfigJSON: `{"token":"my-secret-token"}`,
		},
		{
			name:           "bearer_missing_token",
			authType:       "bearer",
			authConfigJSON: `{"key":"val"}`,
			wantErr:        true,
			wantCode:       400,
		},
		{
			name:           "bearer_invalid_json",
			authType:       "bearer",
			authConfigJSON: "not-json",
			wantErr:        true,
			wantCode:       400,
		},
		{
			name:           "basic_valid",
			authType:       "basic",
			authConfigJSON: `{"username":"user","password":"pass"}`,
		},
		{
			name:           "basic_missing_username",
			authType:       "basic",
			authConfigJSON: `{"password":"pass"}`,
			wantErr:        true,
			wantCode:       400,
		},
		{
			name:           "basic_missing_password",
			authType:       "basic",
			authConfigJSON: `{"username":"user"}`,
			wantErr:        true,
			wantCode:       400,
		},
		{
			name:           "basic_invalid_json",
			authType:       "basic",
			authConfigJSON: "{bad",
			wantErr:        true,
			wantCode:       400,
		},
		{
			name:           "api_key_valid_with_key",
			authType:       "api_key",
			authConfigJSON: `{"key":"X-API-Key","value":"my-key"}`,
		},
		{
			name:           "api_key_valid_with_header",
			authType:       "api_key",
			authConfigJSON: `{"header":"Authorization","value":"Bearer xyz"}`,
		},
		{
			name:           "api_key_missing_key_and_header",
			authType:       "api_key",
			authConfigJSON: `{"value":"my-key"}`,
			wantErr:        true,
			wantCode:       400,
		},
		{
			name:           "api_key_missing_value",
			authType:       "api_key",
			authConfigJSON: `{"key":"X-API-Key"}`,
			wantErr:        true,
			wantCode:       400,
		},
		{
			name:           "api_key_invalid_json",
			authType:       "api_key",
			authConfigJSON: "not-json",
			wantErr:        true,
			wantCode:       400,
		},
		{
			name:     "invalid_auth_type",
			authType: "oauth2",
			wantErr:  true,
			wantCode: 400,
		},
		{
			name:     "whitespace_auth_type_defaults_to_none",
			authType: "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAuthConfig(tt.authType, tt.authConfigJSON)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ValidateAuthConfig() expected error, got nil")
				}
				if tt.wantCode != 0 {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if se.Code != codeFromInt(tt.wantCode) {
						t.Errorf("code = %s, want %d", se.Code, tt.wantCode)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateAuthConfig() unexpected error: %v", err)
			}
		})
	}
}

func TestRegisterRemoteAgent_InvalidAuth(t *testing.T) {
	tests := []struct {
		name     string
		input    RegisterRemoteAgentInput
		setup    func(*mockUsecaseRepo)
		wantErr  bool
		wantCode int32
	}{
		{
			name: "invalid_auth_type_returns_error",
			input: RegisterRemoteAgentInput{
				Workspace:      "ws-1",
				RemoteURL:      "https://remote.example.com",
				AuthType:       "oauth2",
				AuthConfigJSON: `{"token":"abc"}`,
			},
			wantErr:  true,
			wantCode: 400,
		},
		{
			name: "bearer_without_token_returns_error",
			input: RegisterRemoteAgentInput{
				Workspace:      "ws-1",
				RemoteURL:      "https://remote.example.com",
				AuthType:       "bearer",
				AuthConfigJSON: `{"key":"val"}`,
			},
			wantErr:  true,
			wantCode: 400,
		},
		{
			name: "basic_without_username_returns_error",
			input: RegisterRemoteAgentInput{
				Workspace:      "ws-1",
				RemoteURL:      "https://remote.example.com",
				AuthType:       "basic",
				AuthConfigJSON: `{"password":"pass"}`,
			},
			wantErr:  true,
			wantCode: 400,
		},
		{
			name: "basic_without_password_returns_error",
			input: RegisterRemoteAgentInput{
				Workspace:      "ws-1",
				RemoteURL:      "https://remote.example.com",
				AuthType:       "basic",
				AuthConfigJSON: `{"username":"user"}`,
			},
			wantErr:  true,
			wantCode: 400,
		},
		{
			name: "empty_auth_type_valid_as_none",
			input: RegisterRemoteAgentInput{
				Workspace: "ws-1",
				RemoteURL: "https://remote.example.com",
				Enabled:   true,
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{AgentID: "remote-agent-1", DisplayName: "Agent"}, nil
				}
				r.createRemoteAgentFn = func(_ context.Context, agent RemoteAgent) (RemoteAgent, error) {
					return agent, nil
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
			uc := NewUsecase(repo, repo, repo, repo, nil)
			_, err := uc.RegisterRemoteAgent(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("RegisterRemoteAgent() expected error, got nil")
				}
				if tt.wantCode != 0 {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if se.Code != codeFromInt(tt.wantCode) {
						t.Errorf("code = %s, want %d", se.Code, tt.wantCode)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("RegisterRemoteAgent() unexpected error: %v", err)
			}
		})
	}
}

func TestDiscoverRemoteAgent_InvalidAuth(t *testing.T) {
	tests := []struct {
		name     string
		input    RemoteCardDiscoverInput
		setup    func(*mockUsecaseRepo)
		wantErr  bool
		wantCode int32
	}{
		{
			name: "invalid_auth_type_returns_error",
			input: RemoteCardDiscoverInput{
				RemoteURL:      "https://remote.example.com",
				AuthType:       "unknown",
				AuthConfigJSON: `{}`,
			},
			wantErr:  true,
			wantCode: 400,
		},
		{
			name: "bearer_without_token_returns_error",
			input: RemoteCardDiscoverInput{
				RemoteURL:      "https://remote.example.com",
				AuthType:       "bearer",
				AuthConfigJSON: `{}`,
			},
			wantErr:  true,
			wantCode: 400,
		},
		{
			name: "empty_auth_type_valid_as_none",
			input: RemoteCardDiscoverInput{
				RemoteURL: "https://remote.example.com",
			},
			setup: func(r *mockUsecaseRepo) {
				r.discoverRemoteCardFn = func(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
					return AgentCard{AgentID: "discovered-1"}, nil
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
			uc := NewUsecase(repo, repo, repo, repo, nil)
			_, err := uc.DiscoverRemoteAgent(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("DiscoverRemoteAgent() expected error, got nil")
				}
				if tt.wantCode != 0 {
					se, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if se.Code != codeFromInt(tt.wantCode) {
						t.Errorf("code = %s, want %d", se.Code, tt.wantCode)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("DiscoverRemoteAgent() unexpected error: %v", err)
			}
		})
	}
}
