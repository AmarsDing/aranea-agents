<template>
  <q-card flat bordered class="monitor-card selfcheck-card q-mb-md">
    <q-card-section class="row items-center no-wrap">
      <q-icon name="health_and_safety" size="20px" class="selfcheck-card__head-icon" />
      <div class="q-ml-sm">
        <div class="text-h6 text-weight-bold">{{ t('monitorPage.selfCheck.title') }}</div>
        <div class="text-caption text-grey-7 q-mt-xs">{{ t('monitorPage.selfCheck.subtitle') }}</div>
      </div>
      <q-space />
      <q-btn
        flat
        rounded
        no-caps
        icon="play_arrow"
        :label="t('monitorPage.selfCheck.trigger')"
        :loading="triggering"
        :disable="loading"
        @click="onTrigger"
      />
      <q-btn
        flat
        rounded
        no-caps
        icon="refresh"
        :label="t('monitorPage.selfCheck.refresh')"
        :loading="loading"
        @click="onRefresh"
      />
    </q-card-section>
    <q-separator />

    <q-card-section v-if="latestReport">
      <!-- 总状态 hero：状态点 + 大文案 + 末次运行时间 -->
      <div class="selfcheck-hero q-mb-md" :class="`selfcheck-hero--${toneOf(latestReport.overall_status)}`">
        <span
          class="apm-status-dot selfcheck-hero__dot"
          :class="`apm-status-dot--${toneOf(latestReport.overall_status)}`"
        />
        <div class="selfcheck-hero__text">
          <div class="selfcheck-hero__label">{{ t('monitorPage.selfCheck.overallStatus') }}</div>
          <div class="selfcheck-hero__value">{{ statusLabel(latestReport.overall_status) }}</div>
        </div>
        <q-space />
        <div class="selfcheck-hero__time">
          <q-icon name="schedule" size="14px" class="q-mr-xs" />
          {{ t('monitorPage.selfCheck.lastRun', { time: formatTime(latestReport.finished_at) }) }}
        </div>
      </div>

      <!-- stat 卡 -->
      <div class="apm-stat-grid q-mb-md">
        <div class="apm-stat-card">
          <q-icon name="fact_check" size="18px" class="apm-stat-card__icon" />
          <div class="apm-stat-card__text">
            <div class="apm-stat-card__label">{{ t('monitorPage.selfCheck.checkItems') }}</div>
            <div class="apm-stat-card__value">{{ latestReport.check_results.length }}</div>
          </div>
        </div>
        <div class="apm-stat-card" :class="{ 'apm-stat-card--danger': failedCount > 0 }">
          <q-icon name="error_outline" size="18px" class="apm-stat-card__icon" />
          <div class="apm-stat-card__text">
            <div class="apm-stat-card__label">{{ t('monitorPage.selfCheck.failedItems') }}</div>
            <div class="apm-stat-card__value">{{ failedCount }}</div>
          </div>
        </div>
        <div class="apm-stat-card">
          <q-icon name="build" size="18px" class="apm-stat-card__icon" />
          <div class="apm-stat-card__text">
            <div class="apm-stat-card__label">{{ t('monitorPage.selfCheck.repairActions') }}</div>
            <div class="apm-stat-card__value">{{ latestReport.repair_actions.length }}</div>
          </div>
        </div>
        <div class="apm-stat-card">
          <q-icon name="schedule" size="18px" class="apm-stat-card__icon" />
          <div class="apm-stat-card__text">
            <div class="apm-stat-card__label">{{ t('monitorPage.selfCheck.duration') }}</div>
            <div class="apm-stat-card__value">{{ formatDuration(latestReport.duration_ms) }}</div>
          </div>
        </div>
      </div>

      <!-- 检查项明细 -->
      <div class="apm-section-label">
        <q-icon name="checklist" size="14px" />
        {{ t('monitorPage.selfCheck.checkItems') }}
      </div>
      <div class="selfcheck-list">
        <div v-for="result in latestReport.check_results" :key="result.check_id" class="selfcheck-row">
          <span class="apm-status-dot" :class="`apm-status-dot--${toneOf(result.status)}`" />
          <div class="selfcheck-row__main">
            <div class="selfcheck-row__name">{{ result.checker }}</div>
            <div v-if="result.message" class="selfcheck-row__msg">{{ result.message }}</div>
          </div>
          <span class="selfcheck-row__pill" :class="`selfcheck-row__pill--${toneOf(result.status)}`">
            {{ statusLabel(result.status) }}
          </span>
        </div>
      </div>

      <!-- 修复动作 -->
      <template v-if="latestReport.repair_actions.length > 0">
        <div class="apm-section-label q-mt-md">
          <q-icon name="build" size="14px" />
          {{ t('monitorPage.selfCheck.repairActions') }}
        </div>
        <div class="selfcheck-list">
          <div v-for="(action, idx) in latestReport.repair_actions" :key="idx" class="selfcheck-row">
            <span class="apm-status-dot" :class="action.success ? 'apm-status-dot--ok' : 'apm-status-dot--error'" />
            <div class="selfcheck-row__main">
              <div class="selfcheck-row__name">{{ action.action }}</div>
              <div v-if="action.message" class="selfcheck-row__msg">{{ action.message }}</div>
            </div>
          </div>
        </div>
      </template>
    </q-card-section>

    <q-card-section v-else-if="loading">
      <q-skeleton type="rect" height="72px" />
    </q-card-section>

    <q-card-section v-else>
      <div class="monitor-empty">
        <q-icon name="health_and_safety" size="36px" />
        <div>{{ t('monitorPage.selfCheck.empty') }}</div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SelfCheckReport, SelfCheckStatus } from '../../features/monitor/types';

const { t } = useI18n();

const props = defineProps<{
  loading: boolean;
  triggering: boolean;
  latestReport: SelfCheckReport | null;
}>();

const emit = defineEmits<{
  refresh: [];
  trigger: [];
}>();

const failedCount = computed(() => props.latestReport?.check_results.filter((r) => r.status === 'failed').length ?? 0);

function onRefresh() {
  emit('refresh');
}

function onTrigger() {
  emit('trigger');
}

/** 状态 → APM 状态点色调 */
function toneOf(status: SelfCheckStatus): string {
  switch (status) {
    case 'passed':
      return 'ok';
    case 'warning':
      return 'warn';
    case 'failed':
      return 'error';
    default:
      return 'idle';
  }
}

function statusLabel(status: SelfCheckStatus): string {
  switch (status) {
    case 'passed':
      return t('monitorPage.selfCheck.status.passed');
    case 'warning':
      return t('monitorPage.selfCheck.status.warning');
    case 'failed':
      return t('monitorPage.selfCheck.status.failed');
    default:
      return status;
  }
}

function formatDuration(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.round(ms)}ms`;
}

function formatTime(iso: string): string {
  if (!iso) return '-';
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}
</script>

<style scoped>
.selfcheck-card__head-icon {
  color: var(--color-accent);
}

/* ── 总状态 hero ── */
.selfcheck-hero {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 14px 16px;
  border-radius: 14px;
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent);
}

.selfcheck-hero__dot {
  width: 14px;
  height: 14px;
}

.selfcheck-hero--ok {
  border-color: color-mix(in srgb, var(--color-success) 40%, transparent);
}

.selfcheck-hero--warn {
  border-color: color-mix(in srgb, var(--color-warning) 45%, transparent);
}

.selfcheck-hero--error {
  border-color: color-mix(in srgb, var(--color-danger) 50%, transparent);
}

.selfcheck-hero__label {
  font-size: 11px;
  color: var(--color-text-secondary);
}

.selfcheck-hero__value {
  font-size: 17px;
  font-weight: 700;
  line-height: 1.3;
}

.selfcheck-hero--ok .selfcheck-hero__value {
  color: var(--color-success);
}

.selfcheck-hero--warn .selfcheck-hero__value {
  color: var(--color-warning);
}

.selfcheck-hero--error .selfcheck-hero__value {
  color: var(--color-danger);
}

.selfcheck-hero__time {
  display: inline-flex;
  align-items: center;
  color: var(--color-text-secondary);
  font-size: 12px;
  white-space: nowrap;
}

/* ── 明细行 ── */
.selfcheck-list {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--glass-border);
  border-radius: 14px;
  overflow: hidden;
}

.selfcheck-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 14px;
  transition: background 0.15s ease;
}

.selfcheck-row + .selfcheck-row {
  border-top: 1px solid var(--glass-border);
}

.selfcheck-row:hover {
  background: color-mix(in srgb, var(--color-accent) 5%, transparent);
}

.selfcheck-row__main {
  min-width: 0;
  flex: 1;
}

.selfcheck-row__name {
  font-size: 13px;
  font-weight: 600;
}

.selfcheck-row__msg {
  font-size: 12px;
  color: var(--color-text-secondary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.selfcheck-row__pill {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 600;
  padding: 2px 10px;
  border-radius: 999px;
}

.selfcheck-row__pill--ok {
  color: var(--color-success);
  background: var(--color-success-soft);
}

.selfcheck-row__pill--warn {
  color: var(--color-warning);
  background: var(--color-warning-soft);
}

.selfcheck-row__pill--error {
  color: var(--color-danger);
  background: var(--color-danger-soft);
}

.selfcheck-row__pill--idle {
  color: var(--color-text-secondary);
  background: color-mix(in srgb, var(--color-text-secondary) 12%, transparent);
}
</style>
