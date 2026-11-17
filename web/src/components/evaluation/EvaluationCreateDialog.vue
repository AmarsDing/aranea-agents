<template>
  <AppDialog
    :model-value="open"
    :title="title"
    size="sm"
    persistent
    :loading="loading"
    @update:model-value="$emit('update:open', $event)"
  >
    <div class="q-gutter-md">
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
    </div>
    <template #actions>
      <q-btn flat no-caps label="取消" @click="$emit('update:open', false)" />
      <q-btn
        color="primary"
        unelevated
        no-caps
        :label="submitLabel"
        :loading="loading"
        :disable="!name?.trim()"
        @click="$emit('submit')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import AppDialog from '../../components/common/AppDialog.vue';

withDefaults(
  defineProps<{
    open: boolean;
    name: string;
    description: string;
    loading: boolean;
    title?: string;
    submitLabel?: string;
  }>(),
  { title: '新建数据集', submitLabel: '创建' },
);
defineEmits<{
  'update:open': [value: boolean];
  'update:name': [value: string];
  'update:description': [value: string];
  submit: [];
}>();
</script>
