<template>
  <channel-glass-panel dense class="q-mb-md">
    <q-card-section class="app-form-field-grid items-end">
      <q-input
        :model-value="search"
        class="app-field-md app-glass-control"
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
        class="app-glass-control"
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
        class="app-glass-control"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('channelsPage.statusFilter')"
        :options="statusOptions"
        @update:model-value="$emit('update:statusFilter', String($event ?? ''))"
      />
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('channelsPage.reset')" class="app-entity-toolbar-btn" @click="$emit('reset')" />
        <q-btn flat rounded no-caps icon="refresh" :label="t('channelsPage.refresh')" class="app-entity-toolbar-btn" :loading="loading" @click="$emit('refresh')" />
      </div>
    </q-card-section>
  </channel-glass-panel>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import ChannelGlassPanel from "./ChannelGlassPanel.vue";

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
  "update:search": [v: string];
  "update:typeFilter": [v: string];
  "update:statusFilter": [v: string];
  reset: [];
  refresh: [];
}>();
</script>
