import { computed, ref, watch, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { PlatformResource } from '../platform/types';
import { errorMessage } from '../platform/providerUtils';
import type { ModelUsageOverview } from './types';
import { useUsageStore } from '../../stores/usage';
import { USAGE_TREND_METRIC_OPTIONS, type UsageTrendMetric } from './usageTrendMetrics';

export function useProviderTrendDialog(open: Ref<boolean>, row: Ref<PlatformResource | null>) {
  const { t } = useI18n();
  const usageStore = useUsageStore();
  const $q = useQuasar();
  const metricOptions = USAGE_TREND_METRIC_OPTIONS.filter((o) => o.value !== 'success_rate');
  const metric = ref<UsageTrendMetric>('tokens');
  const overview = ref<ModelUsageOverview | null>(null);
  const loading = ref(false);
  const range = ref<string>('30d');

  const rangeOptions = computed(() => [
    { label: t('overviewPage.rangeToday'), value: 'today' },
    { label: t('overviewPage.range7d'), value: '7d' },
    { label: t('overviewPage.range30d'), value: '30d' },
    { label: t('overviewPage.rangeMonth'), value: 'month' },
  ]);

  const trends = computed(() => overview.value?.trends ?? []);
  const metricCaption = computed(() => metricOptions.find((o) => o.value === metric.value)?.label ?? '');
  const rangeLabel = computed(() => rangeOptions.value.find((o) => o.value === range.value)?.label ?? '');

  watch(
    () => [open.value, row.value?.id],
    () => {
      if (open.value && row.value) {
        void loadOverview();
      }
    },
    { immediate: true },
  );

  watch(metric, () => {
    if (open.value && row.value) {
      void loadOverview();
    }
  });

  watch(range, () => {
    if (open.value && row.value) {
      void loadOverview();
    }
  });

  async function loadOverview() {
    const r = row.value;
    if (!r) return;
    loading.value = true;
    try {
      overview.value = await usageStore.fetchOverview({
        range: range.value,
        provider_code: r.provider,
        model_api_id: r.model,
      });
    } catch (error) {
      $q.notify({ type: 'negative', message: errorMessage(error) || '加载趋势数据失败' });
    } finally {
      loading.value = false;
    }
  }

  return {
    metricOptions,
    metric,
    overview,
    loading,
    trends,
    metricCaption,
    range,
    rangeOptions,
    rangeLabel,
    loadOverview,
  };
}
