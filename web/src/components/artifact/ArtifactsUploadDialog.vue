// Container: approved — artifact upload dialog; controlled by page composable.
<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="text-h6">上传制品</q-card-section>
      <q-card-section class="app-dialog-body q-gutter-md q-pt-none">
        <q-input
          :model-value="sessionId"
          class="app-field-md"
          dense
          outlined
          label="Session ID"
          @update:model-value="$emit('update:sessionId', String($event ?? ''))"
        />
        <q-input
          :model-value="name"
          class="app-field-md"
          dense
          outlined
          label="文件名"
          @update:model-value="$emit('update:name', String($event ?? ''))"
        />
        <q-input
          :model-value="mimeType"
          class="app-field-sm"
          dense
          outlined
          label="MIME"
          placeholder="application/octet-stream"
          @update:model-value="$emit('update:mimeType', String($event ?? ''))"
        />
        <q-file :model-value="file" label="选择文件" outlined dense @update:model-value="$emit('update:file', $event)" />
        <div class="text-caption text-grey-7">{{ maxSizeHint }}</div>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="取消" @click="$emit('update:open', false)" />
        <q-btn color="primary" unelevated no-caps label="上传" :loading="loading" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{
  open: boolean;
  loading: boolean;
  file: File | null;
  sessionId: string;
  name: string;
  mimeType: string;
  maxSizeHint: string;
}>();

defineEmits<{
  "update:open": [value: boolean];
  "update:file": [value: File | null];
  "update:sessionId": [value: string];
  "update:name": [value: string];
  "update:mimeType": [value: string];
  submit: [];
}>();
</script>
