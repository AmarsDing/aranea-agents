<template>
  <q-card flat bordered class="monitor-card q-mb-md">
    <q-card-section class="row items-center">
      <div>
        <div class="text-h6 text-weight-bold">{{ t('monitorPage.selfCheck.title') }}</div>
        <div class="text-caption text-grey-7 q-mt-xs">{{ t('monitorPage.selfCheck.subtitle') }}</div>
      </div>
      <q-space />
      <q-btn
        flat
        rounded
        icon="play_arrow"
        :label="t('monitorPage.selfCheck.trigger')"
        :loading="triggering"
        :disable="loading"
        @click="onTrigger"
      />
      <q-btn flat rounded icon="refresh" :label="t('monitorPage.selfCheck.refresh')" :loading="loading" @click="onRefresh" />
    </q-card-section>
    <q-separator />

    <q-card-section v-if="latestReport">
      <div class="app-metrics-grid q-mb-md">
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">{{ t('monitorPage.selfCheck.overallStatus') }}</div>
          <q-chip :color="statusColor(latestReport.overall_status)" text-color="white" dense class="q-mt-xs">
            {{ statusLabel(latestReport.overall_status) }}
          </q-chip>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">{{ t('monitorPage.selfCheck.checkItems') }}</div>
          <div class="text-h6 text-weight-bold">{{ latestReport.check_results.length }}</div>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">{{ t('monitorPage.selfCheck.repairActions') }}</div>
          <div class="text-h6 text-weight-bold">{{ latestReport.repair_actions.length }}</div>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">{{ t('monitorPage.selfCheck.duration') }}</div>
          <div class="text-h6 text-weight-bold">{{ formatDuration(latestReport.duration_ms) }}</div>
        </div>
      </div>

      <q-list dense separator>
        <q-item v-for="result in latestReport.check_results" :key="result.check_id">
          <q-item-section side>
            <q-icon :name="statusIcon(result.status)" :color="statusColor(result.status)" />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ result.checker }}</q-item-label>
            <q-item-label caption>{{ result.message }}</q-item-label>
          </q-item-section>
          <q-item-section side>
            <q-chip :color="statusColor(result.status)" text-color="white" dense size="sm">
              {{ statusLabel(result.status) }}
            </q-chip>
          </q-item-section>
        </q-item>
      </q-list>

      <div v-if="latestReport.repair_actions.length > 0" class="q-mt-md">
        <div class="text-caption text-grey q-mb-xs">{{ t('monitorPage.selfCheck.repairActions') }}</div>
        <q-list dense separator>
          <q-item v-for="(action, idx) in latestReport.repair_actions" :key="idx">
            <q-item-section side>
              <q-icon
                :name="action.success ? 'check_circle' : 'error'"
                :color="action.success ? 'positive' : 'negative'"
              />
            </q-item-section>
            <q-item-section>
              <q-item-label>{{ action.action }}</q-item-label>
              <q-item-label caption>{{ action.message }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
      </div>

      <div class="text-caption text-grey-7 q-mt-sm">{{ t('monitorPage.selfCheck.lastRun', { time: formatTime(latestReport.finished_at) }) }}</div>
    </q-card-section>

    <q-card-section v-else-if="loading">
      <q-skeleton type="rect" height="72px" />
    </q-card-section>

    <q-card-section v-else>
      <div class="text-grey text-center q-pa-md">{{ t('monitorPage.selfCheck.empty') }}</div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { SelfCheckReport, SelfCheckStatus } from '../../features/monitor/types';

const { t } = useI18n();

defineProps<{
  loading: boolean;
  triggering: boolean;
  latestReport: SelfCheckReport | null;
}>();

const emit = defineEmits<{
  refresh: [];
  trigger: [];
}>();

function onRefresh() {
  emit('refresh');
}

async function onTrigger() {
  emit('trigger');
}

function statusColor(status: SelfCheckStatus): string {
  switch (status) {
    case 'passed':
      return 'positive';
    case 'warning':
      return 'warning';
    case 'failed':
      return 'negative';
    default:
      return 'grey';
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

function statusIcon(status: SelfCheckStatus): string {
  switch (status) {
    case 'passed':
      return 'check_circle';
    case 'warning':
      return 'warning';
    case 'failed':
      return 'cancel';
    default:
      return 'help';
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
