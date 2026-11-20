import { storeToRefs } from 'pinia';
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router';
import { copyToClipboard, useQuasar } from 'quasar';
import { mapAgentCreateFieldErrors, parseKratosApiError } from '../../utils/kratosError';
import type { Agent, AgentKind, A2AProxyConfig, AgentTemplatePreset } from './types';
import type { PlatformResource, PlatformResourceTreeNode } from '../platform/types';
import { ROSTER_LIMIT, useAgentsPageStore } from '../../stores/agents';
import { useAppStore } from '../../stores/app';
import { useAvatarCatalogStore } from '../../stores/avatar';
import {
  buildOrgIndex,
  buildRail,
  canDeleteAgent,
  canToggleAgent,
  countByRole,
  groupRoster,
  rosterRole,
  scopeRoster,
  vacancyAgentKey,
  type RosterRole,
  type RosterScope,
  type RosterStatus,
} from './roster';

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

const AGENT_KEY_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
const DEFAULT_PROVIDER = 'deepseek';
const DEFAULT_MODEL = 'deepseek-v4-flash';
const ROLES: Array<RosterRole | ''> = ['', 'company_lead', 'dept_lead', 'specialist', 'custom', 'butler'];
const STATUSES: RosterStatus[] = ['', 'active', 'inactive', 'failed'];

/** 编制花名册（/agents）：左编制树 · 中花名册 · 右检视。数据走 useAgentsPageStore，口径在 roster.ts。 */
export function useAgentsPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const route = useRoute();
  const router = useRouter();
  const appStore = useAppStore();
  const avatarCatalog = useAvatarCatalogStore();
  const pageStore = useAgentsPageStore();
  const { agents, total, listLoading, providerModels, checkingModel, modelCheckPassed, taxonomyTree, providerOptions } =
    storeToRefs(pageStore);

  // ——— 筛选与选中：同步到地址栏，刷新和分享链接都能还原 ———
  const qs = (k: string) => (typeof route.query[k] === 'string' ? (route.query[k] as string) : '');
  const scope = ref<RosterScope>(qs('scope') || 'all');
  const keyword = ref(qs('q'));
  const role = ref<RosterRole | ''>(ROLES.includes(qs('role') as RosterRole) ? (qs('role') as RosterRole) : '');
  const status = ref<RosterStatus>(
    STATUSES.includes(qs('status') as RosterStatus) ? (qs('status') as RosterStatus) : '',
  );
  const selectedId = ref(qs('agent'));

  watch([scope, keyword, role, status, selectedId], () => {
    const query: LocationQueryRaw = {};
    if (scope.value !== 'all') query.scope = scope.value;
    if (keyword.value) query.q = keyword.value;
    if (role.value) query.role = role.value;
    if (status.value) query.status = status.value;
    if (selectedId.value) query.agent = selectedId.value;
    void router.replace({ query });
  });

  const orgIndex = computed(() => buildOrgIndex(taxonomyTree.value));
  const rail = computed(() => buildRail(orgIndex.value, agents.value));
  const scoped = computed(() =>
    scopeRoster(orgIndex.value, agents.value, { scope: scope.value, q: keyword.value ?? '', status: status.value }),
  );
  const roleCounts = computed(() => countByRole(scoped.value));
  const visible = computed(() =>
    role.value ? scoped.value.filter((a) => rosterRole(a) === role.value) : scoped.value,
  );
  const groups = computed(() => groupRoster(orgIndex.value, visible.value));
  const statusCounts = computed(() => {
    const base = scopeRoster(orgIndex.value, agents.value, { scope: scope.value, q: keyword.value ?? '', status: '' });
    return {
      inactive: base.filter((a) => a.status === 'inactive').length,
      failed: base.filter((a) => a.last_run_status === 'failed').length,
    };
  });
  const truncated = computed(() => total.value > agents.value.length);

  const selectedAgent = computed(() => agents.value.find((a) => a.id === selectedId.value) ?? null);
  const scopeNode = computed(() => orgIndex.value.nodes.get(scope.value) ?? null);

  function selectAgent(agent: Agent | null) {
    selectedId.value = agent?.id ?? '';
  }

  function setScope(next: RosterScope) {
    scope.value = next;
    selectedId.value = '';
  }

  function resetFilters() {
    keyword.value = '';
    role.value = '';
    status.value = '';
  }

  async function runLoadList() {
    try {
      await pageStore.loadAgentList();
    } catch {
      // 失败信息在 pageStore.listError，由页面错误面板呈现并可重试（不退化成空态）
    }
  }

  // ——— 选择与批量 ———
  const { selectedAgentIds } = storeToRefs(pageStore);
  const selectableVisible = computed(() => visible.value.filter(canToggleAgent));
  const allVisibleSelected = computed(
    () =>
      selectableVisible.value.length > 0 && selectableVisible.value.every((a) => selectedAgentIds.value.includes(a.id)),
  );
  const selectedAgents = computed(() => agents.value.filter((a) => selectedAgentIds.value.includes(a.id)));
  const deletableSelected = computed(() => selectedAgents.value.filter(canDeleteAgent));
  const batchBusy = ref('');
  const batchError = ref('');

  function toggleSelectAllVisible() {
    pageStore.setSelection(allVisibleSelected.value ? [] : selectableVisible.value.map((a) => a.id));
  }

  async function runBatch(action: 'enable' | 'disable' | 'delete') {
    if (batchBusy.value) return;
    batchBusy.value = action;
    batchError.value = '';
    try {
      if (action === 'delete') {
        // 后端会拒绝内置 / 行业包专项；只把能删的发过去，其余保留选中
        const ids = deletableSelected.value.map((a) => a.id);
        const n = await pageStore.batchUpdateListedAgents({ delete: true }, ids);
        $q.notify({ type: 'positive', message: t('agentRoster.batch.deleted', { n }) });
      } else {
        const n = await pageStore.batchUpdateListedAgents({ status: action === 'enable' ? 'active' : 'inactive' });
        $q.notify({ type: 'positive', message: t('agentRoster.batch.statusDone', { n }) });
      }
    } catch (error) {
      batchError.value = error instanceof Error ? error.message : t('agentRoster.batch.failed');
    } finally {
      batchBusy.value = '';
    }
  }

  // ——— 单个 Agent 的动作 ———
  async function setAgentStatus(agent: Agent, next: 'active' | 'inactive') {
    try {
      await pageStore.batchUpdateListedAgents({ status: next }, [agent.id]);
      $q.notify({ type: 'positive', message: t(next === 'active' ? 'agentRoster.enabled' : 'agentRoster.disabled') });
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : t('agentRoster.actionFailed') });
    }
  }

  async function toggleFavorite(agent: Agent) {
    try {
      await pageStore.toggleAgentFavorite(agent.id);
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : t('agentRoster.actionFailed') });
    }
  }

  async function copyAgentKey(value: string) {
    await copyToClipboard(value);
    $q.notify({ type: 'positive', message: t('agentRoster.keyCopied') });
  }

  async function duplicateListedAgent(agent: Agent) {
    try {
      const created = await pageStore.copyAgent(agent.id);
      appStore.upsertAgent(created);
      await runLoadList();
      selectedId.value = created.id;
      $q.notify({ type: 'positive', message: t('agentRoster.duplicated', { name: created.display_name }) });
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : t('agentRoster.actionFailed') });
    }
  }

  const deleteOpen = ref(false);
  const deleteTarget = ref<Agent | null>(null);

  function confirmDelete(agent: Agent) {
    deleteTarget.value = agent;
    deleteOpen.value = true;
  }

  async function deleteAgentTarget() {
    const target = deleteTarget.value;
    if (!target) return;
    try {
      await pageStore.removeListedAgent(target.id);
      if (selectedId.value === target.id) selectedId.value = '';
      deleteOpen.value = false;
      deleteTarget.value = null;
      $q.notify({ type: 'positive', message: t('agentRoster.deleted') });
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : t('agentRoster.actionFailed') });
    }
  }

  // ——— 新建 / 补人 ———
  const createOpen = ref(false);
  const creating = ref(false);
  // 与后端 DefaultAgentRuntimeSettings（agent_defaults.go）opt-in 语义一致：自进化默认关。
  const selfEvolve = ref(false);
  const agentKind = ref<AgentKind>('llm');
  const a2aProxy = reactive<A2AProxyConfig>({ remote_url: '', enable_streaming: true, timeout_seconds: 30 });
  const form = reactive<CreateAgentForm>(emptyForm());
  const selectedTemplateKey = ref('');
  const createTemplates = ref<AgentTemplatePreset[]>([]);
  const agentKeyServerError = ref('');
  const displayNameError = ref('');
  const providerModelError = ref('');
  const remoteUrlError = ref('');
  const createFormError = ref('');
  let agentKeyCheckTimer: ReturnType<typeof setTimeout> | undefined;

  function emptyForm(): CreateAgentForm {
    return {
      agent_key: '',
      display_name: '',
      provider: DEFAULT_PROVIDER,
      model: DEFAULT_MODEL,
      icon: avatarCatalog.agentsCatalog[0]?.id ?? 'smart_toy',
      agent_description: '',
      position_key: '',
      agent_variant: '',
      variant_description: '',
      taxonomy_position_id: '',
    };
  }

  const modelOptions = computed(() =>
    providerModels.value
      .filter((row: PlatformResource) => row.provider === form.provider)
      .map((row: PlatformResource) => ({ label: row.name, value: row.model })),
  );

  const agentKeyError = computed(() => {
    if (form.agent_key && !AGENT_KEY_RE.test(form.agent_key)) return t('agentRoster.createForm.keyPattern');
    return agentKeyServerError.value;
  });
  const isA2AProxyCreate = computed(() => agentKind.value === 'a2a_proxy');
  // 模型不再要求先点「检查」：创建时自动校验，不通过就在模型字段上就地报错
  const canCreate = computed(() => {
    const base = Boolean(form.display_name && form.agent_key && !agentKeyError.value);
    if (isA2AProxyCreate.value) return base && Boolean(a2aProxy.remote_url.trim());
    return base && Boolean(form.provider && form.model);
  });
  const createDisabledHint = computed(() => {
    if (canCreate.value) return '';
    if (!form.display_name || !form.agent_key) return t('agentsPage.create.hintDisplayNameAndKey');
    if (agentKeyError.value) return agentKeyError.value;
    if (isA2AProxyCreate.value) return t('agentsPage.create.hintA2aUrl');
    return t('agentRoster.createForm.pickModel');
  });

  watch(
    () => form.provider,
    () => {
      // 模板同时带来的 provider + model 若在目录里就保留，否则换成该 provider 的第一个模型
      if (!modelOptions.value.some((o) => o.value === form.model)) form.model = modelOptions.value[0]?.value ?? '';
      modelCheckPassed.value = false;
      providerModelError.value = '';
    },
  );
  watch(
    () => form.model,
    () => {
      modelCheckPassed.value = false;
      providerModelError.value = '';
    },
  );
  watch(
    () => form.agent_key,
    (key) => {
      agentKeyServerError.value = '';
      if (agentKeyCheckTimer) clearTimeout(agentKeyCheckTimer);
      const trimmed = key.trim();
      if (!trimmed || !AGENT_KEY_RE.test(trimmed)) return;
      agentKeyCheckTimer = setTimeout(() => {
        void pageStore
          .verifyAgentKey(trimmed)
          .then((res) => {
            if (!res.available) agentKeyServerError.value = t('agentRoster.createForm.keyTaken');
          })
          .catch(() => {});
      }, 500);
    },
  );

  function clearCreateFieldErrors() {
    agentKeyServerError.value = '';
    displayNameError.value = '';
    providerModelError.value = '';
    remoteUrlError.value = '';
    createFormError.value = '';
  }

  function resetForm() {
    Object.assign(form, emptyForm());
    selectedTemplateKey.value = '';
    clearCreateFieldErrors();
    modelCheckPassed.value = false;
    selfEvolve.value = false;
    agentKind.value = 'llm';
    Object.assign(a2aProxy, { remote_url: '', enable_streaming: true, timeout_seconds: 30 });
  }

  /** 新建；从缺编岗位进来时预填岗位、名称、描述与标识。 */
  async function openCreate(position?: PlatformResourceTreeNode) {
    resetForm();
    try {
      await pageStore.loadAgentsDependencies();
    } catch (error) {
      $q.notify({
        type: 'negative',
        message: error instanceof Error ? error.message : t('agentRoster.loadDepsFailed'),
      });
      return;
    }
    // 默认模型不在目录里（被删 / 停用）时退到目录第一个
    if (!providerOptions.value.some((o) => o.value === form.provider)) {
      form.provider = providerOptions.value[0]?.value ?? '';
      form.model = modelOptions.value[0]?.value ?? '';
    }
    if (position) {
      form.taxonomy_position_id = position.id;
      form.display_name = position.name;
      form.agent_description = position.description ?? '';
      form.agent_key = vacancyAgentKey(position);
    }
    createOpen.value = true;
  }

  async function checkModel() {
    try {
      const result = await pageStore.validateCreateModel(form.provider, form.model);
      providerModelError.value = result.ok ? '' : result.message;
      $q.notify({ type: result.ok ? 'positive' : 'negative', message: result.message });
    } catch (error) {
      $q.notify({ type: 'negative', message: error instanceof Error ? error.message : t('agentRoster.actionFailed') });
    }
  }

  /** 禁用态点击「创建」：必填空字段标红 + toast 说明原因。 */
  function onCreateAttempt() {
    if (canCreate.value) return;
    if (!form.display_name.trim()) displayNameError.value = t('agentsPage.create.requireDisplayName');
    if (!form.agent_key.trim()) agentKeyServerError.value = t('agentsPage.create.requireAgentKey');
    if (isA2AProxyCreate.value && !a2aProxy.remote_url.trim()) remoteUrlError.value = t('agentsPage.create.hintA2aUrl');
    if (createDisabledHint.value) $q.notify({ type: 'warning', message: createDisabledHint.value });
  }

  async function onCreate() {
    if (!canCreate.value || creating.value) return;
    clearCreateFieldErrors();
    creating.value = true;
    try {
      if (!isA2AProxyCreate.value && !modelCheckPassed.value) {
        const check = await pageStore.validateCreateModel(form.provider, form.model);
        if (!check.ok) {
          providerModelError.value = check.message || t('agentRoster.createForm.modelUnavailable');
          return;
        }
      }
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
      const created = await appStore.addAgent(payload);
      await runLoadList();
      resetForm();
      createOpen.value = false;
      if (created?.id) selectedId.value = created.id;
      $q.notify({ type: 'positive', message: t('agentRoster.created') });
    } catch (error) {
      const fields = mapAgentCreateFieldErrors(parseKratosApiError(error));
      if (fields.agent_key) agentKeyServerError.value = fields.agent_key;
      if (fields.display_name) displayNameError.value = fields.display_name;
      if (fields.provider || fields.model) providerModelError.value = fields.provider || fields.model || '';
      if (fields.remote_url) remoteUrlError.value = fields.remote_url;
      if (fields.form) createFormError.value = fields.form;
      const hasInline = Boolean(
        fields.agent_key || fields.display_name || fields.provider || fields.model || fields.remote_url,
      );
      if (!hasInline && fields.form) $q.notify({ type: 'negative', message: fields.form });
    } finally {
      creating.value = false;
    }
  }

  function applyTemplate(template: AgentTemplatePreset) {
    selectedTemplateKey.value = template.key;
    form.display_name = template.display_name?.trim() || template.label;
    if (!isA2AProxyCreate.value) {
      if (template.provider?.trim()) form.provider = template.provider.trim();
      if (template.model?.trim()) form.model = template.model.trim();
    }
    const text = template.description?.trim() || '';
    if (text) form.agent_description = text;
  }

  onMounted(async () => {
    void pageStore
      .fetchAgentTemplates()
      .then((rows) => {
        createTemplates.value = rows;
      })
      .catch(() => {
        // 模板只是创建时的快捷填充，取不到不影响页面
      });
    try {
      await Promise.all([runLoadList(), pageStore.loadAgentsDependencies()]);
    } catch (error) {
      $q.notify({
        type: 'negative',
        message: error instanceof Error ? error.message : t('agentRoster.loadDepsFailed'),
      });
    }
  });

  return {
    // 数据
    agents,
    total,
    truncated,
    rosterLimit: ROSTER_LIMIT,
    loading: listLoading,
    listError: computed(() => pageStore.listError),
    taxonomyTree,
    orgIndex,
    rail,
    scoped,
    visible,
    groups,
    roleCounts,
    statusCounts,
    // 筛选与选中
    scope,
    scopeNode,
    keyword,
    role,
    status,
    selectedId,
    selectedAgent,
    selectAgent,
    setScope,
    resetFilters,
    runLoadList,
    // 选择与批量
    selectedAgentIds,
    selectedAgents,
    deletableSelected,
    allVisibleSelected,
    toggleSelectAllVisible,
    toggleAgentSelected: pageStore.toggleAgentSelected,
    clearAgentSelection: pageStore.clearAgentSelection,
    batchBusy,
    batchError,
    runBatch,
    // 单个动作
    setAgentStatus,
    toggleFavorite,
    copyAgentKey,
    duplicateListedAgent,
    deleteOpen,
    deleteTarget,
    confirmDelete,
    deleteAgentTarget,
    // 新建
    createOpen,
    creating,
    checkingModel,
    selfEvolve,
    agentKind,
    a2aProxy,
    isA2AProxyCreate,
    form,
    selectedTemplateKey,
    createTemplates,
    providerOptions,
    modelOptions,
    agentKeyError,
    displayNameError,
    providerModelError,
    remoteUrlError,
    createFormError,
    canCreate,
    createDisabledHint,
    openCreate,
    checkModel,
    onCreate,
    onCreateAttempt,
    applyTemplate,
  };
}
