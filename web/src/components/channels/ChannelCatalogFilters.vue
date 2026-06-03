<template>
  <AppPageToolbar dense>
    <q-input
      :model-value="search"
      class="app-page-toolbar__search app-glass-control"
      dense
      outlined
      clearable
      debounce="200"
      :label="t('channelsPage.searchLabel')"
      :placeholder="t('channelsPage.searchPlaceholder')"
      @update:model-value="$emit('update:search', String($event ?? ''))"
    >
      <template #prepend><q-icon name="search" /></template>
    </q-input>
    <q-select
      :model-value="typeFilter"
      class="app-page-toolbar__field app-glass-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="t('channelsPage.typeFilter')"
      :options="typeOptions"
      @update:model-value="$emit('update:typeFilter', String($event ?? ''))"
    />
    <q-select
      :model-value="statusFilter"
      class="app-page-toolbar__field app-glass-control"
      dense
      outlined
      clearable
      emit-value
      map-options
      :label="t('channelsPage.statusFilter')"
      :options="statusOptions"
      @update:model-value="$emit('update:statusFilter', String($event ?? ''))"
    />
    <template #actions>
      <q-btn
        flat
        rounded
        no-caps
        icon="restart_alt"
        :label="t('channelsPage.reset')"
        class="app-entity-toolbar-btn"
        @click="$emit('reset')"
      />
      <q-btn
        flat
        rounded
        no-caps
        icon="refresh"
        :label="t('channelsPage.refresh')"
        class="app-entity-toolbar-btn"
        :loading="loading"
        @click="$emit('refresh')"
      />
    </template>
  </AppPageToolbar>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppPageToolbar from '../layout/AppPageToolbar.vue';

const { t } = useI18n();

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
  'update:search': [v: string];
  'update:typeFilter': [v: string];
  'update:statusFilter': [v: string];
  reset: [];
  refresh: [];
}>();
</script>
