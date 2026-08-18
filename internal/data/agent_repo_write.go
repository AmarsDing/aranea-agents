package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/agentruntimesetting"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
)

func (r *agentRepo) CreateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error) {
	if a.AgentKey == "" || a.DisplayName == "" {
		return biz.Agent{}, apierror.BadRequest("AGENT", "missing required fields")
	}
	if (strings.TrimSpace(a.Provider) == "") != (strings.TrimSpace(a.Model) == "") {
		return biz.Agent{}, apierror.BadRequest("AGENT", "provider and model must both be set or both be empty")
	}
	if a.ID == "" {
		a.ID = generateCatalogID()
	}
	now := nowRFC3339()
	if a.CreatedAt == "" {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	if a.Status == "" {
		a.Status = "active"
	}
	_, err := r.data.RW().Write(ctx).Agent.Create().
		SetID(a.ID).
		SetAgentKey(a.AgentKey).
		SetDisplayName(a.DisplayName).
		SetProvider(a.Provider).
		SetModel(a.Model).
		SetStatus(a.Status).
		SetIsDefault(biz.BoolVal(a.IsDefault)).
		SetIsFavorite(biz.BoolVal(a.IsFavorite)).
		SetIcon(a.Icon).
		SetAgentDescription(a.AgentDescription).
		SetPositionID(a.PositionID).
		SetPositionKey(a.PositionKey).
		SetAgentVariant(a.AgentVariant).
		SetVariantDescription(a.VariantDescription).
		SetSystemPromptMode(a.SystemPromptMode).
		SetContextWindow(a.ContextWindow).
		SetBudgetMonthlyCents(a.BudgetMonthlyCents).
		SetConfigJSON(a.ConfigJSON).
		SetRolesJSON(mustMarshalString(a.Roles)).
		SetCreatedBy(a.CreatedBy).
		SetReadonly(a.Readonly).
		SetKind(agent.Kind(a.Kind)).
		SetSource(agent.Source(a.Source)).
		SetCreatedAt(a.CreatedAt).
		SetUpdatedAt(a.UpdatedAt).
		SetDeletedAt(a.DeletedAt).
		SetWorkspaceID(a.WorkspaceID).
		SetMissionStatement(a.MissionStatement).
		SetDomainPath(a.DomainPath).
		Save(ctx)
	if err != nil {
		if sqlgraph.IsConstraintError(err) && isAgentKeyConstraintError(err) {
			return biz.Agent{}, shared.ErrAgentKeyConflict
		}
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	return r.GetAgentByID(ctx, a.ID)
}

func (r *agentRepo) UpdateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error) {
	if a.ID == "" {
		return biz.Agent{}, apierror.BadRequest("AGENT", "id is required")
	}
	current, err := r.GetAgentByID(ctx, a.ID)
	if err != nil {
		return biz.Agent{}, err
	}
	if a.AgentKey == "" {
		a.AgentKey = current.AgentKey
	}
	if a.DisplayName == "" {
		a.DisplayName = current.DisplayName
	}
	if a.Provider == "" {
		a.Provider = current.Provider
	}
	if a.Model == "" {
		a.Model = current.Model
	}
	if a.Status == "" {
		a.Status = current.Status
	}
	a.CreatedAt = current.CreatedAt
	a.UpdatedAt = nowRFC3339()
	_, err = r.data.RW().Write(ctx).Agent.UpdateOneID(a.ID).
		SetDisplayName(a.DisplayName).
		SetProvider(a.Provider).
		SetModel(a.Model).
		SetStatus(a.Status).
		SetIsDefault(biz.BoolVal(a.IsDefault)).
		SetIsFavorite(biz.BoolVal(a.IsFavorite)).
		SetIcon(a.Icon).
		SetAgentDescription(a.AgentDescription).
		SetPositionID(a.PositionID).
		SetPositionKey(a.PositionKey).
		SetAgentVariant(a.AgentVariant).
		SetVariantDescription(a.VariantDescription).
		SetSystemPromptMode(a.SystemPromptMode).
		SetContextWindow(a.ContextWindow).
		SetBudgetMonthlyCents(a.BudgetMonthlyCents).
		SetConfigJSON(a.ConfigJSON).
		SetRolesJSON(mustMarshalString(a.Roles)).
		SetReadonly(a.Readonly).
		SetKind(agent.Kind(a.Kind)).
		SetSource(agent.Source(a.Source)).
		SetMissionStatement(a.MissionStatement).
		SetDomainPath(a.DomainPath).
		SetUpdatedAt(a.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	out, err := r.GetAgentByID(ctx, a.ID)
	return out, err
}

func (r *agentRepo) DeleteAgent(ctx context.Context, id string) error {
	if id == "" {
		return apierror.BadRequest("AGENT", "id is required")
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := nowRFC3339()
		// Tombstone is deleted_at. Status stays inside the catalog FSM
		// (active/inactive/archived); "deleted" is not a valid AgentStatus.
		if _, err := r.data.RW().Write(txCtx).Agent.UpdateOneID(id).
			SetDeletedAt(now).
			SetStatus(string(biz.AgentStatusArchived)).
			SetUpdatedAt(now).
			Save(txCtx); err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		return cascadeDeleteByAgent(txCtx, r.data, id)
	})
}

// ToggleFavorite atomically flips the is_favorite flag using
// UPDATE agents SET is_favorite = NOT is_favorite WHERE id = ?,
// then reads back the updated row. This avoids the read-then-write
// race condition where two concurrent requests could both read the
// same value and flip to the same result.
func (r *agentRepo) ToggleFavorite(ctx context.Context, id string) (biz.Agent, error) {
	if id == "" {
		return biz.Agent{}, apierror.BadRequest("AGENT", "id is required")
	}
	now := nowRFC3339()
	result, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders("UPDATE agents SET is_favorite = NOT is_favorite, updated_at = ? WHERE id = ? AND deleted_at = ''"),
		now, id,
	)
	if err != nil {
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return biz.Agent{}, entErrToBizErr(err, "AGENT")
	}
	if affected == 0 {
		return biz.Agent{}, shared.ErrNotFound
	}
	return r.GetAgentByID(ctx, id)
}

func (r *agentRepo) UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	if v.AgentID == "" {
		return biz.AgentRuntimeSettings{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	now := nowRFC3339()
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	b := r.data.RW().Write(ctx).AgentRuntimeSetting.Create().SetID(v.AgentID)
	applyBizRuntimeToCreate(b, v, r.data.lg)
	if err := b.OnConflict(
		entsql.ConflictColumns(agentruntimesetting.FieldID),
		entsql.ResolveWithNewValues(),
	).Exec(ctx); err != nil {
		return biz.AgentRuntimeSettings{}, entErrToBizErr(err, "AGENT")
	}
	row, err := r.data.RW().Write(ctx).AgentRuntimeSetting.Get(ctx, v.AgentID)
	if err != nil {
		return biz.AgentRuntimeSettings{}, entErrToBizErr(err, "AGENT")
	}
	return entRuntimeToBiz(row), nil
}

// --- 高阶原子化方法（供 pack.Importer 使用）---

// CreateAgentAtomic 创建 agent + 写入 prompt files + upsert runtime settings，
// 三步包在同一个 ExecInTx 中，任意一步失败则整体回滚。
// 比 Pack 手工调用 CreateAgent + ReplaceFiles + UpsertSettings 强一档。
func (r *agentRepo) CreateAgentAtomic(ctx context.Context, a biz.Agent, files []biz.AgentPromptFile, settings biz.AgentRuntimeSettings) (biz.Agent, error) {
	var created biz.Agent
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		var err error
		created, err = r.CreateAgent(txCtx, a)
		if err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		settings.AgentID = created.ID
		if _, err = r.UpsertAgentRuntimeSettings(txCtx, settings); err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		if len(files) > 0 {
			if _, err = r.ReplaceAgentPromptFiles(txCtx, created.ID, files); err != nil {
				return entErrToBizErr(err, "AGENT")
			}
		}
		return nil
	})
	return created, err
}

// UpdateAgentAtomic 覆盖 agent + 替换 prompt files + upsert runtime settings，
// 三步包在同一个 ExecInTx 中，任意一步失败则整体回滚到调用前状态。
// 对应 Pack 场景下的 ConflictOverwrite 路径；之前的"半新半旧"问题由此修复。
func (r *agentRepo) UpdateAgentAtomic(ctx context.Context, a biz.Agent, files []biz.AgentPromptFile, settings *biz.AgentRuntimeSettings) (biz.Agent, error) {
	var updated biz.Agent
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		var err error
		updated, err = r.UpdateAgent(txCtx, a)
		if err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		if settings != nil {
			// 与 CreateAgentAtomic 对齐：pack 导入器按约定以空 AgentID 构建
			// settings，由 atomic 回填；缺了这行会让 ConflictOverwrite 重导全部
			// 失败于 "agent id is required"（TS9-BUG-3 生产事故）。
			settings.AgentID = updated.ID
			if _, err = r.UpsertAgentRuntimeSettings(txCtx, *settings); err != nil {
				return entErrToBizErr(err, "AGENT")
			}
		}
		if files != nil {
			if _, err = r.ReplaceAgentPromptFiles(txCtx, updated.ID, files); err != nil {
				return entErrToBizErr(err, "AGENT")
			}
		}
		return nil
	})
	return updated, err
}
