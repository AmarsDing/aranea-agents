package biz

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// TeamTxProvider provides transactional execution for atomic team +
// materialized-graph writes（D1：物化与 team 保存同一事务）.
// Satisfied by *data.Data in production; nil in unit tests and offline
// tooling falls back to non-transactional execution（保留既有行为）.
// Stability:stable
type TeamTxProvider interface {
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// TeamGraphAssetStore is the narrow port TeamUsecase uses to persist
// materialized team graph assets. Implementations must maintain version
// history (_version_history snapshots) and the definition cache coherence
// （由 *GraphDefinitionUsecase 满足）.
// UpdateOwnedGraph / DeleteOwnedGraph 是 team 生命周期内部路径（B4 物化重建 /
// B5 级联删 / D2 换绑），跳过 B6/B7 用户态 guard——调用方即 Team 保存钩子，
// 反向同步不适用于物化路径（否则 team_source 会被误镜像为 custom）。
// Stability:evolving
type TeamGraphAssetStore interface {
	CreateGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	UpdateOwnedGraph(ctx context.Context, def *GraphDefinition) (*GraphDefinition, error)
	DeleteOwnedGraph(ctx context.Context, id string) error
}

// ProvideTeamGraphAssetStore exposes GraphUsecase's definition sub-usecase as
// the TeamGraphAssetStore port for TeamUsecase.
func ProvideTeamGraphAssetStore(uc *GraphUsecase) TeamGraphAssetStore {
	if uc == nil {
		return nil
	}
	return uc.DefUC()
}

// TeamAgentKeyResolver resolves agent_id → catalog agent_key for materialized
// graph nodes. Mirrors ChannelUsecase.AgentKeyResolver signature.
type TeamAgentKeyResolver func(ctx context.Context) func(agentID string) string

func (u *TeamUsecase) execTeamGraphTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if u.txProvider == nil {
		return fn(ctx)
	}
	return u.txProvider.ExecInTx(ctx, fn)
}

// isTeamOwnedGraph reports whether the graph asset is team-owned (materialized
// by a team), as opposed to an independent graph linked externally（B5/B7/B8
// 删除保护与换绑校验的唯一判定依据；team_id 字段因 ORG-11b 双向引用对
// external 图也会回填，不能作为 owned 判定）.
func isTeamOwnedGraph(def *GraphDefinition) bool {
	if def == nil || def.Metadata == nil {
		return false
	}
	v, _ := def.Metadata[GraphMetadataTeamOwnedKey].(bool)
	return v
}

// reconcileTeamGraphForSave applies the Team × Graph save hook（B3/B4/B8）：
//
//	preset（含缺省）：compile → 物化 owned 图资产 → 回写 linked_graph_id + source
//	custom + 拓扑字段变更：按 preset 重建（前端已确认覆盖，D.4 后端不二次拒绝）
//	custom 拓扑未变：保留资产不动
//	linked_external：校验目标存在且非他队 owned 图（B8）；换绑删除旧 owned 图（D2）
//
// team.DefinitionJSON / team.LinkedGraphID 原地更新。物化失败即保存失败
// （D1——调用方须把 reconcile + team 写入包在同一事务内）.
func (u *TeamUsecase) reconcileTeamGraphForSave(ctx context.Context, team *Team, prev *Team) error {
	if u.graphAssets == nil || u.compiler == nil {
		return nil // 图集成未装配（单测/离线工具）：保持既有行为
	}
	spec, err := ParseOrchestrationSpec(team.DefinitionJSON)
	if err != nil {
		return apierror.BadRequest("TEAM", "definition_json must be valid JSON")
	}
	prevSpec, prevLinked := prevOrchestration(prev)

	source := spec.GraphSource()
	if source == DefinitionGraphSourceLinkedExt {
		return u.bindExternalGraph(ctx, team, &spec, prevLinked)
	}
	if source == DefinitionGraphSourceCustom && prevSpec != nil &&
		prevSpec.GraphSource() == DefinitionGraphSourceCustom &&
		teamTopologyFingerprint(spec) == teamTopologyFingerprint(*prevSpec) {
		// 表单未触碰拓扑字段：保留 custom 资产不动（B4）。
		team.LinkedGraphID = firstNonEmpty(strings.TrimSpace(spec.LinkedGraphID), prevLinked)
		return nil
	}
	// preset 幂等保存（如仅改名）：跳过重建，避免版本历史噪音。
	// 注意：preset 下「资产 == 表单派生」由本钩子维持不变量，指纹相等即拓扑相等。
	if source == DefinitionGraphSourcePreset && prevSpec != nil &&
		prevSpec.GraphSource() == DefinitionGraphSourcePreset &&
		teamTopologyFingerprint(spec) == teamTopologyFingerprint(*prevSpec) &&
		firstNonEmpty(strings.TrimSpace(spec.LinkedGraphID), prevLinked) != "" {
		team.LinkedGraphID = firstNonEmpty(strings.TrimSpace(spec.LinkedGraphID), prevLinked)
		return nil
	}
	// preset（含 custom→重置为派生 / custom+拓扑变更）：按表单重建。
	return u.materializeAndBind(ctx, team, &spec, prevLinked)
}

// bindExternalGraph validates and binds an independent graph asset（B8）.
func (u *TeamUsecase) bindExternalGraph(ctx context.Context, team *Team, spec *OrchestrationSpec, prevLinked string) error {
	targetID := strings.TrimSpace(spec.LinkedGraphID)
	if targetID == "" {
		return apierror.BadRequest("TEAM", "linked_external source requires linked_graph_id")
	}
	if u.graphReader == nil {
		return apierror.Internal("TEAM", "graph reader not configured")
	}
	target, err := u.graphReader.GetDefinition(ctx, targetID)
	if err != nil || target == nil {
		return apierror.BadRequest("TEAM", "linked graph %s not found", targetID)
	}
	// 循环关联防御（B8）：external 目标不得是其他 team 的 owned 图，
	// 避免 team A 的图被 team B 关联后级联删除误伤。
	if isTeamOwnedGraph(target) && target.TeamID != "" && target.TeamID != team.ID {
		return apierror.Conflict("TEAM", "graph %s is owned by another team; link an independent graph instead", targetID)
	}
	// D2：换绑时删除本 team 的旧 owned 图（历史 run 靠 definition_snapshot_json 回放）。
	if prevLinked != "" && prevLinked != targetID {
		if derr := u.deleteOwnedGraphAsset(ctx, prevLinked, team.ID); derr != nil {
			return derr
		}
	}
	spec.Graph = nil
	spec.Source = DefinitionGraphSourceLinkedExt
	out, err := OrchestrationSpecToDefinitionJSON(*spec)
	if err != nil {
		return err
	}
	team.DefinitionJSON = out
	team.LinkedGraphID = targetID
	return nil
}

// materializeAndBind compiles the form definition and persists it as the
// team's owned graph asset, writing back linked_graph_id + source（B3）.
func (u *TeamUsecase) materializeAndBind(ctx context.Context, team *Team, spec *OrchestrationSpec, prevLinked string) error {
	spec.Graph = nil // D.1：不再写 embedded graph；拓扑由表单派生
	spec.Source = DefinitionGraphSourcePreset
	raw, err := OrchestrationSpecToDefinitionJSON(*spec)
	if err != nil {
		return err
	}
	agentKey := func(agentID string) string { return agentID }
	if u.agentKeyResolver != nil {
		if fn := u.agentKeyResolver(ctx); fn != nil {
			agentKey = fn
		}
	}
	cfg, cerr := u.compiler.CompileFromDefinition(teamDefinitionForCompile(team, spec, raw), agentKey)
	if cerr != nil {
		return cerr // D1：物化失败（无启用成员/校验不过）→ 保存整体失败
	}
	cfg.EnableCheckpoint = spec.EnableCheckpoint

	linkedID := firstNonEmpty(strings.TrimSpace(spec.LinkedGraphID), prevLinked)
	var existing *GraphDefinition
	if linkedID != "" && u.graphReader != nil {
		// 仅当既有 linked 图是本 team 的 owned 资产时才原地更新（保版本历史与
		// graph_id 连续性）；external/野图一律新建 owned 资产，旧图由
		// syncGraphTeamID 解绑，绝不误删。
		if g, gerr := u.graphReader.GetDefinition(ctx, linkedID); gerr == nil && g != nil &&
			isTeamOwnedGraph(g) && g.TeamID == team.ID {
			existing = g
		}
	}
	asset := MaterializeTeamGraphDefinition(*team, cfg, existing, DefinitionGraphSourcePreset)
	var saved *GraphDefinition
	if existing == nil {
		saved, err = u.graphAssets.CreateGraph(ctx, asset)
	} else {
		saved, err = u.graphAssets.UpdateOwnedGraph(ctx, asset)
	}
	if err != nil {
		return err
	}
	spec.LinkedGraphID = saved.ID
	out, err := OrchestrationSpecToDefinitionJSON(*spec)
	if err != nil {
		return err
	}
	team.DefinitionJSON = out
	team.LinkedGraphID = saved.ID
	u.lg.Info("team graph materialized",
		loggateway.Str("team_id", team.ID),
		loggateway.Str("graph_id", saved.ID),
		loggateway.Str("source", spec.GraphSource()))
	return nil
}

// deleteOwnedGraphAsset deletes a previously linked graph only when it is an
// owned asset of this team（D2）；external/他队图一律不动.
func (u *TeamUsecase) deleteOwnedGraphAsset(ctx context.Context, graphID, teamID string) error {
	if u.graphReader == nil || strings.TrimSpace(graphID) == "" {
		return nil
	}
	g, err := u.graphReader.GetDefinition(ctx, graphID)
	if err != nil || g == nil {
		return nil // 已不存在
	}
	if !isTeamOwnedGraph(g) || g.TeamID != teamID {
		return nil
	}
	return u.graphAssets.DeleteOwnedGraph(ctx, graphID)
}

// prevOrchestration extracts the pre-patch spec + effective linked graph id
// （spec 级优先，列级兜底）for source/topology diffing.
func prevOrchestration(prev *Team) (*OrchestrationSpec, string) {
	if prev == nil {
		return nil, ""
	}
	linked := strings.TrimSpace(prev.LinkedGraphID)
	raw := strings.TrimSpace(prev.DefinitionJSON)
	if raw == "" {
		return nil, linked
	}
	spec, err := ParseOrchestrationSpec(raw)
	if err != nil {
		return nil, linked
	}
	if strings.TrimSpace(spec.LinkedGraphID) != "" {
		linked = strings.TrimSpace(spec.LinkedGraphID)
	}
	return &spec, linked
}

// teamDefinitionForCompile builds the compiler input from the form-owned spec
// fields（spec.Graph 已清空，编译走 mode 模板派生路径）.
func teamDefinitionForCompile(team *Team, spec *OrchestrationSpec, raw string) TeamDefinition {
	def := TeamDefinition{
		ID:                team.ID,
		Name:              team.DisplayName,
		Mode:              spec.Mode,
		RawDefinitionJSON: raw,
	}
	for _, m := range spec.Members {
		def.Members = append(def.Members, TeamMember{
			AgentID:    strings.TrimSpace(m.AgentID),
			Role:       strings.TrimSpace(m.Role),
			TaskPrompt: m.TaskPrompt,
			Enabled:    m.Enabled(),
			Name:       strings.TrimSpace(m.Name),
		})
	}
	if spec.FailurePolicy != nil {
		def.FailurePolicy = *spec.FailurePolicy
	}
	return def
}

// teamTopologyFingerprint hashes the form-owned topology fields so the save
// hook can distinguish「表单改了拓扑字段」（触发重建）from cosmetic edits
// （B4）。EnabledPtr 归一化为 Enabled()，避免 nil 与 *true 的伪差异。
func teamTopologyFingerprint(spec OrchestrationSpec) string {
	type fpMember struct {
		AgentID string `json:"agent_id"`
		Role    string `json:"role"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Prompt  string `json:"task_prompt,omitempty"`
		Order   int    `json:"sort_order"`
	}
	payload := struct {
		Mode       string             `json:"mode"`
		Members    []fpMember         `json:"members"`
		Checkpoint bool               `json:"enable_checkpoint"`
		Synth      string             `json:"synthesizer_agent_id,omitempty"`
		FP         *TeamFailurePolicy `json:"failure_policy,omitempty"`
		CL         *CriticLoopSpec    `json:"critic_loop,omitempty"`
	}{
		Mode:       strings.ToLower(strings.TrimSpace(spec.Mode)),
		Checkpoint: spec.EnableCheckpoint,
		Synth:      strings.TrimSpace(spec.SynthesizerAgentID),
		FP:         spec.FailurePolicy,
		CL:         spec.CriticLoop,
	}
	for _, m := range spec.Members {
		payload.Members = append(payload.Members, fpMember{
			AgentID: strings.TrimSpace(m.AgentID),
			Role:    strings.TrimSpace(m.Role),
			Name:    strings.TrimSpace(m.Name),
			Enabled: m.Enabled(),
			Prompt:  m.TaskPrompt,
			Order:   m.SortOrder,
		})
	}
	b, _ := json.Marshal(payload)
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}
