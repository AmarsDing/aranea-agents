<template>
  <div class="monitor-alert-rule-row q-pa-md" :class="`monitor-alert-rule-row--${rule.severity || 'warning'}`">
    <div class="row items-center q-col-gutter-sm q-mb-sm">
      <q-toggle v-model="rule.enabled" :aria-label="t('monitorPage.alerts.rule.enabled')" />
      <q-input v-model="rule.name" dense outlined class="col" :label="t('monitorPage.alerts.rule.name')" />
      <q-chip dense :color="stateMeta.color" text-color="white" size="sm">{{ stateMeta.label }}</q-chip>
      <q-btn flat dense round icon="delete_outline" color="negative" @click="$emit('remove')">
        <q-tooltip>{{ t('monitorPage.alerts.rule.delete') }}</q-tooltip>
      </q-btn>
    </div>

    <div class="text-caption text-grey-6 q-mb-xs">{{ t('monitorPage.alerts.rule.monitorSection') }}</div>
    <div class="app-form-field-grid">
      <q-select
        :model-value="rule.metric_key"
        dense
        outlined
        emit-value
        map-options
        class="app-grid-span-full"
        :label="t('monitorPage.alerts.rule.metric')"
        :options="metricOptions"
        @update:model-value="onMetricChange"
      />
      <div v-if="selectedMetric" class="app-grid-span-full monitor-alert-metric-help q-pa-sm rounded-borders">
        <div class="text-caption">{{ metricDescription(selectedMetric) }}</div>
        <div class="text-caption text-grey-6 q-mt-xs">
          {{ t('monitorPage.alerts.rule.currentValue') }}:
          <b>{{ formatMetricValue(selectedMetric.unit, selectedMetric.current_value) }}</b>
          ·
          {{
            t('monitorPage.alerts.catalog.suggested', {
              threshold: formatMetricValue(selectedMetric.unit, selectedMetric.suggested_threshold),
              window: selectedMetric.default_window_minutes,
            })
          }}
        </div>
      </div>
      <q-input
        v-model.number="rule.threshold"
        dense
        outlined
        type="number"
        step="any"
        :label="t('monitorPage.alerts.rule.threshold')"
        :hint="thresholdHint"
      />
      <q-input
        v-model.number="rule.window_minutes"
        dense
        outlined
        type="number"
        :label="t('monitorPage.alerts.rule.window')"
        :hint="t('monitorPage.alerts.rule.windowHint')"
      />
    </div>

    <div class="text-caption text-grey-6 q-mt-md q-mb-xs">{{ t('monitorPage.alerts.rule.notifySection') }}</div>
    <div class="app-form-field-grid">
      <q-select
        v-model="rule.severity"
        dense
        outlined
        emit-value
        map-options
        :label="t('monitorPage.alerts.rule.severity')"
        :options="severityOptions"
      />
      <q-input
        v-model.number="rule.cooldown_minutes"
        dense
        outlined
        type="number"
        :label="t('monitorPage.alerts.rule.cooldown')"
        :hint="t('monitorPage.alerts.rule.cooldownHint')"
      />
      <q-input
        v-model="rule.notify_webhook_url"
        class="app-field-long"
        dense
        outlined
        :label="t('monitorPage.alerts.rule.webhook')"
      />
      <q-select
        v-model="rule.notify_channel_id"
        dense
        outlined
        emit-value
        map-options
        clearable
        :label="t('monitorPage.alerts.rule.channel')"
        :options="channelOptions"
      />
      <div class="app-grid-span-full text-caption text-grey-6">
        {{ t('monitorPage.alerts.rule.notifyHint') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { AlertMetricInfo, MonitorAlertRule } from '../../features/monitor/types';
import { alertRuleStateOf, formatMetricValue, metricI18nKey } from '../../features/monitor/utils';

const rule = defineModel<MonitorAlertRule>('rule', { required: true });

const props = defineProps<{
  metrics: AlertMetricInfo[];
  channelOptions: { label: string; value: string }[];
}>();

defineEmits<{
  remove: [];
}>();

const { t, te } = useI18n();

const metricsByKey = computed(() => new Map(props.metrics.map((m) => [m.key, m])));

const selectedMetric = computed(() => metricsByKey.value.get(rule.value.metric_key));

const metricOptions = computed(() => {
  const options = props.metrics.map((m) => ({ label: `${metricName(m)}（${m.key}）`, value: m.key }));
  // Legacy rules may target a metric that is no longer registered; keep the
  // raw key selectable so the form never silently drops it.
  if (rule.value.metric_key && !metricsByKey.value.has(rule.value.metric_key)) {
    options.push({ label: `${rule.value.metric_key}（${t('monitorPage.alerts.rule.unknownMetric')}）`, value: rule.value.metric_key });
  }
  return options;
});

const severityOptions = computed(() => [
  { label: t('monitorPage.alerts.severity.info'), value: 'info' },
  { label: t('monitorPage.alerts.severity.warning'), value: 'warning' },
  { label: t('monitorPage.alerts.severity.critical'), value: 'critical' },
]);

const stateMeta = computed(() => {
  const state = alertRuleStateOf(rule.value, selectedMetric.value);
  switch (state) {
    case 'firing':
      return { color: 'warning', label: t('monitorPage.alerts.rule.state.firing') };
    case 'disabled':
      return { color: 'grey', label: t('monitorPage.alerts.rule.state.disabled') };
    case 'unknown':
      return { color: 'negative', label: t('monitorPage.alerts.rule.state.unknown') };
    default:
      return { color: 'positive', label: t('monitorPage.alerts.rule.state.ok') };
  }
});

const thresholdHint = computed(() => {
  if (selectedMetric.value?.unit !== 'ratio') return '';
  const pct = ((rule.value.threshold || 0) * 100).toFixed(0);
  return t('monitorPage.alerts.rule.thresholdRatioHint', { value: pct });
});

function metricName(m: AlertMetricInfo): string {
  const key = metricI18nKey(m.key, 'name');
  return te(key) ? t(key) : m.name;
}

function metricDescription(m: AlertMetricInfo): string {
  const key = metricI18nKey(m.key, 'description');
  return te(key) ? t(key) : m.description;
}

function onMetricChange(key: string) {
  rule.value.metric_key = key;
  const m = metricsByKey.value.get(key);
  if (!m) return;
  // Apply the catalog's suggested defaults so a new rule starts from a
  // sensible configuration; the user can still override every field.
  if (!rule.value.name.trim()) {
    rule.value.name = metricName(m);
  }
  rule.value.threshold = m.suggested_threshold;
  rule.value.window_minutes = m.default_window_minutes;
}
</script>

<style scoped>
.monitor-alert-rule-row {
  border-left: 3px solid transparent;
  transition: border-color 0.2s ease;
}
.monitor-alert-rule-row--info {
  border-left-color: var(--q-info);
}
.monitor-alert-rule-row--warning {
  border-left-color: var(--q-warning);
}
.monitor-alert-rule-row--critical {
  border-left-color: var(--q-negative);
}
.monitor-alert-metric-help {
  background: var(--color-status-info-bg-alt);
}
</style>
