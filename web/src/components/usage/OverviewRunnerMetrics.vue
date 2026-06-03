<template>
  <RunnerMetricsPanel
    variant="overview"
    :metrics="metrics"
    :loading="loading"
    :window-minutes="windowMinutes"
    :scope-hint="scopeHint"
    @update:window-minutes="emit('update:windowMinutes', $event)"
    @refresh="emit('refresh')"
    @drill="emit('drill')"
  />
</template>

<script setup lang="ts">
import RunnerMetricsPanel from '../monitor/RunnerMetricsPanel.vue';
import type { RunnerMetricsSummary } from '../../features/monitor/types';

defineProps<{
  metrics: RunnerMetricsSummary | null;
  loading: boolean;
  windowMinutes: number;
}>();

const scopeHint = '基于最近窗口内的 runner.completion 统计，与下方用量筛选时间范围独立。';

const emit = defineEmits<{
  'update:windowMinutes': [value: number];
  refresh: [];
  drill: [];
}>();
</script>
