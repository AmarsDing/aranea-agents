<template>
  <channel-glass-panel dense class="q-mb-md">
    <q-card-section class="row q-col-gutter-md items-center">
      <div class="col-12 col-md-4">
        <q-input
          :model-value="search"
          dense
          outlined
          clearable
          debounce="200"
          label="搜索 Channel"
          placeholder="名称、Key、描述、类型…"
          @update:model-value="$emit('update:search', $event ?? '')"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
      </div>
      <div class="col-12 col-md-3">
        <q-select
          :model-value="typeFilter"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="平台类型"
          :options="typeOptions"
          @update:model-value="$emit('update:typeFilter', $event ?? '')"
        />
      </div>
      <div class="col-12 col-md-3">
        <q-select
          :model-value="statusFilter"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="状态"
          :options="statusOptions"
          @update:model-value="$emit('update:statusFilter', $event ?? '')"
        />
      </div>
      <div class="col-12 col-md-2 row justify-end q-gutter-sm">
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" class="channel-toolbar-btn" @click="$emit('reset')" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" class="channel-toolbar-btn" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>
  </channel-glass-panel>
</template>

<script setup lang="ts">
import ChannelGlassPanel from "./ChannelGlassPanel.vue";

type Opt<T = string> = { label: string; value: T };

defineProps<{
  search: string;
  typeFilter: string;
  statusFilter: string;
  typeOptions: Opt[];
  statusOptions: Opt[];
  loading?: boolean;
}>();

defineEmits<{
  "update:search": [v: string];
  "update:typeFilter": [v: string];
  "update:statusFilter": [v: string];
  reset: [];
  refresh: [];
}>();
</script>

<style scoped lang="sass">
.channel-toolbar-btn
  color: var(--color-text-primary)

body:not(.body--dark) .channel-toolbar-btn:hover
  background: var(--interaction-surface-hover)

body.body--dark .channel-toolbar-btn:hover
  border-color: var(--glass-border-hover)
</style>
