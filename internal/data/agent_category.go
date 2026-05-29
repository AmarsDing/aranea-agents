package data

import (
	"context"
	"database/sql"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/agentcategory"

	entsql "entgo.io/ent/dialect/sql"
)

type agentCategoryRepo struct {
	data *Data
}

var _ biz.AgentCategoryRepo = (*agentCategoryRepo)(nil)

// NewAgentCategoryRepo registers agent category persistence.
func NewAgentCategoryRepo(d *Data) biz.AgentCategoryRepo {
	return &agentCategoryRepo{data: d}
}

func entToBizCat(e *ent.AgentCategory) biz.AgentCategory {
	if e == nil {
		return biz.AgentCategory{}
	}
	return biz.AgentCategory{
		ID:           e.ID,
		Key:          e.CategoryKey,
		Name:         e.Name,
		Description:  e.Description,
		Status:       e.Status,
		Enabled:      e.Enabled,
		SortOrder:    e.SortOrder,
		ParentID:     e.ParentID,
		Level:        e.Level,
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

func (r *agentCategoryRepo) ListAgentCategories(ctx context.Context) ([]biz.AgentCategory, error) {
	rows, err := r.data.entClient.AgentCategory.Query().
		Where(agentcategory.DeletedAtEQ("")).
		Order(
			agentcategory.BySortOrder(),
			agentcategory.ByCreatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.AgentCategory, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizCat(e))
	}
	return out, nil
}

func (r *agentCategoryRepo) GetAgentCategory(ctx context.Context, id string) (biz.AgentCategory, error) {
	row, err := r.data.entClient.AgentCategory.Query().
		Where(
			agentcategory.IDEQ(id),
			agentcategory.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.AgentCategory{}, sql.ErrNoRows
		}
		return biz.AgentCategory{}, err
	}
	return entToBizCat(row), nil
}

func (r *agentCategoryRepo) CreateAgentCategory(ctx context.Context, c biz.AgentCategory) (biz.AgentCategory, error) {
	now := nowRFC3339()
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	saved, err := r.data.entClient.AgentCategory.Create().
		SetID(c.ID).
		SetCategoryKey(c.Key).
		SetName(c.Name).
		SetDescription(c.Description).
		SetStatus(c.Status).
		SetEnabled(c.Enabled).
		SetSortOrder(c.SortOrder).
		SetParentID(c.ParentID).
		SetLevel(c.Level).
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
		return biz.AgentCategory{}, err
	}
	return entToBizCat(saved), nil
}

func (r *agentCategoryRepo) UpdateAgentCategory(ctx context.Context, c biz.AgentCategory) (biz.AgentCategory, error) {
	c.UpdatedAt = nowRFC3339()
	err := r.data.entClient.AgentCategory.UpdateOneID(c.ID).
		SetCategoryKey(c.Key).
		SetName(c.Name).
		SetDescription(c.Description).
		SetStatus(c.Status).
		SetEnabled(c.Enabled).
		SetSortOrder(c.SortOrder).
		SetParentID(c.ParentID).
		SetLevel(c.Level).
		SetWorkspaceID(c.WorkspaceID).
		SetOwnerUserID(c.OwnerUserID).
		SetIsSystem(c.IsSystem).
		SetConfigJSON(c.ConfigJSON).
		SetMetadataJSON(c.MetadataJSON).
		SetUpdatedAt(c.UpdatedAt).
		Exec(ctx)
	if err != nil {
		return biz.AgentCategory{}, err
	}
	return r.GetAgentCategory(ctx, c.ID)
}

func (r *agentCategoryRepo) DeleteAgentCategory(ctx context.Context, id string) error {
	if err := r.ensureCategoryCanDelete(ctx, id); err != nil {
		return err
	}
	now := nowRFC3339()
	return r.data.entClient.AgentCategory.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Exec(ctx)
}

func (r *agentCategoryRepo) ensureCategoryCanDelete(ctx context.Context, id string) error {
	n, err := r.data.entClient.AgentCategory.Query().
		Where(
			agentcategory.ParentIDEQ(id),
			agentcategory.DeletedAtEQ(""),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("category has %d child nodes", n)
	}
	nAgents, err := r.data.entClient.Agent.Query().
		Where(
			agent.CategoryPositionIDEQ(id),
			agent.DeletedAtEQ(""),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if nAgents > 0 {
		return fmt.Errorf("category is used by %d agents", nAgents)
	}
	return nil
}
