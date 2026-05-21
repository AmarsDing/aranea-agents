<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="row items-center justify-between">
        <div class="text-h6">新建知识库集合</div>
        <q-btn flat round dense icon="close" v-close-popup />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog-body q-gutter-md">
        <q-input :model-value="name" class="app-field-md" dense outlined label="名称" @update:model-value="$emit('update:name', String($event ?? ''))" />
        <q-input
          :model-value="description"
          class="app-field-long"
          dense
          outlined
          label="描述"
          @update:model-value="$emit('update:description', String($event ?? ''))"
        />
        <q-input
          :model-value="embeddingModel"
          class="app-field-md"
          dense
          outlined
          label="Embedding 模型"
          hint="例如 text-embedding-3-small"
          @update:model-value="$emit('update:embeddingModel', String($event ?? ''))"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="取消" v-close-popup />
        <q-btn color="primary" unelevated no-caps label="创建" :loading="loading" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{ open: boolean; name: string; description: string; embeddingModel: string; loading: boolean }>();
defineEmits<{
  "update:open": [value: boolean];
  "update:name": [value: string];
  "update:description": [value: string];
  "update:embeddingModel": [value: string];
  submit: [];
}>();
</script>
