package pack

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ExporterRepo 导出引擎所需的只读仓库接口。
type ExporterRepo interface {
	// Agent
	GetAgent(ctx context.Context, id string) (biz.Agent, error)
	GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error)
	SearchAgents(ctx context.Context, q biz.AgentListQuery) (biz.AgentListResult, error)

	// Team
	GetTeam(ctx context.Context, id string) (biz.Team, error)
	ListTeams(ctx context.Context) ([]biz.Team, error)

	// Taxonomy
	GetTaxonomyNode(ctx context.Context, id string) (biz.TaxonomyNode, error)
	GetTaxonomyAncestors(ctx context.Context, positionID string) (biz.TaxonomyAncestors, error)
	ListTaxonomyNodesByParentID(ctx context.Context, parentID string) ([]biz.TaxonomyNode, error)
	ListTaxonomyNodesByLevel(ctx context.Context, level string) ([]biz.TaxonomyNode, error)

	// Graph
	GetGraph(ctx context.Context, id string) (*biz.GraphDefinition, error)
}

// Exporter 导出引擎。
type Exporter struct {
	repo ExporterRepo
}

// NewExporter 创建导出引擎。
func NewExporter(repo ExporterRepo) *Exporter {
	return &Exporter{repo: repo}
}

// ExportAgent 导出单个 Agent。
func (e *Exporter) ExportAgent(ctx context.Context, agentID string) (*Pack, error) {
	agent, err := e.repo.GetAgent(ctx, agentID)
	if err != nil {
		return nil, kerrors.BadRequest("PACK_AGENT_GET", fmt.Sprintf("pack export: 获取 Agent %s 失败: %s", agentID, err.Error()))
	}

	agentSpec, agentFiles, err := e.buildAgentSpec(ctx, agent)
	if err != nil {
		return nil, err
	}

	p := &Pack{
		Manifest: ManifestSpec{
			APIVersion: "v1",
			Kind:       "agent",
			Name:       agent.DisplayName,
			Version:    "1.0.0",
			CreatedAt:  time.Now().Format("2006-01-02"),
			Contents: &PackContents{
				Agents: []PackContentRef{{Key: agent.AgentKey}},
			},
		},
		Agents:     []AgentPackSpec{agentSpec},
		AgentFiles: agentFiles,
	}

	// 收集依赖
	e.collectDependencies(p)

	return p, nil
}

// ExportTeam 导出单个 Team（含成员 Agent）。
func (e *Exporter) ExportTeam(ctx context.Context, teamID string) (*Pack, error) {
	team, err := e.repo.GetTeam(ctx, teamID)
	if err != nil {
		return nil, kerrors.BadRequest("PACK_TEAM_GET", fmt.Sprintf("pack export: 获取 Team %s 失败: %s", teamID, err.Error()))
	}

	teamSpec, err := e.buildTeamSpec(ctx, team)
	if err != nil {
		return nil, err
	}

	// 收集成员 Agent
	agentSpecs, agentFiles, err := e.collectTeamAgentSpecs(ctx, team)
	if err != nil {
		return nil, err
	}

	p := &Pack{
		Manifest: ManifestSpec{
			APIVersion: "v1",
			Kind:       "team",
			Name:       team.DisplayName,
			Version:    "1.0.0",
			CreatedAt:  time.Now().Format("2006-01-02"),
			Contents: &PackContents{
				Agents: buildContentRefsFromAgentSpecs(agentSpecs),
				Teams:  []PackContentRef{{Key: team.TeamKey}},
			},
		},
		Agents:     agentSpecs,
		Teams:      []TeamPackSpec{teamSpec},
		AgentFiles: agentFiles,
	}

	e.collectDependencies(p)
	return p, nil
}

// ExportIndustry 导出整个行业场景。
func (e *Exporter) ExportIndustry(ctx context.Context, industryKey string) (*Pack, error) {
	// 1. 查找 industry 节点
	industryNode, err := e.repo.GetTaxonomyNode(ctx, industryKey)
	if err != nil {
		// 尝试通过 key 查找
		nodes, err2 := e.listIndustryNodes(ctx)
		if err2 != nil {
			return nil, kerrors.BadRequest("PACK_INDUSTRY_NOT_FOUND", fmt.Sprintf("pack export: 查找行业 %s 失败: %s", industryKey, err.Error()))
		}
		found := false
		for _, n := range nodes {
			if n.Key == industryKey {
				industryNode = n
				found = true
				break
			}
		}
		if !found {
			return nil, kerrors.BadRequest("PACK_INDUSTRY_NOT_FOUND", fmt.Sprintf("pack export: 行业 %s 不存在", industryKey))
		}
	}

	// 2. 构建 taxonomy 树
	taxSpec, err := e.buildTaxonomySpec(ctx, industryNode)
	if err != nil {
		return nil, err
	}

	// 3. 收集所有 position 下的 Agent
	agentSpecs, agentFiles, err := e.collectIndustryAgentSpecs(ctx, industryNode)
	if err != nil {
		return nil, err
	}

	// 4. 收集关联 Team
	teamSpecs, err := e.collectIndustryTeamSpecs(ctx, industryNode, agentSpecs)
	if err != nil {
		return nil, err
	}

	// 5. 收集关联 Graph
	graphSpecs, err := e.collectGraphSpecs(ctx, teamSpecs)
	if err != nil {
		return nil, err
	}

	p := &Pack{
		Manifest: ManifestSpec{
			APIVersion: "v1",
			Kind:       "industry",
			Name:       industryNode.Name,
			Version:    "1.0.0",
			CreatedAt:  time.Now().Format("2006-01-02"),
			Contents: &PackContents{
				Taxonomy: true,
				Agents:   buildContentRefsFromAgentSpecs(agentSpecs),
				Teams:    buildContentRefsFromTeamSpecs(teamSpecs),
				Graphs:   buildContentRefsFromGraphSpecs(graphSpecs),
			},
		},
		Taxonomy:   taxSpec,
		Agents:     agentSpecs,
		Teams:      teamSpecs,
		Graphs:     graphSpecs,
		AgentFiles: agentFiles,
	}

	e.collectDependencies(p)
	return p, nil
}

// buildAgentSpec 从 biz.Agent 构建 AgentPackSpec。
func (e *Exporter) buildAgentSpec(ctx context.Context, agent biz.Agent) (AgentPackSpec, map[string]map[string]string, error) {
	spec := AgentPackSpec{
		Key:                agent.AgentKey,
		DisplayName:        agent.DisplayName,
		Description:        agent.AgentDescription,
		Icon:               agent.Icon,
		Variant:            agent.AgentVariant,
		VariantDescription: agent.VariantDescription,
		Provider:           agent.Provider,
		Model:              agent.Model,
		SystemPromptMode:   agent.SystemPromptMode,
		ContextWindow:      agent.ContextWindow,
		Kind:               agent.AgentKind,
		OwnershipKind:      agent.Kind,
		Source:             agent.Source,
		TeamRole:           "", // 从 Team 成员定义中获取
	}

	// position_key 路径转换
	if agent.TaxonomyPositionID != "" {
		ancestors, err := e.repo.GetTaxonomyAncestors(ctx, agent.TaxonomyPositionID)
		if err == nil {
			spec.PositionKey = BuildTaxonomyKey(ancestors.Industry.Key, ancestors.Department.Key, ancestors.Position.Key)
		}
	}

	// A2A Proxy
	if agent.A2AProxy != nil {
		spec.A2AProxy = &A2AProxyPackSpec{
			RemoteURL:       agent.A2AProxy.RemoteURL,
			AgentCardURL:    agent.A2AProxy.AgentCardURL,
			EnableStreaming: agent.A2AProxy.EnableStreaming,
			AuthType:        agent.A2AProxy.AuthType,
			TimeoutSeconds:  agent.A2AProxy.TimeoutSeconds,
		}
	}

	// RuntimeSettings 可移植字段
	if agent.Settings != nil {
		spec.Runtime = buildRuntimePackSpec(agent.Settings)
		spec.ToolsProfile = agent.Settings.ToolsProfile
		spec.SubagentsEnabled = boolPtr(agent.Settings.SubagentsEnabled)
		spec.SubagentsMaxConcurrency = agent.Settings.SubagentsMaxConcurrency
		spec.SubagentsMaxGenerationDepth = agent.Settings.SubagentsMaxGenerationDepth
		spec.CodeExecutor = agent.Settings.CodeExecutorType

		// Tools
		spec.ToolsAllow = jsonListToSlice(agent.Settings.ToolsAllowJSON)
		spec.ToolsDeny = jsonListToSlice(agent.Settings.ToolsDenyJSON)
		if agent.Settings.ToolsParallelEnabled {
			spec.ToolsParallel = boolPtr(true)
		}

		// Skills
		skillRuntime := parseSkillRuntime(agent.Settings.SkillRuntimeJSON)
		if skillRuntime != nil {
			spec.Skills = skillRuntime
		}
	}

	// Files
	agentFiles := make(map[string]map[string]string)
	if len(agent.Files) > 0 {
		spec.Files = make([]AgentFileRef, 0, len(agent.Files))
		files := make(map[string]string)
		for _, f := range agent.Files {
			spec.Files = append(spec.Files, AgentFileRef{Name: f.Name})
			files[f.Name] = f.Body
		}
		agentFiles[agent.AgentKey] = files
	}

	return spec, agentFiles, nil
}

// buildTeamSpec 从 biz.Team 构建 TeamPackSpec。
func (e *Exporter) buildTeamSpec(ctx context.Context, team biz.Team) (TeamPackSpec, error) {
	spec := TeamPackSpec{
		Key:           team.TeamKey,
		DisplayName:   team.DisplayName,
		OwnershipKind: team.Kind,
		Source:        team.Source,
	}

	// 解析 definition_json
	ospec, err := biz.ParseOrchestrationSpec(team.DefinitionJSON)
	if err != nil {
		return spec, kerrors.BadRequest("PACK_TEAM_DEFINITION", fmt.Sprintf("pack export: 解析 Team %s 的 definition_json 失败: %s", team.TeamKey, err.Error()))
	}

	spec.Mode = ospec.Mode
	spec.Description = ospec.Description
	spec.MaxConcurrency = ospec.MaxConcurrency
	spec.TimeoutSeconds = ospec.TimeoutSeconds
	spec.RunTimeoutSec = ospec.RunTimeoutSec
	spec.TurnTimeoutSec = ospec.TurnTimeoutSec
	spec.FirstByteTimeoutSec = ospec.FirstByteTimeoutSec
	spec.LoopMaxIter = ospec.LoopMaxIterations
	spec.EnableCheckpoint = ospec.EnableCheckpoint
	spec.RuntimeEngine = ospec.RuntimeEngine
	spec.TeamGraphRuntime = ospec.TeamGraphRuntime

	// FailurePolicy
	if ospec.FailurePolicy != nil {
		spec.FailurePolicy = buildFailurePolicySpec(ospec.FailurePolicy)
	}

	// 成员：agent_id → agent_key 转换
	for _, m := range ospec.Members {
		memberSpec := TeamMemberPackSpec{
			Role:       m.Role,
			Name:       m.Name,
			TaskPrompt: m.TaskPrompt,
			Enabled:    boolPtr(m.Enabled),
			SortOrder:  m.SortOrder,
		}
		// 解析 agent_key
		agentKey, err := e.resolveAgentKey(ctx, m.AgentID)
		if err != nil {
			memberSpec.AgentKey = m.AgentID // 回退到 ID
		} else {
			memberSpec.AgentKey = agentKey
		}
		spec.Members = append(spec.Members, memberSpec)
	}

	// IntentAnchor / Synthesizer
	if ospec.IntentAnchorAgentID != "" {
		key, err := e.resolveAgentKey(ctx, ospec.IntentAnchorAgentID)
		if err == nil {
			spec.IntentAnchorKey = key
		}
	}
	if ospec.SynthesizerAgentID != "" {
		key, err := e.resolveAgentKey(ctx, ospec.SynthesizerAgentID)
		if err == nil {
			spec.SynthesizerKey = key
		}
	}

	// Graph
	if ospec.LinkedGraphID != "" {
		spec.Graph = &TeamGraphPackSpec{
			Linked:        true,
			LinkedGraphID: ospec.LinkedGraphID,
		}
	} else if ospec.Graph != nil {
		spec.Graph = e.buildEmbeddedGraphSpec(ctx, ospec.Graph)
	}

	// CriticLoop
	if ospec.CriticLoop != nil {
		spec.CriticLoop = &CriticLoopPackSpec{
			MaxIterations:  ospec.CriticLoop.MaxIterations,
			ScoreThreshold: ospec.CriticLoop.ScoreThreshold,
		}
	}

	return spec, nil
}

// buildEmbeddedGraphSpec 从 EmbeddedGraphSpec 构建 TeamGraphPackSpec。
func (e *Exporter) buildEmbeddedGraphSpec(ctx context.Context, eg *biz.EmbeddedGraphSpec) *TeamGraphPackSpec {
	spec := &TeamGraphPackSpec{Linked: false}
	spec.Layout = eg.Layout
	for _, n := range eg.Nodes {
		nodeSpec := TeamGraphNodeSpec{
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
		// agent_id → agent_key
		if n.AgentID != "" {
			key, err := e.resolveAgentKey(ctx, n.AgentID)
			if err == nil {
				nodeSpec.AgentKey = key
			}
		}
		spec.Nodes = append(spec.Nodes, nodeSpec)
	}
	for _, edge := range eg.Edges {
		spec.Edges = append(spec.Edges, TeamGraphEdgeSpec{
			ID:        edge.ID,
			Source:    edge.Source,
			Target:    edge.Target,
			Label:     edge.Label,
			Condition: edge.Condition,
		})
	}
	return spec
}

// buildTaxonomySpec 从 industry 节点构建 TaxonomyPackSpec。
func (e *Exporter) buildTaxonomySpec(ctx context.Context, industryNode biz.TaxonomyNode) (*TaxonomyPackSpec, error) {
	indSpec := IndustrySpec{
		Key:         industryNode.Key,
		Name:        industryNode.Name,
		Icon:        "", // icon 存在 config_json 中，简化处理
		Description: industryNode.Description,
		SortOrder:   industryNode.SortOrder,
	}

	// 获取 departments
	deptNodes, err := e.repo.ListTaxonomyNodesByParentID(ctx, industryNode.ID)
	if err != nil {
		return nil, err
	}
	for _, deptNode := range deptNodes {
		deptSpec := DepartmentSpec{
			Key:         deptNode.Key,
			Name:        deptNode.Name,
			Description: deptNode.Description,
			SortOrder:   deptNode.SortOrder,
		}
		// 获取 positions
		posNodes, err := e.repo.ListTaxonomyNodesByParentID(ctx, deptNode.ID)
		if err != nil {
			return nil, err
		}
		for _, posNode := range posNodes {
			deptSpec.Positions = append(deptSpec.Positions, PositionSpec{
				Key:         posNode.Key,
				Name:        posNode.Name,
				Description: posNode.Description,
				SortOrder:   posNode.SortOrder,
			})
		}
		indSpec.Departments = append(indSpec.Departments, deptSpec)
	}

	return &TaxonomyPackSpec{Industries: []IndustrySpec{indSpec}}, nil
}

// collectIndustryAgentSpecs 收集行业下所有 Agent。
func (e *Exporter) collectIndustryAgentSpecs(ctx context.Context, industryNode biz.TaxonomyNode) ([]AgentPackSpec, map[string]map[string]string, error) {
	var specs []AgentPackSpec
	files := make(map[string]map[string]string)
	seen := make(map[string]bool)

	// 遍历 department → position → 查找关联 Agent
	deptNodes, err := e.repo.ListTaxonomyNodesByParentID(ctx, industryNode.ID)
	if err != nil {
		return nil, nil, err
	}
	for _, deptNode := range deptNodes {
		posNodes, err := e.repo.ListTaxonomyNodesByParentID(ctx, deptNode.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, posNode := range posNodes {
			result, err := e.repo.SearchAgents(ctx, biz.AgentListQuery{CategoryID: posNode.ID, Limit: 1000})
			if err != nil {
				continue
			}
			for _, a := range result.Items {
				if seen[a.AgentKey] {
					continue
				}
				seen[a.AgentKey] = true
				// 需要水合 Files 和 Settings
				full, err := e.repo.GetAgent(ctx, a.ID)
				if err != nil {
					continue
				}
				spec, agentFiles, err := e.buildAgentSpec(ctx, full)
				if err != nil {
					continue
				}
				specs = append(specs, spec)
				for k, v := range agentFiles {
					files[k] = v
				}
			}
		}
	}

	return specs, files, nil
}

// collectIndustryTeamSpecs 收集行业关联的 Team。
func (e *Exporter) collectIndustryTeamSpecs(ctx context.Context, industryNode biz.TaxonomyNode, agentSpecs []AgentPackSpec) ([]TeamPackSpec, error) {
	// 构建已导出的 agent_key 集合
	agentKeys := make(map[string]bool)
	for _, a := range agentSpecs {
		agentKeys[a.Key] = true
	}

	allTeams, err := e.repo.ListTeams(ctx)
	if err != nil {
		return nil, err
	}

	var specs []TeamPackSpec
	for _, team := range allTeams {
		// 检查 Team 是否包含已导出的 Agent
		ospec, err := biz.ParseOrchestrationSpec(team.DefinitionJSON)
		if err != nil {
			continue
		}
		hasMatch := false
		for _, m := range ospec.Members {
			key, err := e.resolveAgentKey(ctx, m.AgentID)
			if err == nil && agentKeys[key] {
				hasMatch = true
				break
			}
		}
		if hasMatch {
			spec, err := e.buildTeamSpec(ctx, team)
			if err != nil {
				continue
			}
			specs = append(specs, spec)
		}
	}

	return specs, nil
}

// collectTeamAgentSpecs 收集 Team 的成员 Agent。
func (e *Exporter) collectTeamAgentSpecs(ctx context.Context, team biz.Team) ([]AgentPackSpec, map[string]map[string]string, error) {
	ospec, err := biz.ParseOrchestrationSpec(team.DefinitionJSON)
	if err != nil {
		return nil, nil, kerrors.BadRequest("PACK_TEAM_DEFINITION", fmt.Sprintf("pack export: 解析 Team definition_json 失败: %s", err.Error()))
	}

	var specs []AgentPackSpec
	files := make(map[string]map[string]string)
	seen := make(map[string]bool)

	for _, m := range ospec.Members {
		agent, err := e.repo.GetAgent(ctx, m.AgentID)
		if err != nil {
			continue
		}
		if seen[agent.AgentKey] {
			continue
		}
		seen[agent.AgentKey] = true

		spec, agentFiles, err := e.buildAgentSpec(ctx, agent)
		if err != nil {
			continue
		}
		spec.TeamRole = m.Role
		specs = append(specs, spec)
		for k, v := range agentFiles {
			files[k] = v
		}
	}

	return specs, files, nil
}

// collectGraphSpecs 收集 Team 关联的 Graph。
func (e *Exporter) collectGraphSpecs(ctx context.Context, teamSpecs []TeamPackSpec) ([]GraphPackSpec, error) {
	var specs []GraphPackSpec
	seen := make(map[string]bool)

	for _, ts := range teamSpecs {
		if ts.Graph == nil || !ts.Graph.Linked || ts.Graph.LinkedGraphID == "" {
			continue
		}
		graphID := ts.Graph.LinkedGraphID
		if seen[graphID] {
			continue
		}
		seen[graphID] = true

		def, err := e.repo.GetGraph(ctx, graphID)
		if err != nil {
			continue
		}
		spec := graphDefToPackSpec(def)
		specs = append(specs, spec)
	}

	return specs, nil
}

// collectDependencies 收集 Skill 和 FuncRef 依赖。
func (e *Exporter) collectDependencies(p *Pack) {
	var skills []string
	var funcRefs []string
	skillSet := make(map[string]bool)
	funcRefSet := make(map[string]bool)

	// 从 Agent 收集 Skill（同时收集 Allowed 和 Denied，保证 round-trip 完整）
	for _, a := range p.Agents {
		if a.Skills != nil {
			for _, s := range a.Skills.Allowed {
				if !skillSet[s] {
					skillSet[s] = true
					skills = append(skills, s)
				}
			}
			for _, s := range a.Skills.Denied {
				if !skillSet[s] {
					skillSet[s] = true
					skills = append(skills, s)
				}
			}
		}
	}

	// 从 Graph 收集 FuncRef
	for _, g := range p.Graphs {
		for _, n := range g.Nodes {
			if n.FuncRef != "" && !funcRefSet[n.FuncRef] {
				funcRefSet[n.FuncRef] = true
				funcRefs = append(funcRefs, n.FuncRef)
			}
		}
		for _, ce := range g.ConditionalEdges {
			if ce.CondFuncRef != "" && !funcRefSet[ce.CondFuncRef] {
				funcRefSet[ce.CondFuncRef] = true
				funcRefs = append(funcRefs, ce.CondFuncRef)
			}
		}
	}

	if len(skills) > 0 || len(funcRefs) > 0 {
		p.Manifest.Dependencies = &PackDependencies{
			Skills:   skills,
			FuncRefs: funcRefs,
		}
	}
}

// resolveAgentKey 将 agent_id 解析为 agent_key。
func (e *Exporter) resolveAgentKey(ctx context.Context, agentID string) (string, error) {
	agent, err := e.repo.GetAgent(ctx, agentID)
	if err != nil {
		return "", err
	}
	return agent.AgentKey, nil
}

// listIndustryNodes 列出所有 industry 级别的节点。
func (e *Exporter) listIndustryNodes(ctx context.Context) ([]biz.TaxonomyNode, error) {
	return e.repo.ListTaxonomyNodesByLevel(ctx, "industry")
}

// --- 辅助函数 ---

func buildRuntimePackSpec(s *biz.AgentRuntimeSettings) *AgentRuntimePackSpec {
	r := &AgentRuntimePackSpec{}

	// Memory
	r.Memory = &RuntimeMemorySpec{
		Enabled:                s.MemoryEnabled,
		L0RecentWindowTurns:    s.L0RecentWindowTurns,
		L0RecentWindowTokens:   s.L0RecentWindowTokens,
		L0SummaryThreshold:     s.L0SummaryThreshold,
		L0SummaryKeepTurns:     s.L0SummaryKeepTurns,
		L0InjectL1:             s.L0InjectL1,
		L0InjectL3:             s.L0InjectL3,
		L0InjectL4:             s.L0InjectL4,
		L0SnapshotMode:         s.L0SnapshotMode,
		L1Enabled:              s.L1Enabled,
		L1BudgetTokens:         s.L1BudgetTokens,
		L2EpisodeEnabled:       s.L2EpisodeEnabled,
		L2EpisodeMinImportance: s.L2EpisodeMinImportance,
		L2RecallEnabled:        s.L2RecallEnabled,
		L2RecallMax:            s.L2RecallMax,
		L2RetentionDays:        s.L2RetentionDays,
		L3Enabled:              s.L3Enabled,
		L3RecallTopK:           s.L3RecallTopK,
		L3RecallMinScore:       s.L3RecallMinScore,
		L4Enabled:              s.L4Enabled,
		L4GraphInjectNeighbors: s.L4GraphInjectNeighbors,
		L4GraphMaxNeighbors:    s.L4GraphMaxNeighbors,
		L4IdentityInject:       s.L4IdentityInject,
	}

	// Tools
	r.Tools = &RuntimeToolsSpec{
		RetryEnabled:           s.ToolsRetryEnabled,
		RetryMaxAttempts:       s.ToolsRetryMaxAttempts,
		RetryInitialIntervalMs: s.ToolsRetryInitialIntervalMs,
		StreamingEnabled:       s.ToolsStreamingEnabled,
		CircuitBreakerEnabled:  s.ToolsCircuitBreakerEnabled,
		CommandSafetyEnabled:   s.ToolsCommandSafetyEnabled,
	}

	// Evolution
	r.Evolution = &RuntimeEvolutionSpec{
		SelfEvolve:         s.EvolutionSelfEvolve,
		SkillEvolve:        s.EvolutionSkillEvolve,
		MetricsEnabled:     s.EvolutionMetricsEnabled,
		SuggestionsEnabled: s.EvolutionSuggestionsEnabled,
	}

	// Reasoning
	r.Reasoning = &RuntimeReasoningSpec{
		Mode:  s.ReasoningMode,
		Level: s.ReasoningLevel,
	}

	// RalphLoop
	r.RalphLoop = &RuntimeRalphLoopSpec{
		MaxIterations:        s.RalphLoopMaxIterations,
		CompletionPromise:    s.RalphLoopCompletionPromise,
		VerifyCommand:        s.RalphLoopVerifyCommand,
		VerifyTimeoutSeconds: s.RalphLoopVerifyTimeoutSeconds,
	}

	// Context
	r.Context = &RuntimeContextSpec{
		CompactionEnabled:     s.ContextCompactionEnabled,
		SessionSummaryEnabled: s.SessionSummaryEnabled,
		IntentPassEnabled:     s.IntentPassEnabled,
	}

	return r
}

func graphDefToPackSpec(def *biz.GraphDefinition) GraphPackSpec {
	spec := GraphPackSpec{
		ID:               def.ID,
		Name:             def.Name,
		Description:      def.Description,
		EntryPoint:       def.EntryPoint,
		FinishPoint:      def.FinishPoint,
		EnableCheckpoint: def.EnableCheckpoint,
		Version:          def.Version,
		SortOrder:        def.SortOrder,
		InterruptBefore:  def.InterruptBefore,
		InterruptAfter:   def.InterruptAfter,
	}
	if def.ExecutionEngine != "" {
		spec.ExecutionEngine = string(def.ExecutionEngine)
	}
	for _, sf := range def.StateFields {
		spec.StateFields = append(spec.StateFields, StateFieldPackSpec{
			Name:            sf.Name,
			Type:            sf.Type,
			Reducer:         string(sf.Reducer),
			DefaultValue:    sf.DefaultValue,
			Required:        sf.Required,
			DisableDeepCopy: sf.DisableDeepCopy,
		})
	}
	for _, n := range def.Nodes {
		spec.Nodes = append(spec.Nodes, nodeDefToPackSpec(n))
	}
	for _, e := range def.Edges {
		spec.Edges = append(spec.Edges, GraphEdgePackSpec{From: e.From, To: e.To, Kind: e.Kind})
	}
	for _, ce := range def.ConditionalEdges {
		spec.ConditionalEdges = append(spec.ConditionalEdges, GraphCondEdgePackSpec{
			From:        ce.From,
			CondFuncRef: ce.CondFuncRef,
			PathMap:     ce.PathMap,
		})
	}
	for _, sg := range def.Subgraphs {
		spec.Subgraphs = append(spec.Subgraphs, subgraphDefToPackSpec(sg))
	}
	return spec
}

func parseSkillRuntime(jsonStr string) *AgentSkillsSpec {
	if jsonStr == "" {
		return nil
	}
	var policy struct {
		AllowedSlugs []string `json:"allowed_slugs"`
		DeniedSlugs  []string `json:"denied_slugs"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &policy); err != nil {
		return nil
	}
	if len(policy.AllowedSlugs) == 0 && len(policy.DeniedSlugs) == 0 {
		return nil
	}
	return &AgentSkillsSpec{
		Allowed: policy.AllowedSlugs,
		Denied:  policy.DeniedSlugs,
	}
}

func jsonListToSlice(jsonStr string) []string {
	if jsonStr == "" || jsonStr == "[]" || jsonStr == "null" {
		return nil
	}
	var slice []string
	if err := json.Unmarshal([]byte(jsonStr), &slice); err != nil {
		return nil
	}
	return slice
}

func boolPtr(b bool) *bool { return &b }

func buildContentRefsFromAgentSpecs(specs []AgentPackSpec) []PackContentRef {
	refs := make([]PackContentRef, len(specs))
	for i, s := range specs {
		refs[i] = PackContentRef{Key: s.Key}
	}
	return refs
}

func buildContentRefsFromTeamSpecs(specs []TeamPackSpec) []PackContentRef {
	refs := make([]PackContentRef, len(specs))
	for i, s := range specs {
		refs[i] = PackContentRef{Key: s.Key}
	}
	return refs
}

func buildContentRefsFromGraphSpecs(specs []GraphPackSpec) []PackContentRef {
	refs := make([]PackContentRef, len(specs))
	for i, s := range specs {
		refs[i] = PackContentRef{Key: s.ID}
	}
	return refs
}

// buildFailurePolicySpec 从 biz.TeamFailurePolicy 构建 TeamFailurePolicySpec。
func buildFailurePolicySpec(fp *biz.TeamFailurePolicy) *TeamFailurePolicySpec {
	spec := &TeamFailurePolicySpec{
		Default:      fp.Default,
		ParallelFail: fp.ParallelFail,
		OnError:      fp.OnError,
	}
	// Retry
	spec.Retry = &TeamRetryPolicySpec{
		MaxAttempts:       fp.Retry.MaxAttempts,
		InitialIntervalMs: fp.Retry.InitialIntervalMs,
		BackoffFactor:     fp.Retry.BackoffFactor,
	}
	// NodeOverrides
	if len(fp.NodeOverrides) > 0 {
		spec.NodeOverrides = make(map[string]TeamNodeFailureOverrideSpec, len(fp.NodeOverrides))
		for k, v := range fp.NodeOverrides {
			spec.NodeOverrides[k] = TeamNodeFailureOverrideSpec{
				Action: v.Policy,
			}
		}
	}
	// CircuitBreaker
	if fp.CircuitBreaker != nil {
		spec.CircuitBreaker = &CircuitBreakerPolicySpec{
			FailureThreshold:  fp.CircuitBreaker.FailureThreshold,
			RecoveryTimeoutMs: fp.CircuitBreaker.ResetTimeoutSeconds * msPerSecond,
		}
	}
	return spec
}

// nodeDefToPackSpec 从 biz.NodeDef 构建 GraphNodePackSpec。
func nodeDefToPackSpec(n biz.NodeDef) GraphNodePackSpec {
	return GraphNodePackSpec{
		ID:                    n.ID,
		Type:                  n.Type,
		Description:           n.Description,
		FuncRef:               n.FuncRef,
		Instruction:           n.Instruction,
		ModelName:             n.ModelName,
		ToolNames:             n.ToolNames,
		AgentKey:              n.AgentName,
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
}

// subgraphDefToPackSpec 从 biz.SubgraphDef 构建 SubgraphPackSpec。
func subgraphDefToPackSpec(sg biz.SubgraphDef) SubgraphPackSpec {
	out := SubgraphPackSpec{
		ID:          sg.ID,
		EntryPoint:  sg.BuildConfig.EntryPoint,
		FinishPoint: sg.BuildConfig.FinishPoint,
	}
	for _, n := range sg.BuildConfig.Nodes {
		out.Nodes = append(out.Nodes, nodeDefToPackSpec(n))
	}
	for _, e := range sg.BuildConfig.Edges {
		out.Edges = append(out.Edges, GraphEdgePackSpec{From: e.From, To: e.To, Kind: e.Kind})
	}
	return out
}
