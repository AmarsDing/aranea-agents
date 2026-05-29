<template>
  <q-dialog v-model="open" persistent>
    <q-card class="graph-run-dialog app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head">
        <div class="app-glass-dialog__title">执行 Graph</div>
        <div v-if="graphName" class="app-glass-dialog__subtitle">为 {{ graphName }} 启动一次执行</div>
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
        <q-input v-model="sessionId" class="app-field-md" dense outlined label="Session ID" hint="关联的会话 ID" />
        <q-input v-model="initialState" class="app-field-long" dense outlined autogrow type="textarea" label="初始状态 (JSON)" hint="可选，JSON 格式的初始状态" />
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
const sessionId = defineModel<string>("sessionId", { required: true });
const initialState = defineModel<string>("initialState", { required: true });

defineProps<{
  graphName?: string;
  loading: boolean;
}>();

defineEmits<{
  submit: [];
}>();
</script>
