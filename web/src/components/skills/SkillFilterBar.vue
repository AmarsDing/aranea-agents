<template>
  <AppPageToolbar class="skill-filter-card">
    <q-input
      :model-value="search"
      class="app-page-toolbar__search"
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
      class="app-page-toolbar__field"
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
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="状态"
      :options="statusOptions"
      @update:model-value="emit('update:status', String($event ?? ''))"
    />
    <q-select
      :model-value="syncOrigin"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="来源"
      :options="originOptions"
      @update:model-value="emit('update:syncOrigin', String($event ?? ''))"
    />
    <q-select
      :model-value="filesystemMissing"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="磁盘状态"
      :options="filesystemOptions"
      @update:model-value="emit('update:filesystemMissing', $event as boolean | null)"
    />
    <template #actions>
      <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="emit('reset')" />
      <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="emit('refresh')" />
    </template>
  </AppPageToolbar>
</template>

<script setup lang="ts">
import AppPageToolbar from "../layout/AppPageToolbar.vue";

defineProps<{
  search: string;
  enabled: boolean | null;
  status: string;
  syncOrigin: string;
  filesystemMissing: boolean | null;
  loading?: boolean;
}>();

const emit = defineEmits<{
  "update:search": [value: string];
  "update:enabled": [value: boolean | null];
  "update:status": [value: string];
  "update:syncOrigin": [value: string];
  "update:filesystemMissing": [value: boolean | null];
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

const originOptions = [
  { label: "磁盘导入", value: "filesystem" },
  { label: "ZIP 导入", value: "import" },
  { label: "手动创建", value: "manual" }
];

const filesystemOptions = [
  { label: "磁盘缺失", value: true },
  { label: "磁盘正常", value: false }
];
</script>
