<!--
  Team 域展示组件：仅 props / emits（aranea-frontend-guide SKILL §1 红线 #1）。
  路径约定：SKILL §3.3 → `web/src/components/teams/`。
-->
<template>
  <AppPageToolbar variant="entity" :is-dark="isDark">
    <q-input
      :model-value="search"
      class="app-page-toolbar__search team-control"
      dense
      outlined
      clearable
      debounce="250"
      placeholder="搜索 Team / Key / 说明"
      @update:model-value="$emit('update:search', String($event ?? ''))"
    >
      <template #prepend><q-icon name="search" /></template>
    </q-input>
    <q-select
      :model-value="modeFilter"
      class="app-page-toolbar__field team-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="编排模式"
      :options="modeOptions"
      @update:model-value="$emit('update:modeFilter', String($event ?? ''))"
    />
    <q-select
      :model-value="statusFilter"
      class="app-page-toolbar__field team-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="状态"
      :options="statusOptions"
      @update:model-value="$emit('update:statusFilter', String($event ?? ''))"
    />
    <q-select
      :model-value="industryFilter"
      class="app-page-toolbar__field team-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      label="行业"
      :options="industryOptions"
      @update:model-value="$emit('update:industryFilter', String($event ?? ''))"
    />
    <q-toggle
      :model-value="showOrchestrated"
      dense
      no-caps
      :label="t('teamsPage.showOrchestrated')"
      @update:model-value="$emit('update:showOrchestrated', Boolean($event))"
    >
      <q-tooltip>{{ t('teamsPage.showOrchestratedHint') }}</q-tooltip>
    </q-toggle>
    <template #actions>
      <q-btn
        flat
        rounded
        no-caps
        color="primary"
        icon="refresh"
        :label="t('common.refresh')"
        :loading="loading"
        @click="$emit('refresh')"
      />
    </template>
  </AppPageToolbar>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppPageToolbar from '../layout/AppPageToolbar.vue';
import { modeOptions, statusOptions } from './teamUtils';

const { t } = useI18n();

defineProps<{
  search: string;
  modeFilter: string;
  statusFilter: string;
  industryFilter: string;
  showOrchestrated: boolean;
  industryOptions: Array<{ label: string; value: string }>;
  loading: boolean;
  isDark: boolean;
}>();

defineEmits<{
  'update:search': [value: string];
  'update:modeFilter': [value: string];
  'update:statusFilter': [value: string];
  'update:industryFilter': [value: string];
  'update:showOrchestrated': [value: boolean];
  refresh: [];
}>();
</script>
