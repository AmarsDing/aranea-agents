package data

import (
	"context"

	bridge "aranea-agents/internal/biz/agentbridge"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/codingagent"
	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// codingAgentRepo implements bridge.AgentRepo via Ent.
type codingAgentRepo struct {
	data *Data
}

var _ bridge.AgentRepo = (*codingAgentRepo)(nil)

// NewCodingAgentRepo returns the Ent-backed AgentRepo.
func NewCodingAgentRepo(d *Data) bridge.AgentRepo {
	return &codingAgentRepo{data: d}
}

func entCodingAgentToBiz(e *dataent.CodingAgent) *bridge.CodingAgent {
	return &bridge.CodingAgent{
		ID:             e.ID,
		Workspace:      e.Workspace,
		AgentKey:       e.AgentKey,
		DisplayName:    e.DisplayName,
		Command:        e.Command,
		Args:           e.Args,
		Env:            e.Env,
		Enabled:        e.Enabled,
		LastProbeOK:    e.LastProbeOk,
		LastProbeError: e.LastProbeError,
		CreatedAt:      e.CreatedAt,
		UpdatedAt:      e.UpdatedAt,
	}
}

func (r *codingAgentRepo) GetByKey(ctx context.Context, workspace, key string) (*bridge.CodingAgent, error) {
	row, err := r.data.RW().Read(ctx).CodingAgent.Query().
		Where(codingagent.WorkspaceEQ(workspace), codingagent.AgentKeyEQ(key)).
		Only(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	return entCodingAgentToBiz(row), nil
}

func (r *codingAgentRepo) List(ctx context.Context, workspace string) ([]*bridge.CodingAgent, error) {
	rows, err := r.data.RW().Read(ctx).CodingAgent.Query().
		Where(codingagent.WorkspaceEQ(workspace)).
		Order(dataent.Asc(codingagent.FieldAgentKey)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	out := make([]*bridge.CodingAgent, 0, len(rows))
	for _, row := range rows {
		out = append(out, entCodingAgentToBiz(row))
	}
	return out, nil
}

func (r *codingAgentRepo) Upsert(ctx context.Context, agent *bridge.CodingAgent) error {
	now := nowRFC3339()
	if agent.Workspace == "" {
		agent.Workspace = "default"
	}
	create := r.data.RW().Write(ctx).CodingAgent.Create().
		SetWorkspace(agent.Workspace).
		SetAgentKey(agent.AgentKey).
		SetDisplayName(agent.DisplayName).
		SetCommand(agent.Command).
		SetEnabled(agent.Enabled).
		SetCreatedAt(now).
		SetUpdatedAt(now)
	if agent.Args != nil {
		create = create.SetArgs(agent.Args)
	}
	if agent.Env != nil {
		create = create.SetEnv(agent.Env)
	}
	if agent.ID != "" {
		create = create.SetID(agent.ID)
	} else {
		create = create.SetID("codingagent_" + uuid.NewString())
	}
	// Upsert by unique (workspace, agent_key): id/created_at excluded from the
	// update set so re-registration preserves identity. Args/Env 仅在显式提供
	// 时更新（nil = 保留旧值）。
	err := create.
		OnConflictColumns(codingagent.FieldWorkspace, codingagent.FieldAgentKey).
		Update(func(u *dataent.CodingAgentUpsert) {
			u.UpdateDisplayName()
			u.UpdateCommand()
			u.UpdateEnabled()
			u.UpdateUpdatedAt()
			if agent.Args != nil {
				u.UpdateArgs()
			}
			if agent.Env != nil {
				u.UpdateEnv()
			}
		}).
		Exec(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	row, err := r.data.RW().Read(ctx).CodingAgent.Query().
		Where(codingagent.WorkspaceEQ(agent.Workspace), codingagent.AgentKeyEQ(agent.AgentKey)).
		Only(ctx)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	*agent = *entCodingAgentToBiz(row)
	return nil
}

func (r *codingAgentRepo) UpdateProbe(ctx context.Context, id string, ok bool, errMsg string) error {
	err := r.data.RW().Write(ctx).CodingAgent.UpdateOneID(id).
		SetLastProbeOk(ok).
		SetLastProbeError(errMsg).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	return entErrToBizErr(err, apierror.DomainAgentBridge)
}
