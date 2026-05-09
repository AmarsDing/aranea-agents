<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col-12 col-md">
        <div class="row items-center q-gutter-sm">
          <div class="text-h6 text-weight-bold">日志</div>
          <q-badge :color="stateColor">{{ stateText }}</q-badge>
          <q-badge outline color="primary">{{ filteredLines.length }}/{{ lines.length }}</q-badge>
        </div>
        <div class="text-caption text-grey-7">Gateway / 后端进程文本日志流</div>
      </div>
      <q-input v-model="keyword" dense outlined clearable debounce="200" class="col-12 col-md-3" label="过滤日志">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-btn-toggle
        v-model="level"
        dense
        rounded
        unelevated
        toggle-color="primary"
        :options="levelOptions"
        class="col-auto"
      />
      <q-btn flat rounded :icon="paused ? 'play_arrow' : 'pause'" :label="paused ? '恢复' : '停止'" @click="toggle" />
      <q-btn flat rounded icon="delete_sweep" label="清除" @click="lines = []" />
    </q-card-section>
    <q-banner v-if="message" rounded class="monitor-log-banner q-ma-md">
      {{ message }}
    </q-banner>
    <q-separator />
    <q-card-section>
      <div class="monitor-log-console">
        <div v-for="line in filteredLines" :key="line.id + line.created_at" class="monitor-log-line" :class="`monitor-log-line--${line.level.toLowerCase()}`">
          <span class="monitor-log-time">{{ line.time }}</span>
          <span class="monitor-log-level">[{{ line.level }}]</span>
          <span>{{ line.message }}</span>
        </div>
        <div v-if="!filteredLines.length" class="monitor-log-empty">暂无日志</div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { getMonitorLogs, subscribeMonitorLogs } from "../../features/monitor/api";
import type { MonitorLogLine, StreamState } from "../../features/monitor/types";

const lines = ref<MonitorLogLine[]>([]);
const keyword = ref("");
const level = ref("INFO");
const paused = ref(false);
const state = ref<StreamState>("connecting");
const message = ref("");
let source: EventSource | null = null;

const levelOptions = [
  { label: "DEBUG", value: "DEBUG" },
  { label: "INFO", value: "INFO" },
  { label: "WARN", value: "WARN" },
  { label: "ERROR", value: "ERROR" }
];

const levelRank: Record<string, number> = { DEBUG: 10, INFO: 20, WARN: 30, ERROR: 40 };
const filteredLines = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  const minRank = levelRank[level.value] ?? 20;
  return lines.value.filter((line) => {
    if ((levelRank[line.level] ?? 20) < minRank) return false;
    if (!q) return true;
    return [line.level, line.message, line.source].some((value) => String(value || "").toLowerCase().includes(q));
  });
});

const stateColor = computed(() => state.value === "live" ? "positive" : state.value === "error" ? "negative" : state.value === "paused" ? "grey" : "orange");
const stateText = computed(() => ({ connecting: "连接中", live: "实时", paused: "已暂停", error: "连接异常" }[state.value]));

onMounted(async () => {
  const snapshot = await getMonitorLogs();
  lines.value = snapshot.items;
  message.value = snapshot.message;
  start();
});

onBeforeUnmount(stop);

watch(paused, (isPaused) => {
  if (isPaused) {
    stop();
    state.value = "paused";
  } else {
    start();
  }
});

function start() {
  stop();
  state.value = "connecting";
  source = subscribeMonitorLogs(
    (line) => {
      state.value = "live";
      lines.value = [...lines.value, line].slice(-5000);
    },
    () => {
      if (!paused.value) state.value = "error";
    },
    () => {
      if (!paused.value) state.value = "live";
    }
  );
}

function stop() {
  source?.close();
  source = null;
}

function toggle() {
  paused.value = !paused.value;
}
</script>
