// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <AppPageHero
    :kicker="t('memory.hero.kicker')"
    :title="t('memory.hero.title')"
    :subtitle="t('memory.hero.subtitle')"
  >
    <template #actions>
      <q-select
        :model-value="selectedAgentId"
        dense
        outlined
        clearable
        emit-value
        map-options
        :label="t('memory.hero.agentLabel')"
        :options="agentOptions"
        class="memory-select"
        @update:model-value="$emit('update:selectedAgentId', $event as string | null)"
      />
      <q-btn
        color="primary"
        rounded
        unelevated
        no-caps
        icon="refresh"
        :label="t('memory.hero.refresh')"
        :loading="loading"
        @click="$emit('refresh')"
      />
    </template>
  </AppPageHero>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppPageHero from '../layout/AppPageHero.vue';

defineProps<{
  selectedAgentId: string | null;
  agentOptions: Array<{ label: string; value: string }>;
  loading: boolean;
}>();

defineEmits<{
  'update:selectedAgentId': [value: string | null];
  refresh: [];
}>();

const { t } = useI18n();
</script>

<style scoped>
.memory-select {
  min-width: 220px;
}

@media (width <= 800px) {
  .memory-select {
    min-width: 100%;
  }
}
</style>
