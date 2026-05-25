<template>
  <tool-glass-panel dense class="q-mb-md">
    <q-card-section class="app-form-field-grid items-end">
      <q-input
        :model-value="search"
        class="app-field-md app-glass-control"
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
        class="app-glass-control"
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
        :model-value="riskLevel"
        class="app-glass-control"
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
        class="app-glass-control"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="启用状态"
        :options="enabledOptions"
        @update:model-value="$emit('update:enabled', $event ?? null)"
      />
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" class="app-entity-toolbar-btn" @click="$emit('reset')" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" class="app-entity-toolbar-btn" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import ToolGlassPanel from "./ToolGlassPanel.vue";

type Opt<T = string> = { label: string; value: T };

defineProps<{
  search: string;
  category: string;
  riskLevel: string;
  enabled: boolean | null;
  categoryOptions: Opt[];
  riskOptions: Opt<string>[];
  enabledOptions: Opt<boolean>[];
  loading?: boolean;
}>();

defineEmits<{
  "update:search": [v: string];
  "update:category": [v: string];
  "update:riskLevel": [v: string];
  "update:enabled": [v: boolean | null];
  reset: [];
  refresh: [];
}>();
</script>
