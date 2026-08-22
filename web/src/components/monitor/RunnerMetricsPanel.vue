<template>
  <q-card flat :bordered="variant === 'monitor'" :class="panelClass">
    <q-card-section class="row items-center">
      <div>
        <div class="text-h6 text-weight-bold">{{ t('monitorPage.runner.title') }}</div>
        <div v-if="scopeHint" class="text-caption text-grey-7 q-mt-xs">{{ scopeHint }}</div>
      </div>
      <q-space />
      <q-select
        :model-value="windowMinutes"
        dense
        outlined
        emit-value
        map-options
        :options="windowOptions"
        :label="t('monitorPage.runner.window')"
        style="min-width: 120px"
        class="q-mr-sm"
        @update:model-value="emit('update:windowMinutes', $event)"
      />
      <q-btn flat rounded icon="refresh" :label="t('monitorPage.runner.refresh')" :loading="loading" @click="emit('refresh')" />
    </q-card-section>
    <q-separator />
    <q-card-section v-if="metrics" :class="{ 'runner-metrics__body--compact': variant === 'overview' }">
      <div class="apm-stat-grid runner-metrics__grid">
        <div
          v-for="card in statCards"
          :key="card.key"
          class="apm-stat-card"
          :class="{
            'apm-stat-card--danger': card.tone === 'danger',
            'apm-stat-card--clickable': card.drill,
          }"
          @click="card.drill && emit('drill')"
        >
          <q-icon :name="card.icon" size="18px" class="apm-stat-card__icon" />
          <div class="apm-stat-card__text">
            <div class="apm-stat-card__label">{{ card.label }}</div>
            <div class="apm-stat-card__value">{{ card.value }}</div>
          </div>
          <q-tooltip v-if="card.drill">{{ drillHint }}</q-tooltip>
          <q-icon v-if="card.drill" name="chevron_right" size="16px" class="runner-metrics__drill-icon" />
        </div>
      </div>
      <div class="text-caption text-grey-7 q-mt-sm">{{ drillHint }}</div>
    </q-card-section>
    <q-card-section v-else-if="loading">
      <q-skeleton type="rect" height="72px" />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { RunnerMetricsSummary } from '../../features/monitor/types';

const { t } = useI18n();

const props = withDefaults(
  defineProps<{
    metrics: RunnerMetricsSummary | null;
    loading: boolean;
    windowMinutes: number;
    variant?: 'monitor' | 'overview';
    scopeHint?: string;
  }>(),
  { variant: 'monitor', scopeHint: '' },
);

const emit = defineEmits<{
  'update:windowMinutes': [value: number];
  refresh: [];
  drill: [];
}>();

const panelClass = computed(() => (props.variant === 'overview' ? 'overview-panel q-mb-md' : 'monitor-card q-mb-md'));

const drillHint = computed(() =>
  props.variant === 'overview' ? t('monitorPage.runner.drillHintOverview') : t('monitorPage.runner.drillHintMonitor'),
);

interface StatCard {
  key: string;
  label: string;
  icon: string;
  value: string;
  tone: '' | 'danger';
  drill: boolean;
}

/** stat 卡配置：4 个核心指标可下钻到 Traces，延迟分位仅展示 */
const statCards = computed<StatCard[]>(() => {
  const m = props.metrics;
  if (!m) return [];
  const cards: StatCard[] = [
    {
      key: 'total',
      label: t('monitorPage.runner.totalRuns'),
      icon: 'play_circle',
      value: String(m.total_runs),
      tone: '',
      drill: true,
    },
    {
      key: 'errors',
      label: t('monitorPage.runner.errorRuns'),
      icon: 'error_outline',
      value: String(m.error_runs),
      tone: 'danger',
      drill: true,
    },
    {
      key: 'errorRate',
      label: t('monitorPage.runner.errorRate'),
      icon: 'percent',
      value: formatPercent(m.error_rate),
      tone: '',
      drill: true,
    },
    {
      key: 'successRate',
      label: t('monitorPage.runner.successRate'),
      icon: 'check_circle',
      value: formatPercent(m.success_rate),
      tone: '',
      drill: true,
    },
  ];
  const latency: [keyof RunnerMetricsSummary, string][] = [
    ['p50_duration_ms', t('monitorPage.runner.p50Latency')],
    ['p95_duration_ms', t('monitorPage.runner.p95Latency')],
    ['p99_duration_ms', t('monitorPage.runner.p99Latency')],
    ['avg_duration_ms', t('monitorPage.runner.avgLatency')],
  ];
  for (const [field, label] of latency) {
    const v = m[field] as number | null | undefined;
    if (v != null) {
      cards.push({ key: field, label, icon: 'speed', value: formatLatency(v), tone: '', drill: false });
    }
  }
  return cards;
});

const windowOptions = computed(() => [
  { label: t('monitorPage.runner.windowOption.m15'), value: 15 },
  { label: t('monitorPage.runner.windowOption.h1'), value: 60 },
  { label: t('monitorPage.runner.windowOption.h6'), value: 360 },
  { label: t('monitorPage.runner.windowOption.h24'), value: 1440 },
]);

function formatPercent(v: number): string {
  if (!Number.isFinite(v)) return '-';
  return `${(v * 100).toFixed(1)}%`;
}

function formatLatency(v?: number): string {
  if (v == null || !Number.isFinite(v)) return '-';
  if (v >= 1000) return `${(v / 1000).toFixed(1)}s`;
  return `${Math.round(v)}ms`;
}
</script>

<style scoped>
/* 下钻箭头：hover 时着色提示可点击 */
.runner-metrics__drill-icon {
  margin-left: auto;
  flex-shrink: 0;
  color: var(--color-text-icon-muted);
  transition: color 0.15s ease;
}

.apm-stat-card--clickable:hover .runner-metrics__drill-icon {
  color: var(--color-accent);
}
</style>
