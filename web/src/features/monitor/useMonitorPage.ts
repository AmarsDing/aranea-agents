import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { Notify } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useMonitorStore } from '../../stores/monitor';
import type { AuditQuery, MonitorTrace, MonitorTracesQuery } from './types';
import { AUDIT_DEFAULT_PAGE_SIZE } from '../constants/queryLimits';
import { useRunnerMetrics } from './useRunnerMetrics';
import { useMonitorRunNavigation } from './useMonitorRunNavigation';
import { useMonitorRealtimeEvents } from './useMonitorRealtimeEvents';
import { useMonitorTraceFlow } from './useMonitorTraceFlow';
import { useMonitorLogStreamPanel } from './useMonitorLogStreamPanel';

const VALID_TABS = ['usage', 'alerts', 'audit', 'events', 'traces', 'logs'] as const;

export function useMonitorPage() {
  const route = useRoute();
  const router = useRouter();
  const { t } = useI18n();
  const monitorStore = useMonitorStore();
  const { auditLogs, auditTotal, selfCheckReports, selfCheckLoading, selfCheckTriggering } =
    storeToRefs(monitorStore);
  const initialTab = String(route.query.tab || 'usage');
  const tab = ref(VALID_TABS.includes(initialTab as (typeof VALID_TABS)[number]) ? initialTab : 'usage');
  const highlightUsageEventId = ref(String(route.query.usage_event_id || '').trim());
  const traces = ref<MonitorTrace[]>([]);
  const loadingAudit = ref(false);
  const loadingTraces = ref(false);
  const error = ref('');

  const filters = reactive<MonitorTracesQuery>({
    limit: 50,
  });

  const rangeOptions = [
    { label: '今日', value: 'today' },
    { label: '7 天', value: '7d' },
    { label: '30 天', value: '30d' },
    { label: '本月', value: 'month' },
  ];

  const loading = computed(() => loadingAudit.value || realtimeEvents.historyLoading.value || loadingTraces.value);

  // ── Runner Metrics (was in MonitorRunnerMetrics.vue) ──
  const {
    runnerMetrics,
    runnerLoading,
    windowMinutes: runnerWindowMinutes,
    reload: reloadRunnerMetrics,
  } = useRunnerMetrics(60);
  const { openRunsTab } = useMonitorRunNavigation();

  // ── Realtime Events (pulse 条 + 历史表，见 useMonitorRealtimeEvents) ──
  const realtimeEvents = useMonitorRealtimeEvents();

  // ── Trace Flow (was in TraceList.vue) ──
  const traceDetail = ref<MonitorTrace | null>(null);
  const traceDetailOpen = ref(false);
  const traceFlow = useMonitorTraceFlow(traceDetail, traceDetailOpen);

  // ── SelfCheck ──
  const selfCheckLatestReport = computed(() => selfCheckReports.value[0] ?? null);

  function loadSelfCheckReports() {
    void monitorStore.loadSelfCheckReports();
  }

  function triggerSelfCheckAction() {
    void monitorStore.triggerSelfCheckAction();
  }

  // ── Log Stream (was in LogStreamPanel.vue) ──
  const logStream = useMonitorLogStreamPanel();

  // ── Notify handler (was inline in MonitorPage.vue) ──
  function notify(payload: { message: string; type: 'positive' | 'negative' | 'warning' }) {
    Notify.create({ message: payload.message, type: payload.type, position: 'top' });
  }

  const autoRefreshMs = 30_000;
  let refreshTimer: ReturnType<typeof setInterval> | null = null;

  function refreshActiveTab() {
    if (tab.value === 'audit') void loadAudit();
    else if (tab.value === 'events') void loadEvents();
    else if (tab.value === 'traces') void loadTraces();
  }

  onMounted(() => {
    void loadAll();
    refreshTimer = setInterval(refreshActiveTab, autoRefreshMs);
  });

  onUnmounted(() => {
    if (refreshTimer != null) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  });

  watch(tab, async (value) => {
    if (!VALID_TABS.includes(value as (typeof VALID_TABS)[number])) {
      tab.value = 'usage';
      return;
    }
    await router.replace({ query: { ...route.query, tab: value } });
  });

  watch(
    () => route.query.usage_event_id,
    (id) => {
      highlightUsageEventId.value = String(id || '').trim();
      if (highlightUsageEventId.value && tab.value !== 'traces') {
        tab.value = 'traces';
      }
    },
  );

  async function loadAll() {
    error.value = '';
    try {
      await Promise.all([loadAudit(), loadEvents(), loadTraces()]);
    } catch (err) {
      error.value = err instanceof Error ? err.message : '加载监控数据失败';
    }
  }

  // AuditTable 服务端分页/筛选：记住最近一次查询，tab 切换/定时刷新时沿用。
  let lastAuditQuery: AuditQuery = { limit: AUDIT_DEFAULT_PAGE_SIZE, offset: 0 };

  async function loadAudit(query?: AuditQuery) {
    if (query) lastAuditQuery = query;
    loadingAudit.value = true;
    try {
      await monitorStore.loadAuditLogs(lastAuditQuery);
    } finally {
      loadingAudit.value = false;
    }
  }

  async function handleClearAudit() {
    try {
      const deleted = await monitorStore.clearAuditLogs();
      notify({ message: t('monitorPage.audit.cleared', { count: deleted }), type: 'positive' });
    } catch (e) {
      notify({
        message: e instanceof Error ? e.message : t('monitorPage.audit.clearFailed'),
        type: 'negative',
      });
    }
  }

  async function loadEvents() {
    await realtimeEvents.refreshHistory();
  }

  async function loadTraces() {
    loadingTraces.value = true;
    try {
      const result = await monitorStore.fetchTraceEvents({ ...filters, limit: 100 });
      traces.value = result.items;
    } finally {
      loadingTraces.value = false;
    }
  }

  function confirmClearEvents() {
    Notify.create({
      // 仅清空 pulse 实时条（页面刷新后也会重建），持久化事件记录不受影响
      message: t('monitorPage.events.clearPulseConfirm'),
      type: 'info',
      position: 'top',
      actions: [
        { label: t('common.cancel'), color: 'white', handler: () => {} },
        { label: t('common.confirm'), color: 'primary', handler: () => realtimeEvents.clearPulse() },
      ],
    });
  }

  function confirmClearFlow() {
    Notify.create({
      message: '确定清除所有流程日志？此操作不可撤销。',
      type: 'warning',
      position: 'top',
      actions: [
        { label: '取消', color: 'white', handler: () => {} },
        { label: '确定', color: 'red', handler: () => monitorStore.clearFlowLogs() },
      ],
    });
  }

  return {
    tab,
    highlightUsageEventId,
    auditRows: auditLogs,
    auditTotal,
    traces,
    loadingAudit,
    loadingTraces,
    error,
    filters,
    rangeOptions,
    loading,
    loadAll,
    loadAudit,
    handleClearAudit,
    loadEvents,
    loadTraces,
    // Runner metrics
    runnerMetrics,
    runnerLoading,
    runnerWindowMinutes,
    reloadRunnerMetrics,
    openRunsTab,
    // Realtime events (pulse + history + actions，含 openChatSession/openLinkedRun)
    ...realtimeEvents,
    // Trace flow
    ...traceFlow,
    traceDetail,
    traceDetailOpen,
    // Log stream
    ...logStream,
    // SelfCheck
    selfCheckLoading,
    selfCheckTriggering,
    selfCheckLatestReport,
    loadSelfCheckReports,
    triggerSelfCheckAction,
    // Notify & confirm
    notify,
    confirmClearEvents,
    confirmClearFlow,
  };
}
