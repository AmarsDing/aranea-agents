<template>
  <tool-glass-panel class="q-mb-md">
    <q-card-section class="row q-col-gutter-sm items-center q-py-sm">
      <div class="col-12 col-md-3">
        <q-input
          :model-value="toolKey"
          dense
          outlined
          clearable
          debounce="350"
          label="Tool Key"
          @update:model-value="$emit('update:toolKey', $event)"
        />
      </div>
      <div class="col-12 col-md-3">
        <q-input
          :model-value="agentId"
          dense
          outlined
          clearable
          debounce="350"
          label="Agent ID"
          @update:model-value="$emit('update:agentId', $event)"
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
          :options="statusOptions"
          @update:model-value="$emit('update:status', $event)"
        />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-input
          :model-value="from"
          dense
          outlined
          clearable
          label="开始时间 ISO"
          @update:model-value="$emit('update:from', $event)"
        />
      </div>
      <div class="col-12 col-md-2 row justify-end q-gutter-sm">
        <q-btn flat rounded icon="restart_alt" label="重置" @click="$emit('reset')" />
        <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import ToolGlassPanel from "./ToolGlassPanel.vue";

defineProps<{
  toolKey: string;
  agentId: string;
  status: string;
  from: string;
  statusOptions: { label: string; value: string }[];
  loading?: boolean;
}>();

defineEmits<{
  "update:toolKey": [v: string];
  "update:agentId": [v: string];
  "update:status": [v: string];
  "update:from": [v: string];
  reset: [];
  refresh: [];
}>();
</script>
