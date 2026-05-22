import { computed, ref, watch, type Ref } from "vue";
import type { PlatformResource } from "../platform/types";
import type { ModelUsageOverview } from "./types";
import { useUsageStore } from "../../stores/usage";
import {
  USAGE_TREND_METRIC_OPTIONS,
  type UsageTrendMetric
} from "./usageTrendMetrics";

export function useProviderTrendDialog(
  open: Ref<boolean>,
  row: Ref<PlatformResource | null>
) {
  const usageStore = useUsageStore();
  const metricOptions = USAGE_TREND_METRIC_OPTIONS.filter((o) => o.value !== "success_rate");
  const metric = ref<UsageTrendMetric>("tokens");
  const overview = ref<ModelUsageOverview | null>(null);
  const loading = ref(false);

  const trends = computed(() => overview.value?.trends ?? []);
  const metricCaption = computed(() => metricOptions.find((o) => o.value === metric.value)?.label ?? "");

  watch(
    () => [open.value, row.value?.id],
    () => {
      if (open.value && row.value) {
        void loadOverview();
      }
    },
    { immediate: true }
  );

  watch(metric, () => {
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
        range: "30d",
        provider_code: r.provider,
        model_api_id: r.model
      });
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
    loadOverview
  };
}
