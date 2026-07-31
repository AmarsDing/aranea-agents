package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// TeamGraphGuard guards team-owned / team-referenced graph assets against
// inconsistent edits and deletion（B6 反向同步 + B7 删除保护）.
// *TeamUsecase implements it; GraphDefinitionUsecase consumes it via
// SetTeamGraphGuard（可选注入，nil = 关闭保护，保持既有行为）.
// Stability:evolving
type TeamGraphGuard interface {
	// OnTeamOwnedGraphSaved wraps the save of a team-owned graph：属主有活跃
	// Run 时拒绝（与 Team 侧锁定对称，E.4）；否则在事务内执行 save 并回写
	// 属主 team（source=custom + members 从图 agent 节点派生）.
	OnTeamOwnedGraphSaved(ctx context.Context, def *GraphDefinition, save func(txCtx context.Context) error) error
	// CheckGraphDeletable rejects deletion of team-owned graphs（属主存在）
	// and independent graphs referenced by external links（列出引用 team）.
	CheckGraphDeletable(ctx context.Context, def *GraphDefinition) error
}

// TeamAgentIDResolver resolves agent_key → catalog agent_id for reverse member
// sync（B6）。镜像 TeamAgentKeyResolver 的签名形态。
type TeamAgentIDResolver func(ctx context.Context) func(agentKey string) (agentID string, ok bool)

// TeamLinkedGraphReader finds teams referencing a graph asset via
// linked_graph_id（B7 external 引用检查）.
// Stability:evolving
type TeamLinkedGraphReader interface {
	ListTeamsByLinkedGraphID(ctx context.Context, graphID string) ([]Team, error)
}

// DeriveMembersFromGraphNodes derives orchestration members from materialized
// graph agent nodes（B6 反向同步共享函数）。resolve 把 agent key 反查为
// agent_id（nil = key 原样保留）；解析失败的节点跳过并计入 skipped（调用方
// 记 warn）。mode 字段由调用方保留原值，本函数只产出 members。
func DeriveMembersFromGraphNodes(nodes []NodeDef, resolve func(agentKey string) (string, bool)) (members []OrchestrationMember, skipped []string) {
	for _, n := range nodes {
		if !strings.EqualFold(strings.TrimSpace(n.Type), "agent") {
			continue
		}
		key := strings.TrimSpace(n.AgentName)
		if key == "" {
			continue
		}
		agentID := key
		if resolve != nil {
			id, ok := resolve(key)
			if !ok {
				skipped = append(skipped, key)
				continue
			}
			agentID = id
		}
		members = append(members, OrchestrationMember{
			AgentID:    agentID,
			Role:       firstNonEmpty(strings.TrimSpace(n.RequiredRole), RoleWorker),
			Name:       firstNonEmpty(strings.TrimSpace(n.Description), "Agent"),
			TaskPrompt: strings.TrimSpace(n.Instruction),
			SortOrder:  len(members) + 1,
		})
	}
	return members, skipped
}

// OnTeamOwnedGraphSaved implements TeamGraphGuard（B6）.
func (u *TeamUsecase) OnTeamOwnedGraphSaved(ctx context.Context, def *GraphDefinition, save func(context.Context) error) error {
	teamID := strings.TrimSpace(def.TeamID)
	if teamID == "" {
		return save(ctx)
	}
	team, err := u.reader.GetTeamByID(ctx, teamID)
	if err != nil {
		// 属主不存在（孤儿 owned 资产）：不阻断保存，跳过反向同步。
		u.lg.Warn("team-owned graph owner missing; saving without reverse sync",
			loggateway.Str("team_id", teamID),
			loggateway.Str("graph_id", def.ID))
		return save(ctx)
	}
	// E.4：属主有活跃 Run → 拒绝保存（与 Team 侧 HasActiveRun 锁定对称）。
	active, err := u.HasActiveRun(ctx, teamID)
	if err != nil {
		return err
	}
	if active {
		return apierror.Conflict("TEAM",
			"team %q has an active run; graph editing is locked until the run finishes",
			team.DisplayName)
	}
	// D1 对称：图保存与 team 回写同一事务（repo 经 ctx 获取事务连接）。
	return u.execTeamGraphTx(ctx, func(txCtx context.Context) error {
		if def.Metadata == nil {
			def.Metadata = map[string]any{}
		}
		def.Metadata[GraphMetadataTeamSourceKey] = DefinitionGraphSourceCustom
		if err := save(txCtx); err != nil {
			return err
		}
		spec, err := ParseOrchestrationSpec(team.DefinitionJSON)
		if err != nil {
			return apierror.BadRequest("TEAM", "definition_json must be valid JSON")
		}
		spec.Source = DefinitionGraphSourceCustom
		spec.Graph = nil
		spec.LinkedGraphID = def.ID
		var skipped []string
		spec.Members, skipped = u.deriveMembersFromGraph(txCtx, def)
		for _, key := range skipped {
			u.lg.Warn("graph agent node skipped in member sync: agent key not resolvable",
				loggateway.Str("agent_key", key),
				loggateway.Str("team_id", teamID),
				loggateway.Str("graph_id", def.ID))
		}
		out, err := OrchestrationSpecToDefinitionJSON(spec)
		if err != nil {
			return err
		}
		team.DefinitionJSON = out
		team.LinkedGraphID = def.ID
		if _, err := u.writer.UpdateTeam(txCtx, team); err != nil {
			return err
		}
		u.lg.Info("team reverse-synced from graph editor save",
			loggateway.Str("team_id", teamID),
			loggateway.Str("graph_id", def.ID),
			loggateway.Str("source", DefinitionGraphSourceCustom))
		return nil
	})
}

// CheckGraphDeletable implements TeamGraphGuard（B7 / 设计 H）.
func (u *TeamUsecase) CheckGraphDeletable(ctx context.Context, def *GraphDefinition) error {
	if def == nil {
		return nil
	}
	// team-owned 图：属主 team 存在 → 拒绝（提示先删 team 或换绑）。
	if isTeamOwnedGraph(def) && strings.TrimSpace(def.TeamID) != "" {
		if owner, err := u.reader.GetTeamByID(ctx, def.TeamID); err == nil && owner.ID != "" {
			return apierror.Conflict("TEAM",
				"graph %q is owned by team %q; delete the team or rebind its orchestration first",
				def.Name, owner.DisplayName)
		}
	}
	// 独立图被 external 引用 → 拒绝并列出引用 team。
	if u.linkedReader != nil {
		refs, err := u.linkedReader.ListTeamsByLinkedGraphID(ctx, def.ID)
		if err != nil {
			return err
		}
		if len(refs) > 0 {
			names := make([]string, 0, len(refs))
			for _, t := range refs {
				names = append(names, firstNonEmpty(t.DisplayName, t.TeamKey))
			}
			return apierror.Conflict("TEAM",
				"graph %q is linked by team(s): %s; unbind them first",
				def.Name, strings.Join(names, ", "))
		}
	}
	return nil
}

func (u *TeamUsecase) deriveMembersFromGraph(ctx context.Context, def *GraphDefinition) ([]OrchestrationMember, []string) {
	var resolve func(string) (string, bool)
	if u.agentIDResolver != nil {
		resolve = u.agentIDResolver(ctx)
	}
	return DeriveMembersFromGraphNodes(def.Nodes, resolve)
}

// Compile-time interface assertion.
var _ TeamGraphGuard = (*TeamUsecase)(nil)
