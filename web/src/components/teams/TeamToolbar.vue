<!--
  Team 域展示组件：仅 props / emits（vue-design.md §0.2）。
  路径约定：vue-design.md §2 → `web/src/components/teams/`。
-->
<template>
  <q-card flat bordered :class="['teams-toolbar', { 'is-dark': isDark }]">
    <q-card-section class="row q-col-gutter-md items-center">
      <q-input :model-value="search" class="col-12 col-md-5 team-control" dense outlined clearable debounce="250" placeholder="搜索 Team / Key / 说明" @update:model-value="$emit('update:search', String($event ?? ''))">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select :model-value="modeFilter" class="col-12 col-md-3 team-control" dense outlined clearable emit-value map-options label="编排模式" :options="modeOptions" @update:model-value="$emit('update:modeFilter', String($event ?? ''))" />
      <q-select :model-value="statusFilter" class="col-12 col-md-2 team-control" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" @update:model-value="$emit('update:statusFilter', String($event ?? ''))" />
      <div class="col-12 col-md-2 row justify-end">
        <q-btn flat rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { modeOptions, statusOptions } from "./teamUtils";

defineProps<{
  search: string;
  modeFilter: string;
  statusFilter: string;
  loading: boolean;
  isDark: boolean;
}>();

defineEmits<{
  "update:search": [value: string];
  "update:modeFilter": [value: string];
  "update:statusFilter": [value: string];
  refresh: [];
}>();
</script>

<style scoped>
.teams-toolbar {
  border: 1px solid rgb(15 23 42 / 8%);
  border-radius: 24px;
  background: rgb(255 255 255 / 86%);
  box-shadow: 0 18px 48px rgb(16 24 40 / 6%);
  backdrop-filter: blur(16px);
}

.team-control :deep(.q-field__control) {
  border-radius: 16px;
  background: var(--color-on-accent);
}

.teams-toolbar.is-dark {
  border-color: rgb(148 163 184 / 16%);
  background: rgb(17 24 39 / 90%);
  box-shadow: 0 14px 38px rgb(0 0 0 / 32%);
}

.teams-toolbar.is-dark .team-control :deep(.q-field__control) {
  border-color: rgb(148 163 184 / 14%);
  background: rgb(30 41 59 / 76%);
}
</style>
