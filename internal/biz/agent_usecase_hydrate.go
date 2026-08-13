package biz

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/loggateway"
)

func (u *AgentUsecase) hydrate(ctx context.Context, agent Agent) (Agent, error) {
	return u.hydrateWithExtras(ctx, agent, false)
}

// hydrateWithExtras hydrates an agent with settings, files, and optionally extras.
// When skipExtras is true, the ListExtrasForAgents query is skipped (suitable for
// orchestration/build paths that only need settings + files, not list-display fields).
func (u *AgentUsecase) hydrateWithExtras(ctx context.Context, agent Agent, skipExtras bool) (Agent, error) {
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, agent.ID)
	if err != nil {
		if !stderrors.Is(err, shared.ErrNotFound) {
			return Agent{}, err
		}
		u.lg.Warn("agent runtime settings not found, migrating from legacy config_json", loggateway.StepID("agent.db_resolve"), loggateway.Str("agent_id", agent.ID))
		settings = u.migrateLegacySettings(ctx, agent)
	}
	files, err := u.files.ListAgentPromptFiles(ctx, agent.ID)
	if err != nil {
		return Agent{}, err
	}
	if len(files) == 0 {
		u.lg.Warn("agent prompt files not found, migrating from legacy config_json", loggateway.StepID("agent.skill_build"), loggateway.Str("agent_id", agent.ID))
		files = u.migrateLegacyFiles(ctx, agent)
	}
	agent.Settings = &settings
	agent.Files = files
	HydrateAgentKind(&agent)
	computed, err := configJSONFromSettings(withSettingDefaults(settings), files)
	if err != nil {
		return Agent{}, err
	}
	computed = EmbedAgentKindInConfigJSON(computed, agent.AgentKind, agent.A2AProxy, u.lg)
	agent.ConfigJSON = mergeEvaluationFromLegacy(computed, agent.ConfigJSON, u.lg)
	if !skipExtras {
		if extras, err := u.reader.ListExtrasForAgents(ctx, []string{agent.ID}); err != nil {
			u.lg.Warn("agent extras query failed, skipping enrichment",
				loggateway.StepID("agent.hydrate_extras"),
				loggateway.Str("agent_id", agent.ID),
				loggateway.Err(err))
		} else if ex, ok := extras[agent.ID]; ok {
			agent.LastRunStatus = ex.LastRunStatus
			agent.LastRunAt = ex.LastRunAt
			agent.PendingEvolutionCount = ex.PendingEvolutionCount
		}
	}
	return agent, nil
}

// BatchHydrateForBuild hydrates multiple agents in bulk for orchestration/build paths.
// Unlike calling Get() in a loop, this method:
//  1. Skips the ListExtrasForAgents query (not needed for runtime builds).
//  2. Batches the extras query when needed (future optimization point).
//
// This avoids the N+1 query pattern that would result from calling Get() per agent.
func (u *AgentUsecase) BatchHydrateForBuild(ctx context.Context, agents []Agent) ([]Agent, error) {
	out := make([]Agent, len(agents))
	for i, a := range agents {
		h, err := u.hydrateWithExtras(ctx, a, true)
		if err != nil {
			return nil, err
		}
		out[i] = h
	}
	return out, nil
}

func (u *AgentUsecase) migrateLegacySettings(ctx context.Context, agent Agent) AgentRuntimeSettings {
	settings := withSettingDefaults(settingsFromLegacyConfig(agent.ConfigJSON))
	settings.AgentID = agent.ID
	migrated, err := u.settings.UpsertAgentRuntimeSettings(ctx, settings)
	if err != nil {
		return settings
	}
	return migrated
}

func (u *AgentUsecase) migrateLegacyFiles(ctx context.Context, agent Agent) []AgentPromptFile {
	files := filesFromLegacyConfig(agent.ConfigJSON)
	if len(files) == 0 {
		files = defaultPromptFiles()
	}
	for i := range files {
		files[i].AgentID = agent.ID
	}
	migrated, err := u.files.ReplaceAgentPromptFiles(ctx, agent.ID, withFileDefaults(files))
	if err != nil {
		return withFileDefaults(files)
	}
	return migrated
}

func mergeEvaluationFromLegacy(computed, legacy string, lg loggateway.Logger) string {
	if strings.TrimSpace(legacy) == "" {
		return computed
	}
	var leg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(legacy), &leg); err != nil {
		lg.Warn("解析 legacy config_json 失败", loggateway.StepID("agent.merge_eval"), loggateway.Err(err))
		return computed
	}
	evalRaw, ok := leg["evaluation"]
	if !ok {
		return computed
	}
	var comp map[string]any
	if err := json.Unmarshal([]byte(computed), &comp); err != nil {
		lg.Warn("解析 computed config_json 失败", loggateway.StepID("agent.merge_eval"), loggateway.Err(err))
		return computed
	}
	var eval any
	if err := json.Unmarshal(evalRaw, &eval); err != nil {
		lg.Warn("解析 evaluation 字段失败", loggateway.StepID("agent.merge_eval"), loggateway.Err(err))
		return computed
	}
	comp["evaluation"] = eval
	out, err := json.Marshal(comp)
	if err != nil {
		return computed
	}
	return string(out)
}
