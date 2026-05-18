package biz

import (
	"context"
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

type PluginRepo interface {
	SearchPlugins(ctx context.Context, q PluginListQuery) (PluginListResult, error)
	GetPlugin(ctx context.Context, id string) (Plugin, error)
	UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (Plugin, error)
	UpdatePluginConfig(ctx context.Context, id string, configJSON string) (Plugin, error)
	UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error)
}

type PluginUsecase struct {
	repo PluginRepo
}

func NewPluginUsecase(repo PluginRepo) *PluginUsecase {
	return &PluginUsecase{repo: repo}
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
	return u.repo.UpdatePluginConfig(ctx, id, configJSON)
}

func (u *PluginUsecase) UpdateSortOrder(ctx context.Context, id string, sortOrder int) (Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return Plugin{}, errors.BadRequest("PLUGIN", "id is required")
	}
	return u.repo.UpdateSortOrder(ctx, id, sortOrder)
}
