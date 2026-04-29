<template>
  <tool-glass-panel dense class="q-mb-md">
    <q-card-section class="row q-col-gutter-sm items-center">
      <div class="col-12 col-md-4">
        <q-input
          :model-value="search"
          dense
          outlined
          clearable
          debounce="350"
          placeholder="搜索 Tool 名称、Key、描述..."
          @update:model-value="$emit('update:search', $event)"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-select
          :model-value="category"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="分类"
          :options="categoryOptions"
          @update:model-value="$emit('update:category', $event)"
        />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-select
          :model-value="riskLevel"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="风险"
          :options="riskOptions"
          @update:model-value="$emit('update:riskLevel', $event)"
        />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-select
          :model-value="enabled"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="启用状态"
          :options="enabledOptions"
          @update:model-value="$emit('update:enabled', $event)"
        />
      </div>
      <div class="col-12 col-md-2 row justify-end q-gutter-sm">
        <q-btn flat rounded icon="restart_alt" label="重置" class="tool-toolbar-btn" @click="$emit('reset')" />
        <q-btn flat rounded icon="refresh" label="刷新" class="tool-toolbar-btn" :loading="loading" @click="$emit('refresh')" />
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

<style scoped lang="sass">
.tool-toolbar-btn
  color: var(--color-text-primary)

body:not(.body--dark) .tool-toolbar-btn:hover
  background: var(--interaction-surface-hover)

body.body--dark .tool-toolbar-btn:hover
  border-color: var(--glass-border-hover)
</style>
