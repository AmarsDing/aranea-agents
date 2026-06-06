import { computed, onMounted, onUnmounted, reactive, ref, toRef, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import { Notify } from 'quasar';
import { useMonitorStore } from '../../stores/monitor';
import type { MonitorTrace, MonitorTracesQuery } from './types';
import { useRunnerMetrics } from './useRunnerMetrics';
import { useMonitorRunNavigation } from './useMonitorRunNavigation';
import { useMonitorRealtimeEvents } from './useMonitorRealtimeEvents';
import { useMonitorTraceFlow } from './useMonitorTraceFlow';
import { useMonitorLogStreamPanel } from './useMonitorLogStreamPanel';

const VALID_TABS = ['usage', 'alerts', 'audit', 'events', 'traces', 'logs'] as const;

export function useMonitorPage() {
  const route = useRoute();
  const router = useRouter();
  const monitorStore = useMonitorStore();
  const { auditLogs, events, selfCheckReports, selfCheckLoading, selfCheckTriggering } = storeToRefs(monitorStore);
  const initialTab = String(route.query.tab || 'usage');
  const tab = ref(VALID_TABS.includes(initialTab as (typeof VALID_TABS)[number]) ? initialTab : 'usage');
  const highlightUsageEventId = ref(String(route.query.usage_event_id || '').trim());
  const traces = ref<MonitorTrace[]>([]);
  const loadingAudit = ref(false);
  const loadingEvents = ref(false);
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

  const loading = computed(() => loadingAudit.value || loadingEvents.value || loadingTraces.value);

  // ── Runner Metrics (was in MonitorRunnerMetrics.vue) ──
  const { runnerMetrics, runnerLoading, windowMinutes: runnerWindowMinutes, reload: reloadRunnerMetrics } = useRunnerMetrics(60);
  const { openRunsTab, openChatSession } = useMonitorRunNavigation();

  // ── Realtime Events (was in RealtimeEvents.vue) ──
  const realtimeEvents = useMonitorRealtimeEvents(
    toRef(() => events.value),
    toRef(() => traces.value),
  );

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

  async function loadAudit() {
    loadingAudit.value = true;
    try {
      await monitorStore.loadAuditLogs({ limit: 200 });
    } finally {
      loadingAudit.value = false;
    }
  }

  async function loadEvents() {
    loadingEvents.value = true;
    try {
      await monitorStore.loadEvents();
    } finally {
      loadingEvents.value = false;
    }
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
      message: '确定清除所有实时事件？此操作不可撤销。',
      type: 'warning',
      position: 'top',
      actions: [
        { label: '取消', color: 'white', handler: () => {} },
        { label: '确定', color: 'red', handler: () => monitorStore.clearRuntimeEvents() },
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
    events,
    traces,
    loadingAudit,
    loadingEvents,
    loadingTraces,
    error,
    filters,
    rangeOptions,
    loading,
    loadAll,
    loadAudit,
    loadEvents,
    loadTraces,
    // Runner metrics
    runnerMetrics,
    runnerLoading,
    runnerWindowMinutes,
    reloadRunnerMetrics,
    openRunsTab,
    openChatSession,
    // Realtime events
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
