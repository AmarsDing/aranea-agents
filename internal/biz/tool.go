package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// Tool is capability catalog row + aggregates（与遗留 JSON 对齐）.
type Tool struct {
	ID                   string
	Key                  string
	DisplayName          string
	Description          string
	Category             string
	Source               string
	RiskLevel            string
	Enabled              bool
	Readonly             bool
	RequiresConfirmation bool
	SupportsStreaming    bool
	SupportsConcurrency  bool
	ParametersSchemaJSON string
	ResultSchemaJSON     string
	ConfigSchemaJSON     string
	ConfigJSON           string
	DefaultConfigJSON    string
	MetadataJSON         string
	RuntimeStatus        string
	RuntimeKind          string
	InvokeCount          int
	InvokeCount24h       int
	SuccessCount         int
	FailureCount         int
	BlockedCount         int
	AgentOverrideCount   int
	AvgDurationMS        *float64
	P95DurationMS        float64
	LastInvokedAt        string
	LastStatus           string
	CreatedAt            string
	UpdatedAt            string
	DeletedAt            string
	Permissions          ToolPermissions
}

type ToolPermissions struct {
	CanManage bool
}

type ToolUpsertInput struct {
	ID                   string
	Key                  string
	DisplayName          string
	Description          string
	Category             string
	Source               string
	RiskLevel            string
	Enabled              bool
	Readonly             bool
	RequiresConfirmation bool
	SupportsStreaming    bool
	SupportsConcurrency  bool
	ParametersSchemaJSON string
	ResultSchemaJSON     string
	ConfigSchemaJSON     string
	ConfigJSON           string
	DefaultConfigJSON    string
	MetadataJSON         string
}

type ToolListQuery struct {
	Search    string
	Category  string
	Source    string
	RiskLevel string
	Enabled   string
	Sort      string
	Limit     int
	Offset    int
}

type ToolListResult struct {
	Items   []Tool
	Total   int
	Limit   int
	Offset  int
	Summary ToolSummary
}

type ToolSummary struct {
	TotalTools      int
	EnabledTools    int
	HighRiskEnabled int
	Calls24h        int
	FailureRate24h  float64
}

type ToolInvocation struct {
	ID               string
	RequestID        string
	InvocationID     string
	ToolID           string
	ToolKey          string
	ToolDisplayName  string
	AgentID          string
	AgentKey         string
	AgentDisplayName string
	SessionID        string
	MessageID        string
	UserID           string
	Source           string
	Status           string
	StartedAt        string
	EndedAt          string
	DurationMS       int
	InputPreview     string
	InputHash        string
	OutputPreview    string
	OutputHash       string
	ErrorCode        string
	ErrorMessage     string
	RedactionApplied bool
	MetadataJSON     string
	CreatedAt        string
}

type ToolInvocationWrite struct {
	ToolKey       string
	AgentID       string
	AgentKey      string
	SessionID     string
	UserID        string
	Status        string
	DurationMS    int
	StartedAt     string
	EndedAt       string
	InputPreview  string
	InputHash     string
	OutputPreview string
	OutputHash    string
	ErrorCode     string
	ErrorMessage  string
	Source        string
	ToolCallID    string
}

type ToolInvocationParam struct {
	ID               string
	InvocationID     string
	ToolKey          string
	ParamsJSON       string
	RedactionApplied bool
	CreatedAt        string
}

type ToolAgentOverride struct {
	ID                   string
	ToolID               string
	ToolKey              string
	AgentID              string
	Enabled              bool
	Mode                 string
	ConfigOverrideJSON   string
	RequiresConfirmation bool
	CreatedAt            string
	UpdatedAt            string
}

type ToolAgentOverrideInput struct {
	ToolKey              string
	AgentID              string
	Enabled              bool
	Mode                 string
	ConfigOverrideJSON   string
	RequiresConfirmation bool
}

type ToolRunQuery struct {
	ToolKey   string
	AgentID   string
	SessionID string
	Status    string
	From      string
	To        string
	HasError  *bool
	Limit     int
	Offset    int
}

type ToolRunResult struct {
	Items  []ToolInvocation
	Total  int
	Limit  int
	Offset int
}

type ToolRepo interface {
	SearchTools(ctx context.Context, q ToolListQuery) (ToolListResult, error)
	GetTool(ctx context.Context, idOrKey string) (Tool, error)
	CreateTool(ctx context.Context, in ToolUpsertInput) (Tool, error)
	UpdateTool(ctx context.Context, idOrKey string, in ToolUpsertInput) (Tool, error)
	DeleteTool(ctx context.Context, idOrKey string) error
	UpdateToolEnabled(ctx context.Context, idOrKey string, enabled bool) (Tool, error)
	UpdateToolConfig(ctx context.Context, idOrKey string, configJSON string) (Tool, error)
	SearchToolInvocations(ctx context.Context, q ToolRunQuery) (ToolRunResult, error)
	RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error
	SyncBuiltinTools(ctx context.Context) error
	GetToolInvocationParams(ctx context.Context, invocationID string) (ToolInvocationParam, error)
	ListToolAgentOverrides(ctx context.Context, toolKey string) ([]ToolAgentOverride, error)
	ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]ToolAgentOverride, error)
	UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput) (ToolAgentOverride, error)
	DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error
}

type ToolUsecase struct {
	repo ToolRepo
}

func NewToolUsecase(repo ToolRepo) *ToolUsecase {
	return &ToolUsecase{repo: repo}
}

func (u *ToolUsecase) ListTools(ctx context.Context, q ToolListQuery) (ToolListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return u.repo.SearchTools(ctx, q)
}

func (u *ToolUsecase) GetTool(ctx context.Context, id string) (Tool, error) {
	if strings.TrimSpace(id) == "" {
		return Tool{}, errors.BadRequest("TOOL", "id is required")
	}
	return u.repo.GetTool(ctx, id)
}

func (u *ToolUsecase) Create(ctx context.Context, in ToolUpsertInput) (Tool, error) {
	return u.repo.CreateTool(ctx, in)
}

func (u *ToolUsecase) Update(ctx context.Context, id string, in ToolUpsertInput) (Tool, error) {
	if strings.TrimSpace(id) == "" {
		return Tool{}, errors.BadRequest("TOOL", "id is required")
	}
	return u.repo.UpdateTool(ctx, id, in)
}

func (u *ToolUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("TOOL", "id is required")
	}
	return u.repo.DeleteTool(ctx, id)
}

func (u *ToolUsecase) ToggleEnabled(ctx context.Context, id string, enabled bool, confirmKey ...string) (Tool, error) {
	if strings.TrimSpace(id) == "" {
		return Tool{}, errors.BadRequest("TOOL", "id is required")
	}
	if enabled {
		t, err := u.repo.GetTool(ctx, id)
		if err != nil {
			return Tool{}, err
		}
		if t.RiskLevel == "high" || t.RiskLevel == "critical" {
			if len(confirmKey) == 0 || confirmKey[0] != t.Key {
				return Tool{}, errors.BadRequest("TOOL", "confirm_key is required and must match tool key for high/critical risk tools")
			}
		}
	}
	return u.repo.UpdateToolEnabled(ctx, id, enabled)
}

func (u *ToolUsecase) UpdateToolConfig(ctx context.Context, id string, configJSON string) (Tool, error) {
	if strings.TrimSpace(id) == "" {
		return Tool{}, errors.BadRequest("TOOL", "id is required")
	}
	if configJSON == "" {
		configJSON = "{}"
	}
	return u.repo.UpdateToolConfig(ctx, id, configJSON)
}

func (u *ToolUsecase) ListRuns(ctx context.Context, q ToolRunQuery) (ToolRunResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return u.repo.SearchToolInvocations(ctx, q)
}

func (u *ToolUsecase) RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error {
	return u.repo.RecordToolInvocation(ctx, in)
}

func (u *ToolUsecase) SyncBuiltinTools(ctx context.Context) error {
	return u.repo.SyncBuiltinTools(ctx)
}

func (u *ToolUsecase) GetToolInvocationParams(ctx context.Context, invocationID string) (ToolInvocationParam, error) {
	if strings.TrimSpace(invocationID) == "" {
		return ToolInvocationParam{}, errors.BadRequest("TOOL", "invocation id is required")
	}
	return u.repo.GetToolInvocationParams(ctx, invocationID)
}

func (u *ToolUsecase) ListToolAgentOverrides(ctx context.Context, toolKey string) ([]ToolAgentOverride, error) {
	if strings.TrimSpace(toolKey) == "" {
		return nil, errors.BadRequest("TOOL", "tool key is required")
	}
	return u.repo.ListToolAgentOverrides(ctx, toolKey)
}

func (u *ToolUsecase) ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]ToolAgentOverride, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.BadRequest("TOOL", "agent id is required")
	}
	return u.repo.ListToolAgentOverridesByAgent(ctx, agentID)
}

func (u *ToolUsecase) UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput) (ToolAgentOverride, error) {
	if strings.TrimSpace(in.ToolKey) == "" {
		return ToolAgentOverride{}, errors.BadRequest("TOOL", "tool key is required")
	}
	if strings.TrimSpace(in.AgentID) == "" {
		return ToolAgentOverride{}, errors.BadRequest("TOOL", "agent id is required")
	}
	if in.Mode == "" {
		in.Mode = "inherit"
	}
	if in.ConfigOverrideJSON == "" {
		in.ConfigOverrideJSON = "{}"
	}
	return u.repo.UpsertToolAgentOverride(ctx, in)
}

func (u *ToolUsecase) DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error {
	if strings.TrimSpace(toolKey) == "" {
		return errors.BadRequest("TOOL", "tool key is required")
	}
	if strings.TrimSpace(agentID) == "" {
		return errors.BadRequest("TOOL", "agent id is required")
	}
	return u.repo.DeleteToolAgentOverride(ctx, toolKey, agentID)
}
