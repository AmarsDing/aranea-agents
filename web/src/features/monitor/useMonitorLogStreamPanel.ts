import { onMounted, provide, ref } from "vue";
import { useMonitorLogHub } from "./useLogStreamHub";
import { useMonitorStore } from "../../stores/monitor/index";

export function useMonitorLogStreamPanel() {
  const subTab = ref<"flow" | "process">("flow");
  const hub = useMonitorLogHub();
  const monitorStore = useMonitorStore();
  const processLogConfigured = ref(false);

  provide("monitorLogHub", hub);
  provide("processLogConfigured", processLogConfigured);

  onMounted(async () => {
    hub.setProcessPaused(true);
    try {
      await monitorStore.loadLogs();
      const snapshot = monitorStore.logSnapshot;
      processLogConfigured.value = Boolean(snapshot?.enabled);
      if (snapshot?.enabled) {
        hub.setProcessEnabled(true);
      }
    } catch {
      processLogConfigured.value = false;
    }
    hub.connect();
  });

  return { subTab };
}
