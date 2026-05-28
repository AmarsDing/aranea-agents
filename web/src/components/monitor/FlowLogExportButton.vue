<template>
  <q-btn flat icon="download" label="导出 JSONL" :disable="!lines.length" @click="onExport" />
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
    Notify.create({ message: "暂无流程日志可导出", color: "warning", position: "top" });
    return;
  }
  downloadFlowDiagnosticJsonl(props.traceId, props.lines);
  Notify.create({ message: "已下载流程诊断 JSONL", color: "positive", position: "top" });
}
</script>
