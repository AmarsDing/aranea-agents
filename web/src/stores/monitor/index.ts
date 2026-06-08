import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listFlowLogs,
  listMonitorAlertRules,
  listMonitorAudit,
  listMonitorEvents,
  listMonitorTraces,
  getMonitorTrace,
  getMonitorLogs,
  getRunnerMetrics,
  putMonitorAlertRules,
  subscribeMonitorRuntimeEventsWs,
  subscribeMonitorLogsWs,
  triggerSelfCheck,
  listSelfCheckReports,
} from '../../features/monitor/api';
import type { MonitorAlertRule, MonitorTrace, MonitorTraceDetail, SelfCheckReport } from '../../features/monitor/types';
import { listChannels } from '../../features/channels/api';
import {
  MONITOR_TRACES_LIMIT,
  MONITOR_RUNNER_WINDOW_MINUTES,
  MONITOR_REPORTS_LIMIT,
} from '../../features/constants/queryLimits';
import type {
  AuditLog,
  PlatformResource,
  MonitorLogSnapshot,
  MonitorLogLine,
  TeamRunEvent,
  AuditQuery,
  MonitorTracesQuery,
  PaginatedResult,
  RunnerMetricsSummary,
} from '../../features/monitor/types';

export const useMonitorStore = defineStore('monitor', () => {
  const auditLogs = ref<AuditLog[]>([]);
  const auditTotal = ref(0);
  const events = ref<PlatformResource[]>([]);
  const logSnapshot = ref<MonitorLogSnapshot | null>(null);
  const loading = ref(false);
  const runnerMetrics = ref<RunnerMetricsSummary | null>(null);
  const runnerLoading = ref(false);
  const alertRules = ref<MonitorAlertRule[]>([]);
  const alertRulesLoading = ref(false);
  const alertRulesSaving = ref(false);
  const alertChannelOptions = ref<{ label: string; value: string }[]>([]);
  const flowPaused = ref(false);
  const processPaused = ref(true);
  const eventsPaused = ref(false);
  const selfCheckReports = ref<SelfCheckReport[]>([]);
  const selfCheckReportsTotal = ref(0);
  const selfCheckLoading = ref(false);
  const selfCheckTriggering = ref(false);

  async function loadAuditLogs(query: AuditQuery = {}) {
    loading.value = true;
    try {
      const result: PaginatedResult<AuditLog> = await listMonitorAudit(query);
      auditLogs.value = result.items;
      auditTotal.value = result.total;
    } finally {
      loading.value = false;
    }
  }

  async function loadEvents() {
    events.value = await listMonitorEvents();
  }

  async function fetchAuditPage(query: AuditQuery = {}) {
    return listMonitorAudit(query);
  }

  async function fetchMonitorEvents() {
    return listMonitorEvents();
  }

  async function fetchTraceEvents(query: MonitorTracesQuery = {}): Promise<PaginatedResult<MonitorTrace>> {
    return listMonitorTraces({ ...query, limit: query.limit ?? MONITOR_TRACES_LIMIT });
  }

  async function fetchTraceDetail(id: string): Promise<MonitorTraceDetail> {
    return getMonitorTrace(id);
  }

  async function loadLogs() {
    logSnapshot.value = await getMonitorLogs();
  }

  async function loadRunnerMetrics(windowMinutes = MONITOR_RUNNER_WINDOW_MINUTES) {
    runnerLoading.value = true;
    try {
      runnerMetrics.value = await getRunnerMetrics(windowMinutes);
    } finally {
      runnerLoading.value = false;
    }
  }

  function startRuntimeEventsStream(
    sessionId: string,
    onEvent: (event: TeamRunEvent) => void,
    onError?: (error: string) => void,
    onConnected?: () => void,
    onDisconnected?: () => void,
  ) {
    return subscribeMonitorRuntimeEventsWs(sessionId, onEvent, onError, onConnected, onDisconnected);
  }

  function startLogsStream(
    sessionId: string,
    onLine: (line: MonitorLogLine) => void,
    onError?: (error: string) => void,
    onConnected?: () => void,
  ) {
    return subscribeMonitorLogsWs(sessionId, onLine, onError, onConnected);
  }

  async function fetchFlowLogs(params: {
    traceId?: string;
    sessionId?: string;
    runId?: string;
    severity?: string;
    domain?: string;
    since?: string;
    until?: string;
    limit?: number;
    offset?: number;
  }) {
    return listFlowLogs(params);
  }

  async function loadAlertChannelOptions() {
    try {
      const rows = await listChannels();
      alertChannelOptions.value = rows.map((c) => ({
        label: `${c.name || c.key} (${c.id})`,
        value: c.id,
      }));
    } catch {
      alertChannelOptions.value = [];
    }
  }

  async function loadAlertRules() {
    alertRulesLoading.value = true;
    try {
      alertRules.value = await listMonitorAlertRules();
    } finally {
      alertRulesLoading.value = false;
    }
  }

  async function saveAlertRules(rules: MonitorAlertRule[]) {
    alertRulesSaving.value = true;
    try {
      alertRules.value = await putMonitorAlertRules(rules);
      return alertRules.value;
    } finally {
      alertRulesSaving.value = false;
    }
  }

  function clearRuntimeEvents() {
    events.value = [];
  }

  function setFlowPaused(paused: boolean) {
    flowPaused.value = paused;
  }

  function setProcessPaused(paused: boolean) {
    processPaused.value = paused;
  }

  function setEventsPaused(paused: boolean) {
    eventsPaused.value = paused;
  }

  function clearFlowLogs() {
    logSnapshot.value = null;
  }

  async function loadSelfCheckReports(limit = MONITOR_REPORTS_LIMIT, offset = 0) {
    selfCheckLoading.value = true;
    try {
      const result = await listSelfCheckReports(limit, offset);
      selfCheckReports.value = result.items;
      selfCheckReportsTotal.value = result.total;
    } finally {
      selfCheckLoading.value = false;
    }
  }

  async function triggerSelfCheckAction() {
    selfCheckTriggering.value = true;
    try {
      const report = await triggerSelfCheck();
      selfCheckReports.value = [report, ...selfCheckReports.value];
      selfCheckReportsTotal.value += 1;
      return report;
    } finally {
      selfCheckTriggering.value = false;
    }
  }

  return {
    auditLogs,
    auditTotal,
    events,
    logSnapshot,
    loading,
    runnerMetrics,
    runnerLoading,
    loadAuditLogs,
    loadEvents,
    fetchAuditPage,
    fetchMonitorEvents,
    fetchTraceEvents,
    fetchTraceDetail,
    loadLogs,
    loadRunnerMetrics,
    startRuntimeEventsStream,
    startLogsStream,
    fetchFlowLogs,
    alertRules,
    alertRulesLoading,
    alertRulesSaving,
    alertChannelOptions,
    loadAlertChannelOptions,
    loadAlertRules,
    saveAlertRules,
    clearRuntimeEvents,
    clearFlowLogs,
    flowPaused,
    processPaused,
    eventsPaused,
    setFlowPaused,
    setProcessPaused,
    setEventsPaused,
    selfCheckReports,
    selfCheckReportsTotal,
    selfCheckLoading,
    selfCheckTriggering,
    loadSelfCheckReports,
    triggerSelfCheckAction,
  };
});
