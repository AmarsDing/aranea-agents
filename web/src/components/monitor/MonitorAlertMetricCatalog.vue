<template>
  <div class="monitor-alert-catalog">
    <div class="row items-center q-mb-sm">
      <div>
        <div class="text-subtitle1 text-weight-medium">{{ t('monitorPage.alerts.catalog.title') }}</div>
        <div class="text-caption text-grey-6">{{ t('monitorPage.alerts.catalog.subtitle') }}</div>
      </div>
      <q-space />
      <q-spinner v-if="loading" size="sm" />
    </div>
    <div v-if="!metrics.length && !loading" class="text-caption text-grey-6 q-pa-md">
      {{ t('monitorPage.alerts.catalog.empty') }}
    </div>
    <div class="monitor-alert-catalog-grid">
      <div v-for="m in metrics" :key="m.key" class="monitor-alert-metric-card q-pa-md rounded-borders">
        <div class="row items-center no-wrap">
          <div class="text-weight-medium ellipsis">{{ metricName(m) }}</div>
          <q-space />
          <div class="text-h6 text-weight-bold monitor-alert-metric-value">
            {{ formatMetricValue(m.unit, m.current_value) }}
          </div>
        </div>
        <div class="text-caption text-grey-6 ellipsis-2-lines q-mt-xs">{{ metricDescription(m) }}</div>
        <div class="row items-center q-mt-sm no-wrap">
          <q-chip dense square size="sm" class="monitor-alert-metric-key">{{ m.key }}</q-chip>
          <q-space />
          <div class="text-caption text-grey-6">
            {{
              t('monitorPage.alerts.catalog.suggested', {
                threshold: formatMetricValue(m.unit, m.suggested_threshold),
                window: m.default_window_minutes,
              })
            }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { AlertMetricInfo } from '../../features/monitor/types';
import { formatMetricValue, metricI18nKey } from '../../features/monitor/utils';

defineProps<{
  metrics: AlertMetricInfo[];
  loading: boolean;
}>();

const { t, te } = useI18n();

function metricName(m: AlertMetricInfo): string {
  const key = metricI18nKey(m.key, 'name');
  return te(key) ? t(key) : m.name;
}

function metricDescription(m: AlertMetricInfo): string {
  const key = metricI18nKey(m.key, 'description');
  return te(key) ? t(key) : m.description;
}
</script>

<style scoped>
.monitor-alert-catalog-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 12px;
}
.monitor-alert-metric-value {
  font-variant-numeric: tabular-nums;
}
.monitor-alert-metric-key {
  font-family: monospace;
}
</style>
