import { computed, onMounted, reactive, ref, watch } from "vue";
import { copyToClipboard, useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import type { Agent } from "./types";
import { useAgentDetailStore } from "../../stores/agents";
import { statusOptions, tokenEstimateFor } from "../../components/agents/agentUi";
import { useAppStore } from "../../stores/app";
import { useAgentPlannerForm } from "./useAgentPlannerForm";
import { useAgentRalphLoopForm } from "./useAgentRalphLoopForm";
import { useAgentRuntimeConfig } from "./useAgentRuntimeConfig";
import { useAgentToolsCatalog } from "./useAgentToolsCatalog";
import { useAgentSkillCatalog } from "./useAgentSkillCatalog";
import { useAgentProviderModelPicker } from "./useAgentProviderModelPicker";
import { useAgentModelValidation } from "./useAgentModelValidation";
import { useAgentPromptFiles, useAgentPromptFilesTabWatcher } from "./useAgentPromptFiles";
import { useAgentPromptPreview } from "./useAgentPromptPreview";
import { useAgentEvolutionSettings } from "./useAgentEvolutionSettings";
import { useAgentAvatarIcon } from "./useAgentAvatarIcon";
import { resetSkillRuntimeDefaults } from "./agentSkillRuntimeConfig";

export function useAgentSettingsPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const store = useAppStore();
  const detailStore = useAgentDetailStore();
  const agentId = computed(() => String(route.params.id ?? "").trim());
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
    toolProfileOptions,
  } = useAgentRuntimeConfig();

  const { loadingCatalogTools, loadCatalogTools, toolSelectOptions, toolConflicts } =
    useAgentToolsCatalog(config);

  const {
    skillSlugOptions,
    loadingSkillSlugs,
    loadSkillSlugOptions,
    codeExecutorCapabilities,
    loadCodeExecutorCapabilities,
  } = useAgentSkillCatalog();

  const tab = ref("agent");
  const advancedDialog = ref(false);

  const form = reactive<Agent>({
    id: "",
    agent_key: "",
    display_name: "",
    provider: "",
    model: "",
    agent_kind: "",
    a2a_proxy_config: undefined,
    status: "active",
    is_default: false,
    is_favorite: false,
    icon: "",
    agent_description: "",
    category_position_id: "",
    system_prompt_mode: "complete",
    context_window: 0,
    budget_monthly_cents: 0,
    config_json: "",
    created_at: "",
    updated_at: "",
    deleted_at: "",
  });

  const systemPromptMode = computed(() => form.system_prompt_mode);
  const agentIcon = computed(() => form.icon);
  const { avatarPickerOpen, primeThumbnailCache } = useAgentAvatarIcon(agentIcon);

  const {
    promptDialog,
    previewMode,
    promptPreview,
    promptModes,
    loadPromptPreview,
    syncPreviewModeFromAgent,
  } = useAgentPromptPreview(agentId, systemPromptMode);

  const { evolutionRange, showEvolving } = useAgentEvolutionSettings(form, config);

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

  const selectedProviderModelIDModel = computed({
    get: () => selectedProviderModelID.value,
    set: (value: string | null | undefined) => selectProviderModel(value ?? null),
  });

  const promptFiles = useAgentPromptFiles(agentId, (opts) => $q.notify(opts));
  const {
    fileSplitter,
    activeFile,
    files,
    fileDirty,
    heartbeatFile,
    snapshotFiles,
    updateFileBody,
    reloadActiveFile,
    hydrateFiles,
    refreshFileTokenEstimates,
    applyAiEdit,
    filesForSave,
    aiEditOpen,
    aiEditing,
    aiInstruction,
    fileTokenByName,
  } = promptFiles;

  useAgentPromptFilesTabWatcher(tab, computed(() => form.id), (id) => void refreshFileTokenEstimates(id));

  const budgetUSD = computed({
    get: () => Math.round((form.budget_monthly_cents || 0) / 100),
    set: (value: number) => {
      form.budget_monthly_cents = Math.round((Number(value) || 0) * 100);
    },
  });

  function runtimeHydrateHooks() {
    return {
      onFromSettings: (agent: Agent) => {
        hydratePlannerForm(agent.settings?.planner_kind, agent.settings?.planner_config_json);
        hydrateRalphLoopForm(agent.settings);
      },
      onFromConfigJson: () => {
        hydratePlannerForm();
        hydrateRalphLoopForm();
      },
    };
  }

  async function applyLoadedAgent(agent: Agent | null | undefined) {
    if (!agent?.id) {
      $q.notify({ type: "warning", message: "未找到该 Agent" });
      router.back();
      return false;
    }
    Object.assign(form, agent);
    hydrateSettings(agent, runtimeHydrateHooks());
    store.upsertAgent(agent);
    snapshotFiles();
    syncPreviewModeFromAgent(form.system_prompt_mode);
    await loadPromptPreview();
    await primeThumbnailCache();
    void refreshFileTokenEstimates(form.id);
    if (agent.files?.length) {
      hydrateFiles(agent.files);
    }
    return true;
  }

  onMounted(async () => {
    const id = String(route.params.id ?? "").trim();
    if (!id) {
      $q.notify({ type: "negative", message: "缺少 Agent ID" });
      router.back();
      return;
    }
    try {
      const [agent] = await Promise.all([
        detailStore.fetchById(id),
        loadProviderModels(),
        loadCatalogTools(),
        loadSkillSlugOptions(),
        loadCodeExecutorCapabilities(),
      ]);
      await applyLoadedAgent(agent);
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载 Agent 失败" });
      router.back();
    }
  });

  watch(
    () => String(route.params.id ?? "").trim(),
    async (newId, prevId) => {
      if (!newId || newId === prevId) return;
      try {
        const agent = await detailStore.fetchById(newId);
        await applyLoadedAgent(agent);
      } catch (e) {
        $q.notify({ type: "negative", message: e instanceof Error ? e.message : "加载 Agent 失败" });
        router.back();
      }
    },
  );

  async function saveAgent() {
    if (!selectedProviderModelID.value) {
      $q.notify({
        type: "negative",
        message: orphanProviderModel.value
          ? "当前模型不在 Provider 目录或已禁用，请在「模型管理」修正后重新选择"
          : "请选择已录入且启用的模型",
      });
      return;
    }
    const modelResult = await runAgentModelValidate();
    if (!modelResult.ok) {
      $q.notify({
        type: "negative",
        message: modelResult.message || "模型不可用，请检查 Provider 管理中的目录配置",
      });
      return;
    }
    const plannerErr = validatePlannerFormState();
    if (plannerErr) {
      $q.notify({ type: "negative", message: plannerErr });
      return;
    }
    const ralphErr = validateRalphLoopFormState();
    if (ralphErr) {
      $q.notify({ type: "negative", message: ralphErr });
      return;
    }
    const fileRows = filesForSave();
    try {
      const updated = await detailStore.patch(form.id, {
        ...form,
        settings: buildSettingsPayload({
          ...serializePlannerFormState(),
          ...serializeRalphLoopFormState(),
        }),
        files: fileRows.map(({ name, body, sort_order }) => ({ name, body, sort_order })),
        config_json: buildConfigJson(fileRows),
      });
      Object.assign(form, updated);
      hydrateSettings(updated, runtimeHydrateHooks());
      store.upsertAgent(updated);
      snapshotFiles();
      await loadPromptPreview();
      await primeThumbnailCache();
      void refreshFileTokenEstimates(form.id);
      $q.notify({ type: "positive", message: "已保存" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "保存失败" });
    }
  }

  function handleAdvancedSave(payload: typeof advancedState) {
    onAdvancedSave(payload, saveAgent);
  }

  async function toggleFavorite() {
    const next = !form.is_favorite;
    form.is_favorite = next;
    try {
      const updated = await detailStore.patch(form.id, {
        ...form,
        is_favorite: next,
        settings: buildSettingsPayload({
          ...serializePlannerFormState(),
          ...serializeRalphLoopFormState(),
        }),
        files: filesForSave().map(({ name, body, sort_order }) => ({ name, body, sort_order })),
      });
      Object.assign(form, updated);
      hydrateSettings(updated, runtimeHydrateHooks());
      store.upsertAgent(updated);
    } catch (error) {
      form.is_favorite = !next;
      $q.notify({ type: "negative", message: error instanceof Error ? error.message : "收藏保存失败" });
    }
  }

  async function copyKey() {
    await copyToClipboard(form.agent_key);
    $q.notify({ type: "positive", message: "Agent 标识已复制" });
  }

  async function reloadAgent() {
    const id = String(route.params.id ?? "").trim();
    if (!id) return;
    try {
      const agent = await detailStore.fetchById(id);
      await applyLoadedAgent(agent);
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "刷新 Agent 失败" });
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
    avatarPickerOpen,
    promptDialog,
    advancedDialog,
    toggleFavorite,
    reloadAgent,
    saveAgent,
    promptModes,
    statusOptions,
    copyKey,
    selectedProviderModelID,
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
    openProviderManager: () => router.push({ name: "models" }),
    budgetUSD,
    toolProfileOptions,
    toolSelectOptions,
    loadingCatalogTools,
    toolConflicts,
    agentId,
    heartbeatFile,
    activeFile,
    fileSplitter,
    files,
    fileDirty,
    updateFileBody,
    reloadActiveFile,
    aiEditOpen,
    aiEditing,
    truncateStrategyOptions,
    snapshotModeOptions,
    memoryScopeOptions,
    loadSkillSlugOptions,
    resetSkillRuntimeDefaults: () =>
      resetSkillRuntimeDefaults(config, (message) => $q.notify({ type: "info", message })),
    loadingSkillSlugs,
    skillSlugOptions,
    codeExecutorCapabilities,
    evolutionRange,
    showEvolving,
    fileTokenByName,
    refreshFileTokenEstimates: () => void refreshFileTokenEstimates(form.id),
    tokenEstimateFor,
    previewMode,
    promptPreview,
    aiInstruction,
    applyAiEdit: () => applyAiEdit(form.id),
    advancedState,
    onAdvancedSave: handleAdvancedSave,
  };
}
