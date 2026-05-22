<template>
  <channel-glass-panel dense class="q-mb-md">
    <q-card-section class="app-form-field-grid items-end">
      <q-input
        :model-value="search"
        class="app-field-md"
        dense
        outlined
        clearable
        debounce="200"
        :label="t('channelsPage.searchLabel')"
        :placeholder="t('channelsPage.searchPlaceholder')"
        @update:model-value="$emit('update:search', $event ?? '')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-select
        :model-value="typeFilter"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('channelsPage.typeFilter')"
        :options="typeOptions"
        @update:model-value="$emit('update:typeFilter', $event ?? '')"
      />
      <q-select
        :model-value="statusFilter"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('channelsPage.statusFilter')"
        :options="statusOptions"
        @update:model-value="$emit('update:statusFilter', $event ?? '')"
      />
      <div class="app-actions-bar app-actions-bar--start">
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('channelsPage.reset')" class="channel-toolbar-btn" @click="$emit('reset')" />
        <q-btn flat rounded no-caps icon="refresh" :label="t('channelsPage.refresh')" class="channel-toolbar-btn" :loading="loading" @click="$emit('refresh')" />
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

<style scoped lang="sass">
.channel-toolbar-btn
  color: var(--color-text-primary)

body:not(.body--dark) .channel-toolbar-btn:hover
  background: var(--interaction-surface-hover)

body.body--dark .channel-toolbar-btn:hover
  border-color: var(--glass-border-hover)
</style>
