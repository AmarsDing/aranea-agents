<template>
  <q-card flat bordered class="monitor-card q-mb-md">
    <q-card-section class="row items-center">
      <div>
        <div class="text-h6 text-weight-bold">自检状态</div>
        <div class="text-caption text-grey-7 q-mt-xs">系统健康自检与自动修复</div>
      </div>
      <q-space />
      <q-btn
        flat
        rounded
        icon="play_arrow"
        label="立即自检"
        :loading="triggering"
        :disable="loading"
        @click="onTrigger"
      />
      <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="onRefresh" />
    </q-card-section>
    <q-separator />

    <q-card-section v-if="latestReport">
      <div class="app-metrics-grid q-mb-md">
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">整体状态</div>
          <q-chip :color="statusColor(latestReport.overall_status)" text-color="white" dense class="q-mt-xs">
            {{ statusLabel(latestReport.overall_status) }}
          </q-chip>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">检查项</div>
          <div class="text-h6 text-weight-bold">{{ latestReport.check_results.length }}</div>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">修复动作</div>
          <div class="text-h6 text-weight-bold">{{ latestReport.repair_actions.length }}</div>
        </div>
        <div class="app-metrics-grid__item">
          <div class="text-caption text-grey">耗时</div>
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
        <div class="text-caption text-grey q-mb-xs">修复动作</div>
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

      <div class="text-caption text-grey-7 q-mt-sm">上次自检: {{ formatTime(latestReport.finished_at) }}</div>
    </q-card-section>

    <q-card-section v-else-if="loading">
      <q-skeleton type="rect" height="72px" />
    </q-card-section>

    <q-card-section v-else>
      <div class="text-grey text-center q-pa-md">暂无自检报告，点击「立即自检」开始检查</div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { SelfCheckReport, SelfCheckStatus } from '../../features/monitor/types';

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
      return '通过';
    case 'warning':
      return '警告';
    case 'failed':
      return '失败';
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
