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
	ApplyIntentSkipPolicy(agent.Settings, agent)
	computed, err := configJSONFromSettings(withSettingDefaults(settings), files)
	if err != nil {
		return Agent{}, err
	}
	computed = EmbedAgentKindInConfigJSON(computed, agent.AgentKind, agent.A2AProxy, u.lg)
	agent.ConfigJSON = mergeConfigJSONKeys(computed, agent.ConfigJSON, u.lg, "evaluation", "knowledge")
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
	return mergeConfigJSONKeys(computed, legacy, lg, "evaluation")
}

// extractConfigJSONKeys keeps only the named top-level objects from raw JSON.
// Used so agents.config_json can persist evaluation/knowledge overlays after
// DEV-10 stopped writing the full settings projection.
func extractConfigJSONKeys(raw string, keys ...string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || len(keys) == 0 {
		return "{}"
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return "{}"
	}
	out := make(map[string]json.RawMessage, len(keys))
	for _, k := range keys {
		if v, ok := root[k]; ok && len(v) > 0 && string(v) != "null" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return "{}"
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func mergeConfigJSONKeys(computed, legacy string, lg loggateway.Logger, keys ...string) string {
	if strings.TrimSpace(legacy) == "" || len(keys) == 0 {
		return computed
	}
	var leg map[string]json.RawMessage
	if err := json.Unmarshal([]byte(legacy), &leg); err != nil {
		if lg != nil {
			lg.Warn("解析 legacy config_json 失败", loggateway.StepID("agent.merge_eval"), loggateway.Err(err))
		}
		return computed
	}
	var comp map[string]any
	if err := json.Unmarshal([]byte(computed), &comp); err != nil {
		if lg != nil {
			lg.Warn("解析 computed config_json 失败", loggateway.StepID("agent.merge_eval"), loggateway.Err(err))
		}
		return computed
	}
	merged := false
	for _, k := range keys {
		raw, ok := leg[k]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var val any
		if err := json.Unmarshal(raw, &val); err != nil {
			if lg != nil {
				lg.Warn("解析 config_json 覆盖字段失败", loggateway.StepID("agent.merge_eval"), loggateway.Str("key", k), loggateway.Err(err))
			}
			continue
		}
		comp[k] = val
		merged = true
	}
	if !merged {
		return computed
	}
	out, err := json.Marshal(comp)
	if err != nil {
		return computed
	}
	return string(out)
}
