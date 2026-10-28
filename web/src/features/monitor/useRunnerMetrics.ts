import { onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { useMonitorStore } from '../../stores/monitor';
import { useI18n } from 'vue-i18n';

/** Runner window metrics — Store action only (frontend-guide §1.1). */
export function useRunnerMetrics(initialWindowMinutes = 60) {
  const store = useMonitorStore();
  const { t } = useI18n();
  const runnerError = ref('');
  const { runnerMetrics, runnerLoading } = storeToRefs(store);
  const windowMinutes = ref(initialWindowMinutes);

  async function reload() {
    runnerError.value = '';
    try { await store.loadRunnerMetrics(windowMinutes.value); }
    catch (reason) { runnerError.value = reason instanceof Error ? reason.message : t('uiSystem.operations.loadFailed'); }
  }

  watch(windowMinutes, () => void reload());

  onMounted(() => void reload());

  return { runnerMetrics, runnerLoading, runnerError, windowMinutes, reload };
}
