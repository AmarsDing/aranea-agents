<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">新建知识库集合</div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <q-input
            :model-value="name"
            class="app-field-md"
            dense
            outlined
            label="名称"
            @update:model-value="$emit('update:name', String($event ?? ''))"
          />
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
      </div>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps label="取消" />
        <q-btn color="primary" unelevated no-caps label="创建" :loading="loading" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{ open: boolean; name: string; description: string; embeddingModel: string; loading: boolean }>();
defineEmits<{
  'update:open': [value: boolean];
  'update:name': [value: string];
  'update:description': [value: string];
  'update:embeddingModel': [value: string];
  submit: [];
}>();
</script>
