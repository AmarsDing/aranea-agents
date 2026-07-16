import { ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useQuasar } from 'quasar';
import { useUsageStore } from '../../stores/usage';
import type { ModelUsageQuery } from './types';
import { formatUsdFromMicro } from './moneyFormat';
import { downloadBlob } from '../../composables/useFileDownload';
import { USAGE_EVENTS_LIMIT } from '../constants/queryLimits';

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
  const { events, eventsLoading, eventsError, exporting } = storeToRefs(usageStore);
  const filters = ref<ModelUsageQuery>({ range: '7d', limit: USAGE_EVENTS_LIMIT });
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

  async function load() {
    await usageStore.loadEvents(filters.value);
  }

  async function exportCsv() {
    const csv = await usageStore.exportEventsCsv(filters.value);
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
    downloadBlob(blob, `usage-events-${new Date().toISOString().slice(0, 10)}.csv`);
  }

  async function purgeEvents() {
    const deleted = await usageStore.purgeEvents(retainDays.value);
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
    filters.value = { range: '7d', limit: USAGE_EVENTS_LIMIT };
    void load();
  }

  return {
    events,
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
