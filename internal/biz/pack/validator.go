package pack

import (
	"context"
	"fmt"
)

// ValidatorRepo 校验引擎所需的只读仓库接口。
type ValidatorRepo interface {
	AgentKeyExists(ctx context.Context, agentKey string) (bool, error)
	TeamKeyExists(ctx context.Context, teamKey string) (bool, error)
	TaxonomyKeyExists(ctx context.Context, key string) (bool, error)
	SkillExists(ctx context.Context, slug string) (bool, error)
	FuncRefExists(funcRef string) bool
}

// Validate 对 Pack 进行 dry-run 校验。
func Validate(ctx context.Context, p *Pack, repo ValidatorRepo) (*ValidationResult, error) {
	result := &ValidationResult{Valid: true}

	// 1. 格式校验
	validateManifest(p.Manifest, result)

	// 2. 实体格式校验
	validateAgentSpecs(p.Agents, result)
	validateTeamSpecs(p.Teams, result)
	validateGraphSpecs(p.Graphs, result)

	// 3. 依赖校验
	if repo != nil {
		validateDependencies(ctx, p, repo, result)
	}

	// 4. 冲突预检
	if repo != nil {
		validateConflicts(ctx, p, repo, result)
	}

	// 5. 引用完整性
	validateReferences(p, result)

	return result, nil
}

func validateManifest(m ManifestSpec, result *ValidationResult) {
	if m.APIVersion != "v1" {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("不支持的 api_version: %s（仅支持 v1）", m.APIVersion))
	}
	if m.Kind == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "manifest 缺少 kind 字段")
	}
	if m.Kind != "agent" && m.Kind != "team" && m.Kind != "industry" {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("无效的 kind: %s（支持 agent/team/industry）", m.Kind))
	}
	if m.Name == "" {
		result.Errors = append(result.Errors, "manifest 缺少 name 字段")
	}
}

func validateAgentSpecs(agents []AgentPackSpec, result *ValidationResult) {
	for _, a := range agents {
		if a.Key == "" {
			result.Valid = false
			result.Errors = append(result.Errors, "Agent 缺少 key 字段")
		}
		if a.DisplayName == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("Agent %s 缺少 display_name", a.Key))
		}
	}
}

func validateTeamSpecs(teams []TeamPackSpec, result *ValidationResult) {
	for _, t := range teams {
		if t.Key == "" {
			result.Valid = false
			result.Errors = append(result.Errors, "Team 缺少 key 字段")
		}
		if t.DisplayName == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("Team %s 缺少 display_name", t.Key))
		}
		if t.Mode == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("Team %s 缺少 mode 字段", t.Key))
		}
	}
}

func validateGraphSpecs(graphs []GraphPackSpec, result *ValidationResult) {
	for _, g := range graphs {
		if g.ID == "" {
			result.Valid = false
			result.Errors = append(result.Errors, "Graph 缺少 id 字段")
		}
		if g.Name == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("Graph %s 缺少 name", g.ID))
		}
		if g.EntryPoint == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("Graph %s 缺少 entry_point", g.ID))
		}
	}
}

func validateDependencies(ctx context.Context, p *Pack, repo ValidatorRepo, result *ValidationResult) {
	if p.Manifest.Dependencies == nil {
		return
	}

	// Skill 依赖
	for _, slug := range p.Manifest.Dependencies.Skills {
		exists, err := repo.SkillExists(ctx, slug)
		if err == nil && !exists {
			result.MissingSkills = append(result.MissingSkills, slug)
		}
	}

	// FuncRef 依赖
	for _, ref := range p.Manifest.Dependencies.FuncRefs {
		if !repo.FuncRefExists(ref) {
			result.MissingFuncRefs = append(result.MissingFuncRefs, ref)
			result.Valid = false // FuncRef 缺失为阻断项
		}
	}
}

func validateConflicts(ctx context.Context, p *Pack, repo ValidatorRepo, result *ValidationResult) {
	// Agent 冲突
	for _, a := range p.Agents {
		exists, err := repo.AgentKeyExists(ctx, a.Key)
		if err == nil && exists {
			result.Conflicts = append(result.Conflicts, ConflictItem{
				EntityType: "agent",
				Key:        a.Key,
			})
		}
	}

	// Team 冲突
	for _, t := range p.Teams {
		exists, err := repo.TeamKeyExists(ctx, t.Key)
		if err == nil && exists {
			result.Conflicts = append(result.Conflicts, ConflictItem{
				EntityType: "team",
				Key:        t.Key,
			})
		}
	}

	// Taxonomy 冲突
	if p.Taxonomy != nil {
		for _, ind := range p.Taxonomy.Industries {
			exists, err := repo.TaxonomyKeyExists(ctx, ind.Key)
			if err == nil && exists {
				result.Conflicts = append(result.Conflicts, ConflictItem{
					EntityType: "taxonomy",
					Key:        ind.Key,
				})
			}
		}
	}
}

func validateReferences(p *Pack, result *ValidationResult) {
	// 收集所有 agent_key
	agentKeys := make(map[string]bool)
	for _, a := range p.Agents {
		agentKeys[a.Key] = true
	}

	// 检查 Team 成员引用
	for _, t := range p.Teams {
		for _, m := range t.Members {
			if !agentKeys[m.AgentKey] {
				result.Errors = append(result.Errors, fmt.Sprintf("Team %s 成员 %s 引用的 agent_key 不在 Pack 中", t.Key, m.AgentKey))
			}
		}
		if t.IntentAnchorKey != "" && !agentKeys[t.IntentAnchorKey] {
			result.Errors = append(result.Errors, fmt.Sprintf("Team %s intent_anchor_key %s 引用的 agent_key 不在 Pack 中", t.Key, t.IntentAnchorKey))
		}
		if t.SynthesizerKey != "" && !agentKeys[t.SynthesizerKey] {
			result.Errors = append(result.Errors, fmt.Sprintf("Team %s synthesizer_key %s 引用的 agent_key 不在 Pack 中", t.Key, t.SynthesizerKey))
		}
		// 检查 Team 内嵌 Graph 节点的 agent_key 引用
		if t.Graph != nil {
			for _, n := range t.Graph.Nodes {
				if n.AgentKey != "" && !agentKeys[n.AgentKey] {
					result.Errors = append(result.Errors, fmt.Sprintf("Team %s Graph 节点 %s 引用的 agent_key %s 不在 Pack 中", t.Key, n.ID, n.AgentKey))
				}
			}
		}
	}

	// 检查 Graph 节点的 agent_key 引用
	for _, g := range p.Graphs {
		for _, n := range g.Nodes {
			if n.AgentKey != "" && !agentKeys[n.AgentKey] {
				result.Errors = append(result.Errors, fmt.Sprintf("Graph %s 节点 %s 引用的 agent_key %s 不在 Pack 中", g.ID, n.ID, n.AgentKey))
			}
		}
	}
}
