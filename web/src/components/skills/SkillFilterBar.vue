<template>
  <q-card flat class="app-registry-panel skill-filter-card">
    <q-card-section class="app-form-field-grid app-registry-toolbar items-end">
      <q-input
        :model-value="search"
        class="app-field-md"
        dense
        outlined
        clearable
        debounce="350"
        label="搜索 Skill"
        placeholder="名称、Slug、描述…"
        @update:model-value="emit('update:search', String($event ?? ''))"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        :model-value="enabled"
        class="app-field-sm"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="启用状态"
        :options="enabledOptions"
        @update:model-value="emit('update:enabled', $event as boolean | null)"
      />
      <q-select
        :model-value="status"
        class="app-field-sm"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="状态"
        :options="statusOptions"
        @update:model-value="emit('update:status', String($event ?? ''))"
      />
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="emit('reset')" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="emit('refresh')" />
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
defineProps<{
  search: string;
  enabled: boolean | null;
  status: string;
  loading?: boolean;
}>();

const emit = defineEmits<{
  "update:search": [value: string];
  "update:enabled": [value: boolean | null];
  "update:status": [value: string];
  reset: [];
  refresh: [];
}>();

const enabledOptions = [
  { label: "仅启用", value: true },
  { label: "仅停用", value: false }
];

const statusOptions = [
  { label: "草稿", value: "draft" },
  { label: "已发布", value: "published" },
  { label: "已归档", value: "archived" }
];
</script>
