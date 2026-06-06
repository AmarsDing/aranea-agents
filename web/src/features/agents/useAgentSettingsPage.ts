import { computed, onMounted, reactive, ref, watch } from 'vue';
import { copyToClipboard, useQuasar } from 'quasar';
import { useRoute, useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import type { Agent } from './types';
import { useAgentDetailStore } from '../../stores/agents';
import { statusOptions, tokenEstimateFor } from '../../components/agents/agentUi';
import { useAppStore } from '../../stores/app';
import { useChannelsStore } from '../../stores/channels';
import { useAgentPlannerForm } from './useAgentPlannerForm';
import { useAgentRalphLoopForm } from './useAgentRalphLoopForm';
import { useAgentRuntimeConfig } from './useAgentRuntimeConfig';
import { useAgentToolsCatalog } from './useAgentToolsCatalog';
import { useAgentSkillCatalog } from './useAgentSkillCatalog';
import { useAgentProviderModelPicker } from './useAgentProviderModelPicker';
import { useAgentModelValidation } from './useAgentModelValidation';
import { useAgentPromptFiles, useAgentPromptFilesTabWatcher } from './useAgentPromptFiles';
import { useAgentPromptPreview } from './useAgentPromptPreview';
import { useAgentEvolutionSettings } from './useAgentEvolutionSettings';
import { useAgentAvatarIcon } from './useAgentAvatarIcon';
import { resetSkillRuntimeDefaults } from './agentSkillRuntimeConfig';
import { useAgentSettingsPersistence } from './useAgentSettingsPersistence';

export function useAgentSettingsPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const store = useAppStore();
  const detailStore = useAgentDetailStore();
  const channelsStore = useChannelsStore();
  const agentId = computed(() => String(route.params.id ?? '').trim());
  const { saving } = storeToRefs(detailStore);

  const {
    form: plannerForm,
    hydrateFromSettings: hydratePlannerForm,
    validate: validatePlannerFormState,
    serialize: serializePlannerFormState,
  } = useAgentPlannerForm();
  const {
    form: ralphLoopForm,
    hydrateFromSettings: hydrateRalphLoopForm,
    validate: validateRalphLoopFormState,
    serialize: serializeRalphLoopFormState,
  } = useAgentRalphLoopForm();

  const {
    config,
    advancedState,
    hydrateSettings,
    buildSettingsPayload,
    buildConfigJson,
    onAdvancedSave,
    truncateStrategyOptions,
    snapshotModeOptions,
    memoryScopeOptions,
    piiPolicyOptions,
    toolProfileOptions,
  } = useAgentRuntimeConfig();

  const { loadingCatalogTools, loadCatalogTools, toolSelectOptions, toolConflicts } = useAgentToolsCatalog(config);

  const {
    skillSlugOptions,
    loadingSkillSlugs,
    loadSkillSlugOptions,
    codeExecutorCapabilities,
    loadCodeExecutorCapabilities,
  } = useAgentSkillCatalog();

  const tab = ref('agent');

  const form = reactive<Agent>({
    id: '',
    agent_key: '',
    display_name: '',
    provider: '',
    model: '',
    agent_kind: '',
    a2a_proxy_config: undefined,
    status: 'active',
    is_default: false,
    is_favorite: false,
    icon: '',
    agent_description: '',
    taxonomy_position_id: '',
    system_prompt_mode: 'complete',
    context_window: 0,
    budget_monthly_cents: 0,
    config_json: '',
    created_at: '',
    updated_at: '',
    deleted_at: '',
  });

  const systemPromptMode = computed(() => form.system_prompt_mode);
  const agentIcon = computed(() => form.icon);
  const { avatarPickerOpen, primeThumbnailCache } = useAgentAvatarIcon(agentIcon);

  const { promptDialog, previewMode, promptPreview, promptModes, loadPromptPreview, syncPreviewModeFromAgent } =
    useAgentPromptPreview(agentId, systemPromptMode);

  const { showEvolving } = useAgentEvolutionSettings(form, config);

  const {
    loadingProviderModels,
    filteredProviderModelOptions,
    selectedProviderModelID,
    orphanProviderModel,
    disabledCatalogMatch,
    loadProviderModels,
    selectProviderModel,
    filterProviderModels,
    resetProviderModelFilter,
  } = useAgentProviderModelPicker(form);

  const {
    checking: checkingAgentModel,
    ok: agentModelCheckOk,
    message: agentModelCheckMessage,
    runValidate: runAgentModelValidate,
  } = useAgentModelValidation(
    () => form.provider,
    () => form.model,
  );

  const promptFiles = useAgentPromptFiles(agentId, (opts) => $q.notify(opts));
  const {
    fileSplitter,
    activeFile,
    files,
    fileDirty,
    availableOptionalFiles,
    addOptionalFile,
    snapshotFiles,
    updateFileBody,
    reloadActiveFile,
    hydrateFiles,
    refreshFileTokenEstimates,
    filesForSave,
    fileTokenByName,
  } = promptFiles;

  useAgentPromptFilesTabWatcher(
    tab,
    computed(() => form.id),
    (id) => void refreshFileTokenEstimates(id),
  );

  const budgetUSD = computed({
    get: () => Math.round((form.budget_monthly_cents || 0) / 100),
    set: (value: number) => {
      form.budget_monthly_cents = Math.round((Number(value) || 0) * 100);
    },
  });

  // ── Persistence ──────────────────────────────────────────────
  const persistence = useAgentSettingsPersistence({
    form,
    $q,
    agentId,
    detailStore,
    appStore: store,
    channelsStore,
    selectedProviderModelID,
    orphanProviderModel,
    loadProviderModels,
    runAgentModelValidate,
    validatePlannerFormState,
    serializePlannerFormState,
    hydratePlannerForm,
    validateRalphLoopFormState,
    serializeRalphLoopFormState,
    hydrateRalphLoopForm,
    hydrateSettings,
    buildSettingsPayload,
    buildConfigJson,
    onAdvancedSave,
    advancedState,
    filesForSave,
    snapshotFiles,
    hydrateFiles,
    refreshFileTokenEstimates,
    loadPromptPreview,
    syncPreviewModeFromAgent,
    primeThumbnailCache,
    loadCatalogTools,
    loadSkillSlugOptions,
    loadCodeExecutorCapabilities,
  });

  const selectedProviderModelIDModel = computed({
    get: () => selectedProviderModelID.value,
    set: (value: string | null | undefined) => {
      selectProviderModel(value ?? null);
      persistence.modelChanged.value = true;
    },
  });

  // ── Lifecycle ────────────────────────────────────────────────
  onMounted(() => void persistence.loadInitial());

  watch(
    () => String(route.params.id ?? '').trim(),
    async (newId, prevId) => {
      if (!newId || newId === prevId) return;
      try {
        const agent = await detailStore.fetchById(newId);
        await persistence.applyLoadedAgent(agent);
      } catch (e) {
        persistence.loadError.value = e instanceof Error ? e.message : '加载 Agent 失败';
      }
    },
  );

  // ── Non-persistence actions ──────────────────────────────────
  async function toggleFavorite() {
    const next = !form.is_favorite;
    form.is_favorite = next;
    try {
      const updated = await detailStore.toggleFavorite(form.id);
      form.is_favorite = updated.is_favorite;
      store.upsertAgent(updated);
    } catch (error) {
      form.is_favorite = !next;
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '收藏保存失败' });
    }
  }

  async function copyKey() {
    await copyToClipboard(form.agent_key);
    $q.notify({ type: 'positive', message: 'Agent 标识已复制' });
  }

  function confirmFileReload() {
    $q.dialog({
      title: '重新召唤',
      message: '未保存的更改将丢失，确定重新召唤？',
      cancel: true,
      persistent: true,
    }).onOk(() => void reloadActiveFile());
  }

  const advancedChannelOptions = computed(() =>
    channelsStore.channels.map((ch) => ({ label: ch.name || ch.key, value: ch.id })),
  );
  const loadingAdvancedChannels = computed(() => channelsStore.loading);

  return {
    tab,
    form,
    config,
    plannerForm,
    ralphLoopForm,
    saving,
    router,
    avatarPickerOpen,
    promptDialog,
    advancedDialog: persistence.advancedDialog,
    loadError: persistence.loadError,
    pageLoading: persistence.pageLoading,
    toggleFavorite,
    reloadAgent: persistence.reloadAgent,
    loadInitial: persistence.loadInitial,
    saveAgent: persistence.saveAgent,
    confirmFileReload,
    promptModes,
    statusOptions,
    copyKey,
    selectedProviderModelIDModel,
    filteredProviderModelOptions,
    loadingProviderModels,
    orphanProviderModel,
    disabledCatalogMatch,
    checkingAgentModel,
    agentModelCheckOk,
    agentModelCheckMessage,
    filterProviderModels,
    resetProviderModelFilter,
    selectProviderModel,
    openProviderManager: () => router.push({ name: 'models' }),
    budgetUSD,
    toolProfileOptions,
    toolSelectOptions,
    loadingCatalogTools,
    toolConflicts,
    agentId,
    availableOptionalFiles,
    addOptionalFile,
    activeFile,
    fileSplitter,
    files,
    fileDirty,
    updateFileBody,
    reloadActiveFile,
    truncateStrategyOptions,
    snapshotModeOptions,
    memoryScopeOptions,
    piiPolicyOptions,
    loadSkillSlugOptions,
    resetSkillRuntimeDefaults: () =>
      resetSkillRuntimeDefaults(config, (message) => $q.notify({ type: 'info', message })),
    confirmResetSkillDefaults: () => {
      $q.dialog({
        title: '恢复默认',
        message: '确定恢复默认 Skill 配置？当前自定义设置将被覆盖。',
        cancel: true,
        persistent: true,
      }).onOk(() => resetSkillRuntimeDefaults(config, (message) => $q.notify({ type: 'info', message })));
    },
    loadingSkillSlugs,
    skillSlugOptions,
    codeExecutorCapabilities,
    showEvolving,
    fileTokenByName,
    refreshFileTokenEstimates: () => void refreshFileTokenEstimates(form.id),
    tokenEstimateFor,
    previewMode,
    promptPreview,
    advancedState,
    onAdvancedSave: persistence.handleAdvancedSave,
    advancedChannelOptions,
    loadingAdvancedChannels,
  };
}
