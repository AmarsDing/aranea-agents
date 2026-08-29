<template>
  <div :class="['graph-run-sidebar', { 'is-dark': isDark, 'graph-run-sidebar--embedded': embedded }]">
    <div class="graph-run-sidebar__header row items-center q-gutter-sm">
      <div class="graph-run-sidebar__title">{{ t('graphs.runSidebarTitle') }}</div>
      <q-badge v-if="streamConnected" rounded color="positive">{{ t('graphs.runSidebarLive') }}</q-badge>
      <q-space />
      <AppStatusChip :status="displayStatus" />
    </div>

    <template v-if="execution">
      <div class="graph-run-sidebar__section q-gutter-sm">
        <div class="text-caption app-text-secondary">{{ t('graphs.runSidebarExecId') }}</div>
        <div class="graph-run-sidebar__mono">{{ execution.executionId }}</div>
        <div v-if="execution.interruptNode" class="text-caption app-text-secondary q-mt-sm">
          {{ t('graphs.runSidebarInterruptNode') }}
        </div>
        <div v-if="execution.interruptNode" class="text-body2">{{ execution.interruptNode }}</div>
        <div v-if="execution.startedAt" class="text-caption app-text-secondary q-mt-sm">
          {{ t('graphs.runSidebarStartedAt') }}
        </div>
        <div v-if="execution.startedAt" class="text-body2">{{ formatTime(execution.startedAt) }}</div>
        <div v-if="execution.finishedAt" class="text-caption app-text-secondary q-mt-sm">
          {{ t('graphs.runSidebarFinishedAt') }}
        </div>
        <div v-if="execution.finishedAt" class="text-body2">{{ formatTime(execution.finishedAt) }}</div>
      </div>

      <q-separator class="q-my-md" />

      <div v-if="executionSummary" class="graph-run-sidebar__section q-mb-md">
        <div class="graph-run-sidebar__title q-mb-sm">{{ t('graphs.runSidebarSummary') }}</div>
        <div class="row q-col-gutter-sm">
          <div class="col-6">
            <div class="text-caption app-text-secondary">{{ t('graphs.runSidebarTotalSteps') }}</div>
            <div class="text-body2">{{ executionSummary.totalSteps }}</div>
          </div>
          <div class="col-6">
            <div class="text-caption app-text-secondary">{{ t('graphs.runSidebarDuration') }}</div>
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
        <div class="graph-run-sidebar__title row items-center q-mb-sm">
          <span>{{ t('graphs.runSidebarStepsTimeline') }}</span>
          <q-space />
          <q-btn
            v-if="execution.steps.length > 1"
            flat
            dense
            no-caps
            size="11px"
            :label="allExpanded ? t('graphs.runSidebarCollapseAll') : t('graphs.runSidebarExpandAll')"
            @click="toggleAll"
          />
        </div>
        <q-timeline v-if="execution.steps.length" color="primary" dense>
          <q-timeline-entry
            v-for="step in execution.steps"
            :key="step.stepIndex"
            :title="step.nodeId"
            :subtitle="formatTime(step.timestamp)"
            :icon="stepIcon(step.status)"
            :color="stepColor(step.status)"
            :class="[
              'graph-run-step',
              {
                'graph-run-step--error': step.status === 'failed' || step.status === 'error',
                'graph-run-step--expanded': expandedSteps.has(step.stepIndex),
              },
            ]"
          >
            <div class="graph-run-step__header row items-center no-wrap">
              <q-badge
                outline
                :color="stepColor(step.status)"
                :label="step.status"
                class="graph-run-step__status-badge"
              />
              <q-space />
              <q-btn
                flat
                dense
                round
                size="xs"
                :icon="expandedSteps.has(step.stepIndex) ? 'expand_less' : 'expand_more'"
                @click="toggleStep(step.stepIndex)"
              />
            </div>
            <div v-if="step.error" class="graph-run-step__error q-mt-xs">{{ step.error }}</div>
            <div v-if="expandedSteps.has(step.stepIndex)" class="graph-run-step__detail q-mt-sm">
              <div v-if="step.inputState" class="q-mb-sm">
                <div class="text-caption app-text-secondary">{{ t('graphs.runSidebarInputState') }}</div>
                <JsonCodeViewer :text="formatJson(step.inputState)" :show-toolbar="false" scroll-height="200px" />
              </div>
              <div v-if="step.outputState">
                <div class="text-caption app-text-secondary">{{ t('graphs.runSidebarOutputState') }}</div>
                <JsonCodeViewer :text="formatJson(step.outputState)" :show-toolbar="false" scroll-height="200px" />
              </div>
            </div>
          </q-timeline-entry>
        </q-timeline>
        <div v-else class="text-caption app-text-secondary">{{ t('graphs.runSidebarNoSteps') }}</div>
      </div>
    </template>
    <q-spinner v-else color="primary" size="32px" />
  </div>
</template>

<script setup lang="ts">
import { reactive, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import JsonCodeViewer from '../common/JsonCodeViewer.vue';
import AppStatusChip from '../common/AppStatusChip.vue';
import type { GraphExecution, GraphRunExecutionSummary } from '../../features/graph/types';
import { formatTime, stepIcon, stepColor } from '../../features/graph/utils';

const { t } = useI18n();

const props = defineProps<{
  execution: GraphExecution | null;
  executionSummary: GraphRunExecutionSummary | null;
  displayStatus: string;
  streamConnected: boolean;
  isDark: boolean;
  embedded?: boolean;
}>();

const expandedSteps = reactive(new Set<number>());

const allExpanded = computed(() => {
  if (!props.execution?.steps.length) return false;
  return props.execution.steps.every((s) => expandedSteps.has(s.stepIndex));
});

function toggleStep(index: number) {
  if (expandedSteps.has(index)) {
    expandedSteps.delete(index);
  } else {
    expandedSteps.add(index);
  }
}

function toggleAll() {
  if (!props.execution?.steps.length) return;
  if (allExpanded.value) {
    expandedSteps.clear();
  } else {
    for (const s of props.execution.steps) {
      expandedSteps.add(s.stepIndex);
    }
  }
}

function formatDurationMs(ms: number) {
  if (!ms) return '—';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

const JSON_PREVIEW_MAX = 4096;

function formatJson(state: Record<string, unknown> | undefined): string {
  if (!state) return '';
  try {
    const raw = JSON.stringify(state, null, 2);
    return raw.length > JSON_PREVIEW_MAX
      ? `${raw.slice(0, JSON_PREVIEW_MAX)}\n${t('graphs.runSidebarJsonTruncated', { count: raw.length })}`
      : raw;
  } catch {
    return String(state);
  }
}
</script>
