package pack

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ImporterRepo 导入引擎所需的写入仓库接口。
type ImporterRepo interface {
	// Taxonomy
	CreateTaxonomyNode(ctx context.Context, node biz.TaxonomyNode) (biz.TaxonomyNode, error)
	UpdateTaxonomyNode(ctx context.Context, node biz.TaxonomyNode) (biz.TaxonomyNode, error)
	GetTaxonomyNodeByKey(ctx context.Context, key string) (biz.TaxonomyNode, error)
	ListTaxonomyNodesByParentID(ctx context.Context, parentID string) ([]biz.TaxonomyNode, error)

	// Agent
	GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error)
	CreateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error)
	UpdateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error)
	DeleteAgent(ctx context.Context, id string) error
	GetAgentRuntimeSettings(ctx context.Context, agentID string) (biz.AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error)
	ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error)

	// Team
	GetTeamByID(ctx context.Context, id string) (biz.Team, error)
	GetTeamByKey(ctx context.Context, teamKey string) (biz.Team, error)
	CreateTeam(ctx context.Context, t biz.Team) (biz.Team, error)
	UpdateTeam(ctx context.Context, t biz.Team) (biz.Team, error)

	// Graph
	SaveGraphDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error)
}

// importConfig holds optional configuration for an Import call.
type importConfig struct {
	kindOverride string // if set, override agent/team kind
}

// ImportOption configures an Import call.
type ImportOption func(*importConfig)

// WithKindOverride overrides the kind field for all imported agents and teams.
func WithKindOverride(kind string) ImportOption {
	return func(c *importConfig) { c.kindOverride = kind }
}

// Importer 导入引擎。
type Importer struct {
	repo ImporterRepo
}

// NewImporter 创建导入引擎。
func NewImporter(repo ImporterRepo) *Importer {
	return &Importer{repo: repo}
}

// Import 执行 Pack 导入。
func (im *Importer) Import(ctx context.Context, p *Pack, strategy ConflictStrategy, opts ...ImportOption) (*ImportResult, error) {
	cfg := &importConfig{}
	for _, o := range opts {
		o(cfg)
	}

	result := &ImportResult{}
	mapper := NewKeyMapper()

	// Phase 1: Taxonomy
	if p.Taxonomy != nil {
		count, warns, err := im.importTaxonomy(ctx, p.Taxonomy, strategy, mapper)
		if err != nil {
			return result, kerrors.BadRequest("PACK_TAXONOMY_IMPORT", fmt.Sprintf("pack import: Phase 1 (Taxonomy) 失败: %s", err.Error()))
		}
		result.TaxonomyNodes = count
		result.Warnings = append(result.Warnings, warns...)
	}

	// Phase 2: Agents
	for _, agentSpec := range p.Agents {
		created, updated, skipped, err := im.importAgent(ctx, agentSpec, p.AgentFiles, strategy, mapper, cfg)
		if err != nil {
			result.Failures = append(result.Failures, ImportFailure{
				EntityType: "agent",
				Key:        agentSpec.Key,
				Reason:     err.Error(),
			})
			continue
		}
		result.AgentsCreated += created
		result.AgentsUpdated += updated
		result.AgentsSkipped += skipped
	}

	// Phase 3: Graphs
	for _, graphSpec := range p.Graphs {
		created, err := im.importGraph(ctx, graphSpec, mapper)
		if err != nil {
			result.Failures = append(result.Failures, ImportFailure{
				EntityType: "graph",
				Key:        graphSpec.ID,
				Reason:     err.Error(),
			})
			continue
		}
		result.GraphsCreated += created
	}

	// Phase 4: Teams
	for _, teamSpec := range p.Teams {
		created, updated, skipped, err := im.importTeam(ctx, teamSpec, strategy, mapper, cfg)
		if err != nil {
			result.Failures = append(result.Failures, ImportFailure{
				EntityType: "team",
				Key:        teamSpec.Key,
				Reason:     err.Error(),
			})
			continue
		}
		result.TeamsCreated += created
		result.TeamsUpdated += updated
		result.TeamsSkipped += skipped
	}

	return result, nil
}

// importTaxonomy 导入行业分类树。
func (im *Importer) importTaxonomy(ctx context.Context, spec *TaxonomyPackSpec, strategy ConflictStrategy, mapper *KeyMapper) (int, []string, error) {
	count := 0
	var warnings []string

	for _, ind := range spec.Industries {
		indNode, err := im.upsertTaxonomyNode(ctx, biz.TaxonomyNode{
			Key:         ind.Key,
			Name:        ind.Name,
			Description: ind.Description,
			Level:       "industry",
			SortOrder:   ind.SortOrder,
			IsSystem:    true,
		}, strategy)
		if err != nil {
			return count, warnings, kerrors.BadRequest("PACK_INDUSTRY_IMPORT", fmt.Sprintf("导入行业 %s 失败: %s", ind.Key, err.Error()))
		}
		mapper.RegisterTaxonomy(ind.Key, indNode.ID)
		count++

		for _, dept := range ind.Departments {
			deptNode, err := im.upsertTaxonomyNode(ctx, biz.TaxonomyNode{
				Key:         dept.Key,
				Name:        dept.Name,
				Description: dept.Description,
				ParentID:    indNode.ID,
				Level:       "department",
				SortOrder:   dept.SortOrder,
				IsSystem:    true,
			}, strategy)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("导入部门 %s/%s 失败，跳过其子节点", ind.Key, dept.Key))
				continue
			}
			deptKey := BuildTaxonomyKey(ind.Key, dept.Key, "")
			mapper.RegisterTaxonomy(deptKey, deptNode.ID)
			count++

			for _, pos := range dept.Positions {
				posNode, err := im.upsertTaxonomyNode(ctx, biz.TaxonomyNode{
					Key:         pos.Key,
					Name:        pos.Name,
					Description: pos.Description,
					ParentID:    deptNode.ID,
					Level:       "position",
					SortOrder:   pos.SortOrder,
					IsSystem:    true,
				}, strategy)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("导入岗位 %s/%s/%s 失败", ind.Key, dept.Key, pos.Key))
					continue
				}
				posKey := BuildTaxonomyKey(ind.Key, dept.Key, pos.Key)
				mapper.RegisterTaxonomy(posKey, posNode.ID)
				count++
			}
		}
	}

	return count, warnings, nil
}

// importAgent 导入单个 Agent。
func (im *Importer) importAgent(ctx context.Context, spec AgentPackSpec, agentFiles map[string]map[string]string, strategy ConflictStrategy, mapper *KeyMapper, cfg *importConfig) (created, updated, skipped int, err error) {
	// 保存原始 key（duplicate 场景下用于映射）
	originalKey := spec.Key

	// 检查是否已存在
	existing, findErr := im.repo.GetAgentByAgentKey(ctx, spec.Key)

	if findErr == nil {
		// Agent 已存在
		switch strategy {
		case ConflictSkip:
			mapper.RegisterAgent(spec.Key, existing.ID)
			return 0, 0, 1, nil
		case ConflictDuplicate:
			// 循环追加后缀直到找到不冲突的 key
			newKey := spec.Key + "-copy"
			for i := 2; ; i++ {
				_, err := im.repo.GetAgentByAgentKey(ctx, newKey)
				if err != nil {
					break // key 可用
				}
				newKey = fmt.Sprintf("%s-copy-%d", spec.Key, i)
			}
			spec.Key = newKey
		case ConflictOverwrite:
			// 继续更新，保留原始 Status/Readonly/Source
		}
	}

	// 构建 biz.Agent
	kind := cfg.kindOverride
	if kind == "" {
		kind = firstNonEmpty(spec.Kind, "llm")
	}
	source := "imported"
	if cfg.kindOverride != "" {
		source = cfg.kindOverride
	}
	agent := biz.Agent{
		AgentKey:           spec.Key,
		DisplayName:        spec.DisplayName,
		AgentDescription:   spec.Description,
		Icon:               spec.Icon,
		AgentVariant:       spec.Variant,
		VariantDescription: spec.VariantDescription,
		Provider:           spec.Provider,
		Model:              spec.Model,
		SystemPromptMode:   spec.SystemPromptMode,
		ContextWindow:      spec.ContextWindow,
		Kind:               kind,
		Status:             "active",
		Readonly:           false,
		Source:             source,
	}

	// overwrite 时保留原始 Status/Readonly/Source
	if findErr == nil && strategy == ConflictOverwrite {
		agent.Status = existing.Status
		agent.Readonly = existing.Readonly
		agent.Source = existing.Source
	}

	// 解析 position_key → taxonomy_position_id
	if spec.PositionKey != "" {
		posID, err := mapper.ResolvePositionKey(spec.PositionKey)
		if err == nil {
			agent.TaxonomyPositionID = posID
		}
		// 同时设置 position_key（取路径最后一段）
		_, _, posKey, _ := ParseTaxonomyKeyPath(spec.PositionKey)
		agent.PositionKey = posKey
	}

	// A2A Proxy
	if spec.A2AProxy != nil {
		agent.A2AProxy = &biz.A2AProxyConfig{
			RemoteURL:      spec.A2AProxy.RemoteURL,
			AgentCardURL:   spec.A2AProxy.AgentCardURL,
			EnableStreaming: spec.A2AProxy.EnableStreaming,
			AuthType:       spec.A2AProxy.AuthType,
			TimeoutSeconds: spec.A2AProxy.TimeoutSeconds,
		}
	}

	var agentID string
	if findErr == nil && strategy == ConflictOverwrite {
		// 更新
		agent.ID = existing.ID
		updatedAgent, err := im.repo.UpdateAgent(ctx, agent)
		if err != nil {
			return 0, 0, 0, kerrors.BadRequest("PACK_AGENT_UPDATE", fmt.Sprintf("更新 Agent %s 失败: %s", spec.Key, err.Error()))
		}
		agentID = updatedAgent.ID
		updated = 1
	} else {
		// 创建
		createdAgent, err := im.repo.CreateAgent(ctx, agent)
		if err != nil {
			return 0, 0, 0, kerrors.BadRequest("PACK_AGENT_CREATE", fmt.Sprintf("创建 Agent %s 失败: %s", spec.Key, err.Error()))
		}
		agentID = createdAgent.ID
		created = 1
	}

	// 注册映射：新 key → 新 ID
	mapper.RegisterAgent(spec.Key, agentID)
	// duplicate 策略下，同时注册原始 key → 新 ID，确保后续 Team/Graph 引用能解析
	if strategy == ConflictDuplicate && spec.Key != originalKey {
		mapper.RegisterAgent(originalKey, agentID)
	}

	// 写入 Prompt Files
	if files, ok := agentFiles[spec.Key]; ok && len(files) > 0 {
		// 按文件名排序保证 SortOrder 确定性
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		var promptFiles []biz.AgentPromptFile
		for i, name := range names {
			promptFiles = append(promptFiles, biz.AgentPromptFile{
				AgentID:   agentID,
				Name:      name,
				Body:      files[name],
				SortOrder: i,
			})
		}
		if _, err := im.repo.ReplaceAgentPromptFiles(ctx, agentID, promptFiles); err != nil {
			// 回滚：删除已创建的 Agent
			if created == 1 {
				if delErr := im.repo.DeleteAgent(ctx, agentID); delErr != nil {
					// 回滚失败：记录到错误信息中
					return 0, 0, 0, kerrors.BadRequest("PACK_AGENT_FILES", fmt.Sprintf("写入 Agent %s 文件失败: %s（回滚删除也失败: %v）", spec.Key, err.Error(), delErr))
				}
			}
			return 0, 0, 0, kerrors.BadRequest("PACK_AGENT_FILES", fmt.Sprintf("写入 Agent %s 文件失败: %s", spec.Key, err.Error()))
		}
	}

	// 写入 RuntimeSettings
	if spec.Runtime != nil {
		settings := im.buildRuntimeSettings(agentID, spec)
		if _, err := im.repo.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
			// 回滚：删除已创建的 Agent（Files 已通过 Replace 覆盖，无需单独清理）
			if created == 1 {
				if delErr := im.repo.DeleteAgent(ctx, agentID); delErr != nil {
					return 0, 0, 0, kerrors.BadRequest("PACK_AGENT_RUNTIME", fmt.Sprintf("写入 Agent %s 运行时设置失败: %s（回滚删除也失败: %v）", spec.Key, err.Error(), delErr))
				}
			}
			return 0, 0, 0, kerrors.BadRequest("PACK_AGENT_RUNTIME", fmt.Sprintf("写入 Agent %s 运行时设置失败: %s", spec.Key, err.Error()))
		}
	}

	return created, updated, skipped, nil
}

// importGraph 导入单个 Graph。
func (im *Importer) importGraph(ctx context.Context, spec GraphPackSpec, mapper *KeyMapper) (int, error) {
	def := &biz.GraphDefinition{
		Name:             spec.Name,
		Description:      spec.Description,
		EntryPoint:       spec.EntryPoint,
		FinishPoint:      spec.FinishPoint,
		EnableCheckpoint: spec.EnableCheckpoint,
		Version:          spec.Version,
		SortOrder:        spec.SortOrder,
		InterruptBefore:  spec.InterruptBefore,
		InterruptAfter:   spec.InterruptAfter,
	}
	if spec.ExecutionEngine != "" {
		def.ExecutionEngine = biz.ExecutionEngineType(spec.ExecutionEngine)
	}

	// StateFields
	for _, sf := range spec.StateFields {
		def.StateFields = append(def.StateFields, biz.StateFieldDef{
			Name:            sf.Name,
			Type:            sf.Type,
			Reducer:         biz.ReducerType(sf.Reducer),
			DefaultValue:    sf.DefaultValue,
			Required:        sf.Required,
			DisableDeepCopy: sf.DisableDeepCopy,
		})
	}

	// Nodes
	for _, n := range spec.Nodes {
		nodeDef := biz.NodeDef{
			ID:                    n.ID,
			Type:                  n.Type,
			Description:           n.Description,
			FuncRef:               n.FuncRef,
			Instruction:           n.Instruction,
			ModelName:             n.ModelName,
			ToolNames:             n.ToolNames,
			AgentName:             n.AgentKey, // AgentName 存储 agent_key
			InterruptBefore:       n.InterruptBefore,
			InterruptAfter:        n.InterruptAfter,
			Destinations:          n.Destinations,
			RetryMaxAttempts:      n.RetryMaxAttempts,
			FailureAction:         n.FailureAction,
			FallbackAgent:         n.FallbackAgent,
			InputMapperJSON:       n.InputMapperJSON,
			OutputMapperJSON:      n.OutputMapperJSON,
			IsolatedMessages:      n.IsolatedMessages,
			InputFromLastResponse: n.InputFromLastResponse,
			CacheEnabled:          n.CacheEnabled,
			CacheTTLSeconds:       n.CacheTTLSeconds,
		}
		def.Nodes = append(def.Nodes, nodeDef)
	}

	// Edges
	for _, e := range spec.Edges {
		def.Edges = append(def.Edges, biz.EdgeDef{From: e.From, To: e.To, Kind: e.Kind})
	}

	// ConditionalEdges
	for _, ce := range spec.ConditionalEdges {
		def.ConditionalEdges = append(def.ConditionalEdges, biz.ConditionalEdgeDef{
			From:        ce.From,
			CondFuncRef: ce.CondFuncRef,
			PathMap:     ce.PathMap,
		})
	}

	// Subgraphs
	for _, sg := range spec.Subgraphs {
		subgraphDef := biz.SubgraphDef{
			ID:          sg.ID,
			InterruptBefore: false,
			InterruptAfter:  false,
		}
		// Build subgraph's BuildConfig
		var nodes []biz.NodeDef
		for _, n := range sg.Nodes {
			nodes = append(nodes, biz.NodeDef{
				ID:                    n.ID,
				Type:                  n.Type,
				Description:           n.Description,
				FuncRef:               n.FuncRef,
				Instruction:           n.Instruction,
				ModelName:             n.ModelName,
				ToolNames:             n.ToolNames,
				AgentName:             n.AgentKey,
				InterruptBefore:       n.InterruptBefore,
				InterruptAfter:        n.InterruptAfter,
				Destinations:          n.Destinations,
				RetryMaxAttempts:      n.RetryMaxAttempts,
				FailureAction:         n.FailureAction,
				FallbackAgent:         n.FallbackAgent,
				InputMapperJSON:       n.InputMapperJSON,
				OutputMapperJSON:      n.OutputMapperJSON,
				IsolatedMessages:      n.IsolatedMessages,
				InputFromLastResponse: n.InputFromLastResponse,
				CacheEnabled:          n.CacheEnabled,
				CacheTTLSeconds:       n.CacheTTLSeconds,
			})
		}
		var edges []biz.EdgeDef
		for _, e := range sg.Edges {
			edges = append(edges, biz.EdgeDef{From: e.From, To: e.To, Kind: e.Kind})
		}
		subgraphDef.BuildConfig = biz.GraphBuildConfig{
			Nodes:       nodes,
			Edges:       edges,
			EntryPoint:  sg.EntryPoint,
			FinishPoint: sg.FinishPoint,
		}
		def.Subgraphs = append(def.Subgraphs, subgraphDef)
	}

	saved, err := im.repo.SaveGraphDefinition(ctx, def)
	if err != nil {
		return 0, kerrors.BadRequest("PACK_GRAPH_CREATE", fmt.Sprintf("创建 Graph %s 失败: %s", spec.ID, err.Error()))
	}

	mapper.RegisterGraph(spec.ID, saved.ID)
	return 1, nil
}

// importTeam 导入单个 Team。
func (im *Importer) importTeam(ctx context.Context, spec TeamPackSpec, strategy ConflictStrategy, mapper *KeyMapper, cfg *importConfig) (created, updated, skipped int, err error) {
	// 构建 OrchestrationSpec
	ospec := biz.OrchestrationSpec{
		Version:            2,
		Mode:               spec.Mode,
		Description:        spec.Description,
		MaxConcurrency:     spec.MaxConcurrency,
		TimeoutSeconds:     spec.TimeoutSeconds,
		RunTimeoutSec:      spec.RunTimeoutSec,
		TurnTimeoutSec:     spec.TurnTimeoutSec,
		FirstByteTimeoutSec: spec.FirstByteTimeoutSec,
		LoopMaxIterations:  spec.LoopMaxIter,
		EnableCheckpoint:   spec.EnableCheckpoint,
		TeamGraphRuntime:   spec.TeamGraphRuntime,
	}
	if spec.RuntimeEngine != "" {
		ospec.RuntimeEngine = spec.RuntimeEngine
	} else {
		ospec.RuntimeEngine = biz.RuntimeEngineGraph
	}

	// 成员：agent_key → agent_id
	for _, m := range spec.Members {
		agentID, err := mapper.ResolveAgentKey(m.AgentKey)
		if err != nil {
			return 0, 0, 0, kerrors.BadRequest("PACK_TEAM_MEMBER", fmt.Sprintf("Team %s 成员 %s 的 agent_key 未找到: %s", spec.Key, m.AgentKey, err.Error()))
		}
		enabled := true
		if m.Enabled != nil {
			enabled = *m.Enabled
		}
		ospec.Members = append(ospec.Members, biz.OrchestrationMember{
			AgentID:    agentID,
			Role:       m.Role,
			Name:       m.Name,
			TaskPrompt: m.TaskPrompt,
			Enabled:    enabled,
			SortOrder:  m.SortOrder,
		})
	}

	// IntentAnchor / Synthesizer
	if spec.IntentAnchorKey != "" {
		id, err := mapper.ResolveAgentKey(spec.IntentAnchorKey)
		if err == nil {
			ospec.IntentAnchorAgentID = id
		}
	}
	if spec.SynthesizerKey != "" {
		id, err := mapper.ResolveAgentKey(spec.SynthesizerKey)
		if err == nil {
			ospec.SynthesizerAgentID = id
		}
	}

	// Graph
	if spec.Graph != nil {
		if spec.Graph.Linked && spec.Graph.LinkedGraphID != "" {
			newID, ok := mapper.GraphID(spec.Graph.LinkedGraphID)
			if ok {
				ospec.LinkedGraphID = newID
			}
		} else if len(spec.Graph.Nodes) > 0 {
			ospec.Graph = im.buildEmbeddedGraph(spec.Graph, mapper)
		}
	}

	// FailurePolicy
	if spec.FailurePolicy != nil {
		fp := &biz.TeamFailurePolicy{
			Default:       spec.FailurePolicy.Default,
			ParallelFail:  spec.FailurePolicy.ParallelFail,
			OnError:       spec.FailurePolicy.OnError,
		}
		if spec.FailurePolicy.Retry != nil {
			fp.Retry = biz.TeamRetryPolicy{
				MaxAttempts:       spec.FailurePolicy.Retry.MaxAttempts,
				InitialIntervalMs: spec.FailurePolicy.Retry.InitialIntervalMs,
				BackoffFactor:     spec.FailurePolicy.Retry.BackoffFactor,
			}
		}
		if len(spec.FailurePolicy.NodeOverrides) > 0 {
			fp.NodeOverrides = make(map[string]biz.TeamNodeFailureOverride, len(spec.FailurePolicy.NodeOverrides))
			for k, v := range spec.FailurePolicy.NodeOverrides {
				fp.NodeOverrides[k] = biz.TeamNodeFailureOverride{Policy: v.Action}
			}
		}
		if spec.FailurePolicy.CircuitBreaker != nil {
			fp.CircuitBreaker = &biz.CircuitBreakerPolicy{
				FailureThreshold:    spec.FailurePolicy.CircuitBreaker.FailureThreshold,
				ResetTimeoutSeconds: spec.FailurePolicy.CircuitBreaker.RecoveryTimeoutMs / 1000,
			}
		}
		ospec.FailurePolicy = fp
	}

	// CriticLoop
	if spec.CriticLoop != nil {
		ospec.CriticLoop = &biz.CriticLoopSpec{
			MaxIterations:  spec.CriticLoop.MaxIterations,
			ScoreThreshold: spec.CriticLoop.ScoreThreshold,
		}
	}

	// 序列化 definition_json
	defJSON, err := biz.OrchestrationSpecToDefinitionJSON(ospec)
	if err != nil {
		return 0, 0, 0, kerrors.BadRequest("PACK_TEAM_DEFINITION", fmt.Sprintf("序列化 Team %s definition_json 失败: %s", spec.Key, err.Error()))
	}

	teamSource := "imported"
	if cfg.kindOverride != "" {
		teamSource = cfg.kindOverride
	}
	team := biz.Team{
		TeamKey:        spec.Key,
		DisplayName:    spec.DisplayName,
		DefinitionJSON: defJSON,
		Status:         biz.TeamStatusPending,
		Source:         teamSource,
		Readonly:       false,
	}

	// 检查是否已存在（通过 key 查找）
	existing, findErr := im.repo.GetTeamByKey(ctx, spec.Key)

	if findErr == nil {
		// Team 已存在
		switch strategy {
		case ConflictSkip:
			return 0, 0, 1, nil
		case ConflictOverwrite:
			team.ID = existing.ID
			// 保留原始 Status/Readonly/Source
			team.Status = existing.Status
			team.Readonly = existing.Readonly
			team.Source = existing.Source
			if _, err := im.repo.UpdateTeam(ctx, team); err != nil {
				return 0, 0, 0, kerrors.BadRequest("PACK_TEAM_UPDATE", fmt.Sprintf("更新 Team %s 失败: %s", spec.Key, err.Error()))
			}
			return 0, 1, 0, nil
		case ConflictDuplicate:
			// 循环追加后缀直到找到不冲突的 key
			newKey := spec.Key + "-copy"
			for i := 2; ; i++ {
				_, err := im.repo.GetTeamByKey(ctx, newKey)
				if err != nil {
					break // key 可用
				}
				newKey = fmt.Sprintf("%s-copy-%d", spec.Key, i)
			}
			team.TeamKey = newKey
		}
	}

	if _, err := im.repo.CreateTeam(ctx, team); err != nil {
		return 0, 0, 0, kerrors.BadRequest("PACK_TEAM_CREATE", fmt.Sprintf("创建 Team %s 失败: %s", spec.Key, err.Error()))
	}

	return 1, 0, 0, nil
}

// buildEmbeddedGraph 从 TeamGraphPackSpec 构建 EmbeddedGraphSpec。
func (im *Importer) buildEmbeddedGraph(spec *TeamGraphPackSpec, mapper *KeyMapper) *biz.EmbeddedGraphSpec {
	eg := &biz.EmbeddedGraphSpec{Version: 1, Layout: spec.Layout}
	for _, n := range spec.Nodes {
		nodeSpec := biz.EmbeddedGraphNodeSpec{
			ID:               n.ID,
			Type:             n.Type,
			Label:            n.Label,
			Role:             n.Role,
			InterruptBefore:  n.InterruptBefore,
			InterruptAfter:   n.InterruptAfter,
			Destinations:     n.Destinations,
			RetryMaxAttempts: n.RetryMaxAttempts,
			FallbackAgent:    n.FallbackAgent,
		}
		// agent_key → agent_id
		if n.AgentKey != "" {
			id, err := mapper.ResolveAgentKey(n.AgentKey)
			if err == nil {
				nodeSpec.AgentID = id
			}
		}
		eg.Nodes = append(eg.Nodes, nodeSpec)
	}
	for _, e := range spec.Edges {
		eg.Edges = append(eg.Edges, biz.EmbeddedGraphEdgeSpec{
			ID:        e.ID,
			Source:    e.Source,
			Target:    e.Target,
			Label:     e.Label,
			Condition: e.Condition,
		})
	}
	return eg
}

// buildRuntimeSettings 从 AgentPackSpec 构建 AgentRuntimeSettings。
func (im *Importer) buildRuntimeSettings(agentID string, spec AgentPackSpec) biz.AgentRuntimeSettings {
	s := biz.AgentRuntimeSettings{AgentID: agentID}

	if spec.Runtime == nil {
		return s
	}

	// Memory
	if spec.Runtime.Memory != nil {
		m := spec.Runtime.Memory
		s.MemoryEnabled = m.Enabled
		s.L0RecentWindowTurns = m.L0RecentWindowTurns
		s.L0RecentWindowTokens = m.L0RecentWindowTokens
		s.L0SummaryThreshold = m.L0SummaryThreshold
		s.L0SummaryKeepTurns = m.L0SummaryKeepTurns
		s.L0InjectL1 = m.L0InjectL1
		s.L0InjectL3 = m.L0InjectL3
		s.L0InjectL4 = m.L0InjectL4
		s.L0SnapshotMode = m.L0SnapshotMode
		s.L1Enabled = m.L1Enabled
		s.L1BudgetTokens = m.L1BudgetTokens
		s.L2EpisodeEnabled = m.L2EpisodeEnabled
		s.L2EpisodeMinImportance = m.L2EpisodeMinImportance
		s.L2RecallEnabled = m.L2RecallEnabled
		s.L2RecallMax = m.L2RecallMax
		s.L2RetentionDays = m.L2RetentionDays
		s.L3Enabled = m.L3Enabled
		s.L3RecallTopK = m.L3RecallTopK
		s.L3RecallMinScore = m.L3RecallMinScore
		s.L4Enabled = m.L4Enabled
		s.L4GraphInjectNeighbors = m.L4GraphInjectNeighbors
		s.L4GraphMaxNeighbors = m.L4GraphMaxNeighbors
		s.L4IdentityInject = m.L4IdentityInject
	}

	// Tools
	if spec.Runtime.Tools != nil {
		s.ToolsRetryEnabled = spec.Runtime.Tools.RetryEnabled
		s.ToolsRetryMaxAttempts = spec.Runtime.Tools.RetryMaxAttempts
		s.ToolsRetryInitialIntervalMs = spec.Runtime.Tools.RetryInitialIntervalMs
		s.ToolsStreamingEnabled = spec.Runtime.Tools.StreamingEnabled
		s.ToolsCircuitBreakerEnabled = spec.Runtime.Tools.CircuitBreakerEnabled
		s.ToolsCommandSafetyEnabled = spec.Runtime.Tools.CommandSafetyEnabled
	}
	s.ToolsProfile = spec.ToolsProfile
	if spec.ToolsAllow != nil {
		s.ToolsAllowJSON = sliceToJSONList(spec.ToolsAllow)
	}
	if spec.ToolsDeny != nil {
		s.ToolsDenyJSON = sliceToJSONList(spec.ToolsDeny)
	}
	if spec.ToolsParallel != nil && *spec.ToolsParallel {
		s.ToolsParallelEnabled = true
	}

	// Evolution
	if spec.Runtime.Evolution != nil {
		s.EvolutionSelfEvolve = spec.Runtime.Evolution.SelfEvolve
		s.EvolutionSkillEvolve = spec.Runtime.Evolution.SkillEvolve
		s.EvolutionMetricsEnabled = spec.Runtime.Evolution.MetricsEnabled
		s.EvolutionSuggestionsEnabled = spec.Runtime.Evolution.SuggestionsEnabled
	}

	// Reasoning
	if spec.Runtime.Reasoning != nil {
		s.ReasoningMode = spec.Runtime.Reasoning.Mode
		s.ReasoningLevel = spec.Runtime.Reasoning.Level
	}

	// RalphLoop
	if spec.Runtime.RalphLoop != nil {
		s.RalphLoopMaxIterations = spec.Runtime.RalphLoop.MaxIterations
		s.RalphLoopCompletionPromise = spec.Runtime.RalphLoop.CompletionPromise
		s.RalphLoopVerifyCommand = spec.Runtime.RalphLoop.VerifyCommand
		s.RalphLoopVerifyTimeoutSeconds = spec.Runtime.RalphLoop.VerifyTimeoutSeconds
	}

	// Context
	if spec.Runtime.Context != nil {
		s.ContextCompactionEnabled = spec.Runtime.Context.CompactionEnabled
		s.SessionSummaryEnabled = spec.Runtime.Context.SessionSummaryEnabled
		s.IntentPassEnabled = spec.Runtime.Context.IntentPassEnabled
	}

	// Subagents
	if spec.SubagentsEnabled != nil {
		s.SubagentsEnabled = *spec.SubagentsEnabled
	}
	s.SubagentsMaxConcurrency = spec.SubagentsMaxConcurrency
	s.SubagentsMaxGenerationDepth = spec.SubagentsMaxGenerationDepth

	// Skills
	if spec.Skills != nil {
		s.SkillLoadMode = spec.Skills.LoadMode
		s.SkillRuntimeJSON = buildSkillRuntimeJSON(spec.Skills)
	}

	// CodeExecutor
	if spec.CodeExecutor != "" {
		s.CodeExecutorType = spec.CodeExecutor
	}

	return s
}

// upsertTaxonomyNode 创建或更新分类节点。
func (im *Importer) upsertTaxonomyNode(ctx context.Context, node biz.TaxonomyNode, strategy ConflictStrategy) (biz.TaxonomyNode, error) {
	existing, err := im.repo.GetTaxonomyNodeByKey(ctx, node.Key)
	if err == nil {
		// 已存在
		if strategy == ConflictSkip {
			return existing, nil
		}
		if strategy == ConflictOverwrite {
			node.ID = existing.ID
			return im.repo.UpdateTaxonomyNode(ctx, node)
		}
		// duplicate: 不应用于 taxonomy
		return existing, nil
	}

	// 创建
	node.Status = "active"
	node.Enabled = true
	return im.repo.CreateTaxonomyNode(ctx, node)
}

// --- 辅助函数 ---

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func sliceToJSONList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func buildSkillRuntimeJSON(spec *AgentSkillsSpec) string {
	type runtimePolicy struct {
		AllowedSlugs []string `json:"allowed_slugs"`
		DeniedSlugs  []string `json:"denied_slugs"`
	}
	p := runtimePolicy{
		AllowedSlugs: spec.Allowed,
		DeniedSlugs:  spec.Denied,
	}
	data, err := json.Marshal(p)
	if err != nil {
		return ""
	}
	return string(data)
}
