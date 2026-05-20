package biz

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// Plugin matches persisted plugins row + runtime permissions (admin UI defaults full).
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
	Permissions       PluginPermissions
}

// PluginPermissions mirrors legacy JSON for admin UI.
type PluginPermissions struct {
	CanView       bool
	CanToggle     bool
	CanEditConfig bool
	CanViewLogs   bool
}

// PluginListQuery filters ListPlugins (enabled: "", "true", "false" string tri-state).
type PluginListQuery struct {
	Search        string
	Category      string
	Enabled       string
	CallbackPoint string
	Limit         int
	Offset        int
}

// PluginListResult is one page of plugins.
type PluginListResult struct {
	Items  []Plugin
	Total  int
	Limit  int
	Offset int
}

func AdminPluginPerms() PluginPermissions {
	return PluginPermissions{CanView: true, CanToggle: true, CanEditConfig: true, CanViewLogs: true}
}

// PluginStatUpdate is a delta applied to persisted plugin invocation counters.
type PluginStatUpdate struct {
	InvokeCount int
	BlockDelta  int
	ErrorDelta  int
	LastStatus  string
}

type PluginRepo interface {
	SearchPlugins(ctx context.Context, q PluginListQuery) (PluginListResult, error)
	GetPlugin(ctx context.Context, id string) (Plugin, error)
	GetByKey(ctx context.Context, key string) (Plugin, error)
	CreatePlugin(ctx context.Context, p Plugin) (Plugin, error)
	UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (Plugin, error)
	UpdatePluginConfig(ctx context.Context, id string, configJSON string) (Plugin, error)
	UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error)
	UpdatePluginScope(ctx context.Context, id string, scope string) (Plugin, error)
	IncrementStats(ctx context.Context, pluginKey string, delta PluginStatUpdate) error
}

// PluginRun is one plugin callback invocation audit row.
type PluginRun struct {
	ID             string
	PluginKey      string
	PluginID       string
	SessionID      string
	AgentID        string
	CallbackPoint  string
	Status         string
	DurationMS     int
	DetailJSON     string
	CreatedAt      string
}

type PluginRunQuery struct {
	PluginKey string
	PluginID  string
	SessionID string
	Limit     int
	Offset    int
}

type PluginRunListResult struct {
	Items  []PluginRun
	Total  int32
	Limit  int
	Offset int
}

type PluginRunRepo interface {
	Insert(ctx context.Context, run PluginRun) error
	List(ctx context.Context, q PluginRunQuery) (PluginRunListResult, error)
}

type PluginUsecase struct {
	repo PluginRepo
	runs PluginRunRepo
}

func NewPluginUsecase(repo PluginRepo, runs PluginRunRepo) *PluginUsecase {
	return &PluginUsecase{repo: repo, runs: runs}
}

func (u *PluginUsecase) List(ctx context.Context, q PluginListQuery) (PluginListResult, error) {
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

func (u *PluginUsecase) ToggleEnabled(ctx context.Context, id string, enabled bool) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.UpdatePluginEnabled(ctx, id, enabled)
}

func (u *PluginUsecase) GetByKey(ctx context.Context, key string) (Plugin, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "key is required")
	}
	p, err := u.repo.GetByKey(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Plugin{}, sql.ErrNoRows
		}
		return Plugin{}, err
	}
	return p, nil
}

func (u *PluginUsecase) Create(ctx context.Context, p Plugin) (Plugin, error) {
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
		if err := validateJSONSchema(schema, p.ConfigJSON); err != nil {
			return Plugin{}, err
		}
	}
	if p.Scope == "" {
		p.Scope = "global"
	}
	return u.repo.CreatePlugin(ctx, p)
}

func (u *PluginUsecase) UpdateConfig(ctx context.Context, id string, configJSON string) (Plugin, error) {
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
		if err := validateJSONSchema(schema, configJSON); err != nil {
			return Plugin{}, err
		}
	}
	return u.repo.UpdatePluginConfig(ctx, id, configJSON)
}

func (u *PluginUsecase) UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.UpdateSortOrder(ctx, id, sortOrder)
}

func (u *PluginUsecase) UpdateScope(ctx context.Context, id string, scope string) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "global"
	}
	return u.repo.UpdatePluginScope(ctx, id, scope)
}

func (u *PluginUsecase) RecordRun(ctx context.Context, run PluginRun) error {
	if u == nil || u.runs == nil {
		return nil
	}
	return u.runs.Insert(ctx, run)
}

func (u *PluginUsecase) ListRuns(ctx context.Context, q PluginRunQuery) (PluginRunListResult, error) {
	if u == nil || u.runs == nil {
		return PluginRunListResult{}, nil
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	return u.runs.List(ctx, q)
}
