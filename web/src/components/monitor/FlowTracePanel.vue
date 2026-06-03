<template>
  <div class="app-flow-trace-panel flow-trace-panel">
    <div v-if="!sortedLines.length" class="text-caption text-grey-7 q-pa-md">
      暂无流程日志。保持详情打开并运行一次对话，或在日志 Tab 查看全局流。
    </div>
    <div
      v-for="line in sortedLines"
      :key="`${line.id}-${line.time}`"
      class="app-flow-trace-row flow-trace-row"
      :class="rowClass(line)"
    >
      <div class="app-flow-trace-marker flow-trace-marker" />
      <div class="app-flow-trace-body flow-trace-body">
        <div class="row items-center q-gutter-xs">
          <span class="text-weight-bold">{{ line.title || line.step_id || 'step' }}</span>
          <q-badge dense :color="severityColor(line.severity)" :label="line.severity || 'info'" />
        </div>
        <div v-if="showMessage(line)" class="text-body2 q-mt-xs">{{ line.message }}</div>
        <div class="text-caption text-grey-7 q-mt-xs">
          {{ line.time }}
          <span v-if="line.step_id"> / {{ line.step_id }}</span>
        </div>
        <div v-if="line.hint" class="text-caption text-warning q-mt-xs">hint: {{ line.hint }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { MonitorLogLine } from '../../features/monitor/types';
import { sortFlowLogLines } from '../../features/monitor/flow';

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
