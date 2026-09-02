package biz

import "strings"

// Graph metadata keys for team-owned materialized assets（Team × Graph 一体化 Phase 11）。
const (
	// GraphMetadataLayoutKey mirrors frontend GRAPH_LAYOUT_METADATA_KEY ('layout'):
	// Record<nodeID, {x, y}>。前端打开图时优先使用已存坐标，缺失节点自动布局。
	GraphMetadataLayoutKey = "layout"
	// GraphMetadataTeamSourceKey mirrors OrchestrationSpec.Source on the graph asset.
	GraphMetadataTeamSourceKey = "team_source"
	// GraphMetadataTeamOwnedKey marks a graph asset materialized/owned by a team.
	// 删除保护（B7）与换绑校验（B8）以此判定 owned vs external。
	GraphMetadataTeamOwnedKey = "team_owned"
	// GraphMetadataTeamAutoCreatedKey marks an owned graph whose owner team was
	// auto-created by spirit/orchestration（会话一次性团队）。GraphsPage 据此
	// 默认隔离编排产物（镜像 TeamsPage 的 showOrchestrated 语义）。
	GraphMetadataTeamAutoCreatedKey = "team_auto_created"
)

// MaterializeTeamGraphDefinition converts a compiled team graph (GraphBuildConfig)
// into a persistable GraphDefinition asset owned by the team.
//
// existing 非 nil 时视为更新路径：保留 ID/Version/Description/未知 metadata 键，
// 并继承未变更节点的 layout 坐标（移除节点的坐标被清理，新节点不留坐标由前端自动布局）。
// source 取 DefinitionGraphSourcePreset / DefinitionGraphSourceCustom。
//
// 纯函数：不做 IO，可单测。持久化（含版本历史快照）由 GraphDefinitionUsecase 负责。
func MaterializeTeamGraphDefinition(team Team, cfg GraphBuildConfig, existing *GraphDefinition, source string) *GraphDefinition {
	def := &GraphDefinition{
		TeamID:           team.ID,
		Name:             team.DisplayName,
		Nodes:            cfg.Nodes,
		Edges:            cfg.Edges,
		ConditionalEdges: cfg.ConditionalEdges,
		Subgraphs:        cfg.Subgraphs,
		StateFields:      cfg.StateFields,
		EntryPoint:       cfg.EntryPoint,
		FinishPoint:      cfg.FinishPoint,
		EnableCheckpoint: cfg.EnableCheckpoint,
		ExecutionEngine:  cfg.ExecutionEngine,
		InterruptBefore:  cfg.InterruptBefore,
		InterruptAfter:   cfg.InterruptAfter,
		Metadata:         map[string]any{},
	}
	if existing != nil {
		def.ID = existing.ID
		def.Version = existing.Version
		def.CreatedAt = existing.CreatedAt
		if strings.TrimSpace(existing.Description) != "" {
			def.Description = existing.Description
		}
		for k, v := range existing.Metadata {
			if k == GraphMetadataLayoutKey || k == GraphMetadataTeamSourceKey || k == GraphMetadataTeamOwnedKey {
				continue
			}
			def.Metadata[k] = v
		}
	}
	def.Metadata[GraphMetadataLayoutKey] = inheritGraphLayout(cfg, existing)
	def.Metadata[GraphMetadataTeamOwnedKey] = true
	def.Metadata[GraphMetadataTeamSourceKey] = source
	// 编排产物标记：属主团队为会话自动创建时打标，前端据此默认隔离。
	// 更新路径需以属主现状为准（不允许 existing 残留旧值），故在下方统一覆盖。
	if team.AutoCreated {
		def.Metadata[GraphMetadataTeamAutoCreatedKey] = true
	} else {
		delete(def.Metadata, GraphMetadataTeamAutoCreatedKey)
	}
	return def
}

// inheritGraphLayout keeps positions of node IDs still present in cfg; drops stale nodes.
func inheritGraphLayout(cfg GraphBuildConfig, existing *GraphDefinition) map[string]any {
	layout := map[string]any{}
	if existing == nil || existing.Metadata == nil {
		return layout
	}
	raw, _ := existing.Metadata[GraphMetadataLayoutKey].(map[string]any)
	if len(raw) == 0 {
		return layout
	}
	for _, n := range cfg.Nodes {
		if pos, ok := raw[n.ID]; ok {
			layout[n.ID] = pos
		}
	}
	return layout
}
