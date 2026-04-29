<template>
  <q-card flat class="sessions-filter-card q-mb-md">
    <q-card-section class="row q-col-gutter-sm items-center">
      <div class="col-12 col-md-4">
        <q-input
          :model-value="keyword"
          dense
          outlined
          clearable
          debounce="350"
          placeholder="搜索标题、摘要或 Session ID"
          class="sessions-field"
          @update:model-value="$emit('update:keyword', $event)"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-select
          :model-value="ownerType"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="类型"
          class="sessions-field"
          :options="ownerOptions"
          @update:model-value="$emit('update:ownerType', $event)"
        />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-select
          :model-value="status"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="状态"
          class="sessions-field"
          :options="statusOptions"
          @update:model-value="$emit('update:status', $event)"
        />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-select
          :model-value="contextStatus"
          dense
          outlined
          clearable
          emit-value
          map-options
          label="上下文"
          class="sessions-field"
          :options="contextOptions"
          @update:model-value="$emit('update:contextStatus', $event)"
        />
      </div>
      <div class="col-12 col-md-2 row justify-end q-gutter-sm">
        <q-btn flat rounded label="重置" icon="restart_alt" class="sessions-btn-ghost" @click="$emit('reset')" />
        <q-btn
          unelevated
          rounded
          label="查询"
          icon="manage_search"
          class="sessions-btn-accent"
          :loading="loading"
          @click="$emit('search')"
        />
      </div>
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
}>();
</script>
