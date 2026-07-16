package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	bizhook "aranea-agents/internal/biz/hook"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformhook"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

type hookRepo struct {
	data *Data
}

var _ bizhook.Repo = (*hookRepo)(nil)

// NewHookRepo implements biz.HookRepo for table hooks.
func NewHookRepo(d *Data) biz.HookRepo {
	return &hookRepo{data: d}
}

func entToBizHook(e *ent.PlatformHook) biz.Hook {
	if e == nil {
		return biz.Hook{}
	}
	return biz.Hook{
		ID:           e.ID,
		Key:          e.HookKey,
		Name:         e.Name,
		Description:  e.Description,
		Status:       e.Status,
		Enabled:      e.Enabled,
		SortOrder:    e.SortOrder,
		ConfigJSON:   e.ConfigJSON,
		MetadataJSON: e.MetadataJSON,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
		DeletedAt:    e.DeletedAt,
	}
}

func (r *hookRepo) ListHooks(ctx context.Context) ([]biz.Hook, error) {
	rows, err := r.data.RW().Read(ctx).PlatformHook.Query().
		Where(platformhook.DeletedAtEQ("")).
		Order(
			platformhook.BySortOrder(),
			platformhook.ByCreatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.Hook, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizHook(e))
	}
	return out, nil
}

func (r *hookRepo) hookListQuery(ctx context.Context, q bizhook.ListQuery) *ent.PlatformHookQuery {
	pq := r.data.RW().Read(ctx).PlatformHook.Query().Where(platformhook.DeletedAtEQ(""))
	if s := strings.TrimSpace(q.Search); s != "" {
		pq = pq.Where(platformhook.Or(
			platformhook.HookKeyContainsFold(s),
			platformhook.NameContainsFold(s),
			platformhook.DescriptionContainsFold(s),
		))
	}
	if cp := strings.TrimSpace(q.CallbackPoint); cp != "" {
		pq = pq.Where(platformhook.ConfigJSONContainsFold(`"callback_point":"` + cp + `"`))
	}
	return pq
}

func (r *hookRepo) ListHooksPaged(ctx context.Context, q bizhook.ListQuery) (bizhook.ListResult, error) {
	total, err := r.hookListQuery(ctx, q).Count(ctx)
	if err != nil {
		return bizhook.ListResult{}, err
	}
	rows, err := r.hookListQuery(ctx, q).
		Order(
			platformhook.BySortOrder(),
			platformhook.ByCreatedAt(entsql.OrderDesc()),
		).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return bizhook.ListResult{}, err
	}
	out := make([]biz.Hook, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizHook(e))
	}
	return bizhook.ListResult{Items: out, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *hookRepo) GetHook(ctx context.Context, id string) (biz.Hook, error) {
	row, err := r.data.RW().Read(ctx).PlatformHook.Query().
		Where(platformhook.IDEQ(id), platformhook.DeletedAtEQ("")).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Hook{}, apierror.NotFound(apierror.DomainHook, "not found")
		}
		return biz.Hook{}, err
	}
	return entToBizHook(row), nil
}

func (r *hookRepo) CreateHook(ctx context.Context, h biz.Hook) (biz.Hook, error) {
	now := nowRFC3339()
	if h.CreatedAt == "" {
		h.CreatedAt = now
	}
	h.UpdatedAt = now
	saved, err := r.data.RW().Write(ctx).PlatformHook.Create().
		SetID(h.ID).
		SetHookKey(h.Key).
		SetName(h.Name).
		SetDescription(h.Description).
		SetStatus(h.Status).
		SetEnabled(h.Enabled).
		SetSortOrder(h.SortOrder).
		SetConfigJSON(h.ConfigJSON).
		SetMetadataJSON(h.MetadataJSON).
		SetCreatedAt(h.CreatedAt).
		SetUpdatedAt(h.UpdatedAt).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		return biz.Hook{}, err
	}
	return entToBizHook(saved), nil
}

func (r *hookRepo) UpdateHook(ctx context.Context, h biz.Hook) (biz.Hook, error) {
	h.UpdatedAt = nowRFC3339()
	err := r.data.RW().Write(ctx).PlatformHook.UpdateOneID(h.ID).
		SetHookKey(h.Key).
		SetName(h.Name).
		SetDescription(h.Description).
		SetStatus(h.Status).
		SetEnabled(h.Enabled).
		SetSortOrder(h.SortOrder).
		SetConfigJSON(h.ConfigJSON).
		SetMetadataJSON(h.MetadataJSON).
		SetUpdatedAt(h.UpdatedAt).
		Exec(ctx)
	if err != nil {
		return biz.Hook{}, err
	}
	return r.GetHook(ctx, h.ID)
}

func (r *hookRepo) DeleteHook(ctx context.Context, id string) error {
	now := nowRFC3339()
	return r.data.RW().Write(ctx).PlatformHook.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Exec(ctx)
}
