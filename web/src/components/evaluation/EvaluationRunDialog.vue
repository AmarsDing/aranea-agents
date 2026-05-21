<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="text-h6">启动评估</q-card-section>
      <q-card-section class="app-dialog-body q-gutter-md q-pt-none">
        <q-select
          :model-value="agentId"
          class="app-field-md"
          dense
          outlined
          emit-value
          map-options
          label="Agent"
          :options="agentOptions"
          @update:model-value="$emit('update:agentId', String($event ?? ''))"
        />
        <q-input
          :model-value="metrics"
          class="app-field-long"
          dense
          outlined
          label="指标（逗号分隔，留空=全部）"
          @update:model-value="$emit('update:metrics', String($event ?? ''))"
        />
        <q-input
          :model-value="numRuns"
          class="app-field-sm"
          dense
          outlined
          type="number"
          min="1"
          max="10"
          label="MultiRun 次数"
          hint="AgentEvaluator 重复运行次数（1–10）"
          @update:model-value="$emit('update:numRuns', Number($event) || 1)"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="取消" @click="$emit('update:open', false)" />
        <q-btn color="primary" unelevated no-caps label="运行" :loading="loading" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{
  open: boolean;
  agentId: string;
  metrics: string;
  numRuns: number;
  loading: boolean;
  agentOptions: { label: string; value: string }[];
}>();
defineEmits<{
  "update:open": [value: boolean];
  "update:agentId": [value: string];
  "update:metrics": [value: string];
  "update:numRuns": [value: number];
  submit: [];
}>();
</script>
