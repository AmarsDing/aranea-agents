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
      :model-value="userId"
      class="app-page-toolbar__field"
      dense
      outlined
      clearable
      debounce="350"
      label="User ID"
      @update:model-value="onUserId($event)"
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
    <template #actions>
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
  userId: string | null;
  status: string | null;
  statusOptions: { label: string; value: string }[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  'update:toolKey': [v: string | null];
  'update:agentId': [v: string | null];
  'update:userId': [v: string | null];
  'update:status': [v: string | null];
  reset: [];
  refresh: [];
}>();

const onToolKey = (v: string | number | null) => emit('update:toolKey', v == null ? null : String(v));
const onAgentId = (v: string | number | null) => emit('update:agentId', v == null ? null : String(v));
const onUserId = (v: string | number | null) => emit('update:userId', v == null ? null : String(v));
const onStatus = (v: string | number | null) => emit('update:status', v == null ? null : String(v));
</script>

<style scoped lang="sass">
:deep(.app-page-toolbar__body)
  flex-wrap: wrap

:deep(.app-page-toolbar__field)
  flex: 0 0 auto
  min-width: 150px
</style>
