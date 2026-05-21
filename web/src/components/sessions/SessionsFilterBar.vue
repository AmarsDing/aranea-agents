<template>
  <q-card flat class="app-registry-panel app-registry-panel--stacked">
    <q-card-section class="app-registry-toolbar--sessions">
      <q-input
        :model-value="keyword"
        class="app-field-md"
        dense
        outlined
        clearable
        debounce="350"
        placeholder="搜索标题、摘要或 Session ID"
        @update:model-value="$emit('update:keyword', $event)"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        :model-value="ownerType"
        class="app-field-sm"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="类型"
        :options="ownerOptions"
        @update:model-value="$emit('update:ownerType', $event)"
      />
      <q-select
        :model-value="status"
        class="app-field-sm"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="状态"
        :options="statusOptions"
        @update:model-value="$emit('update:status', $event)"
      />
      <q-select
        :model-value="contextStatus"
        class="app-field-sm"
        dense
        outlined
        clearable
        emit-value
        map-options
        label="上下文"
        :options="contextOptions"
        @update:model-value="$emit('update:contextStatus', $event)"
      />
      <div class="app-registry-toolbar__actions">
        <q-btn flat rounded no-caps label="重置" icon="restart_alt" @click="$emit('reset')" />
        <q-btn color="primary" unelevated rounded no-caps label="查询" icon="manage_search" :loading="loading" @click="$emit('search')" />
      </div>
    </q-card-section>
    <q-card-section class="app-registry-panel__section app-registry-panel__section--dense app-actions-bar app-actions-bar--start">
      <q-btn
        flat
        rounded
        no-caps
        :icon="selectionMode ? 'close' : 'checklist'"
        :label="selectionMode ? '取消选择' : '批量选择'"
        :color="selectionMode ? 'primary' : undefined"
        @click="$emit('toggle-selection')"
      />
      <q-separator vertical inset class="q-mx-xs" />
      <q-btn flat rounded no-caps icon="inventory_2" label="按天数归档" @click="$emit('retention-archive')" />
      <q-btn flat rounded no-caps icon="delete_sweep" label="按天数删除" @click="$emit('retention-delete')" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
defineProps<{
  keyword: string;
  ownerType: string | null;
  status: string | null;
  contextStatus: string | null;
  loading?: boolean;
  selectionMode: boolean;
  ownerOptions: { label: string; value: string }[];
  statusOptions: { label: string; value: string }[];
  contextOptions: { label: string; value: string }[];
}>();

defineEmits<{
  "update:keyword": [v: string];
  "update:ownerType": [v: string | null];
  "update:status": [v: string | null];
  "update:contextStatus": [v: string | null];
  reset: [];
  search: [];
  "toggle-selection": [];
  "retention-archive": [];
  "retention-delete": [];
}>();
</script>
