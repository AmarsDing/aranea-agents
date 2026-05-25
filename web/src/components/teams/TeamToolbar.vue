<!--
  Team 域展示组件：仅 props / emits（vue-design.md §0.2）。
  路径约定：vue-design.md §2 → `web/src/components/teams/`。
-->
<template>
  <q-card flat bordered :class="['app-entity-toolbar', { 'is-dark': isDark }]">
    <q-card-section class="app-entity-toolbar__body app-form-field-grid items-end">
      <q-input :model-value="search" class="app-field-md team-control" dense outlined clearable debounce="250" placeholder="搜索 Team / Key / 说明" @update:model-value="$emit('update:search', String($event ?? ''))">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select :model-value="modeFilter" class="team-control" dense outlined clearable emit-value map-options label="编排模式" :options="modeOptions" @update:model-value="$emit('update:modeFilter', String($event ?? ''))" />
      <q-select :model-value="statusFilter" class="team-control" dense outlined clearable emit-value map-options label="状态" :options="statusOptions" @update:model-value="$emit('update:statusFilter', String($event ?? ''))" />
      <q-select
        :model-value="industryFilter"
        class="team-control"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="行业"
        :options="industryOptions"
        @update:model-value="$emit('update:industryFilter', String($event ?? ''))"
      />
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
  industryFilter: string;
  industryOptions: Array<{ label: string; value: string }>;
  loading: boolean;
  isDark: boolean;
}>();

defineEmits<{
  "update:search": [value: string];
  "update:modeFilter": [value: string];
  "update:statusFilter": [value: string];
  "update:industryFilter": [value: string];
  refresh: [];
}>();
</script>
