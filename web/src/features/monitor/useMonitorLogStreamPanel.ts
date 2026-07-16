import { onMounted, provide, ref, watch } from 'vue';
import { useMonitorLogHub } from './useLogStreamHub';
import { useMonitorStore } from '../../stores/monitor/index';

export function useMonitorLogStreamPanel() {
  const subTab = ref<'flow' | 'process'>('flow');
  const hub = useMonitorLogHub();
  const monitorStore = useMonitorStore();
  const processLogConfigured = ref(false);

  provide('monitorLogHub', hub);
  provide('processLogConfigured', processLogConfigured);

  // Req §1.1/§1.6: leave process Tab → pause (discard inbound); return → auto-resume.
  // No manual pause button on ProcessLogStream.
  watch(
    subTab,
    (tab) => {
      if (!processLogConfigured.value) {
        hub.setProcessPaused(true);
        return;
      }
      hub.setProcessPaused(tab !== 'process');
    },
    { immediate: true },
  );

  onMounted(async () => {
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
    hub.setProcessPaused(subTab.value !== 'process' || !processLogConfigured.value);
    hub.connect();
  });

  return {
    subTab,
    backpressureMessage: hub.backpressureMessage,
    clearBackpressure: hub.clearBackpressure,
  };
}
