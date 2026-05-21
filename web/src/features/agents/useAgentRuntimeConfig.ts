import { reactive } from "vue";
import type { Agent } from "./types";
import {
  defaultAgentAdvancedSettings,
  defaultAgentRuntimeConfig,
  memoryScopeOptions,
  snapshotModeOptions,
  truncateStrategyOptions,
  toolProfileOptions,
} from "./agentRuntimeConfig";
import {
  applyAdvancedSaveToRuntime,
  hydrateAgentRuntime,
  hydrateRuntimeFromConfigJson,
} from "./agentRuntimeConfigHydrate";
import { buildAgentConfigJson, buildRuntimeSettingsPayload } from "./agentRuntimeConfigSerialize";
import type { AgentAdvancedSettingsForm } from "./agentRuntimeConfig";
import { normalizeSkillRuntimeState, useSkillRuntimeSlugSync } from "./agentSkillRuntimeConfig";

export type AgentRuntimeHydrateHooks = {
  onFromSettings?: (agent: Agent) => void;
  onFromConfigJson?: () => void;
};

/** Runtime + memory + tools + evolution form state and persistence mapping. */
export function useAgentRuntimeConfig() {
  const config = reactive(defaultAgentRuntimeConfig());
  const advancedState = reactive(defaultAgentAdvancedSettings());

  const { normalizeWithSync: normalizeSkillRuntimeStateSynced } = useSkillRuntimeSlugSync(config);

  function hydrateSettings(agent: Agent, hooks?: AgentRuntimeHydrateHooks) {
    const source = hydrateAgentRuntime(config, advancedState, agent);
    if (source === "settings") {
      hooks?.onFromSettings?.(agent);
    } else {
      hooks?.onFromConfigJson?.();
    }
    normalizeSkillRuntimeStateSynced();
  }

  function hydrateConfig(raw: string, files?: { name: string; body: string }[]) {
    hydrateRuntimeFromConfigJson(config, raw, files);
    normalizeSkillRuntimeStateSynced();
  }

  function buildSettingsPayload(extras: Parameters<typeof buildRuntimeSettingsPayload>[2] = {}) {
    return buildRuntimeSettingsPayload(config, advancedState, extras);
  }

  function buildConfigJson(files: { name: string; body: string }[]) {
    return buildAgentConfigJson(config, files);
  }

  function onAdvancedSave(
    payload: AgentAdvancedSettingsForm,
    save: () => void | Promise<void>,
  ) {
    applyAdvancedSaveToRuntime(config, advancedState, payload);
    void save();
  }

  return {
    config,
    advancedState,
    hydrateSettings,
    hydrateConfig,
    buildSettingsPayload,
    buildConfigJson,
    onAdvancedSave,
    normalizeSkillRuntimeState: () => normalizeSkillRuntimeState(config.skillRuntime),
    truncateStrategyOptions,
    snapshotModeOptions,
    memoryScopeOptions,
    toolProfileOptions,
  };
}
