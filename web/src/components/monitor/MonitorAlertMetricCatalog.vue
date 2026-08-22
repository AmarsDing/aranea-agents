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
      <div v-for="m in metrics" :key="m.key" class="catalog-metric-card">
        <div class="row items-center no-wrap">
          <q-icon name="speed" size="16px" class="catalog-metric-card__icon" />
          <div class="text-weight-medium ellipsis q-ml-xs">{{ metricName(m) }}</div>
          <q-space />
          <div class="catalog-metric-card__value">
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

.catalog-metric-card {
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--glass-surface) 80%, transparent);
  transition:
    border-color 0.18s ease,
    transform 0.18s ease;
}

.catalog-metric-card:hover {
  border-color: var(--color-accent);
  transform: translateY(-1px);
}

.catalog-metric-card__icon {
  color: var(--color-accent);
  flex-shrink: 0;
}

.catalog-metric-card__value {
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}

.monitor-alert-metric-key {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
</style>
