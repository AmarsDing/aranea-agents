<template>
  <div class="app-flow-trace-panel flow-trace-panel">
    <div v-if="!sortedLines.length" class="app-flow-trace-empty column items-center q-pa-lg">
      <q-icon name="timeline" size="32px" class="q-mb-xs" />
      <span class="text-caption">{{ t('monitorPage.traces.flowEmpty') }}</span>
    </div>
    <div
      v-for="line in sortedLines"
      :key="`${line.id}-${line.time}`"
      class="app-flow-trace-row flow-trace-row"
      :class="rowClass(line)"
    >
      <div class="app-flow-trace-gutter">
        <span class="app-flow-trace-dot" />
        <span class="app-flow-trace-line" />
      </div>
      <div class="app-flow-trace-body flow-trace-body">
        <div class="row items-center q-gutter-xs no-wrap">
          <span class="text-weight-medium ellipsis">{{ line.title || line.step_id || 'step' }}</span>
          <q-badge dense outline :color="severityColor(line.severity)" :label="line.severity || 'info'" />
        </div>
        <div v-if="showMessage(line)" class="text-body2 q-mt-xs app-flow-trace-message">{{ line.message }}</div>
        <div class="app-flow-trace-meta q-mt-xs">
          <span class="text-mono">{{ line.time }}</span>
          <span v-if="line.step_id" class="text-mono">{{ line.step_id }}</span>
        </div>
        <div v-if="line.hint" class="app-flow-trace-hint text-caption q-mt-xs">hint: {{ line.hint }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MonitorLogLine } from '../../features/monitor/types';
import { sortFlowLogLines } from '../../features/monitor/flow';

const { t } = useI18n();

const props = defineProps<{
  lines: MonitorLogLine[];
}>();

const sortedLines = computed(() => sortFlowLogLines(props.lines.filter((l) => l.kind === 'flow')));

function rowClass(line: MonitorLogLine): string {
  return `app-flow-trace-row--${(line.severity || 'info').toLowerCase()} flow-trace-row--${(line.severity || 'info').toLowerCase()}`;
}

function severityColor(severity?: string): string {
  switch ((severity || '').toLowerCase()) {
    case 'critical':
    case 'error':
      return 'negative';
    case 'warn':
      return 'warning';
    case 'ok':
      return 'positive';
    default:
      return 'info';
  }
}

function showMessage(line: MonitorLogLine): boolean {
  const msg = (line.message || '').trim();
  const title = (line.title || '').trim();
  return Boolean(msg && msg !== title);
}
</script>
