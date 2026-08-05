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

  // 进程日志默认关闭（paused=true）：切离 Tab 强制暂停（丢弃入站）；
  // 切回不自动恢复，由用户在进程日志工具行手动点「恢复」开启。
  watch(
    subTab,
    (tab) => {
      if (!processLogConfigured.value) {
        hub.setProcessPaused(true);
        return;
      }
      if (tab !== 'process') {
        hub.setProcessPaused(true);
      }
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
    hub.setProcessPaused(true);
    hub.connect();
  });

  // 清空流程日志视图：hub 实时行 + Store 快照一并清（此前只清快照，界面行残留）。
  function clearFlowLogs() {
    hub.clearFlow();
    monitorStore.clearFlowLogs();
  }

  return {
    subTab,
    backpressureMessage: hub.backpressureMessage,
    clearBackpressure: hub.clearBackpressure,
    clearFlowLogs,
  };
}
