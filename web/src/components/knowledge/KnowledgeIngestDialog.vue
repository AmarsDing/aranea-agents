<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="row items-center justify-between">
        <div class="text-h6">文档入库</div>
        <q-btn flat round dense icon="close" v-close-popup />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body q-gutter-md">
        <q-input :model-value="source" class="app-field-md" dense outlined label="来源标识" @update:model-value="$emit('update:source', String($event ?? ''))" />
        <q-input
          :model-value="mimeType"
          class="app-field-sm"
          dense
          outlined
          label="MIME 类型"
          placeholder="text/plain"
          @update:model-value="$emit('update:mimeType', String($event ?? ''))"
        />
        <q-file
          :model-value="file"
          label="选择文件"
          outlined
          dense
          accept=".txt,.md,.json,.csv,.log,.html,.htm,.xml,.yaml,.yml,.toml,.pdf,.doc,.docx,.ppt,.pptx,.xls,.xlsx"
          hint="文本类型可在下方预览编辑；二进制（PDF/DOCX/…）按原字节上传，依赖后端解析支持"
          @update:model-value="$emit('update:file', $event)"
        />
        <q-input
          :model-value="text"
          class="app-field-long"
          dense
          outlined
          type="textarea"
          label="或粘贴文本"
          autogrow
          @update:model-value="$emit('update:text', String($event ?? ''))"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="取消" v-close-popup />
        <q-btn color="primary" unelevated no-caps label="入库" :loading="loading" @click="$emit('submit')" />
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
  "update:file": [value: File | null];
  submit: [];
}>();
</script>
