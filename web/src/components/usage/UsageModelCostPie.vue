<template>
  <q-card flat class="overview-panel">
    <q-card-section>
      <div class="text-h6 overview-section-title">模型费用占比</div>
      <div class="text-caption overview-section-caption">Top {{ modelSlices.length }} 模型（按费用）</div>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!modelSlices.length" class="overview-empty">暂无模型费用数据</div>
      <div v-else class="usage-donut">
        <div ref="modelChartEl" class="usage-breakdown-chart" />
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
import { buildModelCostSlices } from '../../features/usage/usageBreakdownSlices';
import { buildCostDonutOption, costDonutTotalLabel } from '../../features/usage/usageDonutOption';

const { t } = useI18n();

const props = defineProps<{
  topModels: ModelUsageBreakdownRow[];
}>();

const modelSlices = computed(() => buildModelCostSlices(props.topModels));
const totalLabel = computed(() => costDonutTotalLabel(modelSlices.value));
const modelChartEl = ref<HTMLElement | null>(null);

useUsageChart(
  modelChartEl,
  () => buildCostDonutOption(modelSlices.value),
  () => [modelSlices.value],
);
</script>
