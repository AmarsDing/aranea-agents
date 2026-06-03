package a2a

import (
	"context"
	"strings"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestAgentIDsFromCards(t *testing.T) {
	tests := []struct {
		name   string
		cards  []AgentCard
		expect []string
	}{
		{
			name:   "nil_slice",
			cards:  nil,
			expect: []string{},
		},
		{
			name:   "empty_slice",
			cards:  []AgentCard{},
			expect: []string{},
		},
		{
			name: "all_valid_ids",
			cards: []AgentCard{
				{AgentID: "agent-1"},
				{AgentID: "agent-2"},
				{AgentID: "agent-3"},
			},
			expect: []string{"agent-1", "agent-2", "agent-3"},
		},
		{
			name: "empty_ids_filtered",
			cards: []AgentCard{
				{AgentID: ""},
				{AgentID: ""},
			},
			expect: []string{},
		},
		{
			name: "whitespace_only_ids_filtered",
			cards: []AgentCard{
				{AgentID: "  "},
				{AgentID: "\t"},
				{AgentID: "\n"},
			},
			expect: []string{},
		},
		{
			name: "mixed_valid_and_empty",
			cards: []AgentCard{
				{AgentID: "agent-1"},
				{AgentID: ""},
				{AgentID: "agent-2"},
				{AgentID: "  "},
				{AgentID: "agent-3"},
			},
			expect: []string{"agent-1", "agent-2", "agent-3"},
		},
		{
			name: "ids_with_surrounding_whitespace_trimmed",
			cards: []AgentCard{
				{AgentID: "  agent-1  "},
				{AgentID: "\tagent-2\t"},
			},
			expect: []string{"agent-1", "agent-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AgentIDsFromCards(tt.cards)
			if len(got) != len(tt.expect) {
				t.Fatalf("AgentIDsFromCards returned %d ids, want %d; got=%v want=%v", len(got), len(tt.expect), got, tt.expect)
			}
			for i := range got {
				if got[i] != tt.expect[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.expect[i])
				}
			}
		})
	}
}

type mockRepo struct {
	invocation Invocation
	created    bool
}

func (m *mockRepo) UpsertAgentCard(_ context.Context, card AgentCard) (AgentCard, error) {
	return card, nil
}
func (m *mockRepo) GetAgentCard(_ context.Context, _ string) (AgentCard, error) {
	return AgentCard{}, nil
}
func (m *mockRepo) ListEnabledCards(_ context.Context, _, _ string) ([]AgentCard, error) {
	return nil, nil
}
func (m *mockRepo) CreateInvocation(_ context.Context, inv Invocation) (Invocation, error) {
	m.invocation = inv
	m.created = true
	return inv, nil
}
func (m *mockRepo) UpdateInvocation(_ context.Context, _ Invocation) error {
	return nil
}
func (m *mockRepo) InsertAudit(_ context.Context, _ AuditEntry) error {
	return nil
}
func (m *mockRepo) ListAudit(_ context.Context, _, _ string, _, _ int) ([]AuditEntry, int, error) {
	return nil, 0, nil
}
func (m *mockRepo) MapEndpointEnabled(_ context.Context, _ []string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (m *mockRepo) CreateRemoteAgent(_ context.Context, agent RemoteAgent) (RemoteAgent, error) {
	return agent, nil
}
func (m *mockRepo) ListRemoteAgents(_ context.Context, _ string) ([]RemoteAgent, error) {
	return nil, nil
}
func (m *mockRepo) DeleteRemoteAgent(_ context.Context, _ string) error {
	return nil
}
func (m *mockRepo) GetRemoteAgent(_ context.Context, _ string) (RemoteAgent, error) {
	return RemoteAgent{}, nil
}
func (m *mockRepo) DiscoverRemoteCard(_ context.Context, _ RemoteCardDiscoverInput) (AgentCard, error) {
	return AgentCard{}, nil
}
func (m *mockRepo) UpdateRemoteAgentHealth(_ context.Context, _ string, _ bool, _ string) error {
	return nil
}

func TestStartInvocation(t *testing.T) {
	tests := []struct {
		name        string
		inv         Invocation
		wantErr     bool
		wantReason  string
		wantMessage string
		wantCode    int32
	}{
		{
			name:        "empty_callee_agent_id",
			inv:         Invocation{CalleeAgentID: "", Capability: "chat"},
			wantErr:     true,
			wantReason:  "A2A",
			wantMessage: "callee_agent_id is required",
			wantCode:    400,
		},
		{
			name:        "whitespace_callee_agent_id",
			inv:         Invocation{CalleeAgentID: "  ", Capability: "chat"},
			wantErr:     true,
			wantReason:  "A2A",
			wantMessage: "callee_agent_id is required",
			wantCode:    400,
		},
		{
			name:        "empty_capability",
			inv:         Invocation{CalleeAgentID: "agent-2", Capability: ""},
			wantErr:     true,
			wantReason:  "A2A",
			wantMessage: "capability is required",
			wantCode:    400,
		},
		{
			name:        "whitespace_capability",
			inv:         Invocation{CalleeAgentID: "agent-2", Capability: "  "},
			wantErr:     true,
			wantReason:  "A2A",
			wantMessage: "capability is required",
			wantCode:    400,
		},
		{
			name:    "valid_input_fills_defaults",
			inv:     Invocation{CalleeAgentID: "agent-2", Capability: "chat"},
			wantErr: false,
		},
		{
			name:    "valid_input_preserves_existing_values",
			inv:     Invocation{ID: "custom-id", CalleeAgentID: "agent-2", Capability: "chat", PayloadJSON: `{"key":"val"}`, Status: "running", TimeoutSeconds: 60},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepo{}
			uc := NewUsecase(repo, repo, repo, repo)
			result, err := uc.StartInvocation(context.Background(), tt.inv)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("StartInvocation() expected error, got nil")
				}
				se := kerrors.FromError(err)
				if se == nil {
					t.Fatalf("expected kratos error, got %T: %v", err, err)
				}
				if se.Reason != tt.wantReason {
					t.Errorf("reason = %q, want %q", se.Reason, tt.wantReason)
				}
				if se.Message != tt.wantMessage {
					t.Errorf("message = %q, want %q", se.Message, tt.wantMessage)
				}
				if se.Code != tt.wantCode {
					t.Errorf("code = %d, want %d", se.Code, tt.wantCode)
				}
				return
			}

			if err != nil {
				t.Fatalf("StartInvocation() unexpected error: %v", err)
			}
			if !repo.created {
				t.Fatal("repo.CreateInvocation was not called")
			}

			if tt.inv.ID == "" {
				if !strings.HasPrefix(result.ID, "a2a-") {
					t.Errorf("expected auto-generated ID with prefix 'a2a-', got %q", result.ID)
				}
			} else {
				if result.ID != tt.inv.ID {
					t.Errorf("ID = %q, want %q", result.ID, tt.inv.ID)
				}
			}

			if tt.inv.PayloadJSON == "" {
				if result.PayloadJSON != "{}" {
					t.Errorf("PayloadJSON = %q, want default '{}'", result.PayloadJSON)
				}
			} else {
				if result.PayloadJSON != tt.inv.PayloadJSON {
					t.Errorf("PayloadJSON = %q, want %q", result.PayloadJSON, tt.inv.PayloadJSON)
				}
			}

			if tt.inv.Status == "" {
				if result.Status != "pending" {
					t.Errorf("Status = %q, want default 'pending'", result.Status)
				}
			} else {
				if result.Status != tt.inv.Status {
					t.Errorf("Status = %q, want %q", result.Status, tt.inv.Status)
				}
			}

			if tt.inv.TimeoutSeconds <= 0 {
				if result.TimeoutSeconds != 30 {
					t.Errorf("TimeoutSeconds = %d, want default 30", result.TimeoutSeconds)
				}
			} else {
				if result.TimeoutSeconds != tt.inv.TimeoutSeconds {
					t.Errorf("TimeoutSeconds = %d, want %d", result.TimeoutSeconds, tt.inv.TimeoutSeconds)
				}
			}
		})
	}
}
