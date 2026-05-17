import { defineStore } from "pinia";
import { ref } from "vue";
import { listMonitorAudit, listMonitorEvents, getMonitorLogs } from "../../features/monitor/api";
import type { AuditLog, PlatformResource, MonitorLogSnapshot } from "../../features/monitor/types";

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

  return { auditLogs, events, logSnapshot, loading, loadAuditLogs, loadEvents, loadLogs };
});
