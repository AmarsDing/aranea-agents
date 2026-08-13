// Package plugin implements plugin CRUD, sandbox, schema, and cost guard workflows.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aranea-agents/pkg/apierror"

	"aranea-agents/internal/biz/shared"
)

// Plugin matches persisted plugins row + runtime permissions.
type Plugin struct {
	ID                string
	Key               string
	Name              string
	Description       string
	Category          string
	RiskLevel         string
	Enabled           bool
	Scope             string
	CallbackPoints    []string
	SortOrder         int
	ConfigSchemaJSON  string
	ConfigJSON        string
	DefaultConfigJSON string
	InvokeCount       int
	BlockCount        int
	ErrorCount        int
	LastInvokedAt     string
	LastStatus        string
	CreatedAt         string
	UpdatedAt         string
	Permissions       Permissions
	// WorkspaceID is the owning workspace ID for tenant isolation (P2-B).
	// empty = shared/legacy (visible to all workspaces, e.g., system builtins);
	// non-empty = tenant-private (visible only to owning workspace).
	WorkspaceID string
}

// Permissions mirrors legacy JSON for admin UI.
type Permissions struct {
	CanView       bool
	CanToggle     bool
	CanEditConfig bool
	CanViewLogs   bool
}

// AdminPerms returns full admin permissions.
func AdminPerms() Permissions {
	return Permissions{CanView: true, CanToggle: true, CanEditConfig: true, CanViewLogs: true}
}

// ListQuery filters ListPlugins.
type ListQuery struct {
	Search        string
	Category      string
	Enabled       string
	CallbackPoint string
	Limit         int
	Offset        int
	// WorkspaceID filters by tenant visibility (P2-B).
	// empty = system caller (see all); non-empty = tenant caller
	// (see shared with workspace_id="" + own with workspace_id==WorkspaceID).
	WorkspaceID string
}

// ListResult is one page of plugins.
type ListResult struct {
	Items  []Plugin
	Total  int
	Limit  int
	Offset int
}

// StatUpdate is a delta applied to persisted plugin invocation counters.
type StatUpdate struct {
	InvokeCount int
	BlockDelta  int
	ErrorDelta  int
	LastStatus  string
}

// Repo abstracts plugin persistence.
type Repo interface {
	SearchPlugins(ctx context.Context, q ListQuery) (ListResult, error)
	GetPlugin(ctx context.Context, id string) (Plugin, error)
	GetByKey(ctx context.Context, key string) (Plugin, error)
	CreatePlugin(ctx context.Context, p Plugin) (Plugin, error)
	UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (Plugin, error)
	UpdatePluginConfig(ctx context.Context, id string, configJSON string) (Plugin, error)
	UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error)
	UpdatePluginScope(ctx context.Context, id string, scope string) (Plugin, error)
	// SyncBuiltinMeta 同步内置插件的平台自有元数据（name/description/category/
	// risk_level/callback_points/config_schema_json/default_config_json），
	// 保留管理员字段（enabled/config_json/sort_order/scope/workspace_id）。
	SyncBuiltinMeta(ctx context.Context, p Plugin) (Plugin, error)
	IncrementStats(ctx context.Context, pluginKey string, delta StatUpdate) error
}

// BuiltinMetaDrifted 报告已有行的平台自有元数据是否与内置定义漂移。
// 仅比较 SyncBuiltinMeta 覆盖的字段；管理员字段不参与比较。
func BuiltinMetaDrifted(cur, want Plugin) bool {
	if cur.Name != want.Name || cur.Description != want.Description ||
		cur.Category != want.Category || cur.RiskLevel != want.RiskLevel ||
		cur.ConfigSchemaJSON != want.ConfigSchemaJSON ||
		cur.DefaultConfigJSON != want.DefaultConfigJSON {
		return true
	}
	if len(cur.CallbackPoints) != len(want.CallbackPoints) {
		return true
	}
	for i := range cur.CallbackPoints {
		if cur.CallbackPoints[i] != want.CallbackPoints[i] {
			return true
		}
	}
	return false
}

// Run is one plugin callback invocation audit row.
type Run struct {
	ID            string
	PluginKey     string
	PluginID      string
	SessionID     string
	AgentID       string
	CallbackPoint string
	Status        string
	DurationMS    int
	DetailJSON    string
	CreatedAt     string
	// WorkspaceID 是写入侧租户归属（N-B5）。empty = 共享行（所有租户可见）；
	// non-empty = 租户私有行。当前写入链路（internal/plugin/trpc StatsRecorder
	// 异步批量落库，调用方显式丢弃请求 ctx）拿不到 workspace，一律写空串
	// （共享语义）；读侧过滤对存量/新写入均安全（共享行全员可见）。
	WorkspaceID string
}

// RunQuery filters plugin run list.
type RunQuery struct {
	PluginKey     string
	PluginID      string
	SessionID     string
	AgentID       string
	CallbackPoint string
	Status        string
	From          string
	To            string
	Limit         int
	Offset        int
	// WorkspaceID 按租户可见性过滤（N-B5），由 service 层注入，不接受客户端输入。
	// empty = 系统调用（看全部）；non-empty = 租户调用
	// （看 workspace_id='' 共享行 + workspace_id=自身 的行）。
	WorkspaceID string
}

// RunListResult is a paginated plugin run list.
type RunListResult struct {
	Items  []Run
	Total  int32
	Limit  int
	Offset int
}

// RunRepo persists plugin run audit rows.
type RunRepo interface {
	Insert(ctx context.Context, run Run) error
	List(ctx context.Context, q RunQuery) (RunListResult, error)
	// DeleteAll 按 RunQuery.WorkspaceID 同一可见性语义删除：
	// empty = 系统调用（全删）；non-empty = 租户调用（删共享行 + 自身行）。
	DeleteAll(ctx context.Context, workspaceID string) (int32, error)
}

// ScopeAgentLookup checks whether an agent exists for scope validation.
type ScopeAgentLookup interface {
	AgentExists(ctx context.Context, id string) error
}

// Usecase implements plugin CRUD workflows.
type Usecase struct {
	repo   Repo
	runs   RunRepo
	agents ScopeAgentLookup
}

// noopScopeAgentLookup skips agent-existence validation for non-global scopes.
// Substituted by NewUsecase when agents is nil.
type noopScopeAgentLookup struct{}

func (noopScopeAgentLookup) AgentExists(context.Context, string) error { return nil }

// NewUsecase constructs the plugin Usecase.
// agents 为 nil 时替换为 noop 实现（AgentExists 恒返回 nil），即跳过 agent 存在性
// 校验——仅限测试/嵌入场景；生产必须注入真实 ScopeAgentLookup。
func NewUsecase(repo Repo, runs RunRepo, agents ScopeAgentLookup) *Usecase {
	if agents == nil {
		agents = noopScopeAgentLookup{}
	}
	return &Usecase{repo: repo, runs: runs, agents: agents}
}

// List returns paginated plugins.
func (u *Usecase) List(ctx context.Context, q ListQuery) (ListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	if q.Limit > 100 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	return u.repo.SearchPlugins(ctx, q)
}

// ToggleEnabled enables or disables a plugin.
func (u *Usecase) ToggleEnabled(ctx context.Context, id string, enabled bool) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, apierror.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.UpdatePluginEnabled(ctx, id, enabled)
}

// GetByKey returns a plugin by its unique key.
func (u *Usecase) GetByKey(ctx context.Context, key string) (Plugin, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Plugin{}, apierror.BadRequest("PLUGIN", "key is required")
	}
	return u.repo.GetByKey(ctx, key)
}

// Get returns a plugin by ID. (P2-B: service-layer IDOR check)
func (u *Usecase) Get(ctx context.Context, id string) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, apierror.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.GetPlugin(ctx, id)
}

// Create validates and stores a new plugin.
func (u *Usecase) Create(ctx context.Context, p Plugin) (Plugin, error) {
	p.Key = strings.TrimSpace(p.Key)
	if p.Key == "" {
		return Plugin{}, apierror.BadRequest("PLUGIN", "key is required")
	}
	if strings.TrimSpace(p.ID) == "" {
		p.ID = "builtin-" + p.Key
	}
	if strings.TrimSpace(p.ConfigJSON) == "" {
		p.ConfigJSON = "{}"
	}
	if !json.Valid([]byte(p.ConfigJSON)) {
		return Plugin{}, apierror.BadRequest("PLUGIN", "config_json must be valid JSON")
	}
	if schema := strings.TrimSpace(p.ConfigSchemaJSON); schema != "" && schema != "{}" {
		if err := ValidateJSONSchema(schema, p.ConfigJSON); err != nil {
			return Plugin{}, err
		}
	}
	if p.Scope == "" {
		p.Scope = "global"
	}
	return u.repo.CreatePlugin(ctx, p)
}

// SyncBuiltinMeta 将内置定义的平台自有元数据同步到已有行（bootstrap 种子用）。
// 漂移判定由调用方经 BuiltinMetaDrifted 完成；此处直接落库。
func (u *Usecase) SyncBuiltinMeta(ctx context.Context, p Plugin) (Plugin, error) {
	if strings.TrimSpace(p.ID) == "" {
		return Plugin{}, apierror.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.SyncBuiltinMeta(ctx, p)
}

// UpdateConfig updates a plugin's configuration.
func (u *Usecase) UpdateConfig(ctx context.Context, id string, configJSON string) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, apierror.BadRequest("PLUGIN", "id is required")
	}
	if strings.TrimSpace(configJSON) == "" {
		configJSON = "{}"
	}
	if !json.Valid([]byte(configJSON)) {
		return Plugin{}, apierror.BadRequest("PLUGIN", "config_json must be valid JSON")
	}
	p, err := u.repo.GetPlugin(ctx, id)
	if err != nil {
		return Plugin{}, err
	}
	if schema := strings.TrimSpace(p.ConfigSchemaJSON); schema != "" && schema != "{}" {
		if err := ValidateJSONSchema(schema, configJSON); err != nil {
			return Plugin{}, err
		}
	}
	return u.repo.UpdatePluginConfig(ctx, id, configJSON)
}

// UpdateSortOrder updates a plugin's sort order.
func (u *Usecase) UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, apierror.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.UpdateSortOrder(ctx, id, sortOrder)
}

// UpdateScope updates a plugin's scope.
func (u *Usecase) UpdateScope(ctx context.Context, id string, scope string) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, apierror.BadRequest("PLUGIN", "id is required")
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "global"
	}
	if !strings.EqualFold(scope, "global") && u.agents != nil {
		if err := u.agents.AgentExists(ctx, scope); err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return Plugin{}, apierror.BadRequest("PLUGIN", "scope agent not found")
			}
			return Plugin{}, err
		}
	}
	return u.repo.UpdatePluginScope(ctx, id, scope)
}

// RecordRun persists a plugin run audit row.
func (u *Usecase) RecordRun(ctx context.Context, run Run) error {
	if u == nil || u.runs == nil {
		return nil
	}
	return u.runs.Insert(ctx, run)
}

// ListRuns returns paginated plugin runs.
func (u *Usecase) ListRuns(ctx context.Context, q RunQuery) (RunListResult, error) {
	if u == nil || u.runs == nil {
		return RunListResult{}, nil
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	return u.runs.List(ctx, q)
}

// DeleteAllRuns deletes plugin run records visible to the given workspace and
// returns the count deleted. empty workspaceID = 系统调用（全删）；
// non-empty = 租户调用（删共享行 + 自身行）。
func (u *Usecase) DeleteAllRuns(ctx context.Context, workspaceID string) (int32, error) {
	if u == nil || u.runs == nil {
		return 0, nil
	}
	return u.runs.DeleteAll(ctx, workspaceID)
}

// ── Schema validation ─────────────────────────────────────────────────────────

func ValidateJSONSchema(schemaJSON, docJSON string) error {
	return shared.ValidateDocumentAgainstSchema("PLUGIN", schemaJSON, docJSON)
}

// ── Cost guard ────────────────────────────────────────────────────────────────

// CostGuardUsageRepo persists daily token totals for cost_guard.
type CostGuardUsageRepo interface {
	GetTokens(ctx context.Context, usageDay, scopeKey string) (int, error)
	AddTokens(ctx context.Context, usageDay, scopeKey string, delta int) error
}
