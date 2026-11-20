import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { storeToRefs } from 'pinia';
import {
  catalogLabelForType,
  channelConfig,
  channelMetadata,
  channelType,
  copyChannelWebhookURL,
  isChannelConnected,
} from '../../components/channels/channelUi';
import { useChannelsStore } from '../../stores/channels';
import { confirm } from '../../composables/useAppConfirm';
import type { ChannelCredential, ChannelRow } from './types';
import { isForbiddenApiError } from '../../utils/kratosError';
import type { CapTone } from '../../components/capability/capTypes';

/** 通道在状态条上归哪一格：在线 / 异常 / 未连上 / 已停用 */
export type ChannelHealth = 'online' | 'alert' | 'pending' | 'off';

export function channelHealth(row: ChannelRow): ChannelHealth {
  if (!row.enabled) return 'off';
  if (isChannelConnected(row)) return 'online';
  if (row.status === 'error' || channelMetadata(row).last_error_message) return 'alert';
  return 'pending';
}

export const CHANNEL_HEALTH_TONE: Record<ChannelHealth, CapTone> = {
  online: 'ok',
  alert: 'alert',
  pending: 'watch',
  off: 'off',
};

export function useChannelsPage() {
  const { t } = useI18n();
  const $q = useQuasar();
  const channelsStore = useChannelsStore();
  const { channels: rows, catalog, loading, routingAgents, routingTeams } = storeToRefs(channelsStore);

  const error = ref('');
  /** 列表接口 403：页面据此渲染 forbidden 态（无重试）。 */
  const forbidden = ref(false);
  const loaded = ref(false);
  const search = ref('');
  const typeFilter = ref('');
  const stateFilter = ref('');
  const page = ref(1);
  const pageSize = ref(24);
  const togglingId = ref('');
  const testingId = ref('');
  const editorOpen = ref(false);
  const editingRow = ref<ChannelRow | null>(null);
  const editingCredentials = ref<ChannelCredential[]>([]);
  const createType = ref('');
  const inspectedId = ref('');

  // shortcut: 一个工作区的通道通常十来个，整表读回前端筛选，状态条的数才准；上百个时改回服务端分页 + 计数接口
  const filteredRows = computed(() => {
    const q = search.value.trim().toLowerCase();
    return rows.value.filter((row) => {
      if (typeFilter.value && channelType(row) !== typeFilter.value) return false;
      if (stateFilter.value && channelHealth(row) !== stateFilter.value) return false;
      return !q || row.name.toLowerCase().includes(q) || row.key.toLowerCase().includes(q);
    });
  });
  const total = computed(() => filteredRows.value.length);
  const pageMax = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
  const pagedRows = computed(() =>
    filteredRows.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value),
  );

  const healthCounts = computed(() => {
    const counts: Record<ChannelHealth, number> = { online: 0, alert: 0, pending: 0, off: 0 };
    for (const row of rows.value) counts[channelHealth(row)] += 1;
    return counts;
  });

  const typeOptions = computed(() => [
    { label: t('connect.channels.allPlatforms'), value: '' },
    ...catalog.value
      .filter((item) => rows.value.some((row) => channelType(row) === item.type))
      .map((item) => ({ label: item.label, value: item.type })),
  ]);

  const inspected = computed(() => rows.value.find((row) => row.id === inspectedId.value) ?? null);
  const inspectorOpen = computed({
    get: () => inspected.value !== null,
    set: (open: boolean) => {
      if (!open) inspectedId.value = '';
    },
  });

  watch([search, typeFilter, stateFilter, pageSize], () => {
    page.value = 1;
  });
  watch(pageMax, (max) => {
    if (page.value > max) page.value = max;
  });

  async function loadAll() {
    error.value = '';
    forbidden.value = false;
    try {
      await channelsStore.loadAll();
      loaded.value = true;
    } catch (err) {
      forbidden.value = isForbiddenApiError(err);
      error.value = err instanceof Error ? err.message : t('channelsPage.loadFailed');
    }
    // 「由谁回答」要 Agent / 团队名册；读不到只影响显示名，不影响列表
    if (!routingAgents.value.length && !routingTeams.value.length) {
      void channelsStore.loadRoutingOptions().catch(() => undefined);
    }
  }

  function platformLabel(row: ChannelRow): string {
    return catalogLabelForType(catalog.value, channelType(row));
  }

  function supportsTest(row: ChannelRow): boolean {
    return catalog.value.find((item) => item.type === channelType(row))?.supports_test ?? false;
  }

  /** 「由谁回答」：默认路由目标 + 规则条数（路由在 config_json.routing，见 biz/channel_routing.go） */
  function answeredBy(row: ChannelRow): { label: string; rules: number } {
    const routing = channelConfig(row).routing ?? {};
    const rules = Array.isArray(routing.rules) ? routing.rules.length : 0;
    const teamId = String(routing.default_team_id ?? '').trim();
    if (teamId) {
      const team = routingTeams.value.find((item) => item.id === teamId || item.team_key === teamId);
      return { label: t('connect.channels.teamPrefix', { name: team?.display_name || teamId }), rules };
    }
    const agentId = String(routing.default_agent_id ?? '').trim();
    if (agentId) {
      const agent = routingAgents.value.find((item) => item.id === agentId || item.agent_key === agentId);
      return {
        label: t('connect.channels.agentPrefix', { name: agent?.display_name || agent?.agent_key || agentId }),
        rules,
      };
    }
    return { label: '', rules };
  }

  function openCreate(type = '') {
    editingRow.value = null;
    editingCredentials.value = [];
    createType.value = type;
    editorOpen.value = true;
  }

  async function openEdit(row: ChannelRow) {
    try {
      editingCredentials.value = await channelsStore.fetchCredentials(row.id);
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('channelsPage.loadFailed') });
      return;
    }
    editingRow.value = row;
    editorOpen.value = true;
  }

  function inspect(row: ChannelRow) {
    inspectedId.value = row.id;
  }

  function onSaved(row: ChannelRow) {
    inspectedId.value = row.id;
    void loadAll();
  }

  async function toggleRow(row: ChannelRow, enabled: boolean) {
    togglingId.value = row.id;
    try {
      await channelsStore.toggle(row.id, enabled);
      $q.notify({
        type: 'positive',
        message: enabled ? t('channelsPage.toggleOkEnabled') : t('channelsPage.toggleOkDisabled'),
      });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('channelsPage.toggleFailed') });
    } finally {
      togglingId.value = '';
    }
  }

  async function testRow(row: ChannelRow) {
    testingId.value = row.id;
    try {
      const result = await channelsStore.testConnection(row.id);
      $q.notify({ type: result.ok ? 'positive' : 'warning', message: result.message || result.status });
      await loadAll();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('channelsPage.testFailed') });
    } finally {
      testingId.value = '';
    }
  }

  async function copyWebhook(row: ChannelRow) {
    try {
      const url = await copyChannelWebhookURL(row);
      $q.notify({ type: 'positive', message: t('channelsPage.copyWebhookOk', { url }) });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('channelsPage.copyWebhookFailed'),
      });
    }
  }

  async function confirmDelete(row: ChannelRow) {
    const ok = await confirm({
      title: t('channelsPage.deleteTitle'),
      message: t('connect.channels.deleteMessage', { name: row.name, platform: platformLabel(row) }),
      tone: 'danger',
    });
    if (!ok) return;
    try {
      await channelsStore.removeChannel(row.id);
      if (inspectedId.value === row.id) inspectedId.value = '';
      $q.notify({ type: 'positive', message: t('channelsPage.deleteOk') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('channelsPage.deleteFailed') });
    }
  }

  void loadAll();

  return {
    t,
    rows,
    catalog,
    loaded,
    loading,
    error,
    forbidden,
    search,
    typeFilter,
    stateFilter,
    typeOptions,
    healthCounts,
    page,
    pageSize,
    pageMax,
    total,
    pagedRows,
    togglingId,
    testingId,
    editorOpen,
    editingRow,
    editingCredentials,
    createType,
    inspected,
    inspectorOpen,
    platformLabel,
    supportsTest,
    answeredBy,
    loadAll,
    openCreate,
    openEdit,
    inspect,
    onSaved,
    toggleRow,
    testRow,
    copyWebhook,
    confirmDelete,
  };
}
