package tool

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type mockSettingRepo struct {
	getWebResearch func(ctx context.Context) (WebResearchSetting, error)
}

func (m *mockSettingRepo) GetWebResearch(ctx context.Context) (WebResearchSetting, error) {
	if m.getWebResearch != nil {
		return m.getWebResearch(ctx)
	}
	return WebResearchSetting{}, nil
}

type mockToolTester struct {
	execute func(ctx context.Context, tool ToolTestInput, argumentsJSON string, timeoutSec int, platform *WebResearchPlatformFields) (ToolTestResult, error)
}

func (m *mockToolTester) Execute(ctx context.Context, tool ToolTestInput, argumentsJSON string, timeoutSec int, platform *WebResearchPlatformFields) (ToolTestResult, error) {
	if m.execute != nil {
		return m.execute(ctx, tool, argumentsJSON, timeoutSec, platform)
	}
	return ToolTestResult{}, nil
}

func TestUpdateToolConfig(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		id         string
		configJSON string
		repo       *mockRepo
		wantErr    bool
		wantMsg    string
	}{
		{
			name:       "empty id",
			id:         "  ",
			configJSON: `{"key":"val"}`,
			repo:       &mockRepo{},
			wantErr:    true,
			wantMsg:    "id is required",
		},
		{
			name:       "empty config defaults to empty object",
			id:         "tool_1",
			configJSON: "",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				updateToolConfig: func(_ context.Context, _ string, configJSON string) (Tool, error) {
					if configJSON != "{}" {
						t.Fatalf("expected default config '{}', got %q", configJSON)
					}
					return Tool{ID: "tool_1", Key: "test_tool", ConfigJSON: configJSON}, nil
				},
			},
		},
		{
			name:       "GetTool error",
			id:         "tool_missing",
			configJSON: `{"key":"val"}`,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, apierror.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
		},
		{
			name:       "schema validation error",
			id:         "tool_1",
			configJSON: `{"age":1}`,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{
						ID:               "tool_1",
						Key:              "test_tool",
						ConfigSchemaJSON: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
					}, nil
				},
			},
			wantErr: true,
		},
		{
			name:       "successful update",
			id:         "tool_1",
			configJSON: `{"transport":"stdio","command":"echo"}`,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				updateToolConfig: func(_ context.Context, idOrKey string, configJSON string) (Tool, error) {
					return Tool{ID: idOrKey, Key: "test_tool", ConfigJSON: configJSON}, nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.UpdateToolConfig(ctx, tt.id, tt.configJSON)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ConfigJSON != `{"transport":"stdio","command":"echo"}` && tt.name == "successful update" {
				t.Fatalf("expected config echoed back, got %q", got.ConfigJSON)
			}
		})
	}
}

func TestUpsertToolAgentOverride(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		input   ToolAgentOverrideInput
		repo    *mockRepo
		wantErr bool
		wantMsg string
	}{
		{
			name: "empty agent id",
			input: ToolAgentOverrideInput{
				ToolKey: "test_tool",
				AgentID: "  ",
			},
			repo:    &mockRepo{},
			wantErr: true,
			wantMsg: "agent id is required",
		},
		{
			name: "GetTool error for empty tool key",
			input: ToolAgentOverrideInput{
				ToolKey: "",
				AgentID: "agent_1",
			},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, apierror.BadRequest("TOOL", "id is required")
				},
			},
			wantErr: true,
			wantMsg: "id is required",
		},
		{
			name: "GetTool not found",
			input: ToolAgentOverrideInput{
				ToolKey: "missing_tool",
				AgentID: "agent_1",
			},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, apierror.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
		},
		{
			name: "default mode and config override",
			input: ToolAgentOverrideInput{
				ToolKey: "test_tool",
				AgentID: "agent_1",
			},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				upsertToolAgentOverride: func(_ context.Context, in ToolAgentOverrideInput, toolID string) (ToolAgentOverride, error) {
					if in.Mode != "inherit" {
						t.Fatalf("expected default mode 'inherit', got %q", in.Mode)
					}
					if in.ConfigOverrideJSON != "{}" {
						t.Fatalf("expected default config '{}', got %q", in.ConfigOverrideJSON)
					}
					if in.ToolKey != "test_tool" {
						t.Fatalf("expected tool key 'test_tool', got %q", in.ToolKey)
					}
					if toolID != "tool_1" {
						t.Fatalf("expected tool id 'tool_1', got %q", toolID)
					}
					return ToolAgentOverride{ID: "ov_1", ToolID: toolID, AgentID: in.AgentID}, nil
				},
			},
		},
		{
			name: "explicit mode and config preserved",
			input: ToolAgentOverrideInput{
				ToolKey:            "test_tool",
				AgentID:            "agent_1",
				Mode:               "override",
				ConfigOverrideJSON: `{"timeout":30}`,
			},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				upsertToolAgentOverride: func(_ context.Context, in ToolAgentOverrideInput, toolID string) (ToolAgentOverride, error) {
					if in.Mode != "override" {
						t.Fatalf("expected mode 'override', got %q", in.Mode)
					}
					if in.ConfigOverrideJSON != `{"timeout":30}` {
						t.Fatalf("expected config '{\"timeout\":30}', got %q", in.ConfigOverrideJSON)
					}
					return ToolAgentOverride{ID: "ov_2", ToolID: toolID, AgentID: in.AgentID, Mode: in.Mode}, nil
				},
			},
		},
		{
			name: "repo error",
			input: ToolAgentOverrideInput{
				ToolKey: "test_tool",
				AgentID: "agent_1",
			},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				upsertToolAgentOverride: func(_ context.Context, _ ToolAgentOverrideInput, _ string) (ToolAgentOverride, error) {
					return ToolAgentOverride{}, apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
			wantMsg: "db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			_, err := uc.UpsertToolAgentOverride(ctx, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDeleteToolAgentOverride(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		toolIDOrKey string
		agentID     string
		repo        *mockRepo
		wantErr     bool
		wantMsg     string
	}{
		{
			name:        "empty agent id",
			toolIDOrKey: "test_tool",
			agentID:     "  ",
			repo:        &mockRepo{},
			wantErr:     true,
			wantMsg:     "agent id is required",
		},
		{
			name:        "ResolveToolKey error",
			toolIDOrKey: "missing",
			agentID:     "agent_1",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, apierror.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
		},
		{
			name:        "successful delete",
			toolIDOrKey: "test_tool",
			agentID:     "agent_1",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				deleteToolAgentOverride: func(_ context.Context, toolKey string, agentID string) error {
					if toolKey != "test_tool" {
						t.Fatalf("expected tool key 'test_tool', got %q", toolKey)
					}
					if agentID != "agent_1" {
						t.Fatalf("expected agent id 'agent_1', got %q", agentID)
					}
					return nil
				},
			},
		},
		{
			name:        "repo error",
			toolIDOrKey: "test_tool",
			agentID:     "agent_1",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				deleteToolAgentOverride: func(_ context.Context, _ string, _ string) error {
					return apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
			wantMsg: "db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			err := uc.DeleteToolAgentOverride(ctx, tt.toolIDOrKey, tt.agentID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestListToolAgentOverrides(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		toolIDOrKey string
		repo        *mockRepo
		wantCount   int
		wantErr     bool
		wantMsg     string
	}{
		{
			name:        "ResolveToolKey error",
			toolIDOrKey: "missing",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, apierror.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
		},
		{
			name:        "successful list",
			toolIDOrKey: "test_tool",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				listToolAgentOverrides: func(_ context.Context, toolKey string) ([]ToolAgentOverride, error) {
					if toolKey != "test_tool" {
						t.Fatalf("expected tool key 'test_tool', got %q", toolKey)
					}
					return []ToolAgentOverride{
						{ID: "ov_1", ToolKey: "test_tool", AgentID: "agent_1"},
						{ID: "ov_2", ToolKey: "test_tool", AgentID: "agent_2"},
					}, nil
				},
			},
			wantCount: 2,
		},
		{
			name:        "repo error",
			toolIDOrKey: "test_tool",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				listToolAgentOverrides: func(_ context.Context, _ string) ([]ToolAgentOverride, error) {
					return nil, apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
			wantMsg: "db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.ListToolAgentOverrides(ctx, tt.toolIDOrKey)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("expected %d overrides, got %d", tt.wantCount, len(got))
			}
		})
	}
}

func TestListToolAgentOverridesByAgent(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		agentID   string
		repo      *mockRepo
		wantCount int
		wantErr   bool
		wantMsg   string
	}{
		{
			name:    "empty agent id",
			agentID: "  ",
			repo:    &mockRepo{},
			wantErr: true,
			wantMsg: "agent id is required",
		},
		{
			name:    "successful list",
			agentID: "agent_1",
			repo: &mockRepo{
				listToolAgentOverridesByAgent: func(_ context.Context, agentID string) ([]ToolAgentOverride, error) {
					if agentID != "agent_1" {
						t.Fatalf("expected agent id 'agent_1', got %q", agentID)
					}
					return []ToolAgentOverride{
						{ID: "ov_1", ToolKey: "test_tool", AgentID: "agent_1"},
					}, nil
				},
			},
			wantCount: 1,
		},
		{
			name:    "repo error",
			agentID: "agent_1",
			repo: &mockRepo{
				listToolAgentOverridesByAgent: func(_ context.Context, _ string) ([]ToolAgentOverride, error) {
					return nil, apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
			wantMsg: "db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.ListToolAgentOverridesByAgent(ctx, tt.agentID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantCount {
				t.Fatalf("expected %d overrides, got %d", tt.wantCount, len(got))
			}
		})
	}
}

func TestRequiresConfirmationForAgent_NilUsecase(t *testing.T) {
	var uc *ToolUsecase
	if uc.RequiresConfirmationForAgent(context.Background(), "agent_1", "test_tool") != false {
		t.Fatal("expected false for nil usecase")
	}
}

func TestRequiresConfirmationForAgent(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		agentID    string
		toolKey    string
		repo       *mockRepo
		wantResult bool
	}{
		{
			name:       "empty agent id",
			agentID:    "  ",
			toolKey:    "test_tool",
			repo:       &mockRepo{},
			wantResult: false,
		},
		{
			name:       "empty tool key",
			agentID:    "agent_1",
			toolKey:    "  ",
			repo:       &mockRepo{},
			wantResult: false,
		},
		{
			name:    "GetTool error returns false",
			agentID: "agent_1",
			toolKey: "missing_tool",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, apierror.NotFound("TOOL", "tool not found")
				},
			},
			wantResult: false,
		},
		{
			name:    "tool requires confirmation no override",
			agentID: "agent_1",
			toolKey: "shell_exec",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "t1", Key: "shell_exec", RequiresConfirmation: true}, nil
				},
				listToolAgentOverridesByAgent: func(_ context.Context, _ string) ([]ToolAgentOverride, error) {
					return nil, nil
				},
			},
			wantResult: true,
		},
		{
			name:    "tool does not require confirmation no override",
			agentID: "agent_1",
			toolKey: "read_file",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "t1", Key: "read_file", RequiresConfirmation: false}, nil
				},
				listToolAgentOverridesByAgent: func(_ context.Context, _ string) ([]ToolAgentOverride, error) {
					return nil, nil
				},
			},
			wantResult: false,
		},
		{
			name:    "override requires confirmation true",
			agentID: "agent_1",
			toolKey: "read_file",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "t1", Key: "read_file", RequiresConfirmation: false}, nil
				},
				listToolAgentOverridesByAgent: func(_ context.Context, _ string) ([]ToolAgentOverride, error) {
					return []ToolAgentOverride{
						{ToolKey: "read_file", AgentID: "agent_1", RequiresConfirmation: true},
					}, nil
				},
			},
			wantResult: true,
		},
		{
			name:    "override requires confirmation false but tool default true still wins",
			agentID: "agent_1",
			toolKey: "shell_exec",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "t1", Key: "shell_exec", RequiresConfirmation: true}, nil
				},
				listToolAgentOverridesByAgent: func(_ context.Context, _ string) ([]ToolAgentOverride, error) {
					return []ToolAgentOverride{
						{ToolKey: "shell_exec", AgentID: "agent_1", RequiresConfirmation: false},
					}, nil
				},
			},
			wantResult: true,
		},
		{
			name:    "override for different tool key not matched",
			agentID: "agent_1",
			toolKey: "read_file",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "t1", Key: "read_file", RequiresConfirmation: false}, nil
				},
				listToolAgentOverridesByAgent: func(_ context.Context, _ string) ([]ToolAgentOverride, error) {
					return []ToolAgentOverride{
						{ToolKey: "shell_exec", AgentID: "agent_1", RequiresConfirmation: true},
					}, nil
				},
			},
			wantResult: false,
		},
		{
			name:    "ListToolAgentOverridesByAgent error falls through",
			agentID: "agent_1",
			toolKey: "shell_exec",
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "t1", Key: "shell_exec", RequiresConfirmation: true}, nil
				},
				listToolAgentOverridesByAgent: func(_ context.Context, _ string) ([]ToolAgentOverride, error) {
					return nil, apierror.Internal("TOOL", "db error")
				},
			},
			wantResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got := uc.RequiresConfirmationForAgent(ctx, tt.agentID, tt.toolKey)
			if got != tt.wantResult {
				t.Fatalf("expected %v, got %v", tt.wantResult, got)
			}
		})
	}
}

func TestListRuns(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		query     ToolRunQuery
		repo      *mockRepo
		wantCount int
		wantTotal int
		wantErr   bool
	}{
		{
			name:  "default limit when zero",
			query: ToolRunQuery{Limit: 0},
			repo: &mockRepo{
				searchToolInvocations: func(_ context.Context, q ToolRunQuery) (ToolRunResult, error) {
					if q.Limit != 20 {
						t.Fatalf("expected default limit 20, got %d", q.Limit)
					}
					return ToolRunResult{Items: []ToolInvocation{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:  "clamp limit over 100",
			query: ToolRunQuery{Limit: 200},
			repo: &mockRepo{
				searchToolInvocations: func(_ context.Context, q ToolRunQuery) (ToolRunResult, error) {
					if q.Limit != 100 {
						t.Fatalf("expected clamped limit 100, got %d", q.Limit)
					}
					return ToolRunResult{Items: []ToolInvocation{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:  "negative offset clamped to zero",
			query: ToolRunQuery{Limit: 10, Offset: -5},
			repo: &mockRepo{
				searchToolInvocations: func(_ context.Context, q ToolRunQuery) (ToolRunResult, error) {
					if q.Offset != 0 {
						t.Fatalf("expected offset 0, got %d", q.Offset)
					}
					return ToolRunResult{Items: []ToolInvocation{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:  "successful list",
			query: ToolRunQuery{Limit: 10, Offset: 0},
			repo: &mockRepo{
				searchToolInvocations: func(_ context.Context, _ ToolRunQuery) (ToolRunResult, error) {
					return ToolRunResult{
						Items: []ToolInvocation{
							{ID: "inv_1", ToolKey: "shell_exec", Status: "success"},
							{ID: "inv_2", ToolKey: "read_file", Status: "error"},
						},
						Total: 2,
					}, nil
				},
			},
			wantCount: 2,
			wantTotal: 2,
		},
		{
			name:  "repo error",
			query: ToolRunQuery{Limit: 10},
			repo: &mockRepo{
				searchToolInvocations: func(_ context.Context, _ ToolRunQuery) (ToolRunResult, error) {
					return ToolRunResult{}, apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.ListRuns(ctx, tt.query)
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

func TestListRunsForTool(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		toolIDOrKey string
		query       ToolRunQuery
		repo        *mockRepo
		wantCount   int
		wantErr     bool
		wantMsg     string
	}{
		{
			name:        "ResolveToolKey error",
			toolIDOrKey: "missing",
			query:       ToolRunQuery{Limit: 10},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, apierror.NotFound("TOOL", "tool not found")
				},
			},
			wantErr: true,
			wantMsg: "tool not found",
		},
		{
			name:        "successful list with tool key set",
			toolIDOrKey: "test_tool",
			query:       ToolRunQuery{Limit: 10},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				searchToolInvocations: func(_ context.Context, q ToolRunQuery) (ToolRunResult, error) {
					if q.ToolKey != "test_tool" {
						t.Fatalf("expected tool key 'test_tool', got %q", q.ToolKey)
					}
					return ToolRunResult{
						Items: []ToolInvocation{
							{ID: "inv_1", ToolKey: "test_tool"},
						},
						Total: 1,
					}, nil
				},
			},
			wantCount: 1,
		},
		{
			name:        "default limit applied",
			toolIDOrKey: "test_tool",
			query:       ToolRunQuery{Limit: 0},
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "test_tool"}, nil
				},
				searchToolInvocations: func(_ context.Context, q ToolRunQuery) (ToolRunResult, error) {
					if q.Limit != 20 {
						t.Fatalf("expected default limit 20, got %d", q.Limit)
					}
					return ToolRunResult{Items: []ToolInvocation{}, Total: 0}, nil
				},
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.ListRunsForTool(ctx, tt.toolIDOrKey, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got.Items) != tt.wantCount {
				t.Fatalf("expected %d items, got %d", tt.wantCount, len(got.Items))
			}
		})
	}
}

func TestRecordToolInvocationAudit(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		input   ToolInvocationAuditWrite
		repo    *mockRepo
		wantErr bool
	}{
		{
			name: "successful insert",
			input: ToolInvocationAuditWrite{
				InvocationID: "inv_1",
				ToolKey:      "shell_exec",
				AgentID:      "agent_1",
				Action:       "execute",
				Status:       "success",
			},
			repo: &mockRepo{
				recordToolInvocationAudit: func(_ context.Context, in ToolInvocationAuditWrite) error {
					if in.InvocationID != "inv_1" {
						t.Fatalf("expected invocation id 'inv_1', got %q", in.InvocationID)
					}
					return nil
				},
			},
		},
		{
			name: "repo error",
			input: ToolInvocationAuditWrite{
				InvocationID: "inv_1",
				ToolKey:      "shell_exec",
			},
			repo: &mockRepo{
				recordToolInvocationAudit: func(_ context.Context, _ ToolInvocationAuditWrite) error {
					return apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			err := uc.RecordToolInvocationAudit(ctx, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestListInvocationAudits(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		query     ToolAuditQuery
		repo      *mockRepo
		wantCount int
		wantTotal int
		wantErr   bool
	}{
		{
			name:  "default limit when zero",
			query: ToolAuditQuery{Limit: 0},
			repo: &mockRepo{
				searchToolInvocationAudits: func(_ context.Context, q ToolAuditQuery) (ToolAuditResult, error) {
					if q.Limit != 20 {
						t.Fatalf("expected default limit 20, got %d", q.Limit)
					}
					return ToolAuditResult{Items: []ToolInvocationAudit{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:  "clamp limit over 100",
			query: ToolAuditQuery{Limit: 200},
			repo: &mockRepo{
				searchToolInvocationAudits: func(_ context.Context, q ToolAuditQuery) (ToolAuditResult, error) {
					if q.Limit != 100 {
						t.Fatalf("expected clamped limit 100, got %d", q.Limit)
					}
					return ToolAuditResult{Items: []ToolInvocationAudit{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:  "negative offset clamped to zero",
			query: ToolAuditQuery{Limit: 10, Offset: -5},
			repo: &mockRepo{
				searchToolInvocationAudits: func(_ context.Context, q ToolAuditQuery) (ToolAuditResult, error) {
					if q.Offset != 0 {
						t.Fatalf("expected offset 0, got %d", q.Offset)
					}
					return ToolAuditResult{Items: []ToolInvocationAudit{}, Total: 0}, nil
				},
			},
			wantCount: 0,
			wantTotal: 0,
		},
		{
			name:  "successful list",
			query: ToolAuditQuery{Limit: 10},
			repo: &mockRepo{
				searchToolInvocationAudits: func(_ context.Context, _ ToolAuditQuery) (ToolAuditResult, error) {
					return ToolAuditResult{
						Items: []ToolInvocationAudit{
							{ID: "a1", ToolKey: "shell_exec", Action: "execute"},
						},
						Total: 1,
					}, nil
				},
			},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:  "repo error",
			query: ToolAuditQuery{Limit: 10},
			repo: &mockRepo{
				searchToolInvocationAudits: func(_ context.Context, _ ToolAuditQuery) (ToolAuditResult, error) {
					return ToolAuditResult{}, apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.ListInvocationAudits(ctx, tt.query)
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

func TestPurgeOldInvocationAudits(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		repo      *mockRepo
		wantCount int64
		wantErr   bool
		wantMsg   string
	}{
		{
			name: "successful purge with correct cutoff",
			repo: &mockRepo{
				purgeToolInvocationAuditsBefore: func(_ context.Context, cutoffRFC3339 string) (int64, error) {
					cutoff, err := time.Parse(time.RFC3339, cutoffRFC3339)
					if err != nil {
						t.Fatalf("invalid cutoff format: %v", err)
					}
					expected := time.Now().UTC().AddDate(0, 0, -ToolAuditRetentionDays)
					delta := cutoff.Sub(expected)
					if delta < 0 {
						delta = -delta
					}
					if delta > time.Minute {
						t.Fatalf("cutoff %v too far from expected %v", cutoff, expected)
					}
					return 42, nil
				},
			},
			wantCount: 42,
		},
		{
			name: "repo error",
			repo: &mockRepo{
				purgeToolInvocationAuditsBefore: func(_ context.Context, _ string) (int64, error) {
					return 0, apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
			wantMsg: "db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.PurgeOldInvocationAudits(ctx)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantCount {
				t.Fatalf("expected %d, got %d", tt.wantCount, got)
			}
		})
	}
}

func TestGetToolInvocationParams(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		invocationID string
		repo         *mockRepo
		wantKey      string
		wantErr      bool
		wantMsg      string
	}{
		{
			name:         "empty invocation id",
			invocationID: "  ",
			repo:         &mockRepo{},
			wantErr:      true,
			wantMsg:      "invocation id is required",
		},
		{
			name:         "successful get",
			invocationID: "inv_1",
			repo: &mockRepo{
				getToolInvocationParams: func(_ context.Context, id string) (ToolInvocationParam, error) {
					if id != "inv_1" {
						t.Fatalf("expected id 'inv_1', got %q", id)
					}
					return ToolInvocationParam{
						ID:           "param_1",
						InvocationID: "inv_1",
						ToolKey:      "shell_exec",
						ParamsJSON:   `{"cmd":"ls"}`,
					}, nil
				},
			},
			wantKey: "shell_exec",
		},
		{
			name:         "repo error",
			invocationID: "inv_missing",
			repo: &mockRepo{
				getToolInvocationParams: func(_ context.Context, _ string) (ToolInvocationParam, error) {
					return ToolInvocationParam{}, apierror.NotFound("TOOL", "invocation not found")
				},
			},
			wantErr: true,
			wantMsg: "invocation not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			got, err := uc.GetToolInvocationParams(ctx, tt.invocationID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ToolKey != tt.wantKey {
				t.Fatalf("expected tool key %q, got %q", tt.wantKey, got.ToolKey)
			}
		})
	}
}

func TestRecordToolInvocation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		input   ToolInvocationWrite
		repo    *mockRepo
		wantErr bool
	}{
		{
			name: "sanitizes and records",
			input: ToolInvocationWrite{
				ToolKey:       "shell_exec",
				InputPreview:  "user email is test@example.com",
				OutputPreview: "result with password=secret123",
				ErrorMessage:  "",
			},
			repo: &mockRepo{
				recordToolInvocation: func(_ context.Context, in ToolInvocationWrite) error {
					if strings.Contains(in.InputPreview, "test@example.com") {
						t.Fatal("expected email to be redacted in input preview")
					}
					if !strings.Contains(in.InputPreview, "[email redacted]") {
						t.Fatal("expected email redaction marker in input preview")
					}
					if strings.Contains(in.OutputPreview, "secret123") {
						t.Fatal("expected secret to be redacted in output preview")
					}
					if !strings.Contains(in.OutputPreview, "[secret redacted]") {
						t.Fatal("expected secret redaction marker in output preview")
					}
					return nil
				},
			},
		},
		{
			name: "repo error",
			input: ToolInvocationWrite{
				ToolKey: "shell_exec",
			},
			repo: &mockRepo{
				recordToolInvocation: func(_ context.Context, _ ToolInvocationWrite) error {
					return apierror.Internal("TOOL", "db error")
				},
			},
			wantErr: true,
		},
		{
			name: "empty input passes through",
			input: ToolInvocationWrite{
				ToolKey:       "read_file",
				InputPreview:  "",
				OutputPreview: "",
			},
			repo: &mockRepo{
				recordToolInvocation: func(_ context.Context, in ToolInvocationWrite) error {
					if in.ToolKey != "read_file" {
						t.Fatalf("expected tool key 'read_file', got %q", in.ToolKey)
					}
					return nil
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			err := uc.RecordToolInvocation(ctx, tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSyncBuiltinTools(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		repo    *mockRepo
		wantErr bool
		wantMsg string
	}{
		{
			name: "successful sync",
			repo: &mockRepo{
				syncBuiltinTools: func(_ context.Context) error {
					return nil
				},
			},
		},
		{
			name: "repo error",
			repo: &mockRepo{
				syncBuiltinTools: func(_ context.Context) error {
					return apierror.Internal("TOOL", "sync failed")
				},
			},
			wantErr: true,
			wantMsg: "sync failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop())
			err := uc.SyncBuiltinTools(ctx)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					ae, ok := apierror.From(err)
					if !ok {
						t.Fatalf("expected apierror.Error, got %T", err)
					}
					if ae.Message != tt.wantMsg {
						t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTestTool(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		toolID        string
		argumentsJSON string
		timeoutSec    int
		repo          *mockRepo
		tester        *mockToolTester
		wantStatus    string
		wantErr       bool
		wantMsg       string
		wantCode      int32
	}{
		{
			name:          "empty tool id",
			toolID:        "  ",
			argumentsJSON: `{"cmd":"ls"}`,
			repo:          &mockRepo{},
			wantErr:       true,
			wantMsg:       "tool id is required",
			wantCode:      400,
		},
		{
			name:          "GetTool error",
			toolID:        "missing_tool",
			argumentsJSON: `{"cmd":"ls"}`,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{}, apierror.NotFound("TOOL", "tool not found")
				},
			},
			wantErr:  true,
			wantMsg:  "tool not found",
			wantCode: 404,
		},
		{
			name:          "no tester configured",
			toolID:        "tool_1",
			argumentsJSON: `{"cmd":"ls"}`,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "shell_exec", Source: "builtin"}, nil
				},
			},
			tester:   nil,
			wantErr:  true,
			wantMsg:  "tool tester not configured",
			wantCode: 500,
		},
		{
			name:          "tester execute success",
			toolID:        "tool_1",
			argumentsJSON: `{"cmd":"ls"}`,
			timeoutSec:    30,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "shell_exec", Source: "builtin", ConfigJSON: "{}", DefaultConfigJSON: "{}", MetadataJSON: ""}, nil
				},
				recordToolInvocation: func(_ context.Context, in ToolInvocationWrite) error {
					if in.ToolKey != "shell_exec" {
						t.Fatalf("expected tool key 'shell_exec', got %q", in.ToolKey)
					}
					if in.Source != "tool_test" {
						t.Fatalf("expected source 'tool_test', got %q", in.Source)
					}
					if in.Status != "success" {
						t.Fatalf("expected status 'success', got %q", in.Status)
					}
					return nil
				},
			},
			tester: &mockToolTester{
				execute: func(_ context.Context, tool ToolTestInput, argumentsJSON string, timeoutSec int, _ *WebResearchPlatformFields) (ToolTestResult, error) {
					if tool.Key != "shell_exec" {
						t.Fatalf("expected tool key 'shell_exec', got %q", tool.Key)
					}
					if argumentsJSON != `{"cmd":"ls"}` {
						t.Fatalf("expected arguments '{\"cmd\":\"ls\"}', got %q", argumentsJSON)
					}
					if timeoutSec != 30 {
						t.Fatalf("expected timeout 30, got %d", timeoutSec)
					}
					return ToolTestResult{Status: "success", ResultPreview: "file1.txt\nfile2.txt", DurationMS: 150}, nil
				},
			},
			wantStatus: "success",
		},
		{
			name:          "tester execute error",
			toolID:        "tool_1",
			argumentsJSON: `{"cmd":"ls"}`,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "shell_exec", Source: "builtin"}, nil
				},
			},
			tester: &mockToolTester{
				execute: func(_ context.Context, _ ToolTestInput, _ string, _ int, _ *WebResearchPlatformFields) (ToolTestResult, error) {
					return ToolTestResult{}, apierror.Internal("TOOL", "execution timeout")
				},
			},
			wantErr:  true,
			wantMsg:  "execution timeout",
			wantCode: 500,
		},
		{
			name:          "tester success even when record invocation fails",
			toolID:        "tool_1",
			argumentsJSON: `{"cmd":"ls"}`,
			repo: &mockRepo{
				getTool: func(_ context.Context, _ string) (Tool, error) {
					return Tool{ID: "tool_1", Key: "shell_exec", Source: "builtin", ConfigJSON: "{}", DefaultConfigJSON: "{}", MetadataJSON: ""}, nil
				},
				recordToolInvocation: func(_ context.Context, _ ToolInvocationWrite) error {
					return apierror.Internal("TOOL", "db write failed")
				},
			},
			tester: &mockToolTester{
				execute: func(_ context.Context, _ ToolTestInput, _ string, _ int, _ *WebResearchPlatformFields) (ToolTestResult, error) {
					return ToolTestResult{Status: "success", ResultPreview: "ok", DurationMS: 50}, nil
				},
			},
			wantStatus: "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []ToolUsecaseOption
			if tt.tester != nil {
				opts = append(opts, WithToolTester(tt.tester))
			}
			uc := NewToolUsecase(tt.repo, nil, loggateway.NewNoop(), opts...)
			got, err := uc.TestTool(ctx, tt.toolID, tt.argumentsJSON, tt.timeoutSec)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ae, ok := apierror.From(err)
				if !ok {
					t.Fatalf("expected apierror.Error, got %T", err)
				}
				if tt.wantCode != 0 {
					var wantCode apierror.Code
					switch tt.wantCode {
					case 400:
						wantCode = apierror.CodeBadRequest
					case 404:
						wantCode = apierror.CodeNotFound
					case 500:
						wantCode = apierror.CodeInternal
					default:
						wantCode = apierror.Code(tt.wantCode)
					}
					if ae.Code != wantCode {
						t.Fatalf("expected code %d, got %s", tt.wantCode, ae.Code)
					}
				}
				if tt.wantMsg != "" && ae.Message != tt.wantMsg {
					t.Fatalf("expected message %q, got %q", tt.wantMsg, ae.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Status != tt.wantStatus {
				t.Fatalf("expected status %q, got %q", tt.wantStatus, got.Status)
			}
		})
	}
}
