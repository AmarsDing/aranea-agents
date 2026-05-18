package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformplugin"

	entsql "entgo.io/ent/dialect/sql"
)

type pluginRepo struct {
	data *Data
}

func NewPluginRepo(d *Data) biz.PluginRepo {
	return &pluginRepo{data: d}
}

func entToBizPlugin(e *ent.PlatformPlugin) biz.Plugin {
	if e == nil {
		return biz.Plugin{}
	}
	var cbs []string
	_ = json.Unmarshal([]byte(e.CallbackPointsJSON), &cbs)
	return biz.Plugin{
		ID:                e.ID,
		Key:               e.PluginKey,
		Name:              e.Name,
		Description:       e.Description,
		Category:          e.Category,
		RiskLevel:         e.RiskLevel,
		Enabled:           e.Enabled,
		Scope:             e.Scope,
		CallbackPoints:    cbs,
		SortOrder:         e.SortOrder,
		ConfigSchemaJSON:  e.ConfigSchemaJSON,
		ConfigJSON:        e.ConfigJSON,
		DefaultConfigJSON: e.FallbackConfigJSON,
		InvokeCount:       e.InvokeCount,
		BlockCount:        e.BlockCount,
		ErrorCount:        e.ErrorCount,
		LastInvokedAt:     e.LastInvokedAt,
		LastStatus:        e.LastStatus,
		CreatedAt:         e.CreatedAt,
		UpdatedAt:         e.UpdatedAt,
		Permissions:       biz.AdminPluginPerms(),
	}
}

func (r *pluginRepo) pluginSearchQuery(q biz.PluginListQuery) *ent.PlatformPluginQuery {
	pq := r.data.entClient.PlatformPlugin.Query().Where(platformplugin.DeletedAtEQ(""))
	if s := strings.TrimSpace(q.Search); s != "" {
		pq = pq.Where(
			platformplugin.Or(
				platformplugin.PluginKeyContainsFold(s),
				platformplugin.NameContainsFold(s),
				platformplugin.DescriptionContainsFold(s),
			),
		)
	}
	if cat := strings.TrimSpace(q.Category); cat != "" {
		pq = pq.Where(platformplugin.CategoryEQ(cat))
	}
	switch strings.TrimSpace(strings.ToLower(q.Enabled)) {
	case "true":
		pq = pq.Where(platformplugin.EnabledEQ(true))
	case "false":
		pq = pq.Where(platformplugin.EnabledEQ(false))
	}
	if cp := strings.TrimSpace(q.CallbackPoint); cp != "" {
		pq = pq.Where(platformplugin.CallbackPointsJSONContainsFold(cp))
	}
	return pq
}

func (r *pluginRepo) SearchPlugins(ctx context.Context, q biz.PluginListQuery) (biz.PluginListResult, error) {
	base := r.pluginSearchQuery(q)
	total, err := base.Count(ctx)
	if err != nil {
		return biz.PluginListResult{}, err
	}
	rows, err := r.pluginSearchQuery(q).
		Order(
			platformplugin.BySortOrder(),
			platformplugin.ByCreatedAt(entsql.OrderDesc()),
		).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.PluginListResult{}, err
	}
	items := make([]biz.Plugin, 0, len(rows))
	for _, e := range rows {
		items = append(items, entToBizPlugin(e))
	}
	return biz.PluginListResult{
		Items:  items,
		Total:  total,
		Limit:  q.Limit,
		Offset: q.Offset,
	}, nil
}

func (r *pluginRepo) GetPlugin(ctx context.Context, id string) (biz.Plugin, error) {
	row, err := r.data.entClient.PlatformPlugin.Query().
		Where(platformplugin.IDEQ(id), platformplugin.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Plugin{}, sql.ErrNoRows
		}
		return biz.Plugin{}, err
	}
	return entToBizPlugin(row), nil
}

func (r *pluginRepo) UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (biz.Plugin, error) {
	err := r.data.entClient.PlatformPlugin.UpdateOneID(id).
		SetEnabled(enabled).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Plugin{}, sql.ErrNoRows
		}
		return biz.Plugin{}, err
	}
	return r.GetPlugin(ctx, id)
}

func (r *pluginRepo) UpdatePluginConfig(ctx context.Context, id string, configJSON string) (biz.Plugin, error) {
	err := r.data.entClient.PlatformPlugin.UpdateOneID(id).
		SetConfigJSON(configJSON).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Plugin{}, sql.ErrNoRows
		}
		return biz.Plugin{}, err
	}
	return r.GetPlugin(ctx, id)
}

func (r *pluginRepo) UpdateSortOrder(ctx context.Context, id string, sortOrder int) (biz.Plugin, error) {
	err := r.data.entClient.PlatformPlugin.UpdateOneID(id).
		SetSortOrder(sortOrder).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Plugin{}, sql.ErrNoRows
		}
		return biz.Plugin{}, err
	}
	return r.GetPlugin(ctx, id)
}
