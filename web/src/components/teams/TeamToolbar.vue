<!--
  Team 域展示组件：仅 props / emits（vue-design.md §0.2）。
  路径约定：vue-design.md §2 → `web/src/components/teams/`。
-->
<template>
  <q-card flat bordered :class="['teams-toolbar', { 'is-dark': isDark }]">
    <q-card-section class="app-form-field-grid items-end">
      <q-input :model-value="search" class="app-field-md team-control" dense outlined clearable debounce="250" placeholder="搜索 Team / Key / 说明" @update:model-value="$emit('update:search', String($event ?? ''))">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select :model-value="modeFilter" class="team-control" dense outlined clearable emit-value map-options label="编排模式" :options="modeOptions" @update:model-value="$emit('update:modeFilter', String($event ?? ''))" />
      <q-select :model-value="statusFilter" class="team-control" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" @update:model-value="$emit('update:statusFilter', String($event ?? ''))" />
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn flat rounded no-caps color="primary" icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
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
  border: 1px solid var(--glass-border);
  border-radius: 24px;
  background: var(--glass-surface);
  box-shadow: var(--shadow-entity-panel);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
}

.team-control :deep(.q-field__control) {
  border-radius: 16px;
  background: var(--glass-elevated);
}

.teams-toolbar.is-dark {
  border-color: var(--glass-border);
  background: var(--glass-surface);
  box-shadow: var(--shadow-entity-panel-dark);
}

.teams-toolbar.is-dark .team-control :deep(.q-field__control) {
  border-color: var(--glass-border);
  background: color-mix(in srgb, var(--glass-elevated) 88%, transparent);
}

.teams-toolbar.is-dark .team-control :deep(.q-field__native),
.teams-toolbar.is-dark .team-control :deep(.q-field__input),
.teams-toolbar.is-dark .team-control :deep(.q-field__label) {
  color: var(--color-text-primary);
}
</style>
