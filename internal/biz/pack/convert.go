package pack

import (
	"fmt"
	"strings"

	"aranea-agents/internal/scenario/loader"
)

// ConvertIndustrySpecToPack 将 loader.IndustrySpec 转换为 Pack 格式。
// 用于将现有 agents.yaml 格式的行业数据转换为 Pack 导入引擎可用的内存模型。
func ConvertIndustrySpecToPack(spec *loader.IndustrySpec) (*Pack, error) {
	if spec == nil {
		return nil, fmt.Errorf("pack: IndustrySpec 为 nil")
	}

	p := &Pack{
		Manifest: ManifestSpec{
			APIVersion:  "v1",
			Kind:        "industry",
			Name:        spec.IndustryKey,
			Description: fmt.Sprintf("%s 行业场景包", spec.IndustryKey),
			Version:     "1.0.0",
			Author:      "system",
		},
		AgentFiles: make(map[string]map[string]string),
	}

	// 转换 Agents
	for i := range spec.Agents {
		as := &spec.Agents[i]
		agentSpec := convertAgentSpec(spec, as)
		p.Agents = append(p.Agents, agentSpec)
	}

	// 转换 Teams
	for i := range spec.Teams {
		ts := &spec.Teams[i]
		teamSpec := convertTeamSpec(ts)
		p.Teams = append(p.Teams, teamSpec)
	}

	// 收集依赖
	collectPackDependencies(p)

	return p, nil
}

// convertAgentSpec 将 loader.AgentSpec 转换为 AgentPackSpec。
func convertAgentSpec(spec *loader.IndustrySpec, as *loader.AgentSpec) AgentPackSpec {
	provider, model := resolveModelFromDefaults(spec.Defaults, as.ModelTier)

	toolsDeny := as.ToolsDeny
	if len(toolsDeny) == 0 {
		toolsDeny = spec.Defaults.ToolsDeny
	}

	toolsProfile := as.ToolsProfile
	if toolsProfile == "" {
		toolsProfile = "general"
	}

	spm := as.SystemPromptMode
	if spm == "" {
		spm = spec.Defaults.SystemPromptMode
	}
	if spm == "" {
		spm = "file"
	}

	cw := as.ContextWindow
	if cw == 0 {
		cw = spec.Defaults.ContextWindow
	}
	if cw == 0 {
		cw = 64000
	}

	ce := as.CodeExecutor
	if ce == "" {
		ce = spec.Defaults.CodeExecutor
	}

	variant := as.Variant
	if variant == "" {
		variant = "general"
	}

	// 构建 position_key 路径格式：industry/dept/pos
	positionKey := buildPositionKeyPath(spec.IndustryKey, as.PositionKey)

	agentSpec := AgentPackSpec{
		Key:               as.Key,
		DisplayName:       as.DisplayName,
		Description:       as.Description,
		PositionKey:       positionKey,
		Variant:           variant,
		Provider:          provider,
		Model:             model,
		ModelTier:         as.ModelTier,
		SystemPromptMode:  spm,
		ContextWindow:     cw,
		ToolsProfile:      toolsProfile,
		ToolsDeny:         toolsDeny,
		ToolsAllow:        as.ToolsAllow,
		CodeExecutor:      ce,
		Kind:              "llm",
	}

	if as.SubagentsEnabled != nil {
		agentSpec.SubagentsEnabled = as.SubagentsEnabled
	}
	if as.ToolsParallel != nil {
		agentSpec.ToolsParallel = as.ToolsParallel
	}

	// Skills
	if len(as.Skills) > 0 {
		agentSpec.Skills = &AgentSkillsSpec{
			Allowed:  as.Skills,
			LoadMode: "manual",
		}
	}

	// Runtime settings
	agentSpec.Runtime = &AgentRuntimePackSpec{
		Tools: &RuntimeToolsSpec{
			RetryEnabled:     true,
			StreamingEnabled: true,
		},
		Evolution: &RuntimeEvolutionSpec{
			SelfEvolve:  false,
			SkillEvolve: false,
		},
		Context: &RuntimeContextSpec{
			CompactionEnabled:    true,
			SessionSummaryEnabled: true,
			IntentPassEnabled:    true,
		},
	}

	return agentSpec
}

// convertTeamSpec 将 loader.TeamSpec 转换为 TeamPackSpec。
func convertTeamSpec(ts *loader.TeamSpec) TeamPackSpec {
	teamSpec := TeamPackSpec{
		Key:               ts.Key,
		DisplayName:       ts.DisplayName,
		Description:       ts.Description,
		Mode:              ts.Mode,
		MaxConcurrency:    ts.MaxConcurrency,
		TimeoutSeconds:    ts.TimeoutSeconds,
		LoopMaxIter:       ts.LoopMaxIter,
		EnableCheckpoint:  ts.EnableCheckpoint,
		IntentAnchorKey:   ts.IntentAnchorKey,
		SynthesizerKey:    ts.SynthesizerKey,
	}

	// Members
	for _, m := range ts.Members {
		agentKey := m.AgentKey
		if agentKey == "" {
			agentKey = m.Key
		}
		enabled := true
		teamSpec.Members = append(teamSpec.Members, TeamMemberPackSpec{
			AgentKey:   agentKey,
			Role:       m.Role,
			Name:       m.Name,
			TaskPrompt: m.TaskPrompt,
			SortOrder:  m.SortOrder,
			Enabled:    &enabled,
		})
	}

	// CriticLoop
	if ts.CriticLoop != nil {
		teamSpec.CriticLoop = &CriticLoopPackSpec{
			MaxIterations:  ts.CriticLoop.MaxIterations,
			ScoreThreshold: ts.CriticLoop.ScoreThreshold,
		}
	}

	// Graph
	if ts.Graph != nil {
		teamSpec.Graph = convertGraphSpec(ts.Graph)
	}

	return teamSpec
}

// convertGraphSpec 将 loader.GraphSpec 转换为 TeamGraphPackSpec。
func convertGraphSpec(gs *loader.GraphSpec) *TeamGraphPackSpec {
	result := &TeamGraphPackSpec{
		Layout: gs.Layout,
	}

	for _, n := range gs.Nodes {
		result.Nodes = append(result.Nodes, TeamGraphNodeSpec{
			ID:       n.ID,
			Type:     n.Type,
			Label:    n.Label,
			AgentKey: n.AgentKey,
			Role:     n.Role,
		})
	}

	for _, e := range gs.Edges {
		result.Edges = append(result.Edges, TeamGraphEdgeSpec{
			ID:     e.ID,
			Source: e.Source,
			Target: e.Target,
		})
	}

	return result
}

// resolveModelFromDefaults 根据模型层级解析 provider 和 model。
func resolveModelFromDefaults(d loader.AgentDefaults, tier string) (string, string) {
	if tier == "strong" {
		return d.Provider, d.StrongModel
	}
	return d.Provider, d.FastModel
}

// buildPositionKeyPath 构建 taxonomy_key 路径格式。
// 由于 agents.yaml 中的 position_key 只是 position 级别的 key，
// 我们无法确定完整的 industry/dept/pos 路径，所以只存储 position key。
// 导入时 Taxonomy 已在 P1 阶段加载，Importer 会通过 mapper 查找。
func buildPositionKeyPath(industryKey, posKey string) string {
	// 只返回 position_key，导入引擎会通过 Taxonomy mapper 解析
	// 这里不构建完整路径，因为同一个 position key 可能在不同行业下
	return posKey
}

// collectPackDependencies 收集 Pack 中的 Skill 和 FuncRef 依赖。
func collectPackDependencies(p *Pack) {
	skillSet := make(map[string]bool)

	for _, a := range p.Agents {
		if a.Skills != nil {
			for _, s := range a.Skills.Allowed {
				skillSet[s] = true
			}
			for _, s := range a.Skills.Denied {
				skillSet[s] = true
			}
		}
	}

	if len(skillSet) > 0 {
		skills := make([]string, 0, len(skillSet))
		for s := range skillSet {
			skills = append(skills, s)
		}
		p.Manifest.Dependencies = &PackDependencies{
			Skills: skills,
		}
	}

	// 收集 contents
	p.Manifest.Contents = &PackContents{}
	if p.Taxonomy != nil {
		p.Manifest.Contents.Taxonomy = true
	}
	for _, a := range p.Agents {
		p.Manifest.Contents.Agents = append(p.Manifest.Contents.Agents, PackContentRef{Key: a.Key})
	}
	for _, t := range p.Teams {
		p.Manifest.Contents.Teams = append(p.Manifest.Contents.Teams, PackContentRef{Key: t.Key})
	}
	for _, g := range p.Graphs {
		p.Manifest.Contents.Graphs = append(p.Manifest.Contents.Graphs, PackContentRef{Key: g.ID})
	}
}

// ConvertAgentTemplatesToPack 将 loader.AgentTemplatesSpec 转换为 Pack 格式。
// Agent templates 是内置模板 Agent（fox/programmer/...），不含 position_key。
func ConvertAgentTemplatesToPack(spec *loader.AgentTemplatesSpec) *Pack {
	p := &Pack{
		Manifest: ManifestSpec{
			APIVersion:  "v1",
			Kind:        "industry",
			Name:        "内置模板",
			Description: "系统内置 Agent 模板",
			Version:     "1.0.0",
			Author:      "system",
		},
		AgentFiles: make(map[string]map[string]string),
	}

	for _, t := range spec.Templates {
		p.Agents = append(p.Agents, AgentPackSpec{
			Key:              t.Key,
			DisplayName:      t.DisplayName,
			Description:      t.Description,
			Icon:             t.Icon,
			Provider:         t.Provider,
			Model:            t.Model,
			SystemPromptMode: "file",
			ContextWindow:    64000,
			Kind:             "llm",
		})
	}

	p.Manifest.Contents = &PackContents{}
	for _, a := range p.Agents {
		p.Manifest.Contents.Agents = append(p.Manifest.Contents.Agents, PackContentRef{Key: a.Key})
	}

	return p
}

// ConvertTaxonomySpecToPack 将 loader.TaxonomySpec 转换为 Pack 中的 TaxonomyPackSpec。
func ConvertTaxonomySpecToPack(spec *loader.TaxonomySpec) *TaxonomyPackSpec {
	result := &TaxonomyPackSpec{}
	for _, ind := range spec.Industries {
		indSpec := IndustrySpec{
			Key:         ind.Key,
			Name:        ind.Name,
			Icon:        ind.Icon,
			Description: ind.Description,
			SortOrder:   ind.SortOrder,
		}
		for _, dept := range ind.Departments {
			deptSpec := DepartmentSpec{
				Key:         dept.Key,
				Name:        dept.Name,
				Description: dept.Description,
				SortOrder:   dept.SortOrder,
			}
			for _, pos := range dept.Positions {
				var variants []VariantSpec
				for _, v := range pos.Variants {
					variants = append(variants, VariantSpec{
						Key:  v.Key,
						Name: v.Name,
					})
				}
				deptSpec.Positions = append(deptSpec.Positions, PositionSpec{
					Key:              pos.Key,
					Name:             pos.Name,
					Description:      pos.Description,
					SortOrder:        pos.SortOrder,
					SeniorityLevel:   pos.SeniorityLevel,
					SkillsRequired:   pos.SkillsRequired,
					Responsibilities: pos.Responsibilities,
					Variants:         variants,
				})
			}
			indSpec.Departments = append(indSpec.Departments, deptSpec)
		}
		result.Industries = append(result.Industries, indSpec)
	}
	return result
}

// ConvertGraphTemplatesToPack 将硬编码的 Graph 模板转换为 GraphPackSpec 列表。
// GraphTemplate 定义在 internal/graph/trpc 包中。
type GraphTemplateSource struct {
	ID          string
	Name        string
	Description string
	Category    string
	EntryPoint  string
	FinishPoint string
	Nodes       []TemplateNodeSource
	Edges       []TemplateEdgeSource
	StateFields []StateFieldSource
}

type TemplateNodeSource struct {
	NodeID      string
	Type        string
	Label       string
	Description string
}

type TemplateEdgeSource struct {
	FromNode string
	ToNode   string
	Type     string
	Label    string
}

type StateFieldSource struct {
	Name    string
	Type    string
	Reducer string
}

// ConvertGraphTemplates converts graph template sources to GraphPackSpec list.
func ConvertGraphTemplates(templates []GraphTemplateSource) []GraphPackSpec {
	var result []GraphPackSpec
	for _, t := range templates {
		gs := GraphPackSpec{
			ID:          t.ID,
			Name:        t.Name,
			Description: t.Description,
			Category:    t.Category,
			EntryPoint:  t.EntryPoint,
			FinishPoint: t.FinishPoint,
		}

		for _, sf := range t.StateFields {
			gs.StateFields = append(gs.StateFields, StateFieldPackSpec{
				Name:    sf.Name,
				Type:    sf.Type,
				Reducer: sf.Reducer,
			})
		}

		for _, n := range t.Nodes {
			gs.Nodes = append(gs.Nodes, GraphNodePackSpec{
				ID:          n.NodeID,
				Type:        n.Type,
				Label:       n.Label,
				Description: n.Description,
			})
		}

		// 分离普通边和条件边
		var condEdgeMap = make(map[string]map[string]string) // from → label → to
		for _, e := range t.Edges {
			if e.Type == "conditional" {
				if condEdgeMap[e.FromNode] == nil {
					condEdgeMap[e.FromNode] = make(map[string]string)
				}
				label := e.Label
				if label == "" {
					label = e.ToNode
				}
				condEdgeMap[e.FromNode][label] = e.ToNode
			} else {
				gs.Edges = append(gs.Edges, GraphEdgePackSpec{
					From: e.FromNode,
					To:   e.ToNode,
				})
			}
		}

		for from, pathMap := range condEdgeMap {
			gs.ConditionalEdges = append(gs.ConditionalEdges, GraphCondEdgePackSpec{
				From:    from,
				PathMap: pathMap,
			})
		}

		result = append(result, gs)
	}
	return result
}

// MergePacks 将多个 Pack 合并为一个（用于将 taxonomy + agent templates + graph templates 合并）。
func MergePacks(packs ...*Pack) *Pack {
	result := &Pack{
		Manifest: ManifestSpec{
			APIVersion: "v1",
			Kind:       "industry",
			Version:    "1.0.0",
			Author:     "system",
		},
		AgentFiles: make(map[string]map[string]string),
	}

	for _, p := range packs {
		if p.Taxonomy != nil {
			result.Taxonomy = p.Taxonomy
		}
		result.Agents = append(result.Agents, p.Agents...)
		result.Teams = append(result.Teams, p.Teams...)
		result.Graphs = append(result.Graphs, p.Graphs...)
		for k, v := range p.AgentFiles {
			result.AgentFiles[k] = v
		}
	}

	// 使用第一个有名称的 Pack 的名称
	for _, p := range packs {
		if p.Manifest.Name != "" {
			result.Manifest.Name = p.Manifest.Name
			result.Manifest.Description = p.Manifest.Description
			break
		}
	}

	collectPackDependencies(result)
	return result
}

// Ensure pack.ConvertIndustrySpecToPack satisfies the unused import check.
var _ = strings.TrimSpace
