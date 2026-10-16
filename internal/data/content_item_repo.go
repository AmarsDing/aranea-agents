package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/data/ent"
	entci "aranea-agents/internal/data/ent/contentitem"
)

type contentItemRepo struct {
	data *Data
}

var _ content.Repo = (*contentItemRepo)(nil)

// NewContentItemRepo wires Ent-backed content.Repo.
func NewContentItemRepo(d *Data) content.Repo {
	return &contentItemRepo{data: d}
}

func (r *contentItemRepo) client(ctx context.Context) *ent.Client {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.data.RW().Write(ctx)
}

func (r *contentItemRepo) Create(item content.Item) (content.Item, error) {
	ctx := context.Background()
	row, err := r.client(ctx).ContentItem.Create().
		SetID(item.ID).
		SetWorkspaceID(item.WorkspaceID).
		SetOrgKey(item.OrgKey).
		SetSourceType(item.SourceType).
		SetSourceURL(item.SourceURL).
		SetTitle(item.Title).
		SetBody(item.Body).
		SetLang(item.Lang).
		SetMediaArtifactIdsJSON(mvpEncodeStrings(item.MediaArtifactIDs)).
		SetDedupeHash(item.DedupeHash).
		SetHotScore(item.HotScore).
		SetStatus(item.Status).
		SetRawPayload(item.RawPayload).
		SetCreatedAt(mvpTimeString(item.CreatedAt)).
		SetUpdatedAt(mvpTimeString(item.UpdatedAt)).
		Save(ctx)
	if err != nil {
		return content.Item{}, fmt.Errorf("content: create: %w", err)
	}
	return entToContentItem(row), nil
}

func (r *contentItemRepo) Get(id string) (content.Item, error) {
	ctx := context.Background()
	row, err := r.client(ctx).ContentItem.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return content.Item{}, fmt.Errorf("content: not found %s", id)
		}
		return content.Item{}, fmt.Errorf("content: get: %w", err)
	}
	return entToContentItem(row), nil
}

func (r *contentItemRepo) Update(item content.Item) (content.Item, error) {
	ctx := context.Background()
	row, err := r.client(ctx).ContentItem.UpdateOneID(item.ID).
		SetWorkspaceID(item.WorkspaceID).
		SetOrgKey(item.OrgKey).
		SetSourceType(item.SourceType).
		SetSourceURL(item.SourceURL).
		SetTitle(item.Title).
		SetBody(item.Body).
		SetLang(item.Lang).
		SetMediaArtifactIdsJSON(mvpEncodeStrings(item.MediaArtifactIDs)).
		SetDedupeHash(item.DedupeHash).
		SetHotScore(item.HotScore).
		SetStatus(item.Status).
		SetRawPayload(item.RawPayload).
		SetUpdatedAt(mvpTimeString(item.UpdatedAt)).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return content.Item{}, fmt.Errorf("content: not found %s", item.ID)
		}
		return content.Item{}, fmt.Errorf("content: update: %w", err)
	}
	return entToContentItem(row), nil
}

func (r *contentItemRepo) List(f content.ListFilter) ([]content.Item, int, error) {
	ctx := context.Background()
	q := r.client(ctx).ContentItem.Query()
	if f.WorkspaceID != "" {
		q = q.Where(entci.WorkspaceIDEQ(f.WorkspaceID))
	}
	if f.SourceType != "" {
		q = q.Where(entci.SourceTypeEQ(f.SourceType))
	}
	if f.Status != "" {
		q = q.Where(entci.StatusEQ(f.Status))
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("content: count: %w", err)
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.Offset(f.Offset).Limit(limit).Order(ent.Desc(entci.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("content: list: %w", err)
	}
	out := make([]content.Item, 0, len(rows))
	for _, row := range rows {
		out = append(out, entToContentItem(row))
	}
	return out, total, nil
}

func (r *contentItemRepo) GetByDedupeHash(workspaceID, hash string) (content.Item, bool, error) {
	ctx := context.Background()
	row, err := r.client(ctx).ContentItem.Query().
		Where(entci.WorkspaceIDEQ(workspaceID), entci.DedupeHashEQ(hash)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return content.Item{}, false, nil
		}
		return content.Item{}, false, fmt.Errorf("content: dedupe: %w", err)
	}
	return entToContentItem(row), true, nil
}

func entToContentItem(row *ent.ContentItem) content.Item {
	return content.Item{
		ID:               row.ID,
		WorkspaceID:      row.WorkspaceID,
		OrgKey:           row.OrgKey,
		SourceType:       row.SourceType,
		SourceURL:        row.SourceURL,
		Title:            row.Title,
		Body:             row.Body,
		Lang:             row.Lang,
		MediaArtifactIDs: mvpDecodeStrings(row.MediaArtifactIdsJSON),
		DedupeHash:       row.DedupeHash,
		HotScore:         row.HotScore,
		Status:           row.Status,
		RawPayload:       row.RawPayload,
		CreatedAt:        mvpParseTime(row.CreatedAt),
		UpdatedAt:        mvpParseTime(row.UpdatedAt),
	}
}
