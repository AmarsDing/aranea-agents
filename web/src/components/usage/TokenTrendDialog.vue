<template>
  <q-dialog v-model="dialogOpen">
    <q-card class="app-dialog-card app-dialog-card--lg app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">{{ t('overviewPage.tokenTrendTitle') }}</div>
          <div class="app-glass-dialog__subtitle">{{ t('overviewPage.tokenTrendSubtitle') }}</div>
        </div>
        <q-btn v-close-popup flat dense round icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body">
          <UsageTrendChart :points="trendPoints" :hourly="false" style="height: 320px" />
        </q-card-section>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { ModelUsageTrendPoint } from '../../features/usage/types';
import UsageTrendChart from './UsageTrendChart.vue';

const { t } = useI18n();

const props = defineProps<{
  open: boolean;
  trendPoints: ModelUsageTrendPoint[];
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
}>();

const dialogOpen = computed({
  get: () => props.open,
  set: (v: boolean) => emit('update:open', v),
});
</script>
