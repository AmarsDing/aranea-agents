<template>
  <q-btn flat icon="download" label="导出 JSONL" :disable="!lines.length" @click="$emit('export')" />
</template>

<script setup lang="ts">
import type { MonitorLogLine } from "../../features/monitor/types";

defineProps<{
  traceId: string;
  lines: MonitorLogLine[];
}>();

defineEmits<{
  export: [];
}>();
</script>
