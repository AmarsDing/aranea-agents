<template>
  <RunnerMetricsPanel
    variant="monitor"
    :metrics="runnerMetrics"
    :loading="runnerLoading"
    :window-minutes="windowMinutes"
    @update:window-minutes="windowMinutes = $event"
    @refresh="reload"
    @drill="openRunsTab({ tab: 'traces' })"
  />
</template>

<script setup lang="ts">
import RunnerMetricsPanel from './RunnerMetricsPanel.vue';
import { useRunnerMetrics } from '../../features/monitor/useRunnerMetrics';
import { useMonitorRunNavigation } from '../../features/monitor/useMonitorRunNavigation';

const { runnerMetrics, runnerLoading, windowMinutes, reload } = useRunnerMetrics(60);
const { openRunsTab } = useMonitorRunNavigation();
</script>
