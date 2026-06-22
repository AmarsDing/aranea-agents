/** Agent settings persistence: save, load, reload, and advanced-save logic. */

import { computed, ref } from 'vue';
import type { ComputedRef, Ref } from 'vue';
import type { Agent, AgentPromptFile, AgentRuntimeSettings } from './types';
import type { AgentAdvancedSettingsForm } from './agentRuntimeConfig';
import type { AgentRuntimeHydrateHooks } from './useAgentRuntimeConfig';

export interface UseAgentSettingsPersistenceDeps {
  form: Agent;
  $q: { notify: (opts: { type?: string; message: string }) => void };
  agentId: ComputedRef<string>;

  detailStore: {
    fetchById: (id: string) => Promise<Agent>;
    patch: (
      id: string,
      data: Partial<Omit<Agent, 'settings' | 'files' | 'config_json'>> & {
        settings?: Record<string, unknown>;
        files?: { name: string; body: string; sort_order: number }[];
        config_json?: string;
      },
    ) => Promise<Agent>;
  };
  appStore: {
    upsertAgent: (agent: Agent) => void;
  };
  channelsStore: {
    loadChannels: () => Promise<void>;
  };

  // Model picker
  selectedProviderModelID: Ref<string | null>;
  orphanProviderModel: Ref<boolean>;
  loadProviderModels: () => Promise<void>;

  // Model validation
  runAgentModelValidate: () => Promise<{ ok: boolean; message: string }>;

  // Planner form
  validatePlannerFormState: () => string | null;
  serializePlannerFormState: () => Record<string, unknown>;
  hydratePlannerForm: (kind?: string, configJson?: string) => void;

  // Ralph loop form
  validateRalphLoopFormState: () => string | null;
  serializeRalphLoopFormState: () => Record<string, unknown>;
  hydrateRalphLoopForm: (settings?: AgentRuntimeSettings | null) => void;

  // Runtime config
  hydrateSettings: (agent: Agent, hooks?: AgentRuntimeHydrateHooks) => void;
  buildSettingsPayload: (extras?: Record<string, unknown>) => Record<string, unknown>;
  buildConfigJson: (files: { name: string; body: string }[]) => string;
  onAdvancedSave: (payload: AgentAdvancedSettingsForm, save: () => void | Promise<void>) => Promise<void>;
  advancedState: AgentAdvancedSettingsForm;

  // Prompt files
  filesForSave: () => { name: string; body: string; sort_order: number }[];
  snapshotFiles: () => void;
  hydrateFiles: (files: AgentPromptFile[]) => void;
  refreshFileTokenEstimates: (id: string) => Promise<void>;

  // Prompt preview
  loadPromptPreview: () => Promise<void>;
  syncPreviewModeFromAgent: (mode: string) => void;

  // Avatar
  primeThumbnailCache: () => Promise<void>;

  // Tools catalog
  loadCatalogTools: () => Promise<void>;

  // Skill catalog
  loadSkillSlugOptions: () => Promise<void>;
  loadCodeExecutorCapabilities: () => Promise<void>;
}

export function useAgentSettingsPersistence(deps: UseAgentSettingsPersistenceDeps) {
  const loadError = ref('');
  const pageLoading = ref(true);
  const modelChanged = ref(false);
  const advancedDialog = ref(false);

  function runtimeHydrateHooks(): AgentRuntimeHydrateHooks {
    return {
      onFromSettings: (agent: Agent) => {
        deps.hydratePlannerForm(agent.settings?.planner_kind, agent.settings?.planner_config_json);
        deps.hydrateRalphLoopForm(agent.settings ?? null);
      },
      onFromConfigJson: () => {
        deps.hydratePlannerForm();
        deps.hydrateRalphLoopForm();
      },
    };
  }

  async function applyLoadedAgent(agent: Agent | null | undefined) {
    if (!agent?.id) {
      loadError.value = '未找到该 Agent';
      return false;
    }
    Object.assign(deps.form, agent);
    deps.hydrateSettings(agent, runtimeHydrateHooks());
    deps.appStore.upsertAgent(agent);
    deps.snapshotFiles();
    deps.syncPreviewModeFromAgent(deps.form.system_prompt_mode);
    await deps.loadPromptPreview();
    await deps.primeThumbnailCache();
    void deps.refreshFileTokenEstimates(deps.form.id);
    if (agent.files?.length) {
      deps.hydrateFiles(agent.files);
    } else {
      deps.hydrateFiles([]);
    }
    modelChanged.value = false;
    return true;
  }

  async function saveAgent() {
    if (!deps.selectedProviderModelID.value) {
      // Allow empty model (agent will inherit from chat interface at runtime).
      // Only block if the model was explicitly set but is orphaned/disabled.
      if (deps.orphanProviderModel.value) {
        deps.$q.notify({
          type: 'negative',
          message: '当前模型不在 Provider 目录或已禁用，请在「模型管理」修正后重新选择',
        });
        return;
      }
    }
    if (modelChanged.value) {
      // Skip model validation when clearing the model (agent will inherit from chat interface).
      if (deps.form.provider || deps.form.model) {
        const modelResult = await deps.runAgentModelValidate();
        if (!modelResult.ok) {
          deps.$q.notify({
            type: 'negative',
            message: modelResult.message || '模型不可用，请检查 Provider 管理中的目录配置',
          });
          return;
        }
      }
    }
    const plannerErr = deps.validatePlannerFormState();
    if (plannerErr) {
      deps.$q.notify({ type: 'negative', message: plannerErr });
      return;
    }
    const ralphErr = deps.validateRalphLoopFormState();
    if (ralphErr) {
      deps.$q.notify({ type: 'negative', message: ralphErr });
      return;
    }
    const fileRows = deps.filesForSave();
    try {
      const { settings: _settings, ...formWithoutSettings } = deps.form;
      const updated = await deps.detailStore.patch(deps.form.id, {
        ...formWithoutSettings,
        settings: deps.buildSettingsPayload({
          ...deps.serializePlannerFormState(),
          ...deps.serializeRalphLoopFormState(),
        }),
        files: fileRows.map(({ name, body, sort_order }) => ({ name, body, sort_order })),
        config_json: deps.buildConfigJson(fileRows),
      });
      Object.assign(deps.form, updated);
      deps.hydrateSettings(updated, runtimeHydrateHooks());
      deps.appStore.upsertAgent(updated);
      deps.snapshotFiles();
      await deps.loadPromptPreview();
      await deps.primeThumbnailCache();
      void deps.refreshFileTokenEstimates(deps.form.id);
      modelChanged.value = false;
      deps.$q.notify({ type: 'positive', message: '已保存' });
    } catch (e) {
      deps.$q.notify({ type: 'negative', message: e instanceof Error ? e.message : '保存失败' });
    }
  }

  async function handleAdvancedSave(payload: AgentAdvancedSettingsForm) {
    try {
      await deps.onAdvancedSave(payload, saveAgent);
      advancedDialog.value = false;
    } catch {
      // saveAgent already shows error notification
    }
  }

  async function reloadAgent() {
    const id = deps.agentId.value;
    if (!id) return;
    try {
      const agent = await deps.detailStore.fetchById(id);
      await applyLoadedAgent(agent);
    } catch (e) {
      deps.$q.notify({ type: 'negative', message: e instanceof Error ? e.message : '刷新 Agent 失败' });
    }
  }

  async function loadInitial() {
    const id = deps.agentId.value;
    if (!id) {
      loadError.value = '缺少 Agent ID';
      pageLoading.value = false;
      return;
    }
    loadError.value = '';
    pageLoading.value = true;
    try {
      const [agent] = await Promise.all([
        deps.detailStore.fetchById(id),
        deps.loadProviderModels(),
        deps.loadCatalogTools(),
        deps.loadSkillSlugOptions(),
        deps.loadCodeExecutorCapabilities(),
        deps.channelsStore.loadChannels().catch(() => {}),
      ]);
      await applyLoadedAgent(agent);
    } catch (e) {
      loadError.value = e instanceof Error ? e.message : '加载 Agent 失败';
    } finally {
      pageLoading.value = false;
    }
  }

  return {
    loadError,
    pageLoading,
    modelChanged,
    advancedDialog,
    saveAgent,
    handleAdvancedSave,
    reloadAgent,
    loadInitial,
    applyLoadedAgent,
  };
}
