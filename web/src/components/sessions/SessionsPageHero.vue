<template>
  <AppPageHero :kicker="displayOverline" :title="displayTitle" :subtitle="displayDescription">
    <template #actions>
      <q-btn
        outline
        rounded
        no-caps
        color="primary"
        icon="refresh"
        :label="t('sessionsPage.refresh')"
        :loading="loading"
        @click="$emit('refresh')"
      />
    </template>
  </AppPageHero>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../layout/AppPageHero.vue';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    title?: string;
    overline?: string;
    description?: string;
    loading?: boolean;
  }>(),
  {
    title: '',
    overline: '',
    description: '',
  },
);

const displayTitle = computed(() => props.title || t('menu.sessions'));
const displayOverline = computed(() => props.overline || t('sessionsPage.kicker'));
const displayDescription = computed(() => props.description || t('sessionsPage.description'));

defineEmits<{ refresh: [] }>();
</script>
