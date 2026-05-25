<template>
  <q-card flat bordered class="monitor-card monitor-log-stream-card">
    <q-card-section class="monitor-log-stream-toolbar">
      <div class="monitor-log-stream-toolbar__info">
        <div class="row items-center q-gutter-sm">
          <div class="text-h6 text-weight-bold">流程日志</div>
          <q-badge :color="stateColor">{{ stateText }}</q-badge>
          <q-badge outline color="primary">{{ filteredLines.length }}/{{ hub.flowLines.value.length }}</q-badge>
        </div>
        <div class="text-caption text-grey-7">业务编排时间线（中文步骤，带 trace 关联）</div>
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
        <LogLevelToggle v-model="level" />
        <q-btn
          dense
          outline
          rounded
          no-caps
          class="monitor-log-toolbar-btn"
          :icon="hub.flowPaused.value ? 'play_arrow' : 'pause'"
          :label="hub.flowPaused.value ? '恢复' : '暂停'"
          @click="togglePause"
        />
        <q-btn
          dense
          outline
          rounded
          no-caps
          class="monitor-log-toolbar-btn"
          icon="delete_sweep"
          label="清除"
          @click="hub.clearFlow()"
        />
      </div>
    </q-card-section>
    <q-separator />
    <q-card-section class="monitor-log-stream-body">
      <div class="monitor-log-console monitor-log-console--fill">
        <div
          v-for="line in filteredLines"
          :key="line.id + line.created_at"
          class="monitor-log-line"
          :class="lineClass(line)"
        >
          <span class="monitor-log-time">{{ line.time }}</span>
          <span class="monitor-log-level">[{{ line.level }}]</span>
          <span v-if="line.title" class="monitor-log-title text-weight-medium">{{ line.title }}</span>
          <span>{{ line.message }}</span>
          <span v-if="line.hint" class="text-caption text-grey-7 q-ml-sm">提示：{{ line.hint }}</span>
        </div>
        <div v-if="!filteredLines.length" class="monitor-log-empty">{{ emptyText }}</div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, inject, ref } from "vue";
import type { MonitorLogHub } from "../../features/monitor/useLogStreamHub";
import type { MonitorLogLine, StreamState } from "../../features/monitor/types";
import LogLevelToggle, { type LogLevel } from "./LogLevelToggle.vue";

const _hub = inject<MonitorLogHub>("monitorLogHub");
if (!_hub) {
  throw new Error("FlowLogStream requires monitorLogHub");
}
const hub: MonitorLogHub = _hub;

const keyword = ref("");
const level = ref<LogLevel>("INFO");

const levelRank: Record<string, number> = { DEBUG: 10, INFO: 20, WARN: 30, ERROR: 40 };

const filteredLines = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  const minRank = levelRank[level.value] ?? 20;
  return hub.flowLines.value.filter((line) => {
    if ((levelRank[line.level] ?? 20) < minRank) return false;
    if (!q) return true;
    return [line.level, line.message, line.source, line.title, line.step_id, line.trace_id].some((value) =>
      String(value || "").toLowerCase().includes(q)
    );
  });
});

const stateTextMap: Record<StreamState, string> = {
  connecting: "连接中",
  connected: "已连接",
  live: "实时",
  paused: "已暂停",
  error: "连接异常"
};

const stateText = computed(() => stateTextMap[hub.flowState.value]);
const stateColor = computed(() => {
  const s = hub.flowState.value;
  if (s === "live" || s === "connected") return "positive";
  if (s === "error") return "negative";
  if (s === "paused") return "grey";
  return "orange";
});

const emptyText = computed(() => {
  if (hub.flowState.value === "connected") {
    return "已连接，等待业务事件（发起一次对话后可看到流程日志）";
  }
  if (hub.flowState.value === "connecting") {
    return "正在连接 WebSocket…";
  }
  return "暂无流程日志";
});

function lineClass(line: MonitorLogLine): string {
  const base = `monitor-log-line--${line.level.toLowerCase()}`;
  const sev = (line.severity || "info").toLowerCase();
  return `${base} monitor-log-line--flow monitor-log-line--flow-${sev}`;
}

function togglePause() {
  hub.setFlowPaused(!hub.flowPaused.value);
}
</script>
