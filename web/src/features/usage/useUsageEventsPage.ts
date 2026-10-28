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

const PURGE_RETAIN_DAYS = [7, 30, 90, 365];

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
  const statusOptions = computed(() =>
    USAGE_EVENT_STATUSES.map((v) => ({
      label: t(appStatusMeta(v)?.labelKey ?? 'common.status.unknown'),
      value: v,
    })),
  );
  // 与 biz/usage Kind* 枚举全量对齐（含 aux_* 辅助计量与 unknown 兜底）
  const usageKindOptions = computed(() => [
    { label: t('usageEventsPage.kindAll'), value: '' },
    { label: t('usageKinds.chat_turn'), value: 'chat_turn' },
    { label: t('usageKinds.team_member'), value: 'team_member' },
    { label: t('usageKinds.team_turn'), value: 'team_turn' },
    { label: t('usageKinds.aux_subagent'), value: 'aux_subagent' },
    { label: t('usageKinds.aux_title'), value: 'aux_title' },
    { label: t('usageKinds.aux_intent'), value: 'aux_intent' },
    { label: t('usageKinds.aux_evolution'), value: 'aux_evolution' },
    { label: t('usageKinds.aux_memory_extract'), value: 'aux_memory_extract' },
    { label: t('usageKinds.aux_embedding'), value: 'aux_embedding' },
    { label: t('usageKinds.aux_eval_judge'), value: 'aux_eval_judge' },
    { label: t('usageKinds.aux_planner_decompose'), value: 'aux_planner_decompose' },
    { label: t('usageKinds.aux_allocator_match'), value: 'aux_allocator_match' },
    { label: t('usageKinds.unknown'), value: 'unknown' },
  ]);
  // LBG-6：思考强度过滤（与 biz.ThinkingEffort* 枚举一致；值为技术枚举不翻译）
  const effortOptions = computed(() => [
    { label: t('usageEventsPage.effortAll'), value: '' },
    { label: 'off', value: 'off' },
    { label: 'low', value: 'low' },
    { label: 'medium', value: 'medium' },
    { label: 'high', value: 'high' },
    { label: 'max', value: 'max' },
  ]);

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
      effort: f.effort || undefined,
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
    try {
      const result = await usageStore.loadEvents(buildQuery());
      if (page.value > pageMax.value) {
        page.value = pageMax.value;
        await usageStore.loadEvents(buildQuery());
      }
      return result;
    } catch (reason) {
      eventsError.value = reason instanceof Error ? reason.message : t('uiSystem.operations.loadFailed');
      return undefined;
    }
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
    if (exporting.value) return;
    // Export uses the same filters but the dedicated export RPC (higher row cap).
    try {
      const csv = await usageStore.exportEventsCsv(filterQuery());
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
      downloadBlob(blob, `usage-events-${new Date().toISOString().slice(0, 10)}.csv`);
    } catch {
      $q.notify({ type: 'negative', message: t('usageEventsPage.exportFailed') });
    }
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
      title: t('usageEventsPage.purgeTitle'),
      message: t('usageEventsPage.purgeMessage'),
      options: {
        type: 'radio',
        model: String(retainDays.value),
        items: PURGE_RETAIN_DAYS.map((days) => ({
          label: t('usageEventsPage.keepDays', { days }),
          value: String(days),
        })),
      },
      cancel: { label: t('common.cancel'), flat: true, rounded: true, noCaps: true },
      ok: { label: t('usageEventsPage.purgeConfirm'), color: 'negative', flat: true, rounded: true, noCaps: true },
      persistent: true,
    }).onOk(async (days) => {
      if (purging.value || !PURGE_RETAIN_DAYS.includes(Number(days))) return;
      retainDays.value = Number(days);
      purging.value = true;
      try {
        const deleted = await purgeEvents();
        $q.notify({ type: 'positive', message: t('usageEventsPage.purgeSuccess', { count: deleted }) });
      } catch (err) {
        console.warn('[usage-events] purge failed', err);
        $q.notify({ type: 'negative', message: t('usageEventsPage.purgeFailed') });
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
    effortOptions,
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
