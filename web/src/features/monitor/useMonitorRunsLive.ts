/**
 * Runs 列表实时化 composable —— 订阅运行时事件流，运行生命周期事件到达后防抖触发刷新。
 *
 * 设计要点（方案 C3 实时化）：
 * - 事件源复用 Store 的运行时事件流（envelope WS：monitor/team/system 频道）；
 * - 只关心「新运行出现 / 运行终结」类事件：team_run_started 让 running 行即时出现，
 *   run_finished（chat runner completion）与 team_run_finished/failed 让状态/指标即时落定；
 * - 列表行与 status/domain counts 均由服务端聚合，事件到达后防抖全量刷新当前页即可保持一致，
 *   不做本地行 patch（避免 counts 漂移）。
 */
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import { useMonitorStore } from '../../stores/monitor';
import type { StreamState } from './types';
import { isRunLifecycleEventType } from './tracesQuery';

export { isRunLifecycleEventType };

/** 防抖窗口：一次对话终结常伴随 completion + flow 多个事件，合并为一次刷新 */
const REFRESH_DEBOUNCE_MS = 800;

export function useMonitorRunsLive(onSettled: () => void) {
  const monitorStore = useMonitorStore();
  const state = ref<StreamState>('connecting');
  let wsSub: ReturnType<typeof monitorStore.startRuntimeEventsStream> | null = null;
  let timer: ReturnType<typeof setTimeout> | null = null;

  function scheduleRefresh() {
    if (timer != null) clearTimeout(timer);
    timer = setTimeout(() => {
      timer = null;
      onSettled();
    }, REFRESH_DEBOUNCE_MS);
  }

  function cancelRefresh() {
    if (timer != null) {
      clearTimeout(timer);
      timer = null;
    }
  }

  function startStream() {
    stopStream();
    state.value = 'connecting';
    wsSub = monitorStore.startRuntimeEventsStream(
      GLOBAL_WS_SESSION_ID,
      (event) => {
        if (!isRunLifecycleEventType(event.type)) return;
        state.value = 'live';
        scheduleRefresh();
      },
      () => {
        state.value = 'error';
      },
      () => {
        if (state.value !== 'live') state.value = 'connected';
      },
      () => {
        state.value = 'error';
      },
    );
  }

  function stopStream() {
    cancelRefresh();
    wsSub?.close();
    wsSub = null;
  }

  onMounted(startStream);
  onBeforeUnmount(stopStream);

  return { state };
}
