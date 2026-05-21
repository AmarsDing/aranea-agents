<template>
  <q-card flat :bordered="variant === 'monitor'" :class="panelClass">
    <q-card-section class="row items-center">
      <div>
        <div class="text-h6 text-weight-bold">Runner 指标</div>
        <div v-if="scopeHint" class="text-caption text-grey-7 q-mt-xs">{{ scopeHint }}</div>
      </div>
      <q-space />
      <q-select
        :model-value="windowMinutes"
        dense
        outlined
        emit-value
        map-options
        :options="windowOptions"
        label="窗口"
        style="min-width: 120px"
        class="q-mr-sm"
        @update:model-value="emit('update:windowMinutes', $event)"
      />
      <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="emit('refresh')" />
    </q-card-section>
    <q-separator />
    <q-card-section v-if="metrics" :class="{ 'runner-metrics__body--compact': variant === 'overview' }">
      <div class="app-metrics-grid runner-metrics__grid">
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">总运行次数</div>
          <q-btn flat dense no-caps class="q-pa-none" @click="emit('drill')">
            <div class="text-h5 text-weight-bold text-primary">{{ metrics.total_runs }}</div>
          </q-btn>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">错误次数</div>
          <q-btn flat dense no-caps class="q-pa-none" @click="emit('drill')">
            <div class="text-h5 text-weight-bold text-negative">{{ metrics.error_runs }}</div>
          </q-btn>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">错误率</div>
          <q-btn flat dense no-caps class="q-pa-none" @click="emit('drill')">
            <div class="text-h5 text-weight-bold">{{ formatPercent(metrics.error_rate) }}</div>
          </q-btn>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">成功率</div>
          <q-btn flat dense no-caps class="q-pa-none" @click="emit('drill')">
            <div class="text-h5 text-weight-bold">{{ formatPercent(metrics.success_rate) }}</div>
          </q-btn>
        </div>
      </div>
      <div class="text-caption text-grey-7 q-mt-sm">{{ drillHint }}</div>
    </q-card-section>
    <q-card-section v-else-if="loading">
      <q-skeleton type="rect" height="72px" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { RunnerMetricsSummary } from "../../features/monitor/types";

const props = withDefaults(
  defineProps<{
    metrics: RunnerMetricsSummary | null;
    loading: boolean;
    windowMinutes: number;
    variant?: "monitor" | "overview";
    scopeHint?: string;
  }>(),
  { variant: "monitor", scopeHint: "" }
);

const emit = defineEmits<{
  "update:windowMinutes": [value: number];
  refresh: [];
  drill: [];
}>();

const panelClass = computed(() =>
  props.variant === "overview" ? "overview-panel q-mb-md" : "monitor-card q-mb-md"
);

const drillHint = computed(() =>
  props.variant === "overview"
    ? "点击指标下钻到 Monitor → Runs（Traces）"
    : "点击指标下钻到 Runs（Traces）列表"
);

const windowOptions = [
  { label: "15 分钟", value: 15 },
  { label: "1 小时", value: 60 },
  { label: "6 小时", value: 360 },
  { label: "24 小时", value: 1440 }
];

function formatPercent(v: number): string {
  if (!Number.isFinite(v)) return "-";
  return `${(v * 100).toFixed(1)}%`;
}
</script>

<style scoped lang="sass">
.runner-metrics__grid
  margin-bottom: 0
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr))

.runner-metrics__body--compact
  padding-top: 12px
  padding-bottom: 12px
</style>
