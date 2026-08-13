<template>
  <div class="monitor-log-stream-panel">
    <q-tabs
      :model-value="subTab"
      dense
      align="left"
      no-caps
      class="monitor-log-stream-tabs"
      @update:model-value="$emit('update:subTab', $event)"
    >
      <q-tab name="flow" icon="timeline" :label="t('monitorPage.logs.tabFlow')" />
      <q-tab name="process" icon="terminal" :label="t('monitorPage.logs.tabProcess')" />
    </q-tabs>
    <q-tab-panels :model-value="subTab" animated class="monitor-log-stream-panels">
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
import { useI18n } from 'vue-i18n';
import FlowLogStream from './FlowLogStream.vue';
import ProcessLogStream from './ProcessLogStream.vue';

const { t } = useI18n();

defineProps<{
  subTab: 'flow' | 'process';
}>();

defineEmits<{
  'update:subTab': [value: 'flow' | 'process'];
  clearFlow: [];
}>();
</script>
