<template>
  <q-card flat class="overview-panel">
    <q-card-section>
      <div class="text-h6 overview-section-title">Provider 费用占比</div>
      <div class="text-caption overview-section-caption">{{ providerCaption }}</div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!providerSlices.length" class="overview-empty">暂无 Provider 费用数据</div>
      <div v-else class="usage-donut">
        <div ref="providerChartEl" class="usage-breakdown-chart" />
        <div class="usage-donut__center">
          <div class="usage-donut__center-value">{{ totalLabel }}</div>
          <div class="usage-donut__center-label">{{ t('overviewPage.donutTotal') }}</div>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ModelUsageBreakdownRow } from '../../features/usage/types';
import { useUsageChart } from '../../features/usage/useUsageChart';
import { USAGE_BREAKDOWN_TOP_N, buildProviderCostSlicesFromTopModels } from '../../features/usage/usageBreakdownSlices';
import { buildCostDonutOption, costDonutTotalLabel } from '../../features/usage/usageDonutOption';

const { t } = useI18n();

const props = defineProps<{
  topModels: ModelUsageBreakdownRow[];
}>();

const providerSlices = computed(() => buildProviderCostSlicesFromTopModels(props.topModels));
const totalLabel = computed(() => costDonutTotalLabel(providerSlices.value));
const providerChartEl = ref<HTMLElement | null>(null);

const providerCaption = computed(() => t('overviewPage.providerPieCaption', { n: USAGE_BREAKDOWN_TOP_N }));

useUsageChart(
  providerChartEl,
  () => buildCostDonutOption(providerSlices.value),
  () => [providerSlices.value],
);
</script>
