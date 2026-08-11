package tool

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Tool key constants (mirrored from biz for subpackage independence).
const (
	ToolKeyMCPToolSet       = "mcp_tool_set"
	ToolKeyMCPBroker        = "mcp_broker"
	ToolKeyKnowledgeSearch  = "knowledge_search"
	ToolKeyKnowledgeReflect = "knowledge_reflect"
	ToolKeyWebResearch      = "web_research"
	ToolKeyKanban           = "kanban"
	ToolKeyCallAgent        = "call_agent"
)

// Tool is capability catalog row + aggregates.
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
	// WorkspaceID is the owning workspace ID for tenant isolation (P2-B).
	// empty = shared/legacy (visible to all workspaces, e.g., system builtins);
	// non-empty = tenant-private (visible only to owning workspace).
	WorkspaceID string
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
	// WorkspaceID is set by the service layer based on caller context (P2-B).
	// empty = shared/system builtin; non-empty = tenant-private.
	WorkspaceID string
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
	// Abnormal=true 仅返回最近一次调用以 error/blocked 收尾的工具（「仅看异常」筛选）。
	Abnormal bool
	// WorkspaceID filters by tenant visibility (P2-B).
	// empty = system caller (see all); non-empty = tenant caller
	// (see shared with workspace_id="" + own with workspace_id==WorkspaceID).
	WorkspaceID string
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
	Streaming        bool
	ChunkCount       int
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
	Streaming     bool
	ChunkCount    int
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
	ID      string
	ToolID  string
	ToolKey string
	AgentID string
	// Enabled 为遗留列：运行时启停判定只读 Mode（见 applyOverrideToEffectiveItem），
	// 本字段不参与判定，写入时恒置 true 归一化存储。
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

type ToolInvocationAuditWrite struct {
	InvocationID  string
	ToolKey       string
	AgentID       string
	UserID        string
	SessionID     string
	Action        string
	ResultSummary string
	Status        string
	Source        string
}

type ToolInvocationAudit struct {
	ID            string
	InvocationID  string
	ToolKey       string
	AgentID       string
	UserID        string
	SessionID     string
	Action        string
	ResultSummary string
	Status        string
	Source        string
	CreatedAt     string
}

type ToolAuditQuery struct {
	ToolKey   string
	AgentID   string
	UserID    string
	SessionID string
	Status    string
	From      string
	To        string
	Limit     int
	Offset    int
}

type ToolAuditResult struct {
	Items  []ToolInvocationAudit
	Total  int
	Limit  int
	Offset int
}

// ToolCatalogEntry is the lightweight build-time catalog row for a tool. It
// carries only the columns needed for runtime-config merging and the
// confirmation gate, so the backing query stays a cheap indexed lookup
// without the invocation-aggregation joins used by SearchTools/GetTool.
type ToolCatalogEntry struct {
	Key                  string
	ConfigJSON           string
	DefaultConfigJSON    string
	RequiresConfirmation bool
}

// Stability:stable
type ToolReader interface {
	SearchTools(ctx context.Context, q ToolListQuery) (ToolListResult, error)
	GetTool(ctx context.Context, idOrKey string) (Tool, error)
	// ListToolCatalogEntries batch-loads lightweight catalog rows for the
	// given tool keys in a single query, replacing per-key GetTool loops at
	// agent-build time. Soft-deleted and unknown keys are simply absent.
	ListToolCatalogEntries(ctx context.Context, keys []string) ([]ToolCatalogEntry, error)
}

// Stability:stable
type ToolWriter interface {
	CreateTool(ctx context.Context, in ToolUpsertInput) (Tool, error)
	UpdateTool(ctx context.Context, idOrKey string, in ToolUpsertInput) (Tool, error)
	DeleteTool(ctx context.Context, idOrKey string) error
	UpdateToolEnabled(ctx context.Context, idOrKey string, enabled bool) (Tool, error)
	UpdateToolConfig(ctx context.Context, idOrKey string, configJSON string) (Tool, error)
}

// Stability:stable
type ToolInvocationReader interface {
	SearchToolInvocations(ctx context.Context, q ToolRunQuery) (ToolRunResult, error)
	GetToolInvocationParams(ctx context.Context, invocationID string) (ToolInvocationParam, error)
}

// Stability:stable
type ToolInvocationWriter interface {
	RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error
}

// Stability:stable
type ToolAuditRepo interface {
	RecordToolInvocationAudit(ctx context.Context, in ToolInvocationAuditWrite) error
	SearchToolInvocationAudits(ctx context.Context, q ToolAuditQuery) (ToolAuditResult, error)
	PurgeToolInvocationAuditsBefore(ctx context.Context, cutoffRFC3339 string) (int64, error)
}

// Stability:stable
type ToolOverrideReader interface {
	ListToolAgentOverrides(ctx context.Context, toolKey string) ([]ToolAgentOverride, error)
	ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]ToolAgentOverride, error)
}

// Stability:stable
type ToolOverrideWriter interface {
	UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput, toolID string) (ToolAgentOverride, error)
	DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error
}

type ToolSyncer interface {
	SyncBuiltinTools(ctx context.Context) error
}

// Stability:stable
type ToolRegistryReader interface {
	ToolReader
	ToolOverrideReader
}

// Stability:stable
type ToolRepo interface {
	ToolReader
	ToolWriter
	ToolInvocationReader
	ToolInvocationWriter
	ToolAuditRepo
	ToolOverrideReader
	ToolOverrideWriter
	ToolSyncer
}

// SettingRepo provides read access to system settings needed by tool usecase.
// Stability:stable
type SettingRepo interface {
	GetWebResearch(ctx context.Context) (WebResearchSetting, error)
}

// WebResearchSetting mirrors biz.WebResearchSetting for tool subpackage independence.
type WebResearchSetting struct {
	Provider    string
	APIKey      string
	HasAPIKey   bool
	MaxResults  int
	FetchTop    int
	SearchDepth string
	TimeoutSec  int
	HTTPProxy   string
}

type ToolUsecase struct {
	repo          ToolRepo
	sys           SettingRepo
	tester        ToolTester
	webResChecker WebResearchReadinessChecker
	grants        ToolGrantStore
	lg            loggateway.Logger
}

type ToolUsecaseOption func(*ToolUsecase)

func WithToolTester(tester ToolTester) ToolUsecaseOption {
	return func(u *ToolUsecase) { u.tester = tester }
}

func WithWebResearchChecker(checker WebResearchReadinessChecker) ToolUsecaseOption {
	return func(u *ToolUsecase) { u.webResChecker = checker }
}

func NewToolUsecase(repo ToolRepo, sys SettingRepo, lg loggateway.Logger, opts ...ToolUsecaseOption) *ToolUsecase {
	uc := &ToolUsecase{repo: repo, sys: sys, lg: lg}
	for _, opt := range opts {
		opt(uc)
	}
	return uc
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
	result, err := u.repo.SearchTools(ctx, q)
	if err != nil {
		return ToolListResult{}, err
	}
	result.Items = enrichToolList(result.Items, LoadWebResearchPlatform(ctx, u.sys), CheckerToReadinessFunc(u.webResChecker))
	return result, nil
}

func (u *ToolUsecase) GetTool(ctx context.Context, id string) (Tool, error) {
	if strings.TrimSpace(id) == "" {
		return Tool{}, apierror.BadRequest("TOOL", "id is required")
	}
	t, err := u.repo.GetTool(ctx, id)
	if err != nil {
		return Tool{}, err
	}
	EnrichToolRuntimeFieldsWithPlatform(&t, LoadWebResearchPlatform(ctx, u.sys), CheckerToReadinessFunc(u.webResChecker))
	return t, nil
}

// ListToolCatalogEntries batch-loads lightweight catalog rows for the given
// tool keys (trimmed + deduped). Empty input short-circuits without a query.
// Unlike GetTool it skips platform enrichment — the catalog row carries no
// runtime fields that enrichment would populate.
func (u *ToolUsecase) ListToolCatalogEntries(ctx context.Context, keys []string) ([]ToolCatalogEntry, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(keys))
	normalized := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		normalized = append(normalized, k)
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return u.repo.ListToolCatalogEntries(ctx, normalized)
}

func (u *ToolUsecase) Create(ctx context.Context, in ToolUpsertInput) (Tool, error) {
	if err := validateToolUpsert(in); err != nil {
		return Tool{}, err
	}
	t, err := u.repo.CreateTool(ctx, in)
	if err != nil {
		return Tool{}, err
	}
	EnrichToolRuntimeFieldsWithPlatform(&t, LoadWebResearchPlatform(ctx, u.sys), CheckerToReadinessFunc(u.webResChecker))
	return t, nil
}

func (u *ToolUsecase) Update(ctx context.Context, id string, in ToolUpsertInput) (Tool, error) {
	if strings.TrimSpace(id) == "" {
		return Tool{}, apierror.BadRequest("TOOL", "id is required")
	}
	if err := validateToolUpsert(in); err != nil {
		return Tool{}, err
	}
	existing, err := u.repo.GetTool(ctx, id)
	if err != nil {
		return Tool{}, err
	}
	if err := assertToolMutable(existing, in); err != nil {
		return Tool{}, err
	}
	t, err := u.repo.UpdateTool(ctx, id, in)
	if err != nil {
		return Tool{}, err
	}
	EnrichToolRuntimeFieldsWithPlatform(&t, LoadWebResearchPlatform(ctx, u.sys), CheckerToReadinessFunc(u.webResChecker))
	return t, nil
}

func (u *ToolUsecase) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return apierror.BadRequest("TOOL", "id is required")
	}
	existing, err := u.repo.GetTool(ctx, id)
	if err != nil {
		return err
	}
	if err := assertToolDeletable(existing); err != nil {
		return err
	}
	return u.repo.DeleteTool(ctx, id)
}

// ConfirmIntentValue is the required value for the confirm_intent parameter
// when enabling high/critical risk tools. This ensures the caller explicitly
// acknowledges the risk rather than matching a guessable key.
const ConfirmIntentValue = "I_UNDERSTAND_RISK"

func (u *ToolUsecase) ToggleEnabled(ctx context.Context, id string, enabled bool, confirmIntent ...string) (Tool, error) {
	if strings.TrimSpace(id) == "" {
		return Tool{}, apierror.BadRequest("TOOL", "id is required")
	}
	if enabled {
		t, err := u.repo.GetTool(ctx, id)
		if err != nil {
			return Tool{}, err
		}
		if t.RiskLevel == "high" || t.RiskLevel == "critical" {
			if len(confirmIntent) == 0 || confirmIntent[0] != ConfirmIntentValue {
				return Tool{}, apierror.BadRequest("TOOL", "confirm_intent is required and must be I_UNDERSTAND_RISK for high/critical risk tools")
			}
		}
	}
	return u.repo.UpdateToolEnabled(ctx, id, enabled)
}

func (u *ToolUsecase) UpdateToolConfig(ctx context.Context, id string, configJSON string) (Tool, error) {
	if strings.TrimSpace(id) == "" {
		return Tool{}, apierror.BadRequest("TOOL", "id is required")
	}
	if configJSON == "" {
		configJSON = "{}"
	}
	existing, err := u.repo.GetTool(ctx, id)
	if err != nil {
		return Tool{}, err
	}
	if err := validateToolConfigAgainstSchema(existing.Source, existing.ConfigSchemaJSON, configJSON); err != nil {
		return Tool{}, err
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

// ListRunsForTool lists invocations for a tool referenced by catalog id or tool_key.
func (u *ToolUsecase) ListRunsForTool(ctx context.Context, toolIDOrKey string, q ToolRunQuery) (ToolRunResult, error) {
	key, err := u.ResolveToolKey(ctx, toolIDOrKey)
	if err != nil {
		return ToolRunResult{}, err
	}
	q.ToolKey = key
	return u.ListRuns(ctx, q)
}

func (u *ToolUsecase) RecordToolInvocationAudit(ctx context.Context, in ToolInvocationAuditWrite) error {
	return u.repo.RecordToolInvocationAudit(ctx, in)
}

func (u *ToolUsecase) ListInvocationAudits(ctx context.Context, q ToolAuditQuery) (ToolAuditResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return u.repo.SearchToolInvocationAudits(ctx, q)
}

// ToolAuditRetentionDays is the default audit log retention policy.
const ToolAuditRetentionDays = 90

func (u *ToolUsecase) PurgeOldInvocationAudits(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -ToolAuditRetentionDays).Format(time.RFC3339)
	return u.repo.PurgeToolInvocationAuditsBefore(ctx, cutoff)
}

func (u *ToolUsecase) SyncBuiltinTools(ctx context.Context) error {
	return u.repo.SyncBuiltinTools(ctx)
}

func (u *ToolUsecase) GetToolInvocationParams(ctx context.Context, invocationID string) (ToolInvocationParam, error) {
	if strings.TrimSpace(invocationID) == "" {
		return ToolInvocationParam{}, apierror.BadRequest("TOOL", "invocation id is required")
	}
	return u.repo.GetToolInvocationParams(ctx, invocationID)
}

func (u *ToolUsecase) ListToolAgentOverrides(ctx context.Context, toolIDOrKey string) ([]ToolAgentOverride, error) {
	key, err := u.ResolveToolKey(ctx, toolIDOrKey)
	if err != nil {
		return nil, err
	}
	return u.repo.ListToolAgentOverrides(ctx, key)
}

func (u *ToolUsecase) ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]ToolAgentOverride, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, apierror.BadRequest("TOOL", "agent id is required")
	}
	return u.repo.ListToolAgentOverridesByAgent(ctx, agentID)
}

func (u *ToolUsecase) UpsertToolAgentOverride(ctx context.Context, in ToolAgentOverrideInput) (ToolAgentOverride, error) {
	if strings.TrimSpace(in.AgentID) == "" {
		return ToolAgentOverride{}, apierror.BadRequest("TOOL", "agent id is required")
	}
	tool, err := u.GetTool(ctx, in.ToolKey)
	if err != nil {
		return ToolAgentOverride{}, err
	}
	in.ToolKey = tool.Key
	if in.Mode == "" {
		in.Mode = "inherit"
	}
	in.ConfigOverrideJSON = strings.TrimSpace(in.ConfigOverrideJSON)
	if in.ConfigOverrideJSON == "" {
		in.ConfigOverrideJSON = "{}"
	}
	if !json.Valid([]byte(in.ConfigOverrideJSON)) {
		return ToolAgentOverride{}, apierror.BadRequest("TOOL", "config_override_json must be valid JSON")
	}
	return u.repo.UpsertToolAgentOverride(ctx, in, tool.ID)
}

func (u *ToolUsecase) DeleteToolAgentOverride(ctx context.Context, toolIDOrKey string, agentID string) error {
	if strings.TrimSpace(agentID) == "" {
		return apierror.BadRequest("TOOL", "agent id is required")
	}
	key, err := u.ResolveToolKey(ctx, toolIDOrKey)
	if err != nil {
		return err
	}
	return u.repo.DeleteToolAgentOverride(ctx, key, agentID)
}
