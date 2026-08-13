package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/organization"
	"aranea-agents/internal/data/ent/predicate"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *agentRepo) categoryPositionIDsForFilter(ctx context.Context, categoryID string) ([]string, error) {
	categoryID = strings.TrimSpace(categoryID)
	if categoryID == "" {
		return nil, nil
	}
	node, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.IDEQ(categoryID),
			organization.DeletedAtEQ(""),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return []string{}, nil
		}
		return nil, entErrToBizErr(err, "AGENT")
	}
	switch node.Level {
	case "position":
		return []string{node.ID}, nil
	case "department":
		ids, err := r.data.RW().Read(ctx).Organization.Query().
			Where(
				organization.ParentIDEQ(categoryID),
				organization.LevelEQ("position"),
				organization.DeletedAtEQ(""),
			).
			IDs(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "AGENT")
		}
		return ids, nil
	case "company":
		deptIDs, err := r.data.RW().Read(ctx).Organization.Query().
			Where(
				organization.ParentIDEQ(categoryID),
				organization.LevelEQ("department"),
				organization.DeletedAtEQ(""),
			).
			IDs(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "AGENT")
		}
		if len(deptIDs) == 0 {
			return []string{}, nil
		}
		posIDs, err := r.data.RW().Read(ctx).Organization.Query().
			Where(
				organization.ParentIDIn(deptIDs...),
				organization.LevelEQ("position"),
				organization.DeletedAtEQ(""),
			).
			IDs(ctx)
		if err != nil {
			return nil, entErrToBizErr(err, "AGENT")
		}
		return posIDs, nil
	default:
		return []string{}, nil
	}
}

func (r *agentRepo) SearchAgents(ctx context.Context, q biz.AgentListQuery) (biz.AgentListResult, error) {
	if q.Limit <= 0 {
		q.Limit = 24
	}
	if q.Limit > 500 {
		q.Limit = 500
	}
	if q.Offset < 0 {
		q.Offset = 0
	}
	preds := []predicate.Agent{agent.DeletedAtEQ("")}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		preds = append(preds, agent.Or(
			agent.AgentKeyContainsFold(kw),
			agent.DisplayNameContainsFold(kw),
			agent.ProviderContainsFold(kw),
			agent.ModelContainsFold(kw),
			agent.AgentDescriptionContainsFold(kw),
		))
	}
	if q.Status != "" {
		preds = append(preds, agent.StatusEQ(q.Status))
	}
	if q.Provider != "" {
		preds = append(preds, agent.ProviderEQ(q.Provider))
	}
	if q.OrgNodeID != "" {
		positionIDs, err := r.categoryPositionIDsForFilter(ctx, q.OrgNodeID)
		if err != nil {
			return biz.AgentListResult{}, err
		}
		if len(positionIDs) == 0 {
			preds = append(preds, agent.PositionIDEQ("__no_such_category__"))
		} else if len(positionIDs) == 1 {
			preds = append(preds, agent.PositionIDEQ(positionIDs[0]))
		} else {
			preds = append(preds, agent.PositionIDIn(positionIDs...))
		}
	}
	if cb := strings.TrimSpace(q.CreatedBy); cb != "" {
		preds = append(preds, agent.CreatedByEQ(cb))
	}
	if role := strings.TrimSpace(q.Role); role != "" {
		preds = append(preds, agent.RolesJSONContains(role))
	}
	if q.Kind != "" {
		preds = append(preds, agent.KindEQ(agent.Kind(q.Kind)))
	}
	// P2-B: workspace 过滤。空 WorkspaceID = system caller（看全部）。
	// 租户 caller 只看：自己私有的（workspace_id == caller）+ 全局共享的（workspace_id == ""）。
	if q.WorkspaceID != "" {
		preds = append(preds, agent.Or(
			agent.WorkspaceIDEQ(""),
			agent.WorkspaceIDEQ(q.WorkspaceID),
		))
	}
	where := agent.And(preds...)
	c := r.data.RW().Read(ctx)
	total, err := c.Agent.Query().Where(where).Count(ctx)
	if err != nil {
		return biz.AgentListResult{}, entErrToBizErr(err, "AGENT")
	}
	rows, err := c.Agent.Query().Where(where).
		// 内置管家优先：system_builtin 且非 dept_lead（精灵/系统/记忆/技能管家）排在最前。
		// 30 个 system_builtin 共享同一 updated_at（同一种子时间），仅靠 kind DESC + id ASC
		// 会把 26 个部门主管挤在 3 个管家前面，管家掉到第 2 页。
		// kind DESC：system_builtin 排在 ecosystem_preset 之前；
		// id ASC：唯一决胜键，保证同 updated_at 组内分页顺序稳定（否则 LIMIT/OFFSET 会跳行/重复）。
		Order(
			agent.ByIsDefault(entsql.OrderDesc()),
			func(s *entsql.Selector) {
				s.OrderBy(entsql.Asc("CASE WHEN " + s.C(agent.FieldKind) + " = 'system_builtin' AND " + s.C(agent.FieldAgentVariant) + " <> 'dept_lead' THEN 0 ELSE 1 END"))
			},
			agent.ByKind(entsql.OrderDesc()),
			agent.ByUpdatedAt(entsql.OrderDesc()),
			agent.ByID(entsql.OrderAsc()),
		).
		Limit(q.Limit).
		Offset(q.Offset).
		All(ctx)
	if err != nil {
		return biz.AgentListResult{}, entErrToBizErr(err, "AGENT")
	}
	items := make([]biz.Agent, 0, len(rows))
	for _, row := range rows {
		items = append(items, entAgentToBiz(row, r.data.lg))
	}
	return biz.AgentListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *agentRepo) ListAgentCreators(ctx context.Context) ([]biz.AgentCreator, error) {
	rows, err := r.data.RW().Read(ctx).Agent.Query().
		Where(agent.DeletedAtEQ(""), agent.CreatedByNEQ("")).
		Select(agent.FieldCreatedBy).
		GroupBy(agent.FieldCreatedBy).
		Strings(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AGENT")
	}
	out := make([]biz.AgentCreator, 0, len(rows)+1)
	if biz.AgentCreatedByFromContext(ctx) != "" {
		out = append(out, biz.AgentCreator{UserID: biz.AgentListCreatedByMine, Label: "仅我的"})
	}
	seen := map[string]bool{}
	for _, id := range rows {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, biz.AgentCreator{UserID: id, Label: creatorLabel(id)})
	}
	return out, nil
}

func creatorLabel(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "未知"
	}
	return "用户 " + userID
}

func (r *agentRepo) GetAgentByID(ctx context.Context, id string) (biz.Agent, error) {
	preds := []predicate.Agent{agent.IDEQ(id), agent.DeletedAtEQ("")}
	if ids := workspaceSharedOrOwnIDs(ctx); ids != nil {
		preds = append(preds, agent.WorkspaceIDIn(ids...))
	}
	row, err := r.data.RW().Read(ctx).Agent.Query().Where(preds...).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Agent{}, shared.ErrNotFound
		}
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	return entAgentToBiz(row, r.data.lg), nil
}

// ListAgentsByIDs returns agents matching the given IDs in a single query.
// Missing IDs are silently skipped. Returns an empty slice for empty input.
func (r *agentRepo) ListAgentsByIDs(ctx context.Context, ids []string) ([]biz.Agent, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.data.RW().Read(ctx).Agent.Query().
		Where(agent.IDIn(ids...), agent.DeletedAtEQ("")).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AGENT")
	}
	items := make([]biz.Agent, 0, len(rows))
	for _, row := range rows {
		items = append(items, entAgentToBiz(row, r.data.lg))
	}
	return items, nil
}

// isAgentKeyConstraintError reports whether a constraint error is for the
// agents.agent_key unique index rather than another unique constraint such as
// agent_position_key_agent_variant.
func isAgentKeyConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// The composite (position_key, agent_variant) constraint name contains
	// "agent_" and "_key" but is not an agent_key conflict; exclude it first.
	if strings.Contains(msg, "agent_position_key_agent_variant") {
		return false
	}
	return strings.Contains(msg, "agent_key")
}

func (r *agentRepo) GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error) {
	agentKey = strings.TrimSpace(agentKey)
	if agentKey == "" {
		return biz.Agent{}, shared.ErrNotFound
	}
	row, err := r.data.RW().Read(ctx).Agent.Query().Where(agent.AgentKeyEQ(agentKey), agent.DeletedAtEQ("")).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Agent{}, shared.ErrNotFound
		}
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	return entAgentToBiz(row, r.data.lg), nil
}

func (r *agentRepo) GetAgentRuntimeSettings(ctx context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	row, err := r.data.RW().Read(ctx).AgentRuntimeSetting.Get(ctx, agentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.AgentRuntimeSettings{}, shared.ErrNotFound
		}
		return biz.AgentRuntimeSettings{}, entErrToBizErr(err, "AGENT")
	}
	return entRuntimeToBiz(row), nil
}

func (r *agentRepo) ListAgentRuntimeSettings(ctx context.Context) (map[string]biz.AgentRuntimeSettings, error) {
	rows, err := r.data.RW().Read(ctx).AgentRuntimeSetting.Query().All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AGENT")
	}
	out := make(map[string]biz.AgentRuntimeSettings, len(rows))
	for _, row := range rows {
		out[row.ID] = entRuntimeToBiz(row)
	}
	return out, nil
}

// CountAgentsByProviderAndModel counts agents referencing a given provider+model.
func (r *agentRepo) CountAgentsByProviderAndModel(ctx context.Context, provider, model string) (int, error) {
	if r.data == nil {
		return 0, nil
	}
	n, err := r.data.RW().Read(ctx).Agent.Query().
		Where(agent.ProviderEQ(provider), agent.ModelEQ(model), agent.DeletedAtEQ("")).
		Count(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "AGENT")
	}
	return n, nil
}

// ClearPositionByDepartment clears the position_id field for all agents
// whose position belongs to the given department. Used during department deletion cascade.
func (r *agentRepo) ClearPositionByDepartment(ctx context.Context, deptID string) (int, error) {
	if deptID == "" || r.data == nil {
		return 0, nil
	}
	// Find all position IDs under this department
	positions, err := r.data.RW().Read(ctx).Organization.Query().
		Where(
			organization.ParentIDEQ(deptID),
			organization.LevelEQ("position"),
			organization.DeletedAtEQ(""),
		).
		All(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "AGENT")
	}
	if len(positions) == 0 {
		return 0, nil
	}
	positionIDs := make([]string, 0, len(positions))
	for _, p := range positions {
		positionIDs = append(positionIDs, p.ID)
	}
	// Clear position_id for agents in those positions
	n, err := r.data.RW().Write(ctx).Agent.Update().
		Where(
			agent.PositionIDIn(positionIDs...),
			agent.DeletedAtEQ(""),
		).
		SetPositionID("").
		Save(ctx)
	if err != nil {
		return 0, entErrToBizErr(err, "AGENT")
	}
	return n, nil
}
