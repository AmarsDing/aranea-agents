<template>
  <q-page class="q-pa-md">
    <q-card flat bordered>
      <q-card-section class="text-h6">{{ t(titleKey) }}</q-card-section>
      <q-separator />
      <q-card-section class="text-body2 text-grey-7">
        {{ t('common.placeholder') }}
      </q-card-section>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { useI18n } from 'vue-i18n';

const route = useRoute();
const { t } = useI18n();

const titleKey = computed(() => (route.meta?.titleKey as string) || 'menu.overview');
</script>
