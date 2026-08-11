<template>
  <tool-glass-panel class="q-mb-md">
    <q-card-section class="row q-col-gutter-sm items-center q-py-sm">
      <div class="col-12 col-sm-6 col-md-2">
        <q-input
          :model-value="toolKey"
          dense
          outlined
          clearable
          debounce="350"
          label="Tool Key"
          @update:model-value="onToolKey($event)"
        />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-input
          :model-value="agentId"
          dense
          outlined
          clearable
          debounce="350"
          label="Agent ID"
          @update:model-value="onAgentId($event)"
        />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-input
          :model-value="sessionId"
          dense
          outlined
          clearable
          debounce="350"
          label="Session ID"
          @update:model-value="onSessionId($event)"
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
          @update:model-value="onStatus($event)"
        />
      </div>
      <div class="col-12 col-sm-6 col-md-2">
        <q-input
          :model-value="from"
          dense
          outlined
          clearable
          label="开始时间 ISO"
          @update:model-value="onFrom($event)"
        />
      </div>
      <div class="col-12 col-md-2 row items-center justify-end q-gutter-sm">
        <q-toggle
          :model-value="hasError"
          dense
          color="negative"
          label="仅看错误"
          @update:model-value="onHasError($event)"
        >
          <q-tooltip>仅显示状态为错误的调用</q-tooltip>
        </q-toggle>
        <q-btn flat rounded icon="restart_alt" label="重置" @click="$emit('reset')" />
        <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import ToolGlassPanel from './ToolGlassPanel.vue';

defineProps<{
  toolKey: string | null;
  agentId: string | null;
  sessionId: string | null;
  status: string | null;
  hasError: boolean;
  from: string | null;
  statusOptions: { label: string; value: string }[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  'update:toolKey': [v: string | null];
  'update:agentId': [v: string | null];
  'update:sessionId': [v: string | null];
  'update:status': [v: string | null];
  'update:hasError': [v: boolean];
  'update:from': [v: string | null];
  reset: [];
  refresh: [];
}>();

const onToolKey = (v: string | number | null) => emit('update:toolKey', v == null ? null : String(v));
const onAgentId = (v: string | number | null) => emit('update:agentId', v == null ? null : String(v));
const onSessionId = (v: string | number | null) => emit('update:sessionId', v == null ? null : String(v));
const onStatus = (v: string | number | null) => emit('update:status', v == null ? null : String(v));
const onHasError = (v: boolean) => emit('update:hasError', Boolean(v));
const onFrom = (v: string | number | null) => emit('update:from', v == null ? null : String(v));
</script>
