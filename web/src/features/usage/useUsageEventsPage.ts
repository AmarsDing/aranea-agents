import { computed, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useUsageStore } from '../../stores/usage';
import type { ModelUsageQuery } from './types';
import { formatUsdFromMicro } from './moneyFormat';
import { downloadBlob } from '../../composables/useFileDownload';
import { USAGE_EVENTS_PAGE_SIZE_DEFAULT } from '../constants/queryLimits';

/** Default retention when purging; independent of the view filter range. */
export const DEFAULT_PURGE_RETAIN_DAYS = 30;

const PURGE_RETAIN_DAY_OPTIONS = [
  { label: '保留 7 天', value: 7 },
  { label: '保留 30 天', value: 30 },
  { label: '保留 90 天', value: 90 },
  { label: '保留 365 天', value: 365 },
];

export function useUsageEventsPage() {
  const $q = useQuasar();
  const usageStore = useUsageStore();
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
  const statusOptions = [
    { label: '成功', value: 'success' },
    { label: '异常', value: 'error' },
    { label: '失败', value: 'failed' },
    { label: '超时', value: 'timeout' },
    { label: '取消', value: 'cancelled' },
  ];
  const usageKindOptions = [
    { label: '全部', value: '' },
    { label: 'Chat Turn', value: 'chat_turn' },
    { label: 'Team 成员', value: 'team_member' },
    { label: 'Team 整轮', value: 'team_turn' },
  ];

  function buildQuery(): ModelUsageQuery {
    return {
      ...filters.value,
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
    const csv = await usageStore.exportEventsCsv({ ...filters.value });
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

  const purging = ref(false);

  function onPurgeConfirm() {
    const days = retainDays.value;
    $q.dialog({
      class: 'app-dialog-card app-dialog-card--sm',
      title: '确认删除',
      message: `将只保留最近 ${days} 天的用量事件，更早的记录将全部删除。此操作与上方「范围」筛选无关，且不可撤销，确认继续？`,
      cancel: { label: '取消', flat: true, rounded: true, noCaps: true },
      ok: { label: '确认删除', color: 'negative', flat: true, rounded: true, noCaps: true },
      persistent: true,
    }).onOk(async () => {
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
    retainDays,
    retainDayOptions: PURGE_RETAIN_DAY_OPTIONS,
    purging,
    load,
    exportCsv,
    purgeEvents,
    onPurgeConfirm,
    resetFilters,
    formatMoney,
  };
}
