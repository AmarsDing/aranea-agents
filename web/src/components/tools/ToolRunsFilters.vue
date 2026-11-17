<template>
  <AppPageToolbar>
    <q-input
      :model-value="toolKey"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      debounce="350"
      label="Tool Key"
      @update:model-value="onToolKey($event)"
    />
    <q-input
      :model-value="agentId"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      debounce="350"
      label="Agent ID"
      @update:model-value="onAgentId($event)"
    />
    <q-input
      :model-value="sessionId"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      debounce="350"
      label="Session ID"
      @update:model-value="onSessionId($event)"
    />
    <q-select
      :model-value="status"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="$t('toolsPage.filters.status')"
      :options="statusOptions"
      @update:model-value="onStatus($event)"
    />
    <q-input
      :model-value="from"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      :label="$t('toolsPage.filters.fromIso')"
      @update:model-value="onFrom($event)"
    />
    <template #actions>
      <q-toggle
        :model-value="hasError"
        dense
        color="negative"
        :label="$t('toolsPage.filters.errorOnly')"
        @update:model-value="onHasError($event)"
      >
        <q-tooltip>{{ $t('toolsPage.filters.errorOnlyTip') }}</q-tooltip>
      </q-toggle>
      <q-btn flat rounded no-caps icon="restart_alt" :label="$t('shell.runs.reset')" @click="$emit('reset')" />
      <q-btn
        flat
        rounded
        no-caps
        icon="refresh"
        :label="$t('shell.runs.refresh')"
        :loading="loading"
        @click="$emit('refresh')"
      />
    </template>
  </AppPageToolbar>
</template>

<script setup lang="ts">
import AppPageToolbar from '../layout/AppPageToolbar.vue';

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

<style scoped lang="sass">
// 6 个筛选字段 + 操作：字段不收缩，空间不足时换行（与插件/投递记录页一致）
:deep(.app-page-toolbar__body)
  flex-wrap: wrap

:deep(.app-page-toolbar__field)
  flex: 0 0 auto
  min-width: 150px
</style>
