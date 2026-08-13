package data

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
	bizplugin "aranea-agents/internal/biz/plugin"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformplugin"
	"aranea-agents/pkg/loggateway"

	entsql "entgo.io/ent/dialect/sql"
)

type pluginRepo struct {
	data *Data
}

var _ bizplugin.Repo = (*pluginRepo)(nil)

func NewPluginRepo(d *Data) biz.PluginRepo {
	return &pluginRepo{data: d}
}

func entToBizPlugin(lg loggateway.Logger, e *ent.PlatformPlugin) biz.Plugin {
	if e == nil {
		return biz.Plugin{}
	}
	var cbs []string
	if err := json.Unmarshal([]byte(e.CallbackPointsJSON), &cbs); err != nil {
		lg.Warn("unmarshal plugin callback_points failed", loggateway.StepID("data.plugin"), loggateway.Err(err))
	}
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
		WorkspaceID:       e.WorkspaceID, // P2-B: tenant isolation
	}
}

func (r *pluginRepo) pluginSearchQuery(ctx context.Context, q biz.PluginListQuery) *ent.PlatformPluginQuery {
	pq := r.data.RW().Read(ctx).PlatformPlugin.Query().Where(platformplugin.DeletedAtEQ(""))
	// P2-B: workspace visibility filter.
	// empty WorkspaceID = system caller (see all); non-empty = tenant caller (shared + own).
	if ws := strings.TrimSpace(q.WorkspaceID); ws != "" {
		pq = pq.Where(platformplugin.Or(
			platformplugin.WorkspaceIDEQ(""),
			platformplugin.WorkspaceIDEQ(ws),
		))
	}
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
	base := r.pluginSearchQuery(ctx, q)
	total, err := base.Count(ctx)
	if err != nil {
		return biz.PluginListResult{}, entErrToBizErr(err, "PLUGIN")
	}
	rows, err := r.pluginSearchQuery(ctx, q).
		Order(
			platformplugin.BySortOrder(),
			platformplugin.ByCreatedAt(entsql.OrderDesc()),
		).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.PluginListResult{}, entErrToBizErr(err, "PLUGIN")
	}
	items := make([]biz.Plugin, 0, len(rows))
	for _, e := range rows {
		items = append(items, entToBizPlugin(r.data.lg, e))
	}
	return biz.PluginListResult{
		Items:  items,
		Total:  total,
		Limit:  q.Limit,
		Offset: q.Offset,
	}, nil
}

func (r *pluginRepo) GetByKey(ctx context.Context, key string) (biz.Plugin, error) {
	row, err := r.data.RW().Read(ctx).PlatformPlugin.Query().
		Where(platformplugin.PluginKeyEQ(key), platformplugin.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return biz.Plugin{}, entErrToBizErr(err, "PLUGIN")
	}
	return entToBizPlugin(r.data.lg, row), nil
}

func (r *pluginRepo) CreatePlugin(ctx context.Context, p biz.Plugin) (biz.Plugin, error) {
	cbsJSON, _ := json.Marshal(p.CallbackPoints)
	if len(p.CallbackPoints) == 0 {
		cbsJSON = []byte("[]")
	}
	cfg := p.ConfigJSON
	if cfg == "" {
		cfg = "{}"
	}
	fallback := p.DefaultConfigJSON
	if fallback == "" {
		fallback = cfg
	}
	schema := p.ConfigSchemaJSON
	if schema == "" {
		schema = "{}"
	}
	now := nowRFC3339()
	row, err := r.data.RW().Write(ctx).PlatformPlugin.Create().
		SetID(p.ID).
		SetPluginKey(p.Key).
		SetName(p.Name).
		SetDescription(p.Description).
		SetCategory(p.Category).
		SetRiskLevel(p.RiskLevel).
		SetEnabled(p.Enabled).
		SetScope(p.Scope).
		SetCallbackPointsJSON(string(cbsJSON)).
		SetSortOrder(p.SortOrder).
		SetConfigSchemaJSON(schema).
		SetConfigJSON(cfg).
		SetFallbackConfigJSON(fallback).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		SetWorkspaceID(p.WorkspaceID). // P2-B: tenant isolation
		Save(ctx)
	if err != nil {
		return biz.Plugin{}, entErrToBizErr(err, "PLUGIN")
	}
	return entToBizPlugin(r.data.lg, row), nil
}

func (r *pluginRepo) GetPlugin(ctx context.Context, id string) (biz.Plugin, error) {
	row, err := r.data.RW().Read(ctx).PlatformPlugin.Query().
		Where(platformplugin.IDEQ(id), platformplugin.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		return biz.Plugin{}, entErrToBizErr(err, "PLUGIN")
	}
	return entToBizPlugin(r.data.lg, row), nil
}

func (r *pluginRepo) UpdatePluginEnabled(ctx context.Context, id string, enabled bool) (biz.Plugin, error) {
	err := r.data.RW().Write(ctx).PlatformPlugin.UpdateOneID(id).
		SetEnabled(enabled).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		return biz.Plugin{}, entErrToBizErr(err, "PLUGIN")
	}
	return r.GetPlugin(ctx, id)
}

func (r *pluginRepo) UpdatePluginConfig(ctx context.Context, id string, configJSON string) (biz.Plugin, error) {
	err := r.data.RW().Write(ctx).PlatformPlugin.UpdateOneID(id).
		SetConfigJSON(configJSON).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		return biz.Plugin{}, entErrToBizErr(err, "PLUGIN")
	}
	return r.GetPlugin(ctx, id)
}

func (r *pluginRepo) UpdateSortOrder(ctx context.Context, id string, sortOrder int) (biz.Plugin, error) {
	err := r.data.RW().Write(ctx).PlatformPlugin.UpdateOneID(id).
		SetSortOrder(sortOrder).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		return biz.Plugin{}, entErrToBizErr(err, "PLUGIN")
	}
	return r.GetPlugin(ctx, id)
}

func (r *pluginRepo) UpdatePluginScope(ctx context.Context, id string, scope string) (biz.Plugin, error) {
	err := r.data.RW().Write(ctx).PlatformPlugin.UpdateOneID(id).
		SetScope(strings.TrimSpace(scope)).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		return biz.Plugin{}, entErrToBizErr(err, "PLUGIN")
	}
	return r.GetPlugin(ctx, id)
}

// SyncBuiltinMeta 仅更新平台自有元数据字段；enabled/config_json/sort_order/
// scope/workspace_id 等管理员字段保持不变（bootstrap 种子同步用）。
func (r *pluginRepo) SyncBuiltinMeta(ctx context.Context, p biz.Plugin) (biz.Plugin, error) {
	cbsJSON, _ := json.Marshal(p.CallbackPoints)
	if len(p.CallbackPoints) == 0 {
		cbsJSON = []byte("[]")
	}
	err := r.data.RW().Write(ctx).PlatformPlugin.UpdateOneID(p.ID).
		SetName(p.Name).
		SetDescription(p.Description).
		SetCategory(p.Category).
		SetRiskLevel(p.RiskLevel).
		SetCallbackPointsJSON(string(cbsJSON)).
		SetConfigSchemaJSON(p.ConfigSchemaJSON).
		SetFallbackConfigJSON(p.DefaultConfigJSON).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		return biz.Plugin{}, entErrToBizErr(err, "PLUGIN")
	}
	return r.GetPlugin(ctx, p.ID)
}

// IncrementStats 原子增量计数（P3-6）：单条 UPDATE ... SET count = count + delta，
// 无读-改-写窗口，多实例/并发调用安全。pluginKey 不存在时静默 no-op（与原语义一致）。
func (r *pluginRepo) IncrementStats(ctx context.Context, pluginKey string, delta biz.PluginStatUpdate) error {
	pluginKey = strings.TrimSpace(pluginKey)
	if pluginKey == "" {
		return nil
	}
	upd := r.data.RW().Write(ctx).PlatformPlugin.Update().
		Where(platformplugin.PluginKeyEQ(pluginKey), platformplugin.DeletedAtEQ("")).
		AddInvokeCount(delta.InvokeCount).
		AddBlockCount(delta.BlockDelta).
		AddErrorCount(delta.ErrorDelta).
		SetLastInvokedAt(nowRFC3339()).
		SetUpdatedAt(nowRFC3339())
	if status := strings.TrimSpace(delta.LastStatus); status != "" {
		upd = upd.SetLastStatus(status)
	}
	return entErrToBizErr(upd.Exec(ctx), "PLUGIN")
}
