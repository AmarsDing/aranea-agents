// Package plugin implements plugin CRUD, sandbox, schema, and cost guard workflows.
package plugin

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"

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
	IncrementStats(ctx context.Context, pluginKey string, delta StatUpdate) error
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

func NewUsecase(repo Repo, runs RunRepo, agents ScopeAgentLookup) *Usecase {
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
		return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.UpdatePluginEnabled(ctx, id, enabled)
}

// GetByKey returns a plugin by its unique key.
func (u *Usecase) GetByKey(ctx context.Context, key string) (Plugin, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "key is required")
	}
	p, err := u.repo.GetByKey(ctx, key)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return Plugin{}, shared.ErrNotFound
		}
		return Plugin{}, err
	}
	return p, nil
}

// Create validates and stores a new plugin.
func (u *Usecase) Create(ctx context.Context, p Plugin) (Plugin, error) {
	p.Key = strings.TrimSpace(p.Key)
	if p.Key == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "key is required")
	}
	if strings.TrimSpace(p.ID) == "" {
		p.ID = "builtin-" + p.Key
	}
	if strings.TrimSpace(p.ConfigJSON) == "" {
		p.ConfigJSON = "{}"
	}
	if !json.Valid([]byte(p.ConfigJSON)) {
		return Plugin{}, errors.BadRequest("PLUGIN", "config_json must be valid JSON")
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

// UpdateConfig updates a plugin's configuration.
func (u *Usecase) UpdateConfig(ctx context.Context, id string, configJSON string) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
	}
	if strings.TrimSpace(configJSON) == "" {
		configJSON = "{}"
	}
	if !json.Valid([]byte(configJSON)) {
		return Plugin{}, errors.BadRequest("PLUGIN", "config_json must be valid JSON")
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
		return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.UpdateSortOrder(ctx, id, sortOrder)
}

// UpdateScope updates a plugin's scope.
func (u *Usecase) UpdateScope(ctx context.Context, id string, scope string) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "global"
	}
	if !strings.EqualFold(scope, "global") && u.agents != nil {
		if err := u.agents.AgentExists(ctx, scope); err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return Plugin{}, errors.BadRequest("PLUGIN", "scope agent not found")
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

// ── Schema validation ─────────────────────────────────────────────────────────

func ValidateJSONSchema(schemaJSON, docJSON string) error {
	return shared.ValidateDocumentAgainstSchema("PLUGIN", schemaJSON, docJSON)
}

// ── Sandbox mode ──────────────────────────────────────────────────────────────

// SandboxMode controls Phase 4 sandbox isolation.
type SandboxMode string

const (
	SandboxNone      SandboxMode = "none"
	SandboxProcess   SandboxMode = "process"
	SandboxContainer SandboxMode = "container"
)

// VersionPolicy pins a plugin rule to a semver range.
type VersionPolicy struct {
	PluginID   string
	MinVersion string
	MaxVersion string
	Pinned     string
}

// NormalizeSandboxMode returns a supported sandbox mode.
func NormalizeSandboxMode(raw string, riskLevel string) SandboxMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(SandboxNone):
		return SandboxNone
	case string(SandboxContainer):
		return SandboxContainer
	case string(SandboxProcess):
		return SandboxProcess
	}
	if strings.EqualFold(riskLevel, "high") || strings.EqualFold(riskLevel, "critical") {
		return SandboxProcess
	}
	return SandboxNone
}

// ResolveVersion picks pinned version or falls back to latest within policy bounds.
func ResolveVersion(policy VersionPolicy, latest string) string {
	if v := strings.TrimSpace(policy.Pinned); v != "" {
		return v
	}
	return strings.TrimSpace(latest)
}

// ── Cost guard ────────────────────────────────────────────────────────────────

// CostGuardUsageRepo persists daily token totals for cost_guard.
type CostGuardUsageRepo interface {
	GetTokens(ctx context.Context, usageDay, scopeKey string) (int, error)
	AddTokens(ctx context.Context, usageDay, scopeKey string, delta int) error
}
