<template>
  <q-card flat class="overview-panel overview-trend-panel">
    <q-card-section class="row items-center justify-between q-col-gutter-sm">
      <div>
        <div class="text-h6 overview-section-title">消耗趋势</div>
        <div class="text-caption overview-section-caption">
          {{ hourly ? "按小时展示调用、Token 和费用变化" : "按天展示调用、Token 和费用变化" }}
        </div>
      </div>
      <q-chip dense outline class="overview-chip">{{ points.length }} {{ hourly ? "小时" : "天" }}</q-chip>
    </q-card-section>
    <q-separator class="overview-separator" />
    <q-card-section>
      <div v-if="!points.length" class="overview-empty">暂无趋势数据</div>
      <div v-else class="overview-trend-grid">
        <div v-for="point in points" :key="point.date_key" class="overview-trend-bar">
          <div class="overview-trend-bar__track">
            <div class="overview-trend-bar__fill" :style="{ height: `${barHeight(point.total_tokens)}%` }" />
          </div>
          <div class="text-caption overview-trend-bar__label ellipsis">{{ formatLabel(point.date_key) }}</div>
          <q-tooltip>
            调用 {{ formatCount(point.call_count) }}；Token {{ formatCount(point.total_tokens) }}；费用
            {{ formatMoney(point.total_cost_micro_usd) }}
          </q-tooltip>
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { ModelUsageTrendPoint } from "../../features/usage/types";

const props = defineProps<{
  points: ModelUsageTrendPoint[];
  hourly?: boolean;
}>();

const maxTokens = computed(() => Math.max(1, ...props.points.map((point) => point.total_tokens)));

function barHeight(value: number) {
  return Math.max(8, Math.round((value / maxTokens.value) * 100));
}

function formatLabel(key: string) {
  if (props.hourly && key.length >= 13) {
    return key.slice(11, 16);
  }
  return key.length >= 10 ? key.slice(5) : key;
}

function formatCount(value: number) {
  return new Intl.NumberFormat("zh-CN", { maximumFractionDigits: 0 }).format(value);
}

function formatMoney(value: number) {
  return `$${(value / 1_000_000).toFixed(4)}`;
}
</script>
