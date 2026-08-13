import { storeToRefs } from 'pinia';
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router';
import { copyToClipboard, useQuasar } from 'quasar';
import { mapAgentCreateFieldErrors, parseKratosApiError } from '../../utils/kratosError';
import type { Agent, AgentKind, A2AProxyConfig, AgentTemplatePreset } from './types';
import type { PlatformResource } from '../platform/types';
import { statusOptions } from '../../components/agents/agentUi';
import { useAgentsPageStore } from '../../stores/agents';
import { useAppStore } from '../../stores/app';
import { useAvatarCatalogStore } from '../../stores/avatar';

export type CreateAgentForm = {
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  icon: string;
  agent_description: string;
  position_key: string;
  agent_variant: string;
  variant_description: string;
  taxonomy_position_id: string;
};

type ViewMode = 'grid' | 'list';

const LS_VIEW = 'agents.viewMode';

/** Agent 列表页：组合 Pinia Store 与局部 UI（表单 / 对话框），不包含裸 HTTP（见 aranea-frontend-guide SKILL §3）。 */
export function useAgentsPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const route = useRoute();
  const router = useRouter();
  const appStore = useAppStore();
  const avatarCatalog = useAvatarCatalogStore();
  const pageStore = useAgentsPageStore();

  const {
    agents,
    keyword,
    selectedStatus,
    selectedProvider,
    selectedTaxonomy,
    selectedCreator,
    creatorOptions,
    page,
    rowsPerPage,
    total,
    listLoading,
    providerModels,
    checkingModel,
    modelCheckPassed,
    taxonomyTree,
    pageMax,
    providerOptions,
    tableColumns,
    selectedAgentIds,
  } = storeToRefs(pageStore);

  const taxonomyLabel = pageStore.taxonomyLabel;

  // ISSUE-004：筛选/分页状态同步 URL query，刷新与分享链接后恢复
  {
    const q = route.query;
    if (typeof q.q === 'string' && q.q) keyword.value = q.q;
    if (typeof q.status === 'string' && q.status) selectedStatus.value = q.status;
    if (typeof q.provider === 'string' && q.provider) selectedProvider.value = q.provider;
    if (typeof q.org === 'string' && q.org) selectedTaxonomy.value = q.org;
    if (typeof q.creator === 'string' && q.creator) selectedCreator.value = q.creator;
    const urlPage = Number(q.page);
    if (Number.isInteger(urlPage) && urlPage > 1) page.value = urlPage;
    const urlSize = Number(q.size);
    if ([10, 20, 50].includes(urlSize)) rowsPerPage.value = urlSize;
  }

  function syncListQueryToUrl() {
    const query: LocationQueryRaw = {};
    if (keyword.value) query.q = keyword.value;
    if (selectedStatus.value) query.status = selectedStatus.value;
    if (selectedProvider.value) query.provider = selectedProvider.value;
    if (selectedTaxonomy.value) query.org = selectedTaxonomy.value;
    if (selectedCreator.value) query.creator = selectedCreator.value;
    if (page.value > 1) query.page = String(page.value);
    if (rowsPerPage.value !== 20) query.size = String(rowsPerPage.value);
    void router.replace({ query });
  }

  const isDark = computed(() => $q.dark.isActive);
  const loading = listLoading;

  const createOpen = ref(false);
  const migrationOpen = ref(false);
  const deleteOpen = ref(false);
  const deleteTarget = ref<Agent | null>(null);
  const creating = ref(false);
  const selfEvolve = ref(true);
  const agentKind = ref<AgentKind>('llm');
  const a2aProxy = reactive<A2AProxyConfig>({
    remote_url: '',
    enable_streaming: true,
    timeout_seconds: 30,
  });
  const viewMode = ref<ViewMode>((localStorage.getItem(LS_VIEW) as ViewMode) || 'grid');

  const form = reactive<CreateAgentForm>({
    agent_key: '',
    display_name: '',
    provider: 'openrouter',
    model: 'gpt-4.1-mini',
    icon: 'smart_toy',
    agent_description: '',
    position_key: '',
    agent_variant: '',
    variant_description: '',
    taxonomy_position_id: '',
  });

  const selectedTemplateKey = ref('');
  const createTemplates = ref<AgentTemplatePreset[]>([]);
  const agentKeyServerError = ref('');
  const displayNameError = ref('');
  const providerModelError = ref('');
  const remoteUrlError = ref('');
  const createFormError = ref('');
  let agentKeyCheckTimer: ReturnType<typeof setTimeout> | undefined;

  const avatars = computed(() => avatarCatalog.agentsCatalog);

  const modelOptions = computed(() =>
    providerModels.value
      .filter((row: PlatformResource) => row.provider === form.provider)
      .map((row: PlatformResource) => ({ label: row.name, value: row.model })),
  );

  const agentKeyError = computed(() => {
    if (!form.agent_key) return '';
    if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(form.agent_key)) {
      return '仅支持小写字母、数字、连字符';
    }
    return agentKeyServerError.value;
  });
  const isA2AProxyCreate = computed(() => agentKind.value === 'a2a_proxy');
  const canCreate = computed(() => {
    const base = Boolean(form.display_name && form.agent_key && !agentKeyError.value);
    if (isA2AProxyCreate.value) {
      return base && Boolean(a2aProxy.remote_url.trim());
    }
    return base && Boolean(form.provider && form.model && modelCheckPassed.value);
  });

  const createDisabledHint = computed(() => {
    if (canCreate.value) return '';
    if (!form.display_name || !form.agent_key) return t('agentsPage.create.hintDisplayNameAndKey');
    if (agentKeyError.value) return agentKeyError.value;
    if (isA2AProxyCreate.value) return t('agentsPage.create.hintA2aUrl');
    if (!modelCheckPassed.value) return t('agentsPage.create.hintCheckModel');
    return '';
  });

  async function runLoadList() {
    try {
      await pageStore.loadAgentList();
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : 'Agent 列表加载失败' });
    }
  }

  watch(viewMode, (value) => localStorage.setItem(LS_VIEW, value));
  watch(rowsPerPage, () => {
    page.value = 1;
    syncListQueryToUrl();
    void runLoadList();
  });
  watch(page, () => {
    syncListQueryToUrl();
    void runLoadList();
  });
  watch([keyword, selectedStatus, selectedProvider, selectedTaxonomy, selectedCreator], () => {
    page.value = 1;
    syncListQueryToUrl();
    void runLoadList();
  });
  watch(
    () => form.provider,
    () => {
      form.model = modelOptions.value[0]?.value ?? '';
      modelCheckPassed.value = false;
    },
  );
  watch(
    () => form.model,
    () => {
      modelCheckPassed.value = false;
    },
  );

  watch(
    () => form.agent_key,
    (key) => {
      agentKeyServerError.value = '';
      if (agentKeyCheckTimer) clearTimeout(agentKeyCheckTimer);
      const trimmed = key.trim();
      if (!trimmed || !/^[a-z0-9]+(-[a-z0-9]+)*$/.test(trimmed)) return;
      agentKeyCheckTimer = setTimeout(() => {
        void pageStore
          .verifyAgentKey(trimmed)
          .then((res) => {
            if (!res.available) {
              agentKeyServerError.value = res.message === 'agent_key already in use' ? '标识已被使用' : res.message;
            }
          })
          .catch(() => {});
      }, 500);
    },
  );

  onMounted(async () => {
    try {
      const remote = await pageStore.fetchAgentTemplates();
      createTemplates.value = remote;
    } catch {
      // intentional empty — template fetch is non-critical
    }
    try {
      await Promise.all([runLoadList(), pageStore.loadAgentsDependencies()]);
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '依赖数据加载失败' });
    }
    const avatarRows = avatarCatalog.agentsCatalog;
    if (!form.icon || form.icon === 'smart_toy') {
      form.icon = avatarRows[0]?.id ?? 'smart_toy';
    }
  });

  function clearCreateFieldErrors() {
    agentKeyServerError.value = '';
    displayNameError.value = '';
    providerModelError.value = '';
    remoteUrlError.value = '';
    createFormError.value = '';
  }

  async function openCreate() {
    clearCreateFieldErrors();
    modelCheckPassed.value = false;
    try {
      await pageStore.loadAgentsDependencies();
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '依赖数据加载失败' });
      return;
    }
    const avatarRows = avatarCatalog.agentsCatalog;
    if (!form.icon || form.icon === 'smart_toy') {
      form.icon = avatarRows[0]?.id ?? 'smart_toy';
    }
    createOpen.value = true;
  }

  async function checkModel() {
    try {
      const result = await pageStore.validateCreateModel(form.provider, form.model);
      $q.notify({ type: result.ok ? 'positive' : 'negative', message: result.message });
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '校验失败' });
    }
  }

  function resetForm() {
    Object.assign(form, {
      agent_key: '',
      display_name: '',
      provider: 'openrouter',
      model: 'gpt-4.1-mini',
      icon: avatars.value[0]?.id ?? 'smart_toy',
      agent_description: '',
      position_key: '',
      agent_variant: '',
      variant_description: '',
      taxonomy_position_id: '',
    });
    selectedTemplateKey.value = '';
    clearCreateFieldErrors();
    modelCheckPassed.value = false;
    selfEvolve.value = true;
    agentKind.value = 'llm';
    Object.assign(a2aProxy, { remote_url: '', enable_streaming: true, timeout_seconds: 30 });
  }

  async function onCreate() {
    if (!canCreate.value) return;
    clearCreateFieldErrors();
    creating.value = true;
    try {
      const payload: Parameters<typeof appStore.addAgent>[0] = {
        ...form,
        agent_kind: agentKind.value,
        config_json: JSON.stringify({
          self_evolve: selfEvolve.value,
          description_template_key: selectedTemplateKey.value,
        }),
      };
      if (isA2AProxyCreate.value) {
        payload.provider = 'a2a';
        payload.model = 'proxy';
        payload.a2a_proxy_config = { ...a2aProxy };
      }
      await appStore.addAgent(payload);
      pageStore.resetListFiltersAfterCreate();
      await runLoadList();
      resetForm();
      createOpen.value = false;
      $q.notify({ type: 'positive', message: '创建成功' });
    } catch (error) {
      const fields = mapAgentCreateFieldErrors(parseKratosApiError(error));
      if (fields.agent_key) agentKeyServerError.value = fields.agent_key;
      if (fields.display_name) displayNameError.value = fields.display_name;
      if (fields.provider || fields.model) {
        providerModelError.value = fields.provider || fields.model || '';
      }
      if (fields.remote_url) remoteUrlError.value = fields.remote_url;
      if (fields.form) createFormError.value = fields.form;
      const hasInline = Boolean(
        fields.agent_key || fields.display_name || fields.provider || fields.model || fields.remote_url,
      );
      if (!hasInline && fields.form) {
        $q.notify({ type: 'negative', message: fields.form });
      }
    } finally {
      creating.value = false;
    }
  }

  async function duplicateListedAgent(agent: Agent) {
    try {
      const created = await pageStore.copyAgent(agent.id);
      appStore.upsertAgent(created);
      await runLoadList();
      $q.notify({ type: 'positive', message: 'Agent 已复制' });
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '复制失败' });
    }
  }

  function applyTemplate(template: AgentTemplatePreset) {
    selectedTemplateKey.value = template.key;
    form.display_name = template.display_name?.trim() || template.label;
    if (!isA2AProxyCreate.value) {
      if (template.provider?.trim()) {
        form.provider = template.provider.trim();
      }
      if (template.model?.trim()) {
        form.model = template.model.trim();
        modelCheckPassed.value = false;
      }
    }
    const text = template.description?.trim() || '';
    if (!text) return;
    form.agent_description = text;
  }

  function confirmDelete(agent: Agent) {
    deleteTarget.value = agent;
    deleteOpen.value = true;
  }

  const batchDeleteOpen = ref(false);
  const batchBusy = ref(false);

  function toggleAgentSelected(id: string) {
    pageStore.toggleAgentSelected(id);
  }

  function clearAgentSelection() {
    pageStore.clearAgentSelection();
  }

  function requestBatchDelete() {
    if (selectedAgentIds.value.length === 0) return;
    batchDeleteOpen.value = true;
  }

  async function runBatchSetStatus(status: 'active' | 'inactive') {
    if (batchBusy.value) return;
    batchBusy.value = true;
    try {
      const affected = await pageStore.batchUpdateListedAgents({ status });
      $q.notify({ type: 'positive', message: t('agentsPage.batch.statusOk', { n: affected }) });
    } catch (error) {
      $q.notify({
        type: 'negative',
        message: error instanceof Error ? error.message : t('agentsPage.batch.failed'),
      });
    } finally {
      batchBusy.value = false;
    }
  }

  async function runBatchDelete() {
    batchDeleteOpen.value = false;
    if (batchBusy.value) return;
    batchBusy.value = true;
    try {
      const affected = await pageStore.batchUpdateListedAgents({ delete: true });
      $q.notify({ type: 'positive', message: t('agentsPage.batch.deleteOk', { n: affected }) });
    } catch (error) {
      $q.notify({
        type: 'negative',
        message: error instanceof Error ? error.message : t('agentsPage.batch.failed'),
      });
    } finally {
      batchBusy.value = false;
    }
  }

  async function deleteAgentTarget() {
    if (!deleteTarget.value) return;
    try {
      await pageStore.removeListedAgent(deleteTarget.value.id);
      deleteOpen.value = false;
      deleteTarget.value = null;
      $q.notify({ type: 'positive', message: '已删除' });
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '删除失败' });
    }
  }

  function isFavorite(id: string) {
    return agents.value.find((agent: Agent) => agent.id === id)?.is_favorite ?? false;
  }

  async function toggleFavorite(id: string) {
    try {
      await pageStore.toggleAgentFavorite(id);
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : '收藏保存失败' });
    }
  }

  async function copyAgentKey(value: string) {
    await copyToClipboard(value);
    $q.notify({ type: 'positive', message: 'Agent 标识已复制' });
  }

  // Drag reorder is disabled in the list UI (no Agent.sort_order persistence / ReorderAgents RPC).
  function onReorder(_ids: string[]) {
    /* intentionally no-op */
  }

  return {
    isDark,
    agents,
    keyword,
    selectedStatus,
    selectedProvider,
    selectedTaxonomy,
    selectedCreator,
    creatorOptions,
    page,
    rowsPerPage,
    total,
    loading,
    createOpen,
    migrationOpen,
    deleteOpen,
    deleteTarget,
    creating,
    checkingModel,
    modelCheckPassed,
    selfEvolve,
    agentKind,
    a2aProxy,
    isA2AProxyCreate,
    viewMode,
    form,
    selectedTemplateKey,
    createTemplates,
    avatars,
    providerOptions,
    modelOptions,
    taxonomyTree,
    pageMax,
    tableColumns,
    agentKeyError,
    displayNameError,
    providerModelError,
    remoteUrlError,
    createFormError,
    canCreate,
    createDisabledHint,
    statusOptions,
    taxonomyLabel,
    isFavorite,
    toggleFavorite,
    copyAgentKey,
    openCreate,
    checkModel,
    onCreate,
    applyTemplate,
    confirmDelete,
    deleteAgentTarget,
    duplicateListedAgent,
    onReorder,
    selectedAgentIds,
    toggleAgentSelected,
    clearAgentSelection,
    batchDeleteOpen,
    batchBusy,
    requestBatchDelete,
    runBatchSetStatus,
    runBatchDelete,
  };
}
