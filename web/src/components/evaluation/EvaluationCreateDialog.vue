<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card style="min-width: 400px">
      <q-card-section class="text-h6">新建数据集</q-card-section>
      <q-card-section class="q-gutter-md">
        <q-input :model-value="name" dense outlined label="名称" @update:model-value="$emit('update:name', String($event ?? ''))" />
        <q-input
          :model-value="description"
          dense
          outlined
          type="textarea"
          label="描述"
          autogrow
          @update:model-value="$emit('update:description', String($event ?? ''))"
        />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat label="取消" @click="$emit('update:open', false)" />
        <q-btn color="primary" unelevated label="创建" :loading="loading" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{ open: boolean; name: string; description: string; loading: boolean }>();
defineEmits<{
  "update:open": [value: boolean];
  "update:name": [value: string];
  "update:description": [value: string];
  submit: [];
}>();
</script>
