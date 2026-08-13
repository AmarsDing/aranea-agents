package data

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/shared"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agentpromptfile"
	"aranea-agents/pkg/apierror"
)

func (r *agentRepo) ListAgentPromptFiles(ctx context.Context, agentID string) ([]biz.AgentPromptFile, error) {
	rows, err := r.data.RW().Read(ctx).AgentPromptFile.Query().
		Where(agentpromptfile.AgentIDEQ(agentID)).
		Order(agentpromptfile.BySortOrder(), agentpromptfile.ByFileName()).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "AGENT")
	}
	out := make([]biz.AgentPromptFile, 0, len(rows))
	for _, row := range rows {
		out = append(out, entPromptToBiz(row))
	}
	return out, nil
}

func (r *agentRepo) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	if agentID == "" {
		return nil, apierror.BadRequest("AGENT", "agent id is required")
	}
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		if _, err := r.data.RW().Write(txCtx).AgentPromptFile.Delete().Where(agentpromptfile.AgentIDEQ(agentID)).Exec(txCtx); err != nil {
			return entErrToBizErr(err, "AGENT")
		}
		now := nowRFC3339()
		builders := make([]*ent.AgentPromptFileCreate, 0, len(files))
		for i, file := range files {
			if strings.TrimSpace(file.Name) == "" {
				continue
			}
			id := file.ID
			if id == "" {
				id = fmt.Sprintf("%s_%s", agentID, sanitizePromptFileID(file.Name))
			}
			sortOrder := file.SortOrder
			if sortOrder == 0 {
				sortOrder = (i + 1) * 10
			}
			builders = append(builders, r.data.RW().Write(txCtx).AgentPromptFile.Create().
				SetID(id).
				SetAgentID(agentID).
				SetFileName(strings.TrimSpace(file.Name)).
				SetBody(file.Body).
				SetSortOrder(sortOrder).
				SetCreatedAt(now).
				SetUpdatedAt(now))
		}
		if len(builders) > 0 {
			if _, err := r.data.RW().Write(txCtx).AgentPromptFile.CreateBulk(builders...).Save(txCtx); err != nil {
				return entErrToBizErr(err, "AGENT")
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out, err := r.ListAgentPromptFiles(ctx, agentID)
	return out, err
}

func (r *agentRepo) CreateAgentPromptFile(ctx context.Context, f biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	if f.AgentID == "" || strings.TrimSpace(f.Name) == "" {
		return biz.AgentPromptFile{}, apierror.BadRequest("AGENT", "agent_id and name are required")
	}
	id := f.ID
	if id == "" {
		id = fmt.Sprintf("%s_%s", f.AgentID, sanitizePromptFileID(f.Name))
	}
	now := nowRFC3339()
	created, err := r.data.RW().Write(ctx).AgentPromptFile.Create().
		SetID(id).
		SetAgentID(f.AgentID).
		SetFileName(strings.TrimSpace(f.Name)).
		SetBody(f.Body).
		SetSortOrder(f.SortOrder).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return biz.AgentPromptFile{}, entErrToBizErr(err, "AGENT")
	}
	return entPromptToBiz(created), nil
}

func (r *agentRepo) UpdateAgentPromptFile(ctx context.Context, f biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	if f.ID == "" || f.AgentID == "" {
		return biz.AgentPromptFile{}, apierror.BadRequest("AGENT", "id and agent_id are required")
	}
	update := r.data.RW().Write(ctx).AgentPromptFile.UpdateOneID(f.ID).
		SetUpdatedAt(nowRFC3339())
	if strings.TrimSpace(f.Name) != "" {
		update = update.SetFileName(strings.TrimSpace(f.Name))
	}
	if f.Body != "" {
		update = update.SetBody(f.Body)
	}
	if f.SortOrder > 0 {
		update = update.SetSortOrder(f.SortOrder)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.AgentPromptFile{}, shared.ErrNotFound
		}
		return biz.AgentPromptFile{}, entErrToBizErr(err, "AGENT")
	}
	return entPromptToBiz(updated), nil
}

func (r *agentRepo) DeleteAgentPromptFile(ctx context.Context, agentID, id string) error {
	if agentID == "" || id == "" {
		return apierror.BadRequest("AGENT", "agent_id and id are required")
	}
	_, err := r.data.RW().Write(ctx).AgentPromptFile.Delete().
		Where(agentpromptfile.IDEQ(id), agentpromptfile.AgentIDEQ(agentID)).
		Exec(ctx)
	return entErrToBizErr(err, "AGENT")
}
