import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listMonitorAudit,
  listMonitorEvents,
  getMonitorLogs,
  getRunnerMetrics,
  subscribeMonitorRuntimeEventsWs,
  subscribeMonitorLogsWs
} from "../../features/monitor/api";
import type {
  AuditLog,
  PlatformResource,
  MonitorLogSnapshot,
  MonitorLogLine,
  TeamRunEvent,
  AuditQuery,
  PaginatedResult,
  RunnerMetricsSummary
} from "../../features/monitor/types";

export const useMonitorStore = defineStore("monitor", () => {
  const auditLogs = ref<AuditLog[]>([]);
  const auditTotal = ref(0);
  const events = ref<PlatformResource[]>([]);
  const logSnapshot = ref<MonitorLogSnapshot | null>(null);
  const loading = ref(false);
  const runnerMetrics = ref<RunnerMetricsSummary | null>(null);
  const runnerLoading = ref(false);

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

  async function loadLogs() {
    logSnapshot.value = await getMonitorLogs();
  }

  async function loadRunnerMetrics(windowMinutes = 60) {
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
    onDisconnected?: () => void
  ) {
    return subscribeMonitorRuntimeEventsWs(sessionId, onEvent, onError, onConnected, onDisconnected);
  }

  function startLogsStream(
    sessionId: string,
    onLine: (line: MonitorLogLine) => void,
    onError?: (error: string) => void,
    onConnected?: () => void
  ) {
    return subscribeMonitorLogsWs(sessionId, onLine, onError, onConnected);
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
    loadLogs,
    loadRunnerMetrics,
    startRuntimeEventsStream,
    startLogsStream
  };
});
