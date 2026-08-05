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
          <q-btn flat dense no-caps class="q-pa-none runner-metrics__drill" @click="emit('drill')">
            <div class="text-h5 text-weight-bold text-primary">{{ metrics.total_runs }}</div>
            <q-tooltip>{{ drillHint }}</q-tooltip>
          </q-btn>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">错误次数</div>
          <q-btn flat dense no-caps class="q-pa-none runner-metrics__drill" @click="emit('drill')">
            <div class="text-h5 text-weight-bold text-negative">{{ metrics.error_runs }}</div>
            <q-tooltip>{{ drillHint }}</q-tooltip>
          </q-btn>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">错误率</div>
          <q-btn flat dense no-caps class="q-pa-none runner-metrics__drill" @click="emit('drill')">
            <div class="text-h5 text-weight-bold">{{ formatPercent(metrics.error_rate) }}</div>
            <q-tooltip>{{ drillHint }}</q-tooltip>
          </q-btn>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">成功率</div>
          <q-btn flat dense no-caps class="q-pa-none runner-metrics__drill" @click="emit('drill')">
            <div class="text-h5 text-weight-bold">{{ formatPercent(metrics.success_rate) }}</div>
            <q-tooltip>{{ drillHint }}</q-tooltip>
          </q-btn>
        </div>
        <div v-if="metrics.p50_duration_ms != null" class="app-metrics-grid__item">
          <div class="text-caption text-grey">P50 延迟</div>
          <div class="text-h6 text-weight-bold">{{ formatLatency(metrics.p50_duration_ms) }}</div>
        </div>
        <div v-if="metrics.p95_duration_ms != null" class="app-metrics-grid__item">
          <div class="text-caption text-grey">P95 延迟</div>
          <div class="text-h6 text-weight-bold">{{ formatLatency(metrics.p95_duration_ms) }}</div>
        </div>
        <div v-if="metrics.p99_duration_ms != null" class="app-metrics-grid__item">
          <div class="text-caption text-grey">P99 延迟</div>
          <div class="text-h6 text-weight-bold">{{ formatLatency(metrics.p99_duration_ms) }}</div>
        </div>
        <div v-if="metrics.avg_duration_ms != null" class="app-metrics-grid__item">
          <div class="text-caption text-grey">平均延迟</div>
          <div class="text-h6 text-weight-bold">{{ formatLatency(metrics.avg_duration_ms) }}</div>
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
import { computed } from 'vue';
import type { RunnerMetricsSummary } from '../../features/monitor/types';

const props = withDefaults(
  defineProps<{
    metrics: RunnerMetricsSummary | null;
    loading: boolean;
    windowMinutes: number;
    variant?: 'monitor' | 'overview';
    scopeHint?: string;
  }>(),
  { variant: 'monitor', scopeHint: '' },
);

const emit = defineEmits<{
  'update:windowMinutes': [value: number];
  refresh: [];
  drill: [];
}>();

const panelClass = computed(() => (props.variant === 'overview' ? 'overview-panel q-mb-md' : 'monitor-card q-mb-md'));

const drillHint = computed(() =>
  props.variant === 'overview' ? '点击指标下钻到 Monitor → Runs（Traces）' : '点击指标下钻到 Runs（Traces）列表',
);

const windowOptions = [
  { label: '15 分钟', value: 15 },
  { label: '1 小时', value: 60 },
  { label: '6 小时', value: 360 },
  { label: '24 小时', value: 1440 },
];

function formatPercent(v: number): string {
  if (!Number.isFinite(v)) return '-';
  return `${(v * 100).toFixed(1)}%`;
}

function formatLatency(v?: number): string {
  if (v == null || !Number.isFinite(v)) return '-';
  if (v >= 1000) return `${(v / 1000).toFixed(1)}s`;
  return `${Math.round(v)}ms`;
}
</script>

<style scoped>
/* 下钻可点击感知：hover 时下划线 + 手型（纯 flat 按钮默认无可点击提示） */
.runner-metrics__drill {
  cursor: pointer;
  border-radius: 4px;
}
.runner-metrics__drill:hover .text-h5 {
  text-decoration: underline;
  text-underline-offset: 4px;
}
</style>
