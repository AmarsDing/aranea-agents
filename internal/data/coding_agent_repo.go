package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
		// M4 P3-6：liveness 阈值覆盖（nil 表示未配置，用全局默认）
		LivenessOverride: e.LivenessOverride,
		CreatedAt:        e.CreatedAt,
		UpdatedAt:        e.UpdatedAt,
	}
}

func (r *codingAgentRepo) GetByKey(ctx context.Context, workspace, key string) (*bridge.CodingAgent, error) {
	row, err := r.data.RW().Read(ctx).CodingAgent.Query().
		Where(codingagent.WorkspaceEQ(workspace), codingagent.AgentKeyEQ(key)).
		Only(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, apierror.DomainAgentBridge)
	}
	agent := entCodingAgentToBiz(row)
	extras, err := r.loadAgentExtras(ctx, workspace)
	if err != nil {
		return nil, err
	}
	if ex, ok := extras[agent.ID]; ok {
		agent.IconURL, agent.Capabilities, agent.DefaultAutonomy = ex.iconURL, ex.caps, ex.autonomy
	}
	return agent, nil
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
	extras, err := r.loadAgentExtras(ctx, workspace)
	if err != nil {
		return nil, err
	}
	for _, a := range out {
		if ex, ok := extras[a.ID]; ok {
			a.IconURL, a.Capabilities, a.DefaultAutonomy = ex.iconURL, ex.caps, ex.autonomy
		}
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
	if agent.LivenessOverride != nil {
		create = create.SetLivenessOverride(agent.LivenessOverride)
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
			if agent.LivenessOverride != nil {
				u.UpdateLivenessOverride()
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
	// 03 P0：extras 列（icon/capabilities/default_autonomy）走 raw SQL；先暂存
	// 调用方入参（回读会覆盖 agent），省略（空值/nil）= 保留旧值，空数组=清空。
	iconURL, caps, autonomy := agent.IconURL, agent.Capabilities, agent.DefaultAutonomy
	*agent = *entCodingAgentToBiz(row)
	extrasIn := &bridge.CodingAgent{IconURL: iconURL, Capabilities: caps, DefaultAutonomy: autonomy}
	if err := r.saveAgentExtras(ctx, agent.Workspace, agent.AgentKey, extrasIn); err != nil {
		return err
	}
	extras, err := r.loadAgentExtras(ctx, agent.Workspace)
	if err != nil {
		return err
	}
	if ex, ok := extras[agent.ID]; ok {
		agent.IconURL, agent.Capabilities, agent.DefaultAutonomy = ex.iconURL, ex.caps, ex.autonomy
	}
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

// --- 03 P0：图标/能力/默认自治级别（raw SQL 列，ent 模型不含，迁移 20261308） ---

type codingAgentExtras struct {
	iconURL  string
	caps     []string
	autonomy string
}

// parseAgentCapabilities 解析 capabilities JSON 数组文本（空/非法=空切片）。
func parseAgentCapabilities(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var caps []string
	if err := json.Unmarshal([]byte(raw), &caps); err != nil {
		return nil
	}
	return caps
}

func (r *codingAgentRepo) loadAgentExtras(ctx context.Context, workspace string) (map[string]codingAgentExtras, error) {
	db := r.data.RWDB().ReadDB(ctx)
	if db == nil {
		return nil, apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
	}
	rows, err := db.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(
		`SELECT id, icon_url, capabilities, default_autonomy FROM coding_agents WHERE workspace=?`), workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]codingAgentExtras{}
	for rows.Next() {
		var id, icon, caps, autonomy string
		if err := rows.Scan(&id, &icon, &caps, &autonomy); err != nil {
			return nil, err
		}
		out[id] = codingAgentExtras{iconURL: icon, caps: parseAgentCapabilities(caps), autonomy: autonomy}
	}
	return out, rows.Err()
}

func (r *codingAgentRepo) saveAgentExtras(ctx context.Context, workspace, key string, agent *bridge.CodingAgent) error {
	sets := []string{}
	args := []any{}
	if agent.IconURL != "" {
		sets = append(sets, "icon_url=?")
		args = append(args, agent.IconURL)
	}
	if agent.Capabilities != nil {
		raw, err := json.Marshal(agent.Capabilities)
		if err != nil {
			return apierror.BadRequest(apierror.DomainAgentBridge, "capabilities must be a string array")
		}
		sets = append(sets, "capabilities=?")
		args = append(args, string(raw))
	}
	if agent.DefaultAutonomy != "" {
		sets = append(sets, "default_autonomy=?")
		args = append(args, agent.DefaultAutonomy)
	}
	if len(sets) == 0 {
		return nil
	}
	db := r.data.RWDB().WriteDB(ctx)
	if db == nil {
		return apierror.Internal(apierror.DomainAgentBridge, "repository unavailable")
	}
	q := fmt.Sprintf(`UPDATE coding_agents SET %s, updated_at=? WHERE workspace=? AND agent_key=?`, strings.Join(sets, ", "))
	args = append(args, nowRFC3339(), workspace, key)
	_, err := db.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	return err
}
