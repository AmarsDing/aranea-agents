<template>
  <q-dialog v-model="open" persistent>
    <q-card class="graph-run-dialog app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head">
        <div class="app-glass-dialog__title">执行 Graph</div>
        <div v-if="graphName" class="app-glass-dialog__subtitle">为 {{ graphName }} 启动一次执行</div>
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
        <q-input :model-value="sessionId" class="app-field-md" dense outlined label="Session ID" hint="关联的会话 ID" @update:model-value="$emit('update:sessionId', $event)" />
        <q-input :model-value="initialState" class="app-field-long" dense outlined autogrow type="textarea" label="初始状态 (JSON)" hint="可选，JSON 格式的初始状态" @update:model-value="$emit('update:initialState', $event)" />
      </q-card-section>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn flat rounded label="取消" @click="open = false" />
        <q-btn color="primary" rounded unelevated label="执行" :loading="loading" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
const open = defineModel<boolean>({ required: true });

defineProps<{
  graphName?: string;
  sessionId: string;
  initialState: string;
  loading: boolean;
}>();

defineEmits<{
  'update:sessionId': [value: string];
  'update:initialState': [value: string];
  submit: [];
}>();
</script>
