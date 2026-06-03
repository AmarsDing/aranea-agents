<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm">
      <q-card-section class="text-h6">新建数据集</q-card-section>
      <q-card-section class="app-dialog-body q-gutter-md q-pt-none">
        <q-input
          :model-value="name"
          class="app-field-md"
          dense
          outlined
          label="名称"
          :rules="[(val) => !!val?.trim() || '名称不能为空']"
          @update:model-value="$emit('update:name', String($event ?? ''))"
        />
        <q-input
          :model-value="description"
          class="app-field-long"
          dense
          outlined
          type="textarea"
          label="描述"
          autogrow
          @update:model-value="$emit('update:description', String($event ?? ''))"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="取消" @click="$emit('update:open', false)" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          label="创建"
          :loading="loading"
          :disable="!name?.trim()"
          @click="$emit('submit')"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
defineProps<{ open: boolean; name: string; description: string; loading: boolean }>();
defineEmits<{
  'update:open': [value: boolean];
  'update:name': [value: string];
  'update:description': [value: string];
  submit: [];
}>();
</script>
