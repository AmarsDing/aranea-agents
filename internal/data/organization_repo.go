package data

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/organization"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
)

type organizationRepo struct {
	data *Data
}

var _ biz.OrganizationRepo = (*organizationRepo)(nil)

func NewOrganizationRepo(d *Data) biz.OrganizationRepo {
	return &organizationRepo{data: d}
}

func entToBizOrganization(e *ent.Organization) biz.OrganizationNode {
	if e == nil {
		return biz.OrganizationNode{}
	}
	n := biz.OrganizationNode{
		ID:                 e.ID,
		Key:                e.OrgKey,
		Name:               e.Name,
		Description:        e.Description,
		Status:             e.Status,
		Enabled:            e.Enabled,
		SortOrder:          e.SortOrder,
		ParentID:           e.ParentID,
		Level:              e.Level,
		ScenarioKey:        e.ScenarioKey,
		WorkspaceID:        e.WorkspaceID,
		OwnerUserID:        e.OwnerUserID,
		IsSystem:           e.IsSystem,
		ConfigJSON:         e.ConfigJSON,
		MetadataJSON:       e.MetadataJSON,
		DeptLeadAgentID:    e.DeptLeadAgentID,
		DeptLeadConfigJSON: e.DeptLeadConfigJSON,
		CreatedAt:          e.CreatedAt,
		UpdatedAt:          e.UpdatedAt,
		DeletedAt:          e.DeletedAt,
	}
	biz.HydrateCompanyLeadFromMetadata(&n)
	return n
}

func (r *organizationRepo) ListOrgNodes(ctx context.Context) ([]biz.OrganizationNode, error) {
	rows, err := r.data.RW().Read(ctx).Organization.Query().
		Where(organization.DeletedAtEQ("")).
		Order(
			organization.BySortOrder(),
			organization.ByCreatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.OrganizationNode, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizOrganization(e))
	}
	return out, nil
}

func (r *organizationRepo) GetOrgNode(ctx context.Context, id string) (biz.OrganizationNode, error) {
	row, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.IDEQ(id),
			organization.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.OrganizationNode{}, apierror.NotFound(apierror.DomainOrg, "not found")
		}
		return biz.OrganizationNode{}, err
	}
	return entToBizOrganization(row), nil
}

func (r *organizationRepo) GetOrgNodeByKey(ctx context.Context, key string) (biz.OrganizationNode, error) {
	row, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.OrgKeyEQ(key),
			organization.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.OrganizationNode{}, apierror.NotFound(apierror.DomainOrg, "not found")
		}
		return biz.OrganizationNode{}, err
	}
	return entToBizOrganization(row), nil
}

func (r *organizationRepo) GetOrgNodeByKeyAnyState(ctx context.Context, key string) (biz.OrganizationNode, error) {
	row, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.OrgKeyEQ(key),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.OrganizationNode{}, apierror.NotFound(apierror.DomainOrg, "not found")
		}
		return biz.OrganizationNode{}, err
	}
	return entToBizOrganization(row), nil
}

func (r *organizationRepo) ListOrgNodesByParentID(ctx context.Context, parentID string) ([]biz.OrganizationNode, error) {
	rows, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.ParentIDEQ(parentID),
			organization.DeletedAtEQ(""),
		).
		Order(organization.BySortOrder()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.OrganizationNode, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizOrganization(e))
	}
	return out, nil
}

// ListOrgNodesByIDs returns org nodes matching the given IDs in a single query.
// Missing IDs are silently skipped. Returns nil for empty input.
func (r *organizationRepo) ListOrgNodesByIDs(ctx context.Context, ids []string) ([]biz.OrganizationNode, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.IDIn(ids...),
			organization.DeletedAtEQ(""),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.OrganizationNode, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizOrganization(e))
	}
	return out, nil
}

func (r *organizationRepo) ListOrgNodesByLevel(ctx context.Context, level string) ([]biz.OrganizationNode, error) {
	rows, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.LevelEQ(level),
			organization.DeletedAtEQ(""),
		).
		Order(organization.BySortOrder()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biz.OrganizationNode, 0, len(rows))
	for _, e := range rows {
		out = append(out, entToBizOrganization(e))
	}
	return out, nil
}

func (r *organizationRepo) CreateOrgNode(ctx context.Context, c biz.OrganizationNode) (biz.OrganizationNode, error) {
	now := nowRFC3339()
	if c.CreatedAt == "" {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	biz.ApplyCompanyLeadToMetadata(&c)
	saved, err := r.data.RW().Write(ctx).Organization.Create().
		SetID(c.ID).
		SetOrgKey(c.Key).
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
		SetDeptLeadAgentID(c.DeptLeadAgentID).
		SetDeptLeadConfigJSON(c.DeptLeadConfigJSON).
		SetCreatedAt(c.CreatedAt).
		SetUpdatedAt(c.UpdatedAt).
		SetDeletedAt("").
		Save(ctx)
	if err != nil {
		return biz.OrganizationNode{}, err
	}
	return entToBizOrganization(saved), nil
}

func (r *organizationRepo) UpdateOrgNode(ctx context.Context, c biz.OrganizationNode) (biz.OrganizationNode, error) {
	c.UpdatedAt = nowRFC3339()
	biz.ApplyCompanyLeadToMetadata(&c)
	update := r.data.RW().Write(ctx).Organization.UpdateOneID(c.ID).
		SetOrgKey(c.Key).
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
		SetDeptLeadAgentID(c.DeptLeadAgentID).
		SetDeptLeadConfigJSON(c.DeptLeadConfigJSON).
		SetUpdatedAt(c.UpdatedAt).
		SetDeletedAt(c.DeletedAt)
	err := update.Exec(ctx)
	if err != nil {
		return biz.OrganizationNode{}, err
	}
	return r.GetOrgNode(ctx, c.ID)
}

func (r *organizationRepo) DeleteOrgNode(ctx context.Context, id string) error {
	if err := r.ensureNodeCanDelete(ctx, id); err != nil {
		return err
	}
	now := nowRFC3339()
	return r.data.RW().Write(ctx).Organization.UpdateOneID(id).
		SetDeletedAt(now).
		SetStatus("deleted").
		SetUpdatedAt(now).
		Exec(ctx)
}

func (r *organizationRepo) ensureNodeCanDelete(ctx context.Context, id string) error {
	node, err := r.data.RW().Read(ctx).Organization.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return apierror.NotFound("ORGANIZATION", "node not found")
		}
		return err
	}
	if node.IsSystem {
		return apierror.BadRequest("ORGANIZATION", "system preset category cannot be deleted")
	}
	n, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.ParentIDEQ(id),
			organization.DeletedAtEQ(""),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return apierror.BadRequest("ORGANIZATION", fmt.Sprintf("node has %d child nodes", n))
	}
	nAgents, err := r.data.RW().Read(ctx).Agent.Query().
		Where(
			agent.PositionIDEQ(id),
			agent.DeletedAtEQ(""),
		).
		Count(ctx)
	if err != nil {
		return err
	}
	if nAgents > 0 {
		return apierror.BadRequest("ORGANIZATION", fmt.Sprintf("node is used by %d agents", nAgents))
	}
	return nil
}

func (r *organizationRepo) ReorderOrgNodes(ctx context.Context, ids []string) error {
	c := r.data.RW().Write(ctx)
	for i, id := range ids {
		_, err := c.Organization.UpdateOneID(id).
			SetSortOrder((i + 1) * 10).
			Save(ctx)
		if err != nil {
			return entErrToBizErr(err, "ORGANIZATION")
		}
	}
	return nil
}
