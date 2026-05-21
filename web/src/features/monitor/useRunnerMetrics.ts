import { onMounted, ref, watch } from "vue";
import { storeToRefs } from "pinia";
import { useMonitorStore } from "../../stores/monitor";

/** Runner window metrics — Store action only (frontend-guide §1.1). */
export function useRunnerMetrics(initialWindowMinutes = 60) {
  const store = useMonitorStore();
  const { runnerMetrics, runnerLoading } = storeToRefs(store);
  const windowMinutes = ref(initialWindowMinutes);

  async function reload() {
    await store.loadRunnerMetrics(windowMinutes.value);
  }

  watch(windowMinutes, () => void reload());

  onMounted(() => void reload());

  return { runnerMetrics, runnerLoading, windowMinutes, reload };
}
