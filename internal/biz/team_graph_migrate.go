package biz

import (
	"context"
	"sort"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Team × Graph 一体化（Phase 11，B10）：存量迁移 + 运行时惰性兜底。
//
// 挂载说明：本逻辑依赖 TeamCompiler（实现位于 internal/team），受依赖方向约束
// 无法放入 data 层 L3 迁移注册表；由 cmd/admin readiness 门控后台任务调用
// MigrateLegacyEmbeddedGraphs，运行时路径由 team.Runner 经 TeamGraphAssetEnsurer
// 端口调用 EnsureTeamGraphAsset。
// 幂等性由「linked_graph_id 非空跳过」天然保证，无需 schema_migrations 门控。

// TeamGraphAssetEnsurer 运行时惰性物化端口（B10）：team.Runner 加载 team 时
// 确保图资产存在（linked_graph_id 为空 → 先物化）。由 *TeamUsecase 实现；
// nil 时 Runner 回退为直接读 TeamReader（单测/离线工具）。
//
// Stability:evolving
type TeamGraphAssetEnsurer interface {
	EnsureTeamGraphAsset(ctx context.Context, teamID string) (Team, error)
}

// MigrateLegacyEmbeddedGraphs 扫描 definition_json 仍含 embedded graph 且
// linked_graph_id 为空的存量 team，批量物化为 graph_definitions 一等资产并回写。
// 单 team 失败记 warn 继续，不阻塞启动。返回 (migrated, skipped, failed)。
func (u *TeamUsecase) MigrateLegacyEmbeddedGraphs(ctx context.Context) (migrated, skipped, failed int) {
	if u.graphAssets == nil || u.compiler == nil {
		return 0, 0, 0 // 图集成未装配（单测/离线工具）
	}
	teams, err := u.reader.ListTeams(ctx)
	if err != nil {
		u.lg.Warn("team graph legacy migration: list teams failed", loggateway.Err(err))
		return 0, 0, 1
	}
	for _, t := range teams {
		if strings.TrimSpace(t.LinkedGraphID) != "" {
			skipped++
			continue
		}
		spec, err := ParseOrchestrationSpec(t.DefinitionJSON)
		if err != nil {
			skipped++ // 非 spec JSON：编译兜底路径仍可运行，不强行迁移
			continue
		}
		if strings.TrimSpace(spec.LinkedGraphID) != "" {
			skipped++
			continue
		}
		if spec.Graph == nil || len(spec.Graph.Nodes) == 0 {
			skipped++ // 无 embedded graph：保存钩子/运行时惰性兜底负责
			continue
		}
		if _, err := u.materializeLegacyTeamGraph(ctx, t); err != nil {
			failed++
			u.lg.Warn("team graph legacy migration: materialize failed",
				loggateway.Str("team_id", t.ID),
				loggateway.Str("team_key", t.TeamKey),
				loggateway.Err(err))
			continue
		}
		migrated++
	}
	if migrated > 0 || failed > 0 {
		u.lg.Info("team graph legacy migration done",
			loggateway.StepID("team.graph_migration"),
			loggateway.Int("migrated", migrated),
			loggateway.Int("skipped", skipped),
			loggateway.Int("failed", failed))
	}
	return migrated, skipped, failed
}

// EnsureTeamGraphAsset 运行时惰性兜底（B10）：team 的 linked_graph_id 为空时
// 先物化再返回最新 team（含回写后的 LinkedGraphID / DefinitionJSON）。
// 已链接或未装配图集成时原样返回，调用方走既有编译兜底。
func (u *TeamUsecase) EnsureTeamGraphAsset(ctx context.Context, teamID string) (Team, error) {
	team, err := u.reader.GetTeamByID(ctx, teamID)
	if err != nil {
		return Team{}, err
	}
	if u.graphAssets == nil || u.compiler == nil {
		return team, nil
	}
	if strings.TrimSpace(team.LinkedGraphID) != "" {
		return team, nil
	}
	if spec, perr := ParseOrchestrationSpec(team.DefinitionJSON); perr == nil &&
		strings.TrimSpace(spec.LinkedGraphID) != "" {
		// 列值滞后于 definition_json：以 spec 为准，同步列值后返回。
		team.LinkedGraphID = strings.TrimSpace(spec.LinkedGraphID)
		return team, nil
	}
	return u.materializeLegacyTeamGraph(ctx, team)
}

// materializeLegacyTeamGraph 物化单个 team（幂等，D1：物化 + 回写同一事务）。
//
// preset/custom 双态判定：embedded graph 与表单（mode/members）派生拓扑等价 →
// preset（以 canonical 派生为准重建）；不等价（用户手改过拓扑）→ custom（以
// embedded graph 为准物化，保留用户拓扑）。
func (u *TeamUsecase) materializeLegacyTeamGraph(ctx context.Context, team Team) (Team, error) {
	spec, err := ParseOrchestrationSpec(team.DefinitionJSON)
	if err != nil {
		return team, apierror.BadRequest("TEAM", "definition_json must be valid JSON")
	}
	hasEmbedded := spec.Graph != nil && len(spec.Graph.Nodes) > 0

	agentKey := func(agentID string) string { return agentID }
	if u.agentKeyResolver != nil {
		if fn := u.agentKeyResolver(ctx); fn != nil {
			agentKey = fn
		}
	}

	// 表单派生编译（raw 中剔除 graph，走 mode 模板路径）。
	presetSpec := spec
	presetSpec.Graph = nil
	rawPreset, err := OrchestrationSpecToDefinitionJSON(presetSpec)
	if err != nil {
		return team, err
	}
	formCfg, formErr := u.compiler.CompileFromDefinition(teamDefinitionForCompile(&team, &presetSpec, rawPreset), agentKey)

	source := DefinitionGraphSourcePreset
	cfg := formCfg
	if hasEmbedded {
		embCfg, embErr := u.compiler.CompileFromDefinition(teamDefinitionForCompile(&team, &spec, team.DefinitionJSON), agentKey)
		switch {
		case embErr == nil && formErr == nil && !graphTopologyEquivalent(formCfg, embCfg):
			// 用户手改过拓扑：以 embedded graph 为准。
			source = DefinitionGraphSourceCustom
			cfg = embCfg
		case embErr == nil && formErr != nil:
			// 表单编译失败但 embedded 可用：保留用户拓扑。
			source = DefinitionGraphSourceCustom
			cfg = embCfg
		case embErr != nil && formErr != nil:
			return team, formErr
		case embErr != nil:
			u.lg.Warn("legacy embedded graph compile failed; rebuilding from form",
				loggateway.Str("team_id", team.ID),
				loggateway.Err(embErr))
		}
	} else if formErr != nil {
		return team, formErr
	}
	cfg.EnableCheckpoint = spec.EnableCheckpoint

	if err := u.execTeamGraphTx(ctx, func(txCtx context.Context) error {
		asset := MaterializeTeamGraphDefinition(team, cfg, nil, source)
		saved, err := u.graphAssets.CreateGraph(txCtx, asset)
		if err != nil {
			return err
		}
		spec.Graph = nil // D.1：embedded 退役，拓扑真相源 = 图资产
		spec.Source = source
		spec.LinkedGraphID = saved.ID
		out, err := OrchestrationSpecToDefinitionJSON(spec)
		if err != nil {
			return err
		}
		team.DefinitionJSON = out
		team.LinkedGraphID = saved.ID
		if _, err := u.writer.UpdateTeam(txCtx, team); err != nil {
			return err
		}
		u.lg.Info("team graph materialized (legacy migration)",
			loggateway.StepID("team.graph_migration"),
			loggateway.Str("team_id", team.ID),
			loggateway.Str("graph_id", saved.ID),
			loggateway.Str("source", source))
		return nil
	}); err != nil {
		return team, err
	}
	return team, nil
}

// graphTopologyEquivalent 比较两份编译产物的拓扑等价性：节点 ID 集合 +
// 边端点对（含条件边分支目标）多重集合。节点字段差异（instruction 等）不影响判定。
func graphTopologyEquivalent(a, b GraphBuildConfig) bool {
	nodeIDs := func(cfg GraphBuildConfig) []string {
		out := make([]string, 0, len(cfg.Nodes))
		for _, n := range cfg.Nodes {
			out = append(out, n.ID)
		}
		sort.Strings(out)
		return out
	}
	edgePairs := func(cfg GraphBuildConfig) []string {
		out := make([]string, 0, len(cfg.Edges))
		for _, e := range cfg.Edges {
			out = append(out, e.From+"->"+e.To)
		}
		for _, ce := range cfg.ConditionalEdges {
			targets := make([]string, 0, len(ce.PathMap))
			for _, to := range ce.PathMap {
				targets = append(targets, to)
			}
			sort.Strings(targets)
			for _, to := range targets {
				out = append(out, ce.From+"~>"+to)
			}
		}
		sort.Strings(out)
		return out
	}
	an, bn := nodeIDs(a), nodeIDs(b)
	if len(an) != len(bn) {
		return false
	}
	for i := range an {
		if an[i] != bn[i] {
			return false
		}
	}
	ae, be := edgePairs(a), edgePairs(b)
	if len(ae) != len(be) {
		return false
	}
	for i := range ae {
		if ae[i] != be[i] {
			return false
		}
	}
	return true
}
