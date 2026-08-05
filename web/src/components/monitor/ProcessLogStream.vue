<template>
  <q-card flat bordered class="monitor-card monitor-log-stream-card">
    <q-card-section class="monitor-log-stream-toolbar">
      <div class="monitor-log-stream-toolbar__info">
        <div class="row items-center q-gutter-sm">
          <div class="text-h6 text-weight-bold">进程日志</div>
          <q-badge :color="stateColor">{{ stateText }}</q-badge>
          <span v-if="hub.processState.value === 'error' && hub.errorHint.value" class="text-caption text-negative">
            {{ hub.errorHint.value }}
          </span>
          <q-badge outline color="accent">{{ filteredLines.length }}/{{ hub.processLines.value.length }}</q-badge>
        </div>
        <div class="text-caption text-grey-7">
          Gateway / 插件 stderr（由 configs/config.yaml server.monitor.process_log_enabled 控制）
        </div>
      </div>
      <div class="monitor-log-toolbar-controls">
        <q-input
          v-model="keyword"
          dense
          outlined
          clearable
          hide-bottom-space
          debounce="200"
          placeholder="过滤"
          class="monitor-log-toolbar-field monitor-log-toolbar-field--filter"
        >
          <template #prepend><q-icon name="search" /></template>
        </q-input>
        <q-select
          v-model="sourceFilter"
          dense
          outlined
          clearable
          hide-bottom-space
          emit-value
          map-options
          options-dense
          :options="sourceOptions"
          placeholder="来源"
          class="monitor-log-toolbar-field monitor-log-toolbar-field--source"
        />
        <LogLevelToggle v-model="level" />
        <q-btn
          dense
          outline
          rounded
          no-caps
          class="monitor-log-toolbar-btn"
          :icon="hub.processPaused.value ? 'play_arrow' : 'pause'"
          :label="hub.processPaused.value ? t('monitorPage.resume') : t('monitorPage.pause')"
          @click="togglePause"
        >
          <q-tooltip>{{ t('monitorPage.pauseHint') }}</q-tooltip>
        </q-btn>
        <q-btn
          dense
          outline
          rounded
          no-caps
          class="monitor-log-toolbar-btn"
          icon="delete_sweep"
          label="清除"
          @click="hub.clearProcess()"
        >
          <q-tooltip>{{ t('monitorPage.clearViewHint') }}</q-tooltip>
        </q-btn>
      </div>
    </q-card-section>
    <q-separator />
    <q-card-section class="monitor-log-stream-body">
      <div class="monitor-log-console monitor-log-console--fill">
        <div
          v-for="line in filteredLines"
          :key="line.id + line.created_at"
          class="monitor-log-line"
          :class="`monitor-log-line--${line.level.toLowerCase()}`"
        >
          <span class="monitor-log-time">{{ line.time }}</span>
          <span class="monitor-log-level">[{{ line.level }}]</span>
          <span class="text-caption text-grey-6 q-mr-sm">[{{ line.source }}]</span>
          <span>{{ line.message }}</span>
        </div>
        <div v-if="!filteredLines.length" class="monitor-log-empty">{{ emptyText }}</div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, inject, ref, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MonitorLogHub } from '../../features/monitor/useLogStreamHub';
import type { StreamState } from '../../features/monitor/types';
import LogLevelToggle, { type LogLevel } from './LogLevelToggle.vue';

const { t } = useI18n();

const _hub = inject<MonitorLogHub>('monitorLogHub');
const _processLogConfigured = inject<Ref<boolean>>('processLogConfigured');
if (!_hub) {
  throw new Error('ProcessLogStream requires monitorLogHub');
}
if (!_processLogConfigured) {
  throw new Error('ProcessLogStream requires processLogConfigured');
}
const hub: MonitorLogHub = _hub;
const processLogConfigured: Ref<boolean> = _processLogConfigured;

const keyword = ref('');
const sourceFilter = ref<string | null>(null);
const level = ref<LogLevel>('INFO');

const levelRank: Record<string, number> = { DEBUG: 10, INFO: 20, WARN: 30, ERROR: 40 };

const sourceOptions = computed(() => {
  const seen = new Set<string>();
  for (const line of hub.processLines.value) {
    const source = String(line.source || '').trim();
    if (source) seen.add(source);
  }
  return Array.from(seen)
    .sort((a, b) => a.localeCompare(b))
    .map((source) => ({ label: source, value: source }));
});

const filteredLines = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  const src = (sourceFilter.value || '').trim();
  const minRank = levelRank[level.value] ?? 20;
  return hub.processLines.value.filter((line) => {
    if ((levelRank[line.level] ?? 20) < minRank) return false;
    if (src && String(line.source || '') !== src) return false;
    if (!q) return true;
    return [line.level, line.message, line.source].some((value) =>
      String(value || '')
        .toLowerCase()
        .includes(q),
    );
  });
});

const stateTextMap: Record<StreamState, string> = {
  connecting: '连接中',
  connected: '已连接',
  live: '实时',
  paused: '已暂停',
  error: '连接异常',
};

const configEnabled = computed(() => processLogConfigured?.value ?? hub.processEnabled.value);

const stateText = computed(() => {
  if (!configEnabled.value) return '已关闭';
  return stateTextMap[hub.processState.value];
});

const stateColor = computed(() => {
  if (!configEnabled.value) return 'grey';
  const s = hub.processState.value;
  if (s === 'live' || s === 'connected') return 'positive';
  if (s === 'error') return 'negative';
  if (s === 'paused') return 'grey';
  return 'orange';
});

const emptyText = computed(() => {
  if (!configEnabled.value) {
    return '进程日志已在 config.yaml 中关闭（server.monitor.process_log_enabled: false）。';
  }
  if (hub.processPaused.value) {
    return '已暂停接收，点击「恢复」开始接收。';
  }
  if (hub.processState.value === 'error') {
    return hub.errorHint.value || t('monitorPage.logs.reconnecting');
  }
  if (hub.processState.value === 'connected') {
    return '已连接，等待进程日志…';
  }
  if (hub.processState.value === 'connecting') {
    return '正在连接 WebSocket…';
  }
  return '暂无进程日志';
});

function togglePause() {
  hub.setProcessPaused(!hub.processPaused.value);
}
</script>
