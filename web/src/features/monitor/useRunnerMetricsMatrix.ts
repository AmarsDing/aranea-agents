/**
 * useRunnerMetricsMatrix：多窗口 Runner 指标（15/60/360/1440 分钟）取数。
 * 每列独立 loading / error：单列失败不影响其余列，失败列可就地重试（五态契约）。
 */
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useMonitorStore } from '../../stores/monitor';
import type { MatrixWindowSlice } from './metricsMatrix';

/** 矩阵固定窗口（与 Runner 指标面板的时间窗选项一致）。 */
export const MATRIX_WINDOWS = [15, 60, 360, 1440] as const;

export function useRunnerMetricsMatrix(windows: readonly number[] = MATRIX_WINDOWS) {
  const store = useMonitorStore();
  const { t } = useI18n();
  const slices = ref<MatrixWindowSlice[]>(
    windows.map((windowMinutes) => ({ windowMinutes, metrics: null, loading: false, error: '' })),
  );

  async function loadOne(windowMinutes: number) {
    const slice = slices.value.find((s) => s.windowMinutes === windowMinutes);
    if (!slice) return;
    slice.loading = true;
    slice.error = '';
    try {
      slice.metrics = await store.fetchRunnerMetrics(windowMinutes);
    } catch (err) {
      slice.metrics = null;
      slice.error = err instanceof Error ? err.message : t('uiSystem.operations.loadFailed');
    } finally {
      slice.loading = false;
    }
  }

  async function reloadAll() {
    await Promise.all(slices.value.map((s) => loadOne(s.windowMinutes)));
  }

  const loading = computed(() => slices.value.some((s) => s.loading));

  onMounted(() => void reloadAll());

  return { slices, loading, reloadAll, reloadOne: loadOne };
}
