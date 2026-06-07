package loader

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz"
)

type Deps struct {
	AgentUC     *biz.AgentUsecase
	TeamUC      *biz.TeamUsecase
	Taxonomy    *biz.TaxonomyUsecase
	ScenarioDir string
}

func SeedFromYAML(ctx context.Context, d Deps, industryKey string, dryRun bool) (int, int, error) {
	spec, err := LoadIndustrySpec(d.ScenarioDir, industryKey)
	if err != nil {
		return 0, 0, err
	}
	agentCount, err := seedAgents(ctx, d, spec, dryRun)
	if err != nil {
		return agentCount, 0, err
	}
	teamCount, err := seedTeams(ctx, d, spec, dryRun)
	if err != nil {
		return agentCount, teamCount, err
	}
	return agentCount, teamCount, nil
}

func SeedAgentsFromYAML(ctx context.Context, d Deps, industryKey string) (int, error) {
	spec, err := LoadIndustrySpec(d.ScenarioDir, industryKey)
	if err != nil {
		return 0, err
	}
	return seedAgents(ctx, d, spec, false)
}

func SeedTeamsFromYAML(ctx context.Context, d Deps, spec *IndustrySpec) (int, error) {
	return seedTeams(ctx, d, spec, false)
}

func LoadIndustrySpec(scenarioDir, industryKey string) (*IndustrySpec, error) {
	path := filepath.Join(scenarioDir, industryKey, "agents.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var spec IndustrySpec
	if err := yamlUnmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	spec.IndustryKey = industryKey
	fillDefaults(&spec)
	return &spec, nil
}

func seedAgents(ctx context.Context, d Deps, spec *IndustrySpec, dryRun bool) (int, error) {
	count := 0
	for i := range spec.Agents {
		as := &spec.Agents[i]
		agent, err := BuildBizAgentFromSpec(ctx, d, spec, as)
		if err != nil {
			return count, fmt.Errorf("agent %s: %w", as.Key, err)
		}
		if dryRun {
			count++
			continue
		}
		existing, findErr := d.AgentUC.GetByAgentKey(ctx, agent.AgentKey)
		if findErr == nil && existing.ID != "" {
			if _, updateErr := d.AgentUC.Update(ctx, existing.ID, agent); updateErr != nil {
				return count, fmt.Errorf("update agent %s: %w", agent.AgentKey, updateErr)
			}
		} else {
			if _, createErr := d.AgentUC.Create(ctx, agent); createErr != nil {
				return count, fmt.Errorf("create agent %s: %w", agent.AgentKey, createErr)
			}
		}
		count++
	}
	return count, nil
}

func seedTeams(ctx context.Context, d Deps, spec *IndustrySpec, dryRun bool) (int, error) {
	if len(spec.Teams) == 0 {
		return 0, nil
	}
	agentKeyToID, err := resolveAgentKeys(ctx, d, spec)
	if err != nil {
		return 0, err
	}
	existingTeams, err := d.TeamUC.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list teams: %w", err)
	}
	teamKeyMap := make(map[string]biz.Team, len(existingTeams))
	for _, t := range existingTeams {
		teamKeyMap[t.TeamKey] = t
	}

	count := 0
	for i := range spec.Teams {
		ts := &spec.Teams[i]
		team, err := BuildBizTeamFromSpec(spec, ts, agentKeyToID)
		if err != nil {
			return count, fmt.Errorf("team %s: %w", ts.Key, err)
		}
		if dryRun {
			count++
			continue
		}
		if existing, ok := teamKeyMap[team.TeamKey]; ok && existing.ID != "" {
			if _, updateErr := d.TeamUC.Update(ctx, existing.ID, team); updateErr != nil {
				return count, fmt.Errorf("update team %s: %w", team.TeamKey, updateErr)
			}
		} else {
			if _, createErr := d.TeamUC.Create(ctx, team); createErr != nil {
				return count, fmt.Errorf("create team %s: %w", team.TeamKey, createErr)
			}
		}
		count++
	}
	return count, nil
}

func BuildBizAgentFromSpec(ctx context.Context, d Deps, spec *IndustrySpec, as *AgentSpec) (biz.Agent, error) {
	posName := as.DisplayName
	posDesc := as.Description
	if as.PositionKey != "" && d.Taxonomy != nil {
		posNode, err := d.Taxonomy.GetByKey(ctx, as.PositionKey)
		if err == nil {
			anc, ancErr := d.Taxonomy.GetAncestors(ctx, posNode.ID)
			if ancErr == nil {
				if posName == "" {
					posName = anc.Position.Name
				}
				if posDesc == "" {
					parts := []string{}
					if anc.Industry.Key != "" {
						parts = append(parts, anc.Industry.Name)
					}
					if anc.Department.Key != "" {
						parts = append(parts, anc.Department.Name)
					}
					parts = append(parts, anc.Position.Name)
					posDesc = strings.Join(parts, " · ") + " 方向专家"
				}
			}
		}
	}
	if posName == "" {
		posName = as.Key
	}
	if posDesc == "" {
		posDesc = as.Key
	}

	provider, model := resolveModel(spec.Defaults, as.ModelTier)
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
	if ce == "" {
		ce = "local"
	}
	subagents := false
	if as.SubagentsEnabled != nil {
		subagents = *as.SubagentsEnabled
	}
	parallel := false
	if as.ToolsParallel != nil {
		parallel = *as.ToolsParallel
	}

	settings := biz.DefaultAgentRuntimeSettings()
	settings.ToolsEnabled = len(as.ToolsAllow) > 0 || subagents
	settings.ToolsProfile = toolsProfile
	settings.ToolsAllowJSON = jsonStringList(as.ToolsAllow)
	settings.ToolsDenyJSON = jsonStringList(toolsDeny)
	settings.ToolsParallelEnabled = parallel
	settings.SubagentsEnabled = subagents
	settings.SkillLoadMode = "manual"
	settings.SkillRuntimeJSON = skillRuntimeJSON(as.Skills...)
	settings.IntentPassEnabled = true
	settings.CodeExecutorType = ce
	settings.ContextCompactionEnabled = true
	settings.SessionSummaryEnabled = true

	variant := as.Variant
	if variant == "" {
		variant = "general"
	}
	variantDesc := ""
	if variant != "" && variant != "general" {
		variantDesc = fmt.Sprintf("本岗位的 %s 方向专家", variant)
	}

	roles := []string{as.RoleKey, as.TeamRole, spec.IndustryKey}
	if as.RoleKey == "" && as.TeamRole == "" {
		roles = []string{spec.IndustryKey}
	}

	return biz.Agent{
		AgentKey:           as.Key,
		DisplayName:        posName,
		Provider:           provider,
		Model:              model,
		AgentDescription:   posDesc,
		PositionKey:        as.PositionKey,
		AgentVariant:       variant,
		VariantDescription: variantDesc,
		Status:             "active",
		SystemPromptMode:   spm,
		ContextWindow:      cw,
		Roles:              roles,
		Settings:           &settings,
		Readonly:           true,
	}, nil
}

func BuildBizTeamFromSpec(spec *IndustrySpec, ts *TeamSpec, keyToID map[string]string) (biz.Team, error) {
	members := make([]biz.OrchestrationMember, 0, len(ts.Members))
	for _, m := range ts.Members {
		agentKey := m.AgentKey
		if agentKey == "" {
			agentKey = m.Key
		}
		agentID, ok := keyToID[agentKey]
		if !ok {
			return biz.Team{}, fmt.Errorf("agent key %q not found in DB", agentKey)
		}
		members = append(members, biz.OrchestrationMember{
			AgentID:    agentID,
			Role:       m.Role,
			Name:       m.Name,
			TaskPrompt: m.TaskPrompt,
			Enabled:    true,
			SortOrder:  m.SortOrder,
		})
	}

	intentAnchorID := keyToID[ts.IntentAnchorKey]
	synthesizerID := keyToID[ts.SynthesizerKey]

	ospec := biz.OrchestrationSpec{
		Version:             biz.OrchestrationSpecVersion,
		Mode:                ts.Mode,
		RuntimeEngine:       "graph",
		TeamGraphRuntime:    true,
		Description:         ts.Description,
		MaxConcurrency:      ts.MaxConcurrency,
		TimeoutSeconds:      ts.TimeoutSeconds,
		LoopMaxIterations:   ts.LoopMaxIter,
		EnableCheckpoint:    ts.EnableCheckpoint,
		IntentAnchorAgentID: intentAnchorID,
		SynthesizerAgentID:  synthesizerID,
		Members:             members,
	}

	if ts.CriticLoop != nil {
		ospec.CriticLoop = &biz.CriticLoopSpec{
			MaxIterations:  ts.CriticLoop.MaxIterations,
			ScoreThreshold: ts.CriticLoop.ScoreThreshold,
		}
	}

	if ts.Graph != nil {
		ospec.Graph = convertGraphSpec(ts.Graph, keyToID)
	}

	defJSON, err := biz.OrchestrationSpecToDefinitionJSON(ospec)
	if err != nil {
		return biz.Team{}, fmt.Errorf("serialize definition: %w", err)
	}

	return biz.Team{
		TeamKey:            ts.Key,
		DisplayName:        ts.DisplayName,
		Status:             "active",
		IsDefault:          false,
		DefinitionJSON:     defJSON,
		CategoryIndustryID: spec.IndustryKey,
	}, nil
}

func convertGraphSpec(gs *GraphSpec, keyToID map[string]string) *biz.EmbeddedGraphSpec {
	nodes := make([]biz.EmbeddedGraphNodeSpec, 0, len(gs.Nodes))
	for _, n := range gs.Nodes {
		bn := biz.EmbeddedGraphNodeSpec{
			ID: n.ID, Type: n.Type, Label: n.Label,
		}
		if n.AgentKey != "" {
			bn.AgentID = keyToID[n.AgentKey]
			bn.Role = n.Role
		}
		nodes = append(nodes, bn)
	}
	edges := make([]biz.EmbeddedGraphEdgeSpec, 0, len(gs.Edges))
	for _, e := range gs.Edges {
		edges = append(edges, biz.EmbeddedGraphEdgeSpec{
			ID: e.ID, Source: e.Source, Target: e.Target,
		})
	}
	return &biz.EmbeddedGraphSpec{
		Version: 1, Layout: gs.Layout, Nodes: nodes, Edges: edges,
	}
}

func resolveAgentKeys(ctx context.Context, d Deps, spec *IndustrySpec) (map[string]string, error) {
	keyToID := make(map[string]string)
	for _, as := range spec.Agents {
		agent, err := d.AgentUC.GetByAgentKey(ctx, as.Key)
		if err != nil {
			return nil, fmt.Errorf("resolve agent %s: %w", as.Key, err)
		}
		keyToID[as.Key] = agent.ID
	}
	return keyToID, nil
}

func resolveModel(d AgentDefaults, tier string) (string, string) {
	if tier == "strong" {
		return d.Provider, d.StrongModel
	}
	return d.Provider, d.FastModel
}

func jsonStringList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(items)
	return string(b)
}

func skillRuntimeJSON(allow ...string) string {
	if len(allow) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(map[string]any{"allowed_slugs": allow})
	return string(b)
}
