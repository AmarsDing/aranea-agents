package pack

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ImporterRepo 导入引擎所需的写入仓库接口。
type ImporterRepo interface {
	// Organization
	CreateOrganizationNode(ctx context.Context, node biz.OrganizationNode) (biz.OrganizationNode, error)
	UpdateOrganizationNode(ctx context.Context, node biz.OrganizationNode) (biz.OrganizationNode, error)
	GetOrganizationNodeByKey(ctx context.Context, key string) (biz.OrganizationNode, error)
	GetOrganizationNodeByKeyAnyState(ctx context.Context, key string) (biz.OrganizationNode, error)
	ListOrganizationNodesByParentID(ctx context.Context, parentID string) ([]biz.OrganizationNode, error)
	ListOrganizationNodesByLevel(ctx context.Context, level string) ([]biz.OrganizationNode, error)

	// Agent
	GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error)
	// CreateAgentAtomic / UpdateAgentAtomic 在同一个 ExecInTx 中完成
	// "agent + prompt files + runtime settings" 的三步写入。
	// Pack 场景必须使用这两个方法以保证 partial failure 安全。
	CreateAgentAtomic(ctx context.Context, a biz.Agent, files []biz.AgentPromptFile, settings biz.AgentRuntimeSettings) (biz.Agent, error)
	UpdateAgentAtomic(ctx context.Context, a biz.Agent, files []biz.AgentPromptFile, settings *biz.AgentRuntimeSettings) (biz.Agent, error)
	DeleteAgent(ctx context.Context, id string) error
	// 以下三个保留以供其他 Usecase 单步调用，Pack Importer 不应直接使用。
	CreateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error)
	UpdateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error)
	GetAgentRuntimeSettings(ctx context.Context, agentID string) (biz.AgentRuntimeSettings, error)
	UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error)
	ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error)

	// Team
	GetTeamByID(ctx context.Context, id string) (biz.Team, error)
	GetTeamByKey(ctx context.Context, teamKey string) (biz.Team, error)
	CreateTeam(ctx context.Context, t biz.Team) (biz.Team, error)
	UpdateTeam(ctx context.Context, t biz.Team) (biz.Team, error)

	// Graph
	GetGraphDefinitionByName(ctx context.Context, name string) (*biz.GraphDefinition, error)
	SaveGraphDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error)
	UpdateGraphDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error)

	// Transaction
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
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

	// Organization 阶段不支持 duplicate 策略（节点 key 必须唯一），提前告知用户
	if p.Organization != nil && strategy == ConflictDuplicate {
		result.Warnings = append(result.Warnings,
			"organization 不支持 duplicate 策略，重复 key 将被忽略并使用现有节点")
	}

	// Phase 1: Organization (wrapped in transaction for atomicity)
	if p.Organization != nil {
		var count int
		var warns []string
		orgErr := im.repo.ExecInTx(ctx, func(txCtx context.Context) error {
			var err error
			count, warns, err = im.importOrganization(txCtx, p.Organization, strategy, mapper, cfg.kindOverride)
			return err
		})
		if orgErr != nil {
			return result, kerrors.BadRequest("PACK_ORGANIZATION_IMPORT", fmt.Sprintf("pack import: Phase 1 (Organization) 失败: %s", orgErr.Error()))
		}
		result.OrgNodes = count
		result.Warnings = append(result.Warnings, warns...)
	}

	// Phase 2: Agents
	for _, agentSpec := range p.Agents {
		created, updated, skipped, agentWarns, err := im.importAgent(ctx, agentSpec, p.AgentFiles, strategy, mapper, cfg)
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
		result.Warnings = append(result.Warnings, agentWarns...)
	}

	// Phase 3: Graphs (wrapped in transaction for atomicity)
	graphErr := im.repo.ExecInTx(ctx, func(txCtx context.Context) error {
		for _, graphSpec := range p.Graphs {
			created, updated, skipped, graphWarns, err := im.importGraph(txCtx, graphSpec, strategy, mapper)
			if err != nil {
				result.Failures = append(result.Failures, ImportFailure{
					EntityType: "graph",
					Key:        graphSpec.ID,
					Reason:     err.Error(),
				})
				continue
			}
			result.GraphsCreated += created
			result.GraphsUpdated += updated
			result.GraphsSkipped += skipped
			result.Warnings = append(result.Warnings, graphWarns...)
		}
		return nil
	})
	if graphErr != nil {
		return result, kerrors.BadRequest("PACK_GRAPH_IMPORT", fmt.Sprintf("pack import: Phase 3 (Graphs) 失败: %s", graphErr.Error()))
	}

	// Phase 4: Teams (wrapped in transaction for atomicity)
	teamErr := im.repo.ExecInTx(ctx, func(txCtx context.Context) error {
		for _, teamSpec := range p.Teams {
			created, updated, skipped, teamWarns, err := im.importTeam(txCtx, teamSpec, strategy, mapper, cfg, p)
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
			result.Warnings = append(result.Warnings, teamWarns...)
		}
		return nil
	})
	if teamErr != nil {
		return result, kerrors.BadRequest("PACK_TEAM_IMPORT", fmt.Sprintf("pack import: Phase 4 (Teams) 失败: %s", teamErr.Error()))
	}

	return result, nil
}

// importOrganization 导入组织分类树。
// 优化：先批量查询所有已存在的 organization 节点，构建 key→node 映射，
// 避免逐节点 SELECT 查询（115 节点从 ~345 次 DB 操作降至 ~120 次）。
func (im *Importer) importOrganization(ctx context.Context, spec *OrganizationPackSpec, strategy ConflictStrategy, mapper *KeyMapper, kindOverride string) (int, []string, error) {
	count := 0
	var warns []string
	// IsSystem: organization nodes from system_builtin or ecosystem_preset packs are system nodes
	isSystem := kindOverride == "system_builtin" || kindOverride == "ecosystem_preset"

	// 预取所有已存在的 organization 节点，构建 key→node 缓存
	existingMap := make(map[string]biz.OrganizationNode)
	for _, level := range []string{"company", "department", "position"} {
		nodes, err := im.repo.ListOrganizationNodesByLevel(ctx, level)
		if err != nil {
			// 预取失败不阻塞，回退到逐节点查询
			break
		}
		for _, n := range nodes {
			existingMap[n.Key] = n
		}
	}

	upsertWithCache := func(node biz.OrganizationNode, s ConflictStrategy) (biz.OrganizationNode, error) {
		if existing, ok := existingMap[node.Key]; ok {
			// 缓存命中，跳过 SELECT
			if s == ConflictSkip {
				return existing, nil
			}
			if s == ConflictOverwrite {
				node.ID = existing.ID
				updated, err := im.repo.UpdateOrganizationNode(ctx, node)
				if err == nil {
					existingMap[node.Key] = updated
				}
				return updated, err
			}
			return existing, nil
		}
		// 缓存未命中，走完整的 upsert 逻辑（检查软删除等）
		result, err := im.upsertOrganizationNode(ctx, node, s)
		if err == nil {
			existingMap[node.Key] = result
		}
		return result, err
	}

	for _, comp := range spec.Companies {
		compNode, err := upsertWithCache(biz.OrganizationNode{
			Key:         comp.Key,
			Name:        comp.Name,
			Description: comp.Description,
			Level:       "company",
			SortOrder:   comp.SortOrder,
			IsSystem:    isSystem,
		}, strategy)
		if err != nil {
			return count, warns, kerrors.BadRequest("PACK_COMPANY_IMPORT", fmt.Sprintf("导入公司 %s 失败: %s", comp.Key, err.Error()))
		}
		mapper.RegisterOrg(comp.Key, compNode.ID)
		count++

		for _, dept := range comp.Departments {
			deptNode, err := upsertWithCache(biz.OrganizationNode{
				Key:         dept.Key,
				Name:        dept.Name,
				Description: dept.Description,
				ParentID:    compNode.ID,
				Level:       "department",
				SortOrder:   dept.SortOrder,
				IsSystem:    isSystem,
			}, strategy)
			if err != nil {
				warns = append(warns, fmt.Sprintf("导入部门 %s/%s 失败，跳过其子节点", comp.Key, dept.Key))
				continue
			}
			deptKey := BuildOrgKey(comp.Key, dept.Key, "")
			mapper.RegisterOrg(deptKey, deptNode.ID)
			count++

			for _, pos := range dept.Positions {
				posNode, err := upsertWithCache(biz.OrganizationNode{
					Key:         pos.Key,
					Name:        pos.Name,
					Description: pos.Description,
					ParentID:    deptNode.ID,
					Level:       "position",
					SortOrder:   pos.SortOrder,
					IsSystem:    isSystem,
				}, strategy)
				if err != nil {
					warns = append(warns, fmt.Sprintf("导入岗位 %s/%s/%s 失败", comp.Key, dept.Key, pos.Key))
					continue
				}
				posKey := BuildOrgKey(comp.Key, dept.Key, pos.Key)
				mapper.RegisterOrg(posKey, posNode.ID)
				count++
			}
		}
	}

	return count, warns, nil
}

// importAgent 导入单个 Agent。
func (im *Importer) importAgent(ctx context.Context, spec AgentPackSpec, agentFiles map[string]map[string]string, strategy ConflictStrategy, mapper *KeyMapper, cfg *importConfig) (created, updated, skipped int, warns []string, err error) {
	// 保存原始 key（duplicate 场景下用于映射）
	originalKey := spec.Key

	// 检查是否已存在
	existing, findErr := im.repo.GetAgentByAgentKey(ctx, spec.Key)

	if findErr == nil {
		// Agent 已存在
		switch strategy {
		case ConflictSkip:
			mapper.RegisterAgent(spec.Key, existing.ID)
			return 0, 0, 1, warns, nil
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
	// Kind = ownership classification (user | system_builtin | ecosystem_preset | ...)
	// Source = origin tracking (user | system | imported)
	// AgentKind = technical type (llm | a2a_proxy), set separately
	kind := cfg.kindOverride
	if kind == "" {
		kind = firstNonEmpty(spec.OwnershipKind, "user")
	} else if spec.OwnershipKind != "" && kind != spec.OwnershipKind {
		warns = append(warns, fmt.Sprintf("agent %q: kindOverride=%q overrides spec.ownershipKind=%q", spec.Key, kind, spec.OwnershipKind))
	}
	// Source: prefer spec.Source for round-trip fidelity; fallback to derived value
	source := spec.Source
	if source == "" {
		source = "imported"
		if kind == "system_builtin" {
			source = "system"
		}
	}
	agent := biz.Agent{
		AgentKey:           spec.Key,
		DisplayName:        spec.DisplayName,
		AgentDescription:   spec.Description,
		Icon:               spec.Icon,
		AgentVariant:       firstNonEmpty(spec.Variant, "general"),
		VariantDescription: spec.VariantDescription,
		Provider:           spec.Provider,
		Model:              spec.Model,
		SystemPromptMode:   spec.SystemPromptMode,
		ContextWindow:      spec.ContextWindow,
		Kind:               kind,
		AgentKind:          firstNonEmpty(spec.Kind, biz.AgentKindLLM),
		Status:             "active",
		Readonly:           false,
		Source:             source,
		PositionKey:        spec.Key, // 默认使用 agent_key 作为 position_key，避免唯一约束冲突
	}

	// overwrite 时保留原始 Status/Readonly/Kind/Source
	if findErr == nil && strategy == ConflictOverwrite {
		agent.Status = existing.Status
		agent.Readonly = existing.Readonly
		agent.Kind = existing.Kind
		agent.Source = existing.Source
	}

	// 解析 position_key → taxonomy_position_id（可选引用；缺失写警告）。
	if spec.PositionKey != "" {
		posID, err := mapper.ResolvePositionKey(spec.PositionKey)
		if err != nil {
			warns = append(warns, fmt.Sprintf("agent %q: position_key=%q 解析失败: %s",
				spec.Key, spec.PositionKey, err.Error()))
		} else {
			agent.PositionID = posID
		}
		// 同时设置 position_key（取路径最后一段）
		_, _, posKey, _ := ParseOrgKeyPath(spec.PositionKey)
		agent.PositionKey = posKey
	}

	// A2A Proxy
	if spec.A2AProxy != nil {
		agent.A2AProxy = &biz.A2AProxyConfig{
			RemoteURL:       spec.A2AProxy.RemoteURL,
			AgentCardURL:    spec.A2AProxy.AgentCardURL,
			EnableStreaming: spec.A2AProxy.EnableStreaming,
			AuthType:        spec.A2AProxy.AuthType,
			TimeoutSeconds:  spec.A2AProxy.TimeoutSeconds,
		}
	}

	// 准备 prompt files + runtime settings，让 atomic 写入一次性完成。
	var promptFiles []biz.AgentPromptFile
	if files, ok := agentFiles[spec.Key]; ok && len(files) > 0 {
		// 按文件名排序保证 SortOrder 确定性
		names := make([]string, 0, len(files))
		for name := range files {
			names = append(names, name)
		}
		sort.Strings(names)
		for i, name := range names {
			promptFiles = append(promptFiles, biz.AgentPromptFile{
				AgentID:   "", // atomic 写入后由内部填入 created.ID
				Name:      name,
				Body:      files[name],
				SortOrder: i,
			})
		}
	}
	var settings biz.AgentRuntimeSettings
	hasSettings := spec.Runtime != nil
	if hasSettings {
		settings = im.buildRuntimeSettings("", spec) // AgentID 由 atomic 填入
	}

	// 使用 atomic 写入：CreateAgent/UpdateAgent + prompt files + runtime settings
	// 三步在同一个 ExecInTx 中，任意一步失败整体回滚，修复了之前"半新半旧"问题。
	var agentID string
	if findErr == nil && strategy == ConflictOverwrite {
		agent.ID = existing.ID
		var settingsPtr *biz.AgentRuntimeSettings
		if hasSettings {
			settingsPtr = &settings
		}
		updatedAgent, err := im.repo.UpdateAgentAtomic(ctx, agent, promptFiles, settingsPtr)
		if err != nil {
			return 0, 0, 0, warns, kerrors.BadRequest("PACK_AGENT_UPDATE", fmt.Sprintf("更新 Agent %s 失败: %s", spec.Key, err.Error()))
		}
		agentID = updatedAgent.ID
		updated = 1
	} else {
		// 创建路径：promptFiles / settings 为 nil 时 atomic 内部跳过对应步骤
		createdAgent, err := im.repo.CreateAgentAtomic(ctx, agent, promptFiles, settings)
		if err != nil {
			return 0, 0, 0, warns, kerrors.BadRequest("PACK_AGENT_CREATE", fmt.Sprintf("创建 Agent %s 失败: %s", spec.Key, err.Error()))
		}
		agentID = createdAgent.ID
		created = 1
	}

	// 注册映射：仅在原子写入成功后注册，避免 mapper 指向被回滚的 ID。
	mapper.RegisterAgent(spec.Key, agentID)
	// duplicate 策略下，同时注册原始 key → 新 ID，确保后续 Team/Graph 引用能解析
	if strategy == ConflictDuplicate && spec.Key != originalKey {
		mapper.RegisterAgent(originalKey, agentID)
	}

	return created, updated, skipped, warns, nil
}

// importGraph 导入单个 Graph。
func (im *Importer) importGraph(ctx context.Context, spec GraphPackSpec, strategy ConflictStrategy, mapper *KeyMapper) (created, updated, skipped int, warns []string, err error) {
	// 检查是否已存在（按 name 查找）
	existing, findErr := im.repo.GetGraphDefinitionByName(ctx, spec.Name)

	if findErr == nil {
		// Graph 已存在
		switch strategy {
		case ConflictSkip:
			mapper.RegisterGraph(spec.ID, existing.ID)
			return 0, 0, 1, nil, nil
		case ConflictDuplicate:
			// Graph 不支持 duplicate（无独立 key 机制），按 overwrite 处理
			warns = append(warns, fmt.Sprintf("graph %q: ConflictDuplicate 被降级为 Overwrite（Graph 无独立 key 机制）", spec.Name))
		case ConflictOverwrite:
			// 继续更新
		}
	}

	def := im.buildGraphDefinition(spec)

	if findErr == nil && (strategy == ConflictOverwrite || strategy == ConflictDuplicate) {
		// 更新
		def.ID = existing.ID
		saved, updateErr := im.repo.UpdateGraphDefinition(ctx, def)
		if updateErr != nil {
			return 0, 0, 0, warns, kerrors.BadRequest("PACK_GRAPH_UPDATE", fmt.Sprintf("更新 Graph %s 失败: %s", spec.ID, updateErr.Error()))
		}
		mapper.RegisterGraph(spec.ID, saved.ID)
		return 0, 1, 0, warns, nil
	}

	// 创建
	saved, saveErr := im.repo.SaveGraphDefinition(ctx, def)
	if saveErr != nil {
		return 0, 0, 0, warns, kerrors.BadRequest("PACK_GRAPH_CREATE", fmt.Sprintf("创建 Graph %s 失败: %s", spec.ID, saveErr.Error()))
	}

	mapper.RegisterGraph(spec.ID, saved.ID)
	return 1, 0, 0, warns, nil
}

// buildGraphDefinition 从 GraphPackSpec 构建 biz.GraphDefinition。
func (im *Importer) buildGraphDefinition(spec GraphPackSpec) *biz.GraphDefinition {
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
			ID:              sg.ID,
			InterruptBefore: sg.InterruptBefore,
			InterruptAfter:  sg.InterruptAfter,
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

	return def
}

// OrchestrationSpec 当前协议版本。Manifest.APIVersion 推导规则：
//   - "v1" → 1（老协议，回退）
//   - 其他/未指定 → 2（当前默认）
//
// 未来 manifest 升级时在此函数集中调整映射。
const currentOrchestrationVersion = 2

// orchestrationVersionFromPack 根据 Manifest.APIVersion 推导 OrchestrationSpec.Version。
func orchestrationVersionFromPack(p *Pack) int {
	if p != nil && p.Manifest.APIVersion == "v1" {
		return 1
	}
	return currentOrchestrationVersion
}

// importTeam 导入单个 Team。
func (im *Importer) importTeam(ctx context.Context, spec TeamPackSpec, strategy ConflictStrategy, mapper *KeyMapper, cfg *importConfig, p *Pack) (created, updated, skipped int, warns []string, err error) {
	// 构建 OrchestrationSpec
	ospec := biz.OrchestrationSpec{
		Version:             orchestrationVersionFromPack(p),
		Mode:                spec.Mode,
		Description:         spec.Description,
		MaxConcurrency:      spec.MaxConcurrency,
		TimeoutSeconds:      spec.TimeoutSeconds,
		RunTimeoutSec:       spec.RunTimeoutSec,
		TurnTimeoutSec:      spec.TurnTimeoutSec,
		FirstByteTimeoutSec: spec.FirstByteTimeoutSec,
		LoopMaxIterations:   spec.LoopMaxIter,
		EnableCheckpoint:    spec.EnableCheckpoint,
		TeamGraphRuntime:    spec.TeamGraphRuntime,
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
			return 0, 0, 0, warns, kerrors.BadRequest("PACK_TEAM_MEMBER", fmt.Sprintf("Team %s 成员 %s 的 agent_key 未找到: %s", spec.Key, m.AgentKey, err.Error()))
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
	// IntentAnchor / Synthesizer 是运行时必填引用；缺失必须硬失败以保持
	// 与 validator.validateReferences 的语义一致（dry-run 与 real-import 对齐）。
	if spec.IntentAnchorKey != "" {
		id, err := mapper.ResolveAgentKey(spec.IntentAnchorKey)
		if err != nil {
			return 0, 0, 0, warns, kerrors.BadRequest("PACK_TEAM_INTENT_ANCHOR",
				fmt.Sprintf("Team %s intent_anchor_key=%q 未找到: %s",
					spec.Key, spec.IntentAnchorKey, err.Error()))
		}
		ospec.IntentAnchorAgentID = id
	}
	if spec.SynthesizerKey != "" {
		id, err := mapper.ResolveAgentKey(spec.SynthesizerKey)
		if err != nil {
			return 0, 0, 0, warns, kerrors.BadRequest("PACK_TEAM_SYNTHESIZER",
				fmt.Sprintf("Team %s synthesizer_key=%q 未找到: %s",
					spec.Key, spec.SynthesizerKey, err.Error()))
		}
		ospec.SynthesizerAgentID = id
	}

	// Graph
	if spec.Graph != nil {
		if spec.Graph.Linked && spec.Graph.LinkedGraphID != "" {
			newID, ok := mapper.GraphID(spec.Graph.LinkedGraphID)
			if ok {
				ospec.LinkedGraphID = newID
			} else {
				// LinkedGraph 是可选引用；缺失写警告便于用户诊断，但继续 import。
				warns = append(warns, fmt.Sprintf("Team %s linked_graph_id=%q 在本次 import 中未生成，已忽略",
					spec.Key, spec.Graph.LinkedGraphID))
			}
		} else if len(spec.Graph.Nodes) > 0 {
			eg, err := im.buildEmbeddedGraph(spec.Graph, mapper, spec.Key, p)
			if err != nil {
				return 0, 0, 0, warns, err
			}
			ospec.Graph = eg
		}
	}

	// 当 Pack YAML 只定义了 graph 没有 members 时，从 graph.nodes 预构建 members。
	// 这确保 definition_json 中 members 与 graph.nodes 一致，避免前端展示"无成员"。
	if len(ospec.Members) == 0 && ospec.Graph != nil && len(ospec.Graph.Nodes) > 0 {
		for _, n := range ospec.Graph.Nodes {
			if n.Type == "agent" && strings.TrimSpace(n.AgentID) != "" {
				enabled := true
				if n.Enabled != nil {
					enabled = *n.Enabled
				}
				ospec.Members = append(ospec.Members, biz.OrchestrationMember{
					AgentID:    n.AgentID,
					Role:       firstNonEmpty(strings.TrimSpace(n.Role), biz.RoleWorker),
					Name:       firstNonEmpty(strings.TrimSpace(n.Label), "Agent"),
					TaskPrompt: strings.TrimSpace(n.TaskPrompt),
					Enabled:    enabled,
					SortOrder:  len(ospec.Members) + 1,
				})
			}
		}
	}

	// FailurePolicy
	if spec.FailurePolicy != nil {
		fp := &biz.TeamFailurePolicy{
			Default:      spec.FailurePolicy.Default,
			ParallelFail: spec.FailurePolicy.ParallelFail,
			OnError:      spec.FailurePolicy.OnError,
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
				FailureThreshold: spec.FailurePolicy.CircuitBreaker.FailureThreshold,
				// 单位换算：spec 字段是毫秒（兼容旧 yaml），biz 字段是秒。
				// 常量化避免散落 magic 1000。
				ResetTimeoutSeconds: msToSec(spec.FailurePolicy.CircuitBreaker.RecoveryTimeoutMs),
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
		return 0, 0, 0, warns, kerrors.BadRequest("PACK_TEAM_DEFINITION", fmt.Sprintf("序列化 Team %s definition_json 失败: %s", spec.Key, err.Error()))
	}

	// Kind = ownership classification (user | system_builtin | ecosystem_preset | ...)
	// Source = origin tracking (user | system | imported), aligned with Agent import logic
	teamKind := cfg.kindOverride
	if teamKind == "" {
		teamKind = firstNonEmpty(spec.OwnershipKind, "user")
	} else if spec.OwnershipKind != "" && teamKind != spec.OwnershipKind {
		warns = append(warns, fmt.Sprintf("team %q: kindOverride=%q overrides spec.ownershipKind=%q", spec.Key, teamKind, spec.OwnershipKind))
	}
	// Source: prefer spec.Source for round-trip fidelity; fallback to derived value
	teamSource := spec.Source
	if teamSource == "" {
		teamSource = "imported"
		if teamKind == "system_builtin" {
			teamSource = "system"
		}
	}
	team := biz.Team{
		TeamKey:        spec.Key,
		DisplayName:    spec.DisplayName,
		DefinitionJSON: defJSON,
		Status:         biz.TeamStatusPending,
		Kind:           teamKind,
		Source:         teamSource,
		Readonly:       false,
	}

	// 检查是否已存在（通过 key 查找）
	existing, findErr := im.repo.GetTeamByKey(ctx, spec.Key)

	if findErr == nil {
		// Team 已存在
		switch strategy {
		case ConflictSkip:
			// 与 Agent 行为对齐：注册原 key → 现存 ID，便于下游引用解析
			mapper.RegisterTeam(spec.Key, existing.ID)
			return 0, 0, 1, warns, nil
		case ConflictOverwrite:
			team.ID = existing.ID
			// 保留原始 Status/Readonly/Kind/Source
			team.Status = existing.Status
			team.Readonly = existing.Readonly
			team.Kind = existing.Kind
			team.Source = existing.Source
			if _, err := im.repo.UpdateTeam(ctx, team); err != nil {
				return 0, 0, 0, warns, kerrors.BadRequest("PACK_TEAM_UPDATE", fmt.Sprintf("更新 Team %s 失败: %s", spec.Key, err.Error()))
			}
			return 0, 1, 0, warns, nil
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
		return 0, 0, 0, warns, kerrors.BadRequest("PACK_TEAM_CREATE", fmt.Sprintf("创建 Team %s 失败: %s", spec.Key, err.Error()))
	}

	return 1, 0, 0, warns, nil
}

// buildEmbeddedGraph 从 TeamGraphPackSpec 构建 EmbeddedGraphSpec。
// 节点的 agent_key 引用若解析不到，返回硬错误（与 validator.validateReferences 对齐）。
func (im *Importer) buildEmbeddedGraph(spec *TeamGraphPackSpec, mapper *KeyMapper, teamKey string, p *Pack) (*biz.EmbeddedGraphSpec, error) {
	eg := &biz.EmbeddedGraphSpec{Version: orchestrationVersionFromPack(p), Layout: spec.Layout}
	for _, n := range spec.Nodes {
		nodeSpec := biz.EmbeddedGraphNodeSpec{
			ID:               n.ID,
			Type:             n.Type,
			Label:            n.Label,
			Role:             n.Role,
			TaskPrompt:       n.TaskPrompt,
			Enabled:          n.Enabled,
			InterruptBefore:  n.InterruptBefore,
			InterruptAfter:   n.InterruptAfter,
			Destinations:     n.Destinations,
			RetryMaxAttempts: n.RetryMaxAttempts,
			FallbackAgent:    n.FallbackAgent,
		}
		// agent_key → agent_id：必填引用，缺失必须硬失败
		if n.AgentKey != "" {
			id, err := mapper.ResolveAgentKey(n.AgentKey)
			if err != nil {
				return nil, kerrors.BadRequest("PACK_TEAM_GRAPH_NODE",
					fmt.Sprintf("Team Graph 节点 %s agent_key=%q 未找到: %s",
						n.ID, n.AgentKey, err.Error()))
			}
			nodeSpec.AgentID = id
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
	return eg, nil
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
		s.L0SnapshotEnabled = m.L0SnapshotEnabled
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

// upsertOrganizationNode 创建或更新组织节点。
func (im *Importer) upsertOrganizationNode(ctx context.Context, node biz.OrganizationNode, strategy ConflictStrategy) (biz.OrganizationNode, error) {
	existing, err := im.repo.GetOrganizationNodeByKey(ctx, node.Key)
	if err == nil {
		// 已存在
		if strategy == ConflictSkip {
			return existing, nil
		}
		if strategy == ConflictOverwrite {
			node.ID = existing.ID
			return im.repo.UpdateOrganizationNode(ctx, node)
		}
		// duplicate: 不应用于 organization
		return existing, nil
	}

	// 活跃记录不存在，检查是否存在软删除的同 key 记录
	softDeleted, err2 := im.repo.GetOrganizationNodeByKeyAnyState(ctx, node.Key)
	if err2 == nil && softDeleted.DeletedAt != "" {
		// 软删除记录存在，恢复并更新
		node.ID = softDeleted.ID
		node.DeletedAt = ""
		return im.repo.UpdateOrganizationNode(ctx, node)
	}

	// 创建
	node.Status = "active"
	node.Enabled = true
	return im.repo.CreateOrganizationNode(ctx, node)
}

// --- 辅助函数 ---

// 毫秒→秒的单位换算常量。spec 字段名是 RecoveryTimeoutMs，biz 字段名是
// ResetTimeoutSeconds，避免散落的 magic 1000。
const msPerSecond = 1000

func msToSec(ms int) int { return ms / msPerSecond }

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
