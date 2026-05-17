import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listMonitorAudit,
  listMonitorEvents,
  getMonitorLogs,
  subscribeMonitorRuntimeEventsWs,
  subscribeMonitorLogsWs
} from "../../features/monitor/api";
import type { AuditLog, PlatformResource, MonitorLogSnapshot, MonitorLogLine, TeamRunEvent } from "../../features/monitor/types";

export const useMonitorStore = defineStore("monitor", () => {
  const auditLogs = ref<AuditLog[]>([]);
  const events = ref<PlatformResource[]>([]);
  const logSnapshot = ref<MonitorLogSnapshot | null>(null);
  const loading = ref(false);

  async function loadAuditLogs(limit = 200) {
    loading.value = true;
    try {
      auditLogs.value = await listMonitorAudit(limit);
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

  // EP-FE-01: WS subscription wrappers so components use the store rather than features/monitor/api.
  function startRuntimeEventsStream(
    sessionId: string,
    onEvent: (event: TeamRunEvent) => void,
    onError?: (error: string) => void
  ) {
    return subscribeMonitorRuntimeEventsWs(sessionId, onEvent, onError);
  }

  function startLogsStream(
    sessionId: string,
    onLine: (line: MonitorLogLine) => void,
    onError?: (error: string) => void,
    onConnected?: () => void
  ) {
    return subscribeMonitorLogsWs(sessionId, onLine, onError, onConnected);
  }

  return { auditLogs, events, logSnapshot, loading, loadAuditLogs, loadEvents, loadLogs, startRuntimeEventsStream, startLogsStream };
});
