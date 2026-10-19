package biz

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"

	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func (u *AgentUsecase) hydrate(ctx context.Context, agent Agent) (Agent, error) {
	return u.hydrateWithExtras(ctx, agent, false)
}

// assembleHydrated builds the hydrated agent view from settings + files:
// computes config_json (with defaults and evaluation/knowledge legacy overlay)
// and runtime kind flags. Shared by the single-Get and batch-hydrate paths.
func (u *AgentUsecase) assembleHydrated(agent Agent, settings AgentRuntimeSettings, files []AgentPromptFile) (Agent, error) {
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
	return agent, nil
}

// hydrateWithExtras hydrates an agent with settings, files, and optionally extras.
// When skipExtras is true, the ListExtrasForAgents query is skipped (suitable for
// orchestration/build paths that only need settings + files, not list-display fields).
func (u *AgentUsecase) hydrateWithExtras(ctx context.Context, agent Agent, skipExtras bool) (Agent, error) {
	settings, err := u.settingsFor(ctx, agent)
	if err != nil {
		return Agent{}, err
	}
	files, err := u.filesFor(ctx, agent)
	if err != nil {
		return Agent{}, err
	}
	agent, err = u.assembleHydrated(agent, settings, files)
	if err != nil {
		return Agent{}, err
	}
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

// settingsFor loads the settings row, lazily migrating legacy config_json when
// the row is missing.
func (u *AgentUsecase) settingsFor(ctx context.Context, agent Agent) (AgentRuntimeSettings, error) {
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, agent.ID)
	if err != nil {
		if !stderrors.Is(err, shared.ErrNotFound) {
			return AgentRuntimeSettings{}, err
		}
		u.lg.Warn("agent runtime settings not found, migrating from legacy config_json", loggateway.StepID("agent.db_resolve"), loggateway.Str("agent_id", agent.ID))
		return u.migrateLegacySettings(ctx, agent), nil
	}
	return settings, nil
}

// filesFor loads prompt files, lazily migrating legacy config_json when none exist.
func (u *AgentUsecase) filesFor(ctx context.Context, agent Agent) ([]AgentPromptFile, error) {
	files, err := u.files.ListAgentPromptFiles(ctx, agent.ID)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		u.lg.Warn("agent prompt files not found, migrating from legacy config_json", loggateway.StepID("agent.skill_build"), loggateway.Str("agent_id", agent.ID))
		return u.migrateLegacyFiles(ctx, agent), nil
	}
	return files, nil
}

// Optional batch-read interfaces implemented by the production repo. Legacy
// test fakes keep the per-agent fallback path below.
type agentSettingsBatchReader interface {
	ListAgentRuntimeSettingsByIDs(ctx context.Context, ids []string) (map[string]AgentRuntimeSettings, error)
}

type agentFilesBatchReader interface {
	ListAgentPromptFilesByAgentIDs(ctx context.Context, ids []string) (map[string][]AgentPromptFile, error)
}

// BatchHydrateForBuild hydrates multiple agents in bulk for orchestration/build paths.
// With the production repo this is TWO queries total (settings IN ids, files IN
// ids) instead of 2×N per-agent queries; extras are skipped (not needed for
// runtime builds). Repos lacking the batch interfaces fall back to the per-agent
// loop (legacy test fakes).
func (u *AgentUsecase) BatchHydrateForBuild(ctx context.Context, agents []Agent) ([]Agent, error) {
	if len(agents) == 0 {
		return agents, nil
	}
	sbr, sOK := u.settings.(agentSettingsBatchReader)
	fbr, fOK := u.files.(agentFilesBatchReader)
	if !sOK || !fOK {
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
	ids := make([]string, 0, len(agents))
	for _, a := range agents {
		if a.ID != "" {
			ids = append(ids, a.ID)
		}
	}
	settingsByID, err := sbr.ListAgentRuntimeSettingsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	filesByID, err := fbr.ListAgentPromptFilesByAgentIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Agent, len(agents))
	for i, a := range agents {
		settings, ok := settingsByID[a.ID]
		if !ok {
			u.lg.Warn("agent runtime settings not found, migrating from legacy config_json", loggateway.StepID("agent.db_resolve"), loggateway.Str("agent_id", a.ID))
			settings = u.migrateLegacySettings(ctx, a)
		}
		files := filesByID[a.ID]
		if len(files) == 0 {
			u.lg.Warn("agent prompt files not found, migrating from legacy config_json", loggateway.StepID("agent.skill_build"), loggateway.Str("agent_id", a.ID))
			files = u.migrateLegacyFiles(ctx, a)
		}
		h, err := u.assembleHydrated(a, settings, files)
		if err != nil {
			return nil, err
		}
		out[i] = h
	}
	return out, nil
}

// GetForBuild returns the hydrated agent for runtime build/turn paths WITHOUT
// the extras enrichment — the ListExtrasForAgents read is skipped (2 fewer SQL
// queries per turn: the extras fan-out is two statements, latest-session window
// + pending-count; see internal/data/agent_list_extras.go; P3-1, 2026-10-19).
// Steady state: 3 queries (agent + settings + files) vs Get's 5 — the same
// 5-query baseline GetRuntimeSettingsForAdmission's "1 query steady state vs 5
// for hydrate" counts against. Semantic parity with Get: same settings +
// files hydrate including the legacy config_json migration; only the
// display-only extras (LastRunStatus/LastRunAt/PendingEvolutionCount) stay
// zero, so list/admin display paths must keep using Get.
func (u *AgentUsecase) GetForBuild(ctx context.Context, id string) (Agent, error) {
	id, err := requireNonEmpty(id, "AGENT", "id")
	if err != nil {
		return Agent{}, err
	}
	a, err := u.reader.GetAgentByID(ctx, id)
	if err != nil {
		if stderrors.Is(err, shared.ErrNotFound) {
			return Agent{}, shared.ErrNotFound
		}
		return Agent{}, err
	}
	return u.hydrateWithExtras(ctx, a, true)
}

func (u *AgentUsecase) migrateLegacySettings(ctx context.Context, agent Agent) AgentRuntimeSettings {
	settings := withSettingDefaults(settingsFromLegacyConfig(agent.ConfigJSON))
	settings.AgentID = agent.ID
	migrated, err := u.settings.UpsertAgentRuntimeSettings(ctx, settings)
	if err != nil {
		// P2 fix: the read-path migration previously swallowed write failures
		// silently; make them visible (read still succeeds with in-memory value).
		u.lg.Warn("legacy settings migration upsert failed; serving in-memory value",
			loggateway.StepID("agent.db_resolve"),
			loggateway.Str("agent_id", agent.ID),
			loggateway.Err(err))
		return settings
	}
	return migrated
}

// GetRuntimeSettingsForAdmission returns the agent's raw runtime settings for
// cheap read-only checks (e.g. the turn-admission context threshold probe),
// WITHOUT the full hydrate (no prompt-files/extras queries) — 1 query steady
// state vs 5 for hydrate.
//
// P3-1 (2026-10-19): semantics match the hydrated read exactly:
//   - rows missing entirely fall back to legacy config_json migration
//     (settingsFor parity — cold agents pre-migration still resolve),
//   - values are returned RAW (no withSettingDefaults fill-in), so callers
//     keep the same "unset (<=0) → caller default" semantics as reading
//     Agent.Settings after hydrate (assembleHydrated also stores raw).
func (u *AgentUsecase) GetRuntimeSettingsForAdmission(ctx context.Context, agentID string) (AgentRuntimeSettings, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return AgentRuntimeSettings{}, apierror.BadRequest("AGENT", "agent id is required")
	}
	settings, err := u.settings.GetAgentRuntimeSettings(ctx, agentID)
	if err == nil {
		return settings, nil
	}
	if !stderrors.Is(err, shared.ErrNotFound) {
		return AgentRuntimeSettings{}, err
	}
	a, err := u.reader.GetAgentByID(ctx, agentID)
	if err != nil {
		return AgentRuntimeSettings{}, err
	}
	return u.migrateLegacySettings(ctx, a), nil
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
		u.lg.Warn("legacy prompt files migration replace failed; serving in-memory value",
			loggateway.StepID("agent.skill_build"),
			loggateway.Str("agent_id", agent.ID),
			loggateway.Err(err))
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
