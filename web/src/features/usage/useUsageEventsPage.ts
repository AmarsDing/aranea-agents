import { computed, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useUsageStore } from '../../stores/usage';
import { usePlatformStore } from '../../stores/platform';
import { useAgentsCatalogStore } from '../../stores/agents/catalog';
import { useTeamsStore } from '../../stores/teams';
import type { ModelUsageQuery } from './types';
import { formatUsdFromMicro } from './moneyFormat';
import { downloadBlob } from '../../composables/useFileDownload';
import { USAGE_EVENTS_PAGE_SIZE_DEFAULT } from '../constants/queryLimits';
import { appStatusMeta } from '../ui/appStatusMeta';

/** Default retention when purging; independent of the view filter range. */
export const DEFAULT_PURGE_RETAIN_DAYS = 30;

const PURGE_RETAIN_DAY_OPTIONS = [
  { label: '保留 7 天', value: 7 },
  { label: '保留 30 天', value: 30 },
  { label: '保留 90 天', value: 90 },
  { label: '保留 365 天', value: 365 },
];

/** 用量事件状态枚举（label 复用 AppStatusChip 的 i18n 元数据） */
const USAGE_EVENT_STATUSES = ['success', 'error', 'failed', 'timeout', 'cancelled'];

type FilterOption = { label: string; value: string };

export function useUsageEventsPage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const usageStore = useUsageStore();
  const platformStore = usePlatformStore();
  const agentsCatalog = useAgentsCatalogStore();
  const teamsStore = useTeamsStore();
  const { events, eventsTotal, eventsLoading, eventsError, exporting } = storeToRefs(usageStore);
  const page = ref(1);
  const pageSize = ref(USAGE_EVENTS_PAGE_SIZE_DEFAULT);
  const pageMax = computed(() => Math.max(1, Math.ceil(Math.max(0, eventsTotal.value) / pageSize.value)));
  const filters = ref<Omit<ModelUsageQuery, 'limit' | 'offset'>>({ range: '7d' });
  /** Purge retention is independent of the list filter `range` (e.g. 24h must not mean 24 days). */
  const retainDays = ref(DEFAULT_PURGE_RETAIN_DAYS);

  const rangeOptions = [
    { label: '24h', value: '24h' },
    { label: '7d', value: '7d' },
    { label: '30d', value: '30d' },
  ];
  const statusOptions = USAGE_EVENT_STATUSES.map((v) => ({
    label: t(appStatusMeta(v)?.labelKey ?? 'common.status.unknown'),
    value: v,
  }));
  const usageKindOptions = [
    { label: '全部', value: '' },
    { label: 'Chat Turn', value: 'chat_turn' },
    { label: 'Team 成员', value: 'team_member' },
    { label: 'Team 整轮', value: 'team_turn' },
  ];

  // ── 筛选下拉选项（显示名称、提交 ID/code；数据源为现有目录 Store，无新增 API） ──
  const agentOptions = ref<FilterOption[]>([]);
  const teamOptions = ref<FilterOption[]>([]);

  const providerOptions = computed<FilterOption[]>(() => {
    const codes = new Set<string>();
    for (const row of platformStore.providerModels) {
      if (row.provider) codes.add(row.provider);
    }
    return Array.from(codes)
      .sort((a, b) => a.localeCompare(b))
      .map((p) => ({ label: p, value: p }));
  });

  /** 模型选项随 Provider 联动；未选 Provider 时列出全部模型 */
  const modelOptions = computed<FilterOption[]>(() => {
    const provider = filters.value.provider_code?.trim() ?? '';
    const seen = new Map<string, string>();
    for (const row of platformStore.providerModels) {
      if (!row.model || (provider && row.provider !== provider)) continue;
      if (!seen.has(row.model)) {
        seen.set(row.model, row.name || row.model);
      }
    }
    return Array.from(seen.entries())
      .map(([value, label]) => ({ label, value }))
      .sort((a, b) => a.label.localeCompare(b.label));
  });

  async function loadFilterOptions() {
    const tasks: Promise<void>[] = [];
    if (platformStore.providerModels.length === 0) {
      tasks.push(
        platformStore.loadProviderModels().catch(() => {
          /* 选项加载失败不阻塞列表 */
        }),
      );
    }
    tasks.push(
      agentsCatalog
        .fetchAgents({ limit: 200 })
        .then((agents) => {
          agentOptions.value = agents.map((a) => ({
            label: a.display_name || a.agent_key || a.id,
            value: a.id,
          }));
        })
        .catch(() => {
          /* 选项加载失败不阻塞列表 */
        }),
    );
    tasks.push(
      teamsStore
        .loadTeams()
        .then(() => {
          teamOptions.value = teamsStore.teams.map((t) => ({
            label: t.display_name || t.team_key || t.id,
            value: t.id,
          }));
        })
        .catch(() => {
          /* 选项加载失败不阻塞列表 */
        }),
    );
    await Promise.all(tasks);
  }

  /** Provider 变化时，若已选模型不属于新 Provider 则清空模型筛选 */
  function onProviderFilterChange() {
    const model = filters.value.model_api_id?.trim();
    if (model && !modelOptions.value.some((o) => o.value === model)) {
      filters.value.model_api_id = undefined;
    }
  }

  /** 规范化筛选（q-select clear 产生 null，统一转 undefined，避免请求体出现 null） */
  function filterQuery(): Omit<ModelUsageQuery, 'limit' | 'offset'> {
    const f = filters.value;
    return {
      range: f.range,
      provider_code: f.provider_code || undefined,
      model_api_id: f.model_api_id || undefined,
      agent_id: f.agent_id || undefined,
      team_id: f.team_id || undefined,
      usage_kind: f.usage_kind || undefined,
      status: f.status || undefined,
    };
  }

  function buildQuery(): ModelUsageQuery {
    return {
      ...filterQuery(),
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value,
    };
  }

  async function load() {
    const result = await usageStore.loadEvents(buildQuery());
    if (page.value > pageMax.value) {
      page.value = pageMax.value;
      await usageStore.loadEvents(buildQuery());
    }
    return result;
  }

  function onPage(next: number) {
    if (next === page.value) return;
    page.value = next;
    void load();
  }

  function onPageSize(next: number) {
    pageSize.value = next;
    page.value = 1;
    void load();
  }

  async function exportCsv() {
    // Export uses the same filters but the dedicated export RPC (higher row cap).
    const csv = await usageStore.exportEventsCsv(filterQuery());
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
    downloadBlob(blob, `usage-events-${new Date().toISOString().slice(0, 10)}.csv`);
  }

  async function purgeEvents() {
    const deleted = await usageStore.purgeEvents(retainDays.value);
    page.value = 1;
    await load();
    return deleted;
  }

  function formatMoney(value?: number) {
    return formatUsdFromMicro(value);
  }

  /** 延迟列：原始毫秒格式化为可读时长（如 4m27s / 267.0s / 320ms） */
  function formatLatency(ms?: number) {
    if (ms == null || Number.isNaN(ms)) return '—';
    if (ms < 1000) return `${Math.round(ms)}ms`;
    const s = ms / 1000;
    if (s < 60) return `${s.toFixed(1)}s`;
    const m = Math.floor(s / 60);
    return `${m}m${Math.round(s % 60)}s`;
  }

  function truncate(msg?: string, max = 80) {
    const s = (msg ?? '').trim();
    if (s.length <= max) return s || '—';
    return `${s.slice(0, max)}…`;
  }

  const purging = ref(false);

  /** 保留天数选择收进确认 Dialog（radio），不再占用工具栏坑位 */
  function onPurgeConfirm() {
    $q.dialog({
      class: 'app-dialog-card app-dialog-card--sm',
      title: '删除历史用量事件',
      message: '将删除早于所选保留天数的全部用量事件（与列表「范围」筛选无关），此操作不可撤销。',
      options: {
        type: 'radio',
        model: retainDays.value,
        items: PURGE_RETAIN_DAY_OPTIONS,
      },
      cancel: { label: '取消', flat: true, rounded: true, noCaps: true },
      ok: { label: '确认删除', color: 'negative', flat: true, rounded: true, noCaps: true },
      persistent: true,
    }).onOk(async (days) => {
      retainDays.value = Number(days) || DEFAULT_PURGE_RETAIN_DAYS;
      purging.value = true;
      try {
        const deleted = await purgeEvents();
        $q.notify({ type: 'positive', message: `已删除 ${deleted} 条用量事件` });
      } catch {
        $q.notify({ type: 'negative', message: '删除用量事件失败' });
      } finally {
        purging.value = false;
      }
    });
  }

  function resetFilters() {
    filters.value = { range: '7d' };
    page.value = 1;
    void load();
  }

  return {
    events,
    eventsTotal,
    page,
    pageSize,
    pageMax,
    onPage,
    onPageSize,
    loading: eventsLoading,
    error: eventsError,
    exporting,
    filters,
    rangeOptions,
    statusOptions,
    usageKindOptions,
    providerOptions,
    modelOptions,
    agentOptions,
    teamOptions,
    loadFilterOptions,
    onProviderFilterChange,
    purging,
    load,
    exportCsv,
    purgeEvents,
    onPurgeConfirm,
    resetFilters,
    formatMoney,
    formatLatency,
    truncate,
  };
}
