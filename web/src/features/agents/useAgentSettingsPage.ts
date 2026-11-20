import { computed, nextTick, onMounted, reactive, ref, watch } from 'vue';
import { copyToClipboard, useQuasar } from 'quasar';
import { confirm } from '../../composables/useAppConfirm';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { storeToRefs } from 'pinia';
import type { Agent } from './types';
import { useAgentDetailStore, useAgentsPageStore } from '../../stores/agents';
import { statusOptions, tokenEstimateFor } from '../../components/agents/agentUi';
import { useAppStore } from '../../stores/app';
import { useChannelsStore } from '../../stores/channels';
import { useEvaluationStore } from '../../stores/evaluation';
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
import { rosterRole } from './roster';
import type { UseAgentSettingsPersistenceDeps } from './useAgentSettingsPersistence';

export function useAgentSettingsPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const route = useRoute();
  const router = useRouter();
  const store = useAppStore();
  const detailStore = useAgentDetailStore();
  const agentsPageStore = useAgentsPageStore();
  const channelsStore = useChannelsStore();
  const agentId = computed(() => String(route.params.id ?? '').trim());
  const { saving } = storeToRefs(detailStore);
  const { taxonomyTree } = storeToRefs(agentsPageStore);

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
    skillTagOptions,
    loadingSkillTags,
    loadSkillTagOptions,
    codeExecutorCapabilities,
    loadCodeExecutorCapabilities,
  } = useAgentSkillCatalog();

  // Tab 深链：?tab= 初始化 + 切换时写回 URL（replace，不污染历史栈）
  // 默认进「概览」（装配图）；其余分区按业务分组排在左侧导航里
  const AGENT_SETTINGS_TABS = [
    'overview',
    'agent',
    'files',
    'skills',
    'memory',
    'permissions',
    'hooks',
    'a2a',
    'evolution',
    'learning',
  ];
  const initialTab = String(route.query.tab ?? '');
  const tab = ref((AGENT_SETTINGS_TABS as string[]).includes(initialTab) ? initialTab : 'overview');
  watch(tab, (value) => {
    if (String(route.query.tab ?? '') === value) return;
    void router.replace({ query: { ...route.query, tab: value } });
  });

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
    removeFile,
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
    t,
    agentId,
    detailStore: detailStore as unknown as UseAgentSettingsPersistenceDeps['detailStore'],
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
    loadSkillTagOptions,
    loadCodeExecutorCapabilities,
    onClean: () => void markClean(),
  });

  const selectedProviderModelIDModel = computed({
    get: () => selectedProviderModelID.value,
    set: (value: string | null | undefined) => {
      selectProviderModel(value ?? null);
      persistence.modelChanged.value = true;
    },
  });

  // ── 未保存改动：加载 / 保存后拍快照，之后任何字段变化都算改动；离开页面时确认 ──
  const cleanSnapshot = ref('');
  const snapshot = () => JSON.stringify([form, config, plannerForm, ralphLoopForm, advancedState]);
  async function markClean() {
    // 等水合引起的级联 watch（模型选择器等）跑完再拍，避免一打开就显示「有改动」
    await nextTick();
    cleanSnapshot.value = snapshot();
  }
  const dirty = computed(() => Boolean(cleanSnapshot.value) && (fileDirty.value || snapshot() !== cleanSnapshot.value));
  onBeforeRouteLeave(async (to, from) => {
    // 同一页面只改了 ?tab= 不算离开
    if (!dirty.value || to.path === from.path) return true;
    return confirm({
      title: t('agentAssembly.leaveTitle'),
      message: t('agentAssembly.leaveMessage'),
      confirmLabel: t('agentAssembly.leaveConfirm'),
      tone: 'danger',
    });
  });

  // ── 概览（装配图）用到的派生数据 ──
  const overviewOrgPath = computed(() => agentsPageStore.taxonomyLabel(form.taxonomy_position_id));
  const overviewRoleLabel = computed(() => t(`agentRoster.role.${rosterRole(form)}`));
  const overviewFileTokens = computed(() =>
    Object.values(fileTokenByName.value).reduce((n, v) => n + (Number(v) || 0), 0),
  );
  const overviewChannelName = computed(() => {
    const id = advancedState.channel_id;
    if (!id) return '';
    const ch = channelsStore.channels.find((c) => c.id === id);
    return ch ? ch.name || ch.key : id;
  });

  // ── Lifecycle ────────────────────────────────────────────────
  async function loadInitial() {
    try {
      await Promise.all([persistence.loadInitial(), agentsPageStore.ensureTaxonomyTree()]);
    } catch (reason) {
      persistence.loadError.value = reason instanceof Error ? reason.message : t('uiSystem.operations.loadFailed');
    }
  }

  onMounted(() => {
    void loadInitial();
  });

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

  async function confirmFileReload() {
    const ok = await confirm({
      title: t('agentSettings.files.reload'),
      message: t('agentSettings.files.reloadConfirmMessage'),
      tone: 'danger',
    });
    if (!ok) return;
    await reloadActiveFile();
  }

  async function confirmRemoveFile(name: string) {
    const ok = await confirm({
      title: t('agentSettings.files.removeConfirmTitle'),
      message: t('agentSettings.files.removeConfirmMessage', { name }),
      tone: 'danger',
    });
    if (!ok) return;
    removeFile(name);
  }

  const advancedChannelOptions = computed(() =>
    channelsStore.channels.map((ch) => ({ label: ch.name || ch.key, value: ch.id })),
  );
  const loadingAdvancedChannels = computed(() => channelsStore.loading);

  // ── After-Turn 自动评估（US-5）：数据集选项懒加载，进化 Tab 的 q-select 首次展开时触发 ──
  const evaluationStore = useEvaluationStore();
  const evalDatasetOptions = ref<{ label: string; value: string }[]>([]);
  const loadingEvalDatasets = ref(false);
  async function loadEvalDatasetOptions() {
    if (loadingEvalDatasets.value || evalDatasetOptions.value.length) return;
    loadingEvalDatasets.value = true;
    try {
      const res = await evaluationStore.loadDatasets({ limit: 200 });
      evalDatasetOptions.value = res.items.map((d) => ({
        label: t('evaluationPage.datasetOptionLabel', { name: d.name, count: d.case_count }),
        value: d.id,
      }));
    } catch {
      evalDatasetOptions.value = [];
      $q.notify({
        type: 'negative',
        message: t('agentSettings.evolution.evalDatasetLoadFailed'),
      });
    } finally {
      loadingEvalDatasets.value = false;
    }
  }

  return {
    tab,
    form,
    config,
    plannerForm,
    ralphLoopForm,
    saving,
    router,
    taxonomyTree,
    avatarPickerOpen,
    promptDialog,
    advancedDialog: persistence.advancedDialog,
    loadError: persistence.loadError,
    pageLoading: persistence.pageLoading,
    toggleFavorite,
    reloadAgent: persistence.reloadAgent,
    loadInitial,
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
    confirmRemoveFile,
    activeFile,
    fileSplitter,
    files,
    fileDirty,
    updateFileBody,
    truncateStrategyOptions,
    snapshotModeOptions,
    memoryScopeOptions,
    piiPolicyOptions,
    loadSkillSlugOptions,
    resetSkillRuntimeDefaults: () =>
      resetSkillRuntimeDefaults(config, (message) => $q.notify({ type: 'info', message })),
    onRefineError: (message: string) => {
      $q.notify({ type: 'negative', message });
    },
    confirmResetSkillDefaults: async () => {
      const ok = await confirm({
        title: '恢复默认',
        message: '确定恢复默认 Skill 配置？当前自定义设置将被覆盖。',
        tone: 'danger',
      });
      if (!ok) return;
      resetSkillRuntimeDefaults(config, (message) => $q.notify({ type: 'info', message }));
    },
    loadingSkillSlugs,
    skillSlugOptions,
    loadingSkillTags,
    skillTagOptions,
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
    evalDatasetOptions,
    loadingEvalDatasets,
    loadEvalDatasetOptions,
    dirty,
    overviewOrgPath,
    overviewRoleLabel,
    overviewFileTokens,
    overviewChannelName,
  };
}
