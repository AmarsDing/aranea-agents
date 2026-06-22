package service_test

import (
	"testing"

	v1 "aranea-agents/api/kratos/a2a/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoA2ACard(t *testing.T) {
	tests := []struct {
		name string
		card biz.A2AAgentCard
		want *v1.A2AAgentCard
	}{
		{
			name: "full_card",
			card: biz.A2AAgentCard{
				AgentID:     "agent-1",
				DisplayName: "Test Agent",
				Workspace:   "ws-1",
				Enabled:     true,
				Capabilities: []biz.A2ACapability{
					{Name: "chat", Description: "Chat capability", InputSchemaJSON: `{"type":"object"}`, OutputSchemaJSON: `{"type":"string"}`},
				},
				UpdatedAt:   "2025-01-01T00:00:00Z",
				Source:      "local",
				EndpointURL: "https://example.com/agent-1",
				RemoteURL:   "",
			},
			want: &v1.A2AAgentCard{
				AgentId:     "agent-1",
				DisplayName: "Test Agent",
				Workspace:   "ws-1",
				Enabled:     true,
				Capabilities: []*v1.A2ACapability{
					{Name: "chat", Description: "Chat capability", InputSchemaJson: `{"type":"object"}`, OutputSchemaJson: `{"type":"string"}`},
				},
				UpdatedAt:   "2025-01-01T00:00:00Z",
				Source:      "local",
				EndpointUrl: "https://example.com/agent-1",
				RemoteUrl:   "",
			},
		},
		{
			name: "empty_capabilities",
			card: biz.A2AAgentCard{
				AgentID:     "agent-2",
				DisplayName: "No Caps",
				Enabled:     false,
			},
			want: &v1.A2AAgentCard{
				AgentId:      "agent-2",
				DisplayName:  "No Caps",
				Enabled:      false,
				Capabilities: []*v1.A2ACapability{},
			},
		},
		{
			name: "remote_card_with_url",
			card: biz.A2AAgentCard{
				AgentID:     "remote-1",
				Source:      "remote",
				RemoteURL:   "https://remote.example.com",
				EndpointURL: "https://remote.example.com/ep",
			},
			want: &v1.A2AAgentCard{
				AgentId:      "remote-1",
				Source:       "remote",
				RemoteUrl:    "https://remote.example.com",
				EndpointUrl:  "https://remote.example.com/ep",
				Capabilities: []*v1.A2ACapability{},
			},
		},
		{
			name: "multiple_capabilities",
			card: biz.A2AAgentCard{
				AgentID: "agent-3",
				Capabilities: []biz.A2ACapability{
					{Name: "summarize", Description: "Summarize text"},
					{Name: "translate", Description: "Translate text", InputSchemaJSON: `{"type":"object"}`},
				},
			},
			want: &v1.A2AAgentCard{
				AgentId: "agent-3",
				Capabilities: []*v1.A2ACapability{
					{Name: "summarize", Description: "Summarize text"},
					{Name: "translate", Description: "Translate text", InputSchemaJson: `{"type":"object"}`},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoA2ACard(tt.card)
			if got.AgentId != tt.want.AgentId {
				t.Errorf("AgentId = %q, want %q", got.AgentId, tt.want.AgentId)
			}
			if got.DisplayName != tt.want.DisplayName {
				t.Errorf("DisplayName = %q, want %q", got.DisplayName, tt.want.DisplayName)
			}
			if got.Workspace != tt.want.Workspace {
				t.Errorf("Workspace = %q, want %q", got.Workspace, tt.want.Workspace)
			}
			if got.Enabled != tt.want.Enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tt.want.Enabled)
			}
			if got.UpdatedAt != tt.want.UpdatedAt {
				t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, tt.want.UpdatedAt)
			}
			if got.Source != tt.want.Source {
				t.Errorf("Source = %q, want %q", got.Source, tt.want.Source)
			}
			if got.EndpointUrl != tt.want.EndpointUrl {
				t.Errorf("EndpointUrl = %q, want %q", got.EndpointUrl, tt.want.EndpointUrl)
			}
			if got.RemoteUrl != tt.want.RemoteUrl {
				t.Errorf("RemoteUrl = %q, want %q", got.RemoteUrl, tt.want.RemoteUrl)
			}
			if len(got.Capabilities) != len(tt.want.Capabilities) {
				t.Fatalf("len(Capabilities) = %d, want %d", len(got.Capabilities), len(tt.want.Capabilities))
			}
			for i, cap := range got.Capabilities {
				if cap.Name != tt.want.Capabilities[i].Name {
					t.Errorf("Capabilities[%d].Name = %q, want %q", i, cap.Name, tt.want.Capabilities[i].Name)
				}
				if cap.Description != tt.want.Capabilities[i].Description {
					t.Errorf("Capabilities[%d].Description = %q, want %q", i, cap.Description, tt.want.Capabilities[i].Description)
				}
				if cap.InputSchemaJson != tt.want.Capabilities[i].InputSchemaJson {
					t.Errorf("Capabilities[%d].InputSchemaJson = %q, want %q", i, cap.InputSchemaJson, tt.want.Capabilities[i].InputSchemaJson)
				}
				if cap.OutputSchemaJson != tt.want.Capabilities[i].OutputSchemaJson {
					t.Errorf("Capabilities[%d].OutputSchemaJson = %q, want %q", i, cap.OutputSchemaJson, tt.want.Capabilities[i].OutputSchemaJson)
				}
			}
		})
	}
}

func TestToProtoRemoteAgent(t *testing.T) {
	tests := []struct {
		name  string
		agent biz.A2ARemoteAgent
		want  *v1.A2ARemoteAgent
	}{
		{
			name: "full_remote_agent",
			agent: biz.A2ARemoteAgent{
				ID:           "ra-1",
				Workspace:    "ws-1",
				DisplayName:  "Remote Agent",
				RemoteURL:    "https://remote.example.com",
				AgentCardURL: "https://remote.example.com/card",
				AuthType:     "bearer",
				Enabled:      true,
				DiscoveredCard: biz.A2AAgentCard{
					AgentID:     "discovered-1",
					DisplayName: "Discovered",
					Capabilities: []biz.A2ACapability{
						{Name: "chat"},
					},
				},
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-02T00:00:00Z",
			},
			want: &v1.A2ARemoteAgent{
				Id:           "ra-1",
				Workspace:    "ws-1",
				DisplayName:  "Remote Agent",
				RemoteUrl:    "https://remote.example.com",
				AgentCardUrl: "https://remote.example.com/card",
				AuthType:     "bearer",
				Enabled:      true,
				DiscoveredCard: &v1.A2AAgentCard{
					AgentId:     "discovered-1",
					DisplayName: "Discovered",
					Capabilities: []*v1.A2ACapability{
						{Name: "chat"},
					},
				},
				CreatedAt: "2025-01-01T00:00:00Z",
				UpdatedAt: "2025-01-02T00:00:00Z",
			},
		},
		{
			name: "minimal_remote_agent",
			agent: biz.A2ARemoteAgent{
				ID: "ra-2",
			},
			want: &v1.A2ARemoteAgent{
				Id:             "ra-2",
				DiscoveredCard: &v1.A2AAgentCard{Capabilities: []*v1.A2ACapability{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoRemoteAgent(tt.agent)
			if got.Id != tt.want.Id {
				t.Errorf("Id = %q, want %q", got.Id, tt.want.Id)
			}
			if got.Workspace != tt.want.Workspace {
				t.Errorf("Workspace = %q, want %q", got.Workspace, tt.want.Workspace)
			}
			if got.DisplayName != tt.want.DisplayName {
				t.Errorf("DisplayName = %q, want %q", got.DisplayName, tt.want.DisplayName)
			}
			if got.RemoteUrl != tt.want.RemoteUrl {
				t.Errorf("RemoteUrl = %q, want %q", got.RemoteUrl, tt.want.RemoteUrl)
			}
			if got.AgentCardUrl != tt.want.AgentCardUrl {
				t.Errorf("AgentCardUrl = %q, want %q", got.AgentCardUrl, tt.want.AgentCardUrl)
			}
			if got.AuthType != tt.want.AuthType {
				t.Errorf("AuthType = %q, want %q", got.AuthType, tt.want.AuthType)
			}
			if got.Enabled != tt.want.Enabled {
				t.Errorf("Enabled = %v, want %v", got.Enabled, tt.want.Enabled)
			}
			if got.CreatedAt != tt.want.CreatedAt {
				t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, tt.want.CreatedAt)
			}
			if got.UpdatedAt != tt.want.UpdatedAt {
				t.Errorf("UpdatedAt = %q, want %q", got.UpdatedAt, tt.want.UpdatedAt)
			}
			if tt.want.DiscoveredCard != nil {
				if got.DiscoveredCard == nil {
					t.Fatal("DiscoveredCard = nil, want non-nil")
				}
				if got.DiscoveredCard.AgentId != tt.want.DiscoveredCard.AgentId {
					t.Errorf("DiscoveredCard.AgentId = %q, want %q", got.DiscoveredCard.AgentId, tt.want.DiscoveredCard.AgentId)
				}
			}
		})
	}
}
