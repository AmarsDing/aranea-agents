<template>
  <RunnerMetricsPanel
    variant="overview"
    :metrics="runnerMetrics"
    :loading="runnerLoading"
    :window-minutes="windowMinutes"
    :scope-hint="scopeHint"
    @update:window-minutes="windowMinutes = $event"
    @refresh="reload"
    @drill="openRunsTab({ tab: 'traces' })"
  />
</template>

<script setup lang="ts">
import RunnerMetricsPanel from "../monitor/RunnerMetricsPanel.vue";
import { useRunnerMetrics } from "../../features/monitor/useRunnerMetrics";
import { useMonitorRunNavigation } from "../../features/monitor/useMonitorRunNavigation";

const scopeHint = "基于最近窗口内的 runner.completion 统计，与下方用量筛选时间范围独立。";

const { runnerMetrics, runnerLoading, windowMinutes, reload } = useRunnerMetrics(60);
const { openRunsTab } = useMonitorRunNavigation();
</script>
