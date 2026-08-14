<template>
  <AppPageToolbar dense>
    <q-input
      :model-value="search"
      class="app-page-toolbar__search app-glass-control"
      dense
      outlined
      clearable
      debounce="350"
      :placeholder="$t('toolsPage.filters.searchPlaceholder')"
      @update:model-value="$emit('update:search', String($event ?? ''))"
    >
      <template #prepend><q-icon name="search" /></template>
    </q-input>
    <q-select
      :model-value="category"
      class="app-page-toolbar__field app-glass-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="$t('toolsPage.filters.category')"
      :options="categoryOptions"
      @update:model-value="$emit('update:category', String($event ?? ''))"
    />
    <q-select
      :model-value="source"
      class="app-page-toolbar__field app-glass-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="$t('toolsPage.filters.source')"
      :options="sourceOptions"
      @update:model-value="$emit('update:source', String($event ?? ''))"
    />
    <q-select
      :model-value="riskLevel"
      class="app-page-toolbar__field app-glass-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="$t('toolsPage.filters.risk')"
      :options="riskOptions"
      @update:model-value="$emit('update:riskLevel', String($event ?? ''))"
    />
    <q-select
      :model-value="enabled"
      class="app-page-toolbar__field app-glass-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="$t('toolsPage.filters.enabled')"
      :options="enabledOptions"
      @update:model-value="$emit('update:enabled', $event ?? null)"
    />
    <q-toggle
      :model-value="abnormal"
      dense
      color="warning"
      :label="$t('toolsPage.filters.abnormalOnly')"
      @update:model-value="$emit('update:abnormal', Boolean($event))"
    >
      <q-tooltip>{{ $t('toolsPage.filters.abnormalTip') }}</q-tooltip>
    </q-toggle>
    <template #actions>
      <q-btn
        flat
        rounded
        no-caps
        icon="restart_alt"
        :label="$t('toolsPage.filters.reset')"
        class="app-entity-toolbar-btn"
        @click="$emit('reset')"
      />
      <q-btn
        flat
        rounded
        no-caps
        icon="refresh"
        :label="$t('toolsPage.filters.refresh')"
        class="app-entity-toolbar-btn"
        :loading="loading"
        @click="$emit('refresh')"
      />
    </template>
  </AppPageToolbar>
</template>

<script setup lang="ts">
import AppPageToolbar from '../layout/AppPageToolbar.vue';

type Opt<T = string> = { label: string; value: T };

defineProps<{
  search: string;
  category: string;
  source: string;
  riskLevel: string;
  enabled: boolean | null;
  abnormal: boolean;
  categoryOptions: Opt[];
  sourceOptions: Opt[];
  riskOptions: Opt<string>[];
  enabledOptions: Opt<boolean>[];
  loading?: boolean;
}>();

defineEmits<{
  'update:search': [v: string];
  'update:category': [v: string];
  'update:source': [v: string];
  'update:riskLevel': [v: string];
  'update:enabled': [v: boolean | null];
  'update:abnormal': [v: boolean];
  reset: [];
  refresh: [];
}>();
</script>
