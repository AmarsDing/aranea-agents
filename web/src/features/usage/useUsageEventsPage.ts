import { computed, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useUsageStore } from '../../stores/usage';
import type { ModelUsageQuery } from './types';
import { formatUsdFromMicro } from './moneyFormat';

export function useUsageEventsPage() {
  const usageStore = useUsageStore();
  const { events, eventsLoading, eventsError, exporting } = storeToRefs(usageStore);
  const filters = ref<ModelUsageQuery>({ range: '7d', limit: 200 });

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

  const retainDays = computed(() => {
    const r = filters.value.range ?? '7d';
    const m = r.match(/^(\d+)/);
    if (!m) return 7;
    return parseInt(m[1], 10);
  });

  async function load() {
    await usageStore.loadEvents(filters.value);
  }

  async function exportCsv() {
    const csv = await usageStore.exportEventsCsv(filters.value);
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `usage-events-${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  async function purgeEvents() {
    const deleted = await usageStore.purgeEvents(retainDays.value);
    await load();
    return deleted;
  }

  function formatMoney(value?: number) {
    return formatUsdFromMicro(value);
  }

  function truncate(msg?: string, max = 80) {
    const s = (msg ?? '').trim();
    if (s.length <= max) return s || '—';
    return `${s.slice(0, max)}…`;
  }

  function resetFilters() {
    filters.value = { range: '7d', limit: 200 };
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
    load,
    exportCsv,
    purgeEvents,
    resetFilters,
    formatMoney,
    truncate,
  };
}
