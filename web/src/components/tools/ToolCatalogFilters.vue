<template>
  <AppPageToolbar dense>
    <q-input
      :model-value="search"
      class="app-page-toolbar__search app-glass-control"
      dense
      outlined
      clearable
      debounce="350"
      placeholder="搜索 Tool 名称、Key、描述..."
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
      label="分类"
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
      label="来源"
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
      label="风险"
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
      label="启用状态"
      :options="enabledOptions"
      @update:model-value="$emit('update:enabled', $event ?? null)"
    />
    <q-toggle
      :model-value="abnormal"
      dense
      color="warning"
      label="仅看异常"
      @update:model-value="$emit('update:abnormal', Boolean($event))"
    >
      <q-tooltip>仅显示最近一次调用以错误 / 阻断收尾的工具</q-tooltip>
    </q-toggle>
    <template #actions>
      <q-btn
        flat
        rounded
        no-caps
        icon="restart_alt"
        label="重置"
        class="app-entity-toolbar-btn"
        @click="$emit('reset')"
      />
      <q-btn
        flat
        rounded
        no-caps
        icon="refresh"
        label="刷新"
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
