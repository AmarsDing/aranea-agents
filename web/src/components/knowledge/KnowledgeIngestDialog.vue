<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card style="min-width: 480px; max-width: 94vw">
      <q-card-section class="text-h6">文档入库</q-card-section>
      <q-card-section class="q-gutter-md">
        <q-input :model-value="source" dense outlined label="来源标识" @update:model-value="$emit('update:source', String($event ?? ''))" />
        <q-input
          :model-value="mimeType"
          dense
          outlined
          label="MIME 类型"
          placeholder="text/plain"
          @update:model-value="$emit('update:mimeType', String($event ?? ''))"
        />
        <q-file :model-value="file" label="选择文件" outlined dense accept=".txt,.md,.json,.csv" @update:model-value="$emit('update:file', $event)" />
        <q-input
          :model-value="text"
          dense
          outlined
          type="textarea"
          label="或粘贴文本"
          autogrow
          @update:model-value="$emit('update:text', String($event ?? ''))"
        />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat label="取消" @click="$emit('update:open', false)" />
        <q-btn color="primary" unelevated label="入库" :loading="loading" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{
  open: boolean;
  source: string;
  mimeType: string;
  text: string;
  file: File | null;
  loading: boolean;
}>();
defineEmits<{
  "update:open": [value: boolean];
  "update:source": [value: string];
  "update:mimeType": [value: string];
  "update:text": [value: string];
  "update:file": [file: File | null];
  submit: [];
}>();
</script>
