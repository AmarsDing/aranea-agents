<template>
  <RunnerMetricsPanel
    variant="monitor"
    :metrics="metrics"
    :loading="loading"
    :window-minutes="windowMinutes"
    @update:window-minutes="$emit('update:windowMinutes', $event)"
    @refresh="$emit('refresh')"
    @drill="$emit('drill')"
  />
</template>

<script setup lang="ts">
import RunnerMetricsPanel from './RunnerMetricsPanel.vue';
import type { RunnerMetricsSummary } from '../../features/monitor/types';

defineProps<{
  metrics: RunnerMetricsSummary | null;
  loading: boolean;
  windowMinutes: number;
}>();

defineEmits<{
  'update:windowMinutes': [value: number];
  refresh: [];
  drill: [];
}>();
</script>
