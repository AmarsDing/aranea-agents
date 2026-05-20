<template>
  <q-btn flat icon="download" label="Export JSONL" :disable="!lines.length" @click="onExport" />
</template>

<script setup lang="ts">
import { Notify } from "quasar";
import type { MonitorLogLine } from "../../features/monitor/types";
import { downloadFlowDiagnosticJsonl } from "../../features/monitor/flow";

const props = defineProps<{
  traceId: string;
  lines: MonitorLogLine[];
}>();

function onExport() {
  if (!props.lines.length) {
    Notify.create({ message: "No flow logs to export", color: "warning", position: "top" });
    return;
  }
  downloadFlowDiagnosticJsonl(props.traceId, props.lines);
  Notify.create({ message: "Downloaded flow diagnostic JSONL", color: "positive", position: "top" });
}
</script>
