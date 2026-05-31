package tool

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type mockRepo struct {
	searchTools                   func(ctx context.Context, q ToolListQuery) (ToolListResult, error)
	getTool                       func(ctx context.Context, idOrKey string) (Tool, error)
	createTool                    func(ctx context.Context, in ToolUpsertInput) (Tool, error)
	updateTool                    func(ctx context.Context, idOrKey string, in ToolUpsertInput) (Tool, error)
	deleteTool                    func(ctx context.Context, idOrKey string) error
	updateToolEnabled             func(ctx context.Context, idOrKey string, enabled bool) (Tool, error)
	updateToolConfig              func(ctx context.Context, idOrKey string, configJSON string) (Tool, error)
	searchToolInvocations         func(ctx context.Context, q ToolRunQuery) (ToolRunResult, error)
	getToolInvocationParams       func(ctx context.Context, invocationID string) (ToolInvocationParam, error)
	recordToolInvocation          func(ctx context.Context, in ToolInvocationWrite) error
	recordToolInvocationAudit     func(ctx context.Context, in ToolInvocationAuditWrite) error
	searchToolInvocationAudits    func(ctx context.Context, q ToolAuditQuery) (ToolAuditResult, error)
	purgeToolInvocationAuditsBefore func(ctx context.Context, cutoffRFC3339 string) (int64, error)
	listToolAgentOverrides        func(ctx context.Context, toolKey string) ([]ToolAgentOverride, error)
	listToolAgentOverridesByAgent func(ctx context.Context, agentID string) ([]ToolAgentOverride, error)
	upsertToolAgentOverride       func(ctx context.Context, in ToolAgentOverrideInput, toolID string) (ToolAgentOverride, error)
	deleteToolAgentOverride       func(ctx context.Context, toolKey string, agentID string) error
	syncBuiltinTools              func(ctx context.Context) error
}

func (m *mockRepo) SearchTools(ctx context.Context, q ToolListQuery) (ToolListResult, error) {
	if m.searchTools != nil {
		return m.searchTools(ctx, q)
	}
	return ToolListResult{}, nil
}

func (m *mockRepo) GetTool(ctx context.Context, idOrKey string) (Tool, error) {
	if m.getTool != nil {
		return m.getTool(ctx, idOrKey)
	}
	return Tool{}, nil
}

func (m *mockRepo) CreateTool(ctx context.Context, in ToolUpsertInput) (Tool, error) {
	if m.createTool != nil {
		return m.createTool(ctx, in)
	}
	return Tool{}, nil
}

func (m *mockRepo) UpdateTool(ctx context.Context, idOrKey string, in ToolUpsertInput) (Tool, error) {
	if m.updateTool != nil {
		return m.updateTool(ctx, idOrKey, in)
	}
	return Tool{}, nil
}

func (m *mockRepo) DeleteTool(ctx context.Context, idOrKey string) error {
	if m.deleteTool != nil {
		return m.deleteTool(ctx, idOrKey)
	}
	return nil
}

func (m *mockRepo) UpdateToolEnabled(ctx context.Context, idOrKey string, enabled bool) (Tool, error) {
	if m.updateToolEnabled != nil {
		return m.updateToolEnabled(ctx, idOrKey, enabled)
	}
	return Tool{}, nil
}

func (m *mockRepo) UpdateToolConfig(ctx context.Context, idOrKey string, configJSON string) (Tool, error) {
	if m.updateToolConfig != nil {
		return m.updateToolConfig(ctx, idOrKey, configJSON)
	}
	return Tool{}, nil
}

func (m *mockRepo) SearchToolInvocations(ctx context.Context, q ToolRunQuery) (ToolRunResult, error) {
	if m.searchToolInvocations != nil {
		return m.searchToolInvocations(ctx, q)
	}
	return ToolRunResult{}, nil
}

func (m *mockRepo) GetToolInvocationParams(ctx context.Context, invocationID string) (ToolInvocationParam, error) {
	if m.getToolInvocationParams != nil {
		return m.getToolInvocationParams(ctx, invocationID)
	}
	return ToolInvocationParam{}, nil
}

func (m *mockRepo) RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error {
	if m.recordToolInvocation != nil {
		return m.recordToolInvocation(ctx, in)
	}
	return nil
}

func (m *mockRepo) RecordToolInvocationAudit(ctx context.Context, in ToolInvocationAuditWrite) error {
	if m.recordToolInvocationAudit != nil {
		return m.recordToolInvocationAudit(ctx, in)
	}
	return nil
}

func (m *mockRepo) SearchToolInvocationAudits(ctx context.Context, q ToolAuditQuery) (ToolAuditResult, error) {
	if m.searchToolInvocationAudits != nil {
		return m.searchToolInvocationAudits(ctx, q)
	}
	return ToolAuditResult{}, nil
}

func (m *mockRepo) PurgeToolInvocationAuditsBefore(ctx context.Context, cutoffRFC3339 string) (int64, error) {
	if m.purgeToolInvocationAuditsBefore != nil {
		return m.purgeToolInvocationAuditsBefore(ctx, cutoffRFC3339)
	}
	return 0, nil
}

func (m *mockRepo) ListToolAgentOverrides(ctx context.Context, toolKey string) ([]ToolAgentOverride, error) {
	if m.listToolAgentOverrides != nil {
		return m.listToolAgentOverrides(ctx, toolKey)
	}
	return nil, nil
}

func (m *mockRepo) ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]ToolAgentOverride, error) {
	if m.listToolAgentOverridesByAgent != nil {
		return m.listToolAgentOverridesByAgent(ctx, agentID)
	}
	return nil, nil
}

func (m *mockRepo) UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput, toolID string) (ToolAgentOverride, error) {
	if m.upsertToolAgentOverride != nil {
		return m.upsertToolAgentOverride(ctx, in, toolID)
	}
	return ToolAgentOverride{}, nil
}

func (m *mockRepo) DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error {
	if m.deleteToolAgentOverride != nil {
		return m.deleteToolAgentOverride(ctx, toolKey, agentID)
	}
	return nil
}

func (m *mockRepo) SyncBuiltinTools(ctx context.Context) error {
	if m.syncBuiltinTools != nil {
		return m.syncBuiltinTools(ctx)
	}
	return nil
}

func assertBadRequest(t *testing.T, err error, wantReason, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	ke := kerrors.FromError(err)
	if ke.Code != 400 {
		t.Fatalf("expected code 400, got %d", ke.Code)
	}
	if ke.Reason != wantReason {
		t.Fatalf("expected reason %q, got %q", wantReason, ke.Reason)
	}
	if wantMsg != "" && ke.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, ke.Message)
	}
}

func TestCreate(t *testing.T) {
	ctx := context.Background()

	validInput := ToolUpsertInput{
		Key:         "test_tool",
		DisplayName: "Test Tool",
		RiskLevel:   "low",
		Source:      "custom",
	}

	tests := []struct {
		name    string
		input   ToolUpsertInput
		repo    *mockRepo
		wantErr bool
		wantMsg string
	}{
		{
			name:  "valid creation",
			input: validInput,
			repo: &mockRepo{
				createTool: func(_ context.Context, in ToolUpsertInput) (Tool, error) {
					return Tool{ID: "t1", Key: in.Key, DisplayName: in.DisplayName}, nil
				},
			},
		},
		{
			name: "missing key",
			input: ToolUpsertInput{
				DisplayName: "Test Tool",
			},
			repo:    &mockRepo{},
			wantErr: true,
			wantMsg: "tool key is required",
		},
		{
			name: "missing display name",
			input: ToolUpsertInput{
				Key: "test_tool",
			},
			repo:    &mockRepo{},
			wantErr: true,
			wantMsg: "display name is required",
		},
		{
			name:  "repo error",
			input: validInput,
			repo: &mockRepo{
				createTool: func(_ context.Context, _ ToolUpsertInput) (Tool, error) {
					return Tool{}, kerrors.InternalServer("TOOL", "db write failed")
				},
			},
			wantErr: true,
			wantMsg: "db write failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.Create(ctx, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ke := kerrors.FromError(err)
				if tt.wantMsg != "" && ke.Message != tt.wantMsg {
					t.Fatalf("expected message %q, got %q", tt.wantMsg, ke.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Key != tt.input.Key {
				t.Fatalf("expected key %q, got %q", tt.input.Key, got.Key)
			}
			if got.DisplayName != tt.input.DisplayName {
				t.Fatalf("expected display_name %q, got %q", tt.input.DisplayName, got.DisplayName)
			}
		})
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()

	validInput := ToolUpsertInput{
		Key:         "shell_exec",
		DisplayName: "Shell Exec",
		RiskLevel:   "medium",
		Source:      "builtin",
	}

	tests := []struct {
		name    string
		id      string
		input   ToolUpsertInput
		repo    *mockRepo
		wantErr bool
		wantMsg string
	}{
		{
			name:  "valid update",
			id:    "tool_shell_exec",
			input: validInput,
			repo: &mockRepo{
				getTool: func(_ context.Context, idOrKey string) (Tool, error) {
					return Tool{ID: idOrKey, Key: "shell_exec", Source: "builtin", Readonly: false}, nil
				},
				updateTool: func(_ context.Context, idOrKey string, in ToolUpsertInput) (Tool, error) {
					return Tool{ID: idOrKey, Key: in.Key, DisplayName: in.DisplayName}, nil
				},
			},
		},
		{
			name: "readonly tool key cannot change",
			id:   "tool_shell_exec",
			input: ToolUpsertInput{
				Key:         "changed_key",
				DisplayName: "Shell Exec",
				Source:      "builtin",
			},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_shell_exec", Key: "shell_exec", Source: "builtin", Readonly: true}, nil
				},
			},
			wantErr: true,
			wantMsg: "readonly tool key cannot change",
		},
		{
			name:  "repo error on GetTool",
			id:    "tool_missing",
			input: validInput,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, kerrors.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
		},
		{
			name: "empty id",
			id:   "  ",
			input: ToolUpsertInput{
				Key:         "test_tool",
				DisplayName: "Test",
			},
			repo:    &mockRepo{},
			wantErr: true,
			wantMsg: "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.Update(ctx, tt.id, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ke := kerrors.FromError(err)
				if tt.wantMsg != "" && ke.Message != tt.wantMsg {
					t.Fatalf("expected message %q, got %q", tt.wantMsg, ke.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Key != tt.input.Key {
				t.Fatalf("expected key %q, got %q", tt.input.Key, got.Key)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		id      string
		repo    *mockRepo
		wantErr bool
		wantMsg string
	}{
		{
			name: "valid delete",
			id:   "tool_custom_1",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_custom_1", Key: "custom_tool", Readonly: false}, nil
				},
				deleteTool: func(_ context.Context, idOrKey string) error {
					return nil
				},
			},
		},
		{
			name: "readonly tool cannot be deleted",
			id:   "tool_shell_exec",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_shell_exec", Key: "shell_exec", Readonly: true}, nil
				},
			},
			wantErr: true,
			wantMsg: "readonly tool cannot be deleted",
		},
		{
			name: "repo error on GetTool",
			id:   "tool_missing",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, kerrors.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
		},
		{
			name:    "empty id",
			id:      "  ",
			repo:    &mockRepo{},
			wantErr: true,
			wantMsg: "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			err := uc.Delete(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ke := kerrors.FromError(err)
				if tt.wantMsg != "" && ke.Message != tt.wantMsg {
					t.Fatalf("expected message %q, got %q", tt.wantMsg, ke.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestToggleEnabled(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		id         string
		enabled    bool
		confirmKey []string
		repo       *mockRepo
		wantErr    bool
		wantMsg    string
	}{
		{
			name:    "toggle on low risk",
			id:      "tool_read_file",
			enabled: true,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_read_file", Key: "read_file", RiskLevel: "low"}, nil
				},
				updateToolEnabled: func(_ context.Context, idOrKey string, enabled bool) (Tool, error) {
					return Tool{ID: idOrKey, Key: "read_file", Enabled: enabled}, nil
				},
			},
		},
		{
			name:    "toggle off",
			id:      "tool_read_file",
			enabled: false,
			repo: &mockRepo{
				updateToolEnabled: func(_ context.Context, idOrKey string, enabled bool) (Tool, error) {
					return Tool{ID: idOrKey, Key: "read_file", Enabled: enabled}, nil
				},
			},
		},
		{
			name:    "not found on toggle on",
			id:      "tool_missing",
			enabled: true,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, kerrors.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
		},
		{
			name:       "high risk toggle on without confirm key",
			id:         "tool_shell_exec",
			enabled:    true,
			confirmKey: nil,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_shell_exec", Key: "shell_exec", RiskLevel: "high"}, nil
				},
			},
			wantErr: true,
			wantMsg: "confirm_key is required and must match tool key for high/critical risk tools",
		},
		{
			name:       "high risk toggle on with wrong confirm key",
			id:         "tool_shell_exec",
			enabled:    true,
			confirmKey: []string{"wrong_key"},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_shell_exec", Key: "shell_exec", RiskLevel: "high"}, nil
				},
			},
			wantErr: true,
			wantMsg: "confirm_key is required and must match tool key for high/critical risk tools",
		},
		{
			name:       "high risk toggle on with correct confirm key",
			id:         "tool_shell_exec",
			enabled:    true,
			confirmKey: []string{"shell_exec"},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_shell_exec", Key: "shell_exec", RiskLevel: "high"}, nil
				},
				updateToolEnabled: func(_ context.Context, idOrKey string, enabled bool) (Tool, error) {
					return Tool{ID: idOrKey, Key: "shell_exec", Enabled: enabled}, nil
				},
			},
		},
		{
			name:       "critical risk toggle on with correct confirm key",
			id:         "tool_critical",
			enabled:    true,
			confirmKey: []string{"critical_tool"},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_critical", Key: "critical_tool", RiskLevel: "critical"}, nil
				},
				updateToolEnabled: func(_ context.Context, idOrKey string, enabled bool) (Tool, error) {
					return Tool{ID: idOrKey, Key: "critical_tool", Enabled: enabled}, nil
				},
			},
		},
		{
			name:    "empty id",
			id:      "  ",
			enabled: true,
			repo:    &mockRepo{},
			wantErr: true,
			wantMsg: "id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.ToggleEnabled(ctx, tt.id, tt.enabled, tt.confirmKey...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ke := kerrors.FromError(err)
				if tt.wantMsg != "" && ke.Message != tt.wantMsg {
					t.Fatalf("expected message %q, got %q", tt.wantMsg, ke.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Enabled != tt.enabled {
				t.Fatalf("expected enabled %v, got %v", tt.enabled, got.Enabled)
			}
		})
	}
}

func TestListTools(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		query     ToolListQuery
		repo      *mockRepo
		wantCount int
		wantTotal int
		wantErr   bool
	}{
		{
			name:  "returns tools from repo",
			query: ToolListQuery{Limit: 10, Offset: 0},
			repo: &mockRepo{
				searchTools: func(_ context.Context, _ ToolListQuery) (ToolListResult, error) {
					return ToolListResult{
						Items: []Tool{
							{ID: "t1", Key: "read_file", DisplayName: "Read File", Enabled: true, Source: "builtin"},
							{ID: "t2", Key: "shell_exec", DisplayName: "Shell Exec", Enabled: false, Source: "builtin"},
						},
						Total: 2,
					}, nil
				},
			},
			wantCount: 2,
			wantTotal: 2,
		},
		{
			name:  "empty list",
			query: ToolListQuery{Limit: 10, Offset: 0},
			repo: &mockRepo{
				searchTools: func(_ context.Context, _ ToolListQuery) (ToolListResult, error) {
					return ToolListResult{Items: []Tool{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:  "repo error",
			query: ToolListQuery{Limit: 10},
			repo: &mockRepo{
				searchTools: func(_ context.Context, _ ToolListQuery) (ToolListResult, error) {
					return ToolListResult{}, kerrors.InternalServer("TOOL", "db error")
				},
			},
			wantErr: true,
		},
		{
			name:  "default limit when zero",
			query: ToolListQuery{Limit: 0},
			repo: &mockRepo{
				searchTools: func(_ context.Context, q ToolListQuery) (ToolListResult, error) {
					if q.Limit != 20 {
						t.Fatalf("expected default limit 20, got %d", q.Limit)
					}
					return ToolListResult{Items: []Tool{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:  "clamp limit over 100",
			query: ToolListQuery{Limit: 200},
			repo: &mockRepo{
				searchTools: func(_ context.Context, q ToolListQuery) (ToolListResult, error) {
					if q.Limit != 100 {
						t.Fatalf("expected clamped limit 100, got %d", q.Limit)
					}
					return ToolListResult{Items: []Tool{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.ListTools(ctx, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.Items) != tt.wantCount {
				t.Fatalf("expected %d items, got %d", tt.wantCount, len(got.Items))
			}
			if got.Total != tt.wantTotal {
				t.Fatalf("expected total %d, got %d", tt.wantTotal, got.Total)
			}
		})
	}
}

func TestGetTool(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		id       string
		repo     *mockRepo
		wantKey  string
		wantErr  bool
		wantMsg  string
		wantCode int
	}{
		{
			name: "returns tool",
			id:   "tool_read_file",
			repo: &mockRepo{
				getTool: func(_ context.Context, idOrKey string) (Tool, error) {
					return Tool{ID: idOrKey, Key: "read_file", DisplayName: "Read File"}, nil
				},
			},
			wantKey: "read_file",
		},
		{
			name: "not found",
			id:   "tool_missing",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, kerrors.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
			wantCode: 404,
		},
		{
			name:    "empty id",
			id:      "  ",
			repo:    &mockRepo{},
			wantErr: true,
			wantMsg: "id is required",
			wantCode: 400,
		},
		{
			name: "resolve by key",
			id:   "read_file",
			repo: &mockRepo{
				getTool: func(_ context.Context, idOrKey string) (Tool, error) {
					return Tool{ID: "tool_read_file", Key: idOrKey, DisplayName: "Read File"}, nil
				},
			},
			wantKey: "read_file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.GetTool(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ke := kerrors.FromError(err)
				if tt.wantCode != 0 && ke.Code != int32(tt.wantCode) {
					t.Fatalf("expected code %d, got %d", tt.wantCode, ke.Code)
				}
				if tt.wantMsg != "" && ke.Message != tt.wantMsg {
					t.Fatalf("expected message %q, got %q", tt.wantMsg, ke.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Key != tt.wantKey {
				t.Fatalf("expected key %q, got %q", tt.wantKey, got.Key)
			}
		})
	}
}
