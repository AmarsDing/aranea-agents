<template>
  <div class="monitor-log-stream-panel">
    <q-tabs v-model="subTab" dense align="left" no-caps class="monitor-log-stream-tabs">
      <q-tab name="flow" icon="timeline" label="流程日志" />
      <q-tab name="process" icon="terminal" label="进程日志" />
    </q-tabs>
    <q-tab-panels v-model="subTab" animated class="monitor-log-stream-panels">
      <q-tab-panel name="flow" class="q-pa-none">
        <FlowLogStream @clear="$emit('clearFlow')" />
      </q-tab-panel>
      <q-tab-panel name="process" class="q-pa-none">
        <ProcessLogStream />
      </q-tab-panel>
    </q-tab-panels>
  </div>
</template>

<script setup lang="ts">
import { useMonitorLogStreamPanel } from '../../features/monitor/useMonitorLogStreamPanel';
import FlowLogStream from './FlowLogStream.vue';
import ProcessLogStream from './ProcessLogStream.vue';

const { subTab } = useMonitorLogStreamPanel();

defineEmits<{
  clearFlow: [];
}>();
</script>
