/**
 * 健康与告警页（/monitor/health）取数：最新一次自检、最近触发的告警、运行时自愈。
 * 三块各自读取，一块失败只在那一块里说明，不整页报错；每分钟和切回页面时刷新。
 */
import { computed, onMounted, onUnmounted, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useMonitorStore } from '../../stores/monitor';
import { getHealStats, listHealRecords, listMonitorEvents } from './api';
import type { HealRecord, HealStats, PlatformResource } from './types';

const REFRESH_MS = 60_000;
const DAY_MS = 24 * 3600_000;

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export function useMonitorHealth() {
  const store = useMonitorStore();
  const { selfCheckReports, selfCheckLoading, selfCheckTriggering } = storeToRefs(store);
  const selfCheckError = ref('');
  const alerts = ref<PlatformResource[]>([]);
  /** 告警事件总数（接口给的 total；alerts 只取最近 50 条） */
  const alertsTotal = ref(0);
  const alertsError = ref('');
  const alertsLoading = ref(false);
  const heal = ref<HealStats | null>(null);
  const healRecords = ref<HealRecord[]>([]);
  const healError = ref('');

  const latest = computed(() => selfCheckReports.value[0] ?? null);
  /** 失败项排在前面：一眼看到哪里坏了。 */
  const checks = computed(() => {
    const rows = latest.value?.check_results ?? [];
    const rank = (s: string) => (s === 'failed' ? 0 : s === 'warning' ? 1 : 2);
    return [...rows].sort((a, b) => rank(a.status) - rank(b.status));
  });
  const failedChecks = computed(() => checks.value.filter((c) => c.status !== 'passed').length);
  const alerts24h = computed(() => {
    const since = Date.now() - DAY_MS;
    return alerts.value.filter((a) => Date.parse(a.created_at) >= since).length;
  });

  async function loadSelfCheck() {
    try {
      await store.loadSelfCheckReports(1);
      selfCheckError.value = '';
    } catch (err) {
      selfCheckError.value = message(err);
    }
  }

  async function triggerSelfCheck() {
    try {
      await store.triggerSelfCheckAction();
      selfCheckError.value = '';
    } catch (err) {
      selfCheckError.value = message(err);
    }
  }

  async function loadAlerts() {
    alertsLoading.value = true;
    try {
      const res = await listMonitorEvents({ event_type: 'alert.fired', limit: 50 });
      alerts.value = res.items;
      alertsTotal.value = res.total;
      alertsError.value = '';
    } catch (err) {
      alertsError.value = message(err);
    } finally {
      alertsLoading.value = false;
    }
  }

  async function loadHeal() {
    try {
      const [stats, records] = await Promise.all([getHealStats(), listHealRecords(8)]);
      heal.value = stats;
      healRecords.value = records.items;
      healError.value = '';
    } catch (err) {
      healError.value = message(err);
    }
  }

  let lastAt = 0;
  function refresh() {
    lastAt = Date.now();
    return Promise.all([loadSelfCheck(), loadAlerts(), loadHeal()]);
  }

  let timer: ReturnType<typeof setInterval> | null = null;
  // 内置浏览器等环境会频繁切换可见性，15 秒内不重复拉
  function onVisible() {
    if (document.visibilityState === 'visible' && Date.now() - lastAt > 15_000) void refresh();
  }
  onMounted(() => {
    void refresh();
    timer = setInterval(() => void refresh(), REFRESH_MS);
    document.addEventListener('visibilitychange', onVisible);
  });
  onUnmounted(() => {
    if (timer) clearInterval(timer);
    document.removeEventListener('visibilitychange', onVisible);
  });

  return {
    latest,
    checks,
    failedChecks,
    selfCheckLoading,
    selfCheckTriggering,
    selfCheckError,
    triggerSelfCheck,
    alerts,
    alertsTotal,
    alerts24h,
    alertsLoading,
    alertsError,
    heal,
    healRecords,
    healError,
    refresh,
  };
}
