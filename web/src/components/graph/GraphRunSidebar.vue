<template>
  <div :class="['graph-run-sidebar', { 'is-dark': isDark, 'graph-run-sidebar--embedded': embedded }]">
    <div class="graph-run-sidebar__header row items-center q-gutter-sm">
      <div class="graph-run-sidebar__title">执行详情</div>
      <q-badge v-if="streamConnected" rounded color="positive">实时</q-badge>
      <q-space />
      <q-badge rounded :color="statusColor">{{ displayStatus }}</q-badge>
    </div>

    <template v-if="execution">
      <div class="graph-run-sidebar__section q-gutter-sm">
        <div class="text-caption app-text-secondary">执行 ID</div>
        <div class="graph-run-sidebar__mono">{{ execution.executionId }}</div>
        <div v-if="execution.interruptNode" class="text-caption app-text-secondary q-mt-sm">中断节点</div>
        <div v-if="execution.interruptNode" class="text-body2">{{ execution.interruptNode }}</div>
        <div v-if="execution.startedAt" class="text-caption app-text-secondary q-mt-sm">开始时间</div>
        <div v-if="execution.startedAt" class="text-body2">{{ formatTime(execution.startedAt) }}</div>
        <div v-if="execution.finishedAt" class="text-caption app-text-secondary q-mt-sm">结束时间</div>
        <div v-if="execution.finishedAt" class="text-body2">{{ formatTime(execution.finishedAt) }}</div>
      </div>

      <q-separator class="q-my-md" />

      <div v-if="executionSummary" class="graph-run-sidebar__section q-mb-md">
        <div class="graph-run-sidebar__title q-mb-sm">执行摘要</div>
        <div class="row q-col-gutter-sm">
          <div class="col-6">
            <div class="text-caption app-text-secondary">总步数</div>
            <div class="text-body2">{{ executionSummary.totalSteps }}</div>
          </div>
          <div class="col-6">
            <div class="text-caption app-text-secondary">耗时</div>
            <div class="text-body2">{{ formatDurationMs(executionSummary.durationMs) }}</div>
          </div>
        </div>
        <q-list v-if="executionSummary.nodes.length" dense bordered separator class="rounded-borders q-mt-sm">
          <q-item v-for="node in executionSummary.nodes" :key="node.nodeId">
            <q-item-section>
              <q-item-label>{{ node.nodeId }}</q-item-label>
              <q-item-label caption>{{ node.nodeType }} · {{ node.status }} · {{ node.durationMs }}ms</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
      </div>

      <div class="graph-run-sidebar__section">
        <div class="graph-run-sidebar__title q-mb-sm">步骤时间线</div>
        <q-timeline v-if="execution.steps.length" color="primary" dense>
          <q-timeline-entry
            v-for="step in execution.steps"
            :key="step.stepIndex"
            :title="step.nodeId"
            :subtitle="formatTime(step.timestamp)"
            :icon="stepIcon(step.status)"
            :color="stepColor(step.status)"
          >
            <div class="text-caption">{{ step.status }}</div>
            <div v-if="step.error" class="text-negative text-caption">{{ step.error }}</div>
          </q-timeline-entry>
        </q-timeline>
        <div v-else class="text-caption app-text-secondary">暂无步骤记录。</div>
      </div>
    </template>
    <q-spinner v-else color="primary" size="32px" />
  </div>
</template>

<script setup lang="ts">
import type { GraphExecution, GraphRunExecutionSummary } from "../../features/graph/types";

defineProps<{
  execution: GraphExecution | null;
  executionSummary: GraphRunExecutionSummary | null;
  displayStatus: string;
  statusColor: string;
  streamConnected: boolean;
  isDark: boolean;
  formatTime: (ts: string) => string;
  stepIcon: (status: string) => string;
  stepColor: (status: string) => string;
  embedded?: boolean;
}>();

function formatDurationMs(ms: number) {
  if (!ms) return "—";
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}
</script>
