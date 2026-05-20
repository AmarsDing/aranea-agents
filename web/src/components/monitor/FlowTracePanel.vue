<template>
  <div class="flow-trace-panel">
    <div v-if="!sortedLines.length" class="text-caption text-grey-7 q-pa-md">
      No flow logs yet. Keep this detail open and run a chat turn, or use the Logs tab for the global stream.
    </div>
    <div
      v-for="line in sortedLines"
      :key="`${line.id}-${line.time}`"
      class="flow-trace-row"
      :class="rowClass(line)"
    >
      <div class="flow-trace-marker" />
      <div class="flow-trace-body">
        <div class="row items-center q-gutter-xs">
          <span class="text-weight-bold">{{ line.title || line.step_id || "step" }}</span>
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
import { computed } from "vue";
import type { MonitorLogLine } from "../../features/monitor/types";
import { sortFlowLogLines } from "../../features/monitor/flow";

const props = defineProps<{
  lines: MonitorLogLine[];
}>();

const sortedLines = computed(() => sortFlowLogLines(props.lines.filter((l) => l.kind === "flow")));

function rowClass(line: MonitorLogLine): string {
  return `flow-trace-row--${(line.severity || "info").toLowerCase()}`;
}

function severityColor(severity?: string): string {
  switch ((severity || "").toLowerCase()) {
    case "critical":
    case "error":
      return "negative";
    case "warn":
      return "warning";
    case "ok":
      return "positive";
    default:
      return "info";
  }
}

function showMessage(line: MonitorLogLine): boolean {
  const msg = (line.message || "").trim();
  const title = (line.title || "").trim();
  return Boolean(msg && msg !== title);
}
</script>

<style scoped>
.flow-trace-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 420px;
  overflow-y: auto;
  padding: 4px 0;
}
.flow-trace-row {
  display: flex;
  gap: 12px;
  align-items: flex-start;
}
.flow-trace-marker {
  width: 4px;
  min-height: 48px;
  border-radius: 2px;
  background: rgba(0, 0, 0, 0.12);
  flex-shrink: 0;
}
.flow-trace-row--ok .flow-trace-marker,
.flow-trace-row--info .flow-trace-marker {
  background: var(--color-success, #21ba45);
}
.flow-trace-row--warn .flow-trace-marker {
  background: var(--color-warning, #f2c037);
}
.flow-trace-row--error .flow-trace-marker,
.flow-trace-row--critical .flow-trace-marker {
  background: var(--color-danger, #c10015);
}
.flow-trace-body {
  flex: 1;
  min-width: 0;
}
</style>
