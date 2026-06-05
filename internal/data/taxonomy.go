package data

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/industrytaxonomy"

	entsql "entgo.io/ent/dialect/sql"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type TaxonomyRepo struct {
	data *Data
}

var _ biz.TaxonomyRepo = (*TaxonomyRepo)(nil)

func NewTaxonomyRepo(d *Data) biz.TaxonomyRepo {
	return &TaxonomyRepo{data: d}
}

func entToBizTaxonomy(e *ent.IndustryTaxonomy) biz.TaxonomyNode {
	if e == nil {
		return biz.TaxonomyNode{}
	}
	return biz.TaxonomyNode{
		ID:           e.ID,
		Key:          e.TaxonomyKey,
		Name:         e.Name,
		Description:  e.Description,
		Status:       e.Status,
		Enabled:      e.Enabled,
		SortOrder:    e.SortOrder,
		ParentID:     e.ParentID,
		Level:        e.Level,
		ScenarioKey:  e.ScenarioKey,
		WorkspaceID:  e.WorkspaceID,
		OwnerUserID:  e.OwnerUserID,
		IsSystem:     e.IsSystem,
		ConfigJSON:   e.ConfigJSON,
		MetadataJSON: e.MetadataJSON,
		CreatedAt:    e.CreatedAt,
		UpdatedAt:    e.UpdatedAt,
		DeletedAt:    e.DeletedAt,
	}
}

func (r *TaxonomyRepo) ListTaxonomyNodes(ctx context.Context) ([]biz.TaxonomyNode, error) {
	rows, err := r.data.RW().Read(ctx).IndustryTaxonomy.Query().
		Where(industrytaxonomy.DeletedAtEQ("")).
		Order(
			industrytaxonomy.BySortOrder(),
			industrytaxonomy.ByCreatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.TaxonomyNode, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizTaxonomy(e))
	}
	return out, nil
}

func (r *TaxonomyRepo) GetTaxonomyNode(ctx context.Context, id string) (biz.TaxonomyNode, error) {
	row, err := r.data.RW().Read(ctx).IndustryTaxonomy.Query().
		Where(
			industrytaxonomy.IDEQ(id),
			industrytaxonomy.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.TaxonomyNode{}, sql.ErrNoRows
		}
		return biz.TaxonomyNode{}, err
	}
	return entToBizTaxonomy(row), nil
}

func (r *TaxonomyRepo) GetTaxonomyNodeByKey(ctx context.Context, key string) (biz.TaxonomyNode, error) {
	row, err := r.data.RW().Read(ctx).IndustryTaxonomy.Query().
		Where(
			industrytaxonomy.TaxonomyKeyEQ(key),
			industrytaxonomy.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.TaxonomyNode{}, sql.ErrNoRows
		}
		return biz.TaxonomyNode{}, err
	}
	return entToBizTaxonomy(row), nil
}

func (r *TaxonomyRepo) GetTaxonomyNodeByKeyAnyState(ctx context.Context, key string) (biz.TaxonomyNode, error) {
	row, err := r.data.RW().Read(ctx).IndustryTaxonomy.Query().
		Where(
			industrytaxonomy.TaxonomyKeyEQ(key),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.TaxonomyNode{}, sql.ErrNoRows
		}
		return biz.TaxonomyNode{}, err
	}
	return entToBizTaxonomy(row), nil
}

func (r *TaxonomyRepo) ListTaxonomyNodesByParentID(ctx context.Context, parentID string) ([]biz.TaxonomyNode, error) {
	rows, err := r.data.RW().Read(ctx).IndustryTaxonomy.Query().
		Where(
			industrytaxonomy.ParentIDEQ(parentID),
			industrytaxonomy.DeletedAtEQ(""),
		).
		Order(industrytaxonomy.BySortOrder()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.TaxonomyNode, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizTaxonomy(e))
	}
	return out, nil
}

func (r *TaxonomyRepo) ListTaxonomyNodesByLevel(ctx context.Context, level string) ([]biz.TaxonomyNode, error) {
	rows, err := r.data.RW().Read(ctx).IndustryTaxonomy.Query().
		Where(
			industrytaxonomy.LevelEQ(level),
			industrytaxonomy.DeletedAtEQ(""),
		).
		Order(industrytaxonomy.BySortOrder()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.TaxonomyNode, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizTaxonomy(e))
	}
	return out, nil
}

func (r *TaxonomyRepo) CreateTaxonomyNode(ctx context.Context, c biz.TaxonomyNode) (biz.TaxonomyNode, error) {
	now := nowRFC3339()
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	saved, err := r.data.RW().Write(ctx).IndustryTaxonomy.Create().
		SetID(c.ID).
		SetTaxonomyKey(c.Key).
		SetName(c.Name).
		SetDescription(c.Description).
		SetStatus(c.Status).
		SetEnabled(c.Enabled).
		SetSortOrder(c.SortOrder).
		SetParentID(c.ParentID).
		SetLevel(c.Level).
		SetScenarioKey(c.ScenarioKey).
		SetWorkspaceID(c.WorkspaceID).
		SetOwnerUserID(c.OwnerUserID).
		SetIsSystem(c.IsSystem).
		SetConfigJSON(c.ConfigJSON).
		SetMetadataJSON(c.MetadataJSON).
		SetCreatedAt(c.CreatedAt).
		SetUpdatedAt(c.UpdatedAt).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		return biz.TaxonomyNode{}, err
	}
	return entToBizTaxonomy(saved), nil
}

func (r *TaxonomyRepo) UpdateTaxonomyNode(ctx context.Context, c biz.TaxonomyNode) (biz.TaxonomyNode, error) {
	c.UpdatedAt = nowRFC3339()
	update := r.data.RW().Write(ctx).IndustryTaxonomy.UpdateOneID(c.ID).
		SetTaxonomyKey(c.Key).
		SetName(c.Name).
		SetDescription(c.Description).
		SetStatus(c.Status).
		SetEnabled(c.Enabled).
		SetSortOrder(c.SortOrder).
		SetParentID(c.ParentID).
		SetLevel(c.Level).
		SetScenarioKey(c.ScenarioKey).
		SetWorkspaceID(c.WorkspaceID).
		SetOwnerUserID(c.OwnerUserID).
		SetIsSystem(c.IsSystem).
		SetConfigJSON(c.ConfigJSON).
		SetMetadataJSON(c.MetadataJSON).
		SetUpdatedAt(c.UpdatedAt).
		SetDeletedAt(c.DeletedAt)
	err := update.Exec(ctx)
	if err != nil {
		return biz.TaxonomyNode{}, err
	}
	return r.GetTaxonomyNode(ctx, c.ID)
}

func (r *TaxonomyRepo) DeleteTaxonomyNode(ctx context.Context, id string) error {
	if err := r.ensureNodeCanDelete(ctx, id); err != nil {
		return err
	}
	now := nowRFC3339()
	return r.data.RW().Write(ctx).IndustryTaxonomy.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Exec(ctx)
}

func (r *TaxonomyRepo) ensureNodeCanDelete(ctx context.Context, id string) error {
	n, err := r.data.RW().Read(ctx).IndustryTaxonomy.Query().
		Where(
			industrytaxonomy.ParentIDEQ(id),
			industrytaxonomy.DeletedAtEQ(""),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return kerrors.BadRequest("TAXONOMY", fmt.Sprintf("node has %d child nodes", n))
	}
	nAgents, err := r.data.RW().Read(ctx).Agent.Query().
		Where(
			agent.TaxonomyPositionIDEQ(id),
			agent.DeletedAtEQ(""),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if nAgents > 0 {
		return kerrors.BadRequest("TAXONOMY", fmt.Sprintf("node is used by %d agents", nAgents))
	}
	return nil
}

func (r *TaxonomyRepo) ReorderTaxonomyNodes(ctx context.Context, ids []string) error {
	c := r.data.RW().Write(ctx)
	for i, id := range ids {
		_, err := c.IndustryTaxonomy.UpdateOneID(id).
			SetSortOrder(i + 1).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("taxonomy repo reorder [%s]: %w", id, err)
		}
	}
	return nil
}
