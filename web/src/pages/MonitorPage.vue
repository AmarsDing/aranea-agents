<template>
  <q-page class="app-page-cream monitor-page q-pa-sm q-pa-md-md">
    <section class="monitor-hero">
      <div>
        <div class="monitor-kicker">Observability</div>
        <h1 class="monitor-title">运行监控</h1>
        <p class="monitor-subtitle">审计、实时事件、真实模型调用 Trace 与日志预留入口统一查看。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-select
          v-model="filters.range"
          dense
          outlined
          emit-value
          map-options
          label="时间范围"
          :options="rangeOptions"
          class="monitor-range"
          @update:model-value="loadTraces"
        />
        <q-btn color="primary" rounded unelevated icon="refresh" label="刷新" :loading="loading" @click="loadAll" />
      </div>
    </section>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadAll" />
      </template>
    </q-banner>

    <q-card flat class="dashboard-glass-card monitor-tabs-card">
      <q-tabs
        v-model="tab"
        align="left"
        active-color="primary"
        indicator-color="primary"
        no-caps
        outside-arrows
        mobile-arrows
      >
        <q-tab name="audit" icon="fact_check" label="Audit" />
        <q-tab name="events" icon="sensors" label="Events" />
        <q-tab name="traces" icon="account_tree" label="Traces" />
        <q-tab name="logs" icon="terminal" label="Logs" />
      </q-tabs>
    </q-card>

    <q-tab-panels v-model="tab" animated class="monitor-panels">
      <q-tab-panel name="audit">
        <AuditTable :rows="auditRows" :loading="loadingAudit" @reload="loadAudit" />
      </q-tab-panel>
      <q-tab-panel name="events">
        <RealtimeEvents :persisted-events="events" />
      </q-tab-panel>
      <q-tab-panel name="traces">
        <TraceList :rows="traces" :loading="loadingTraces" @reload="loadTraces" />
      </q-tab-panel>
      <q-tab-panel name="logs">
        <LogStream />
      </q-tab-panel>
    </q-tab-panels>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import AuditTable from "../features/monitor/AuditTable.vue";
import LogStream from "../features/monitor/LogStream.vue";
import RealtimeEvents from "../features/monitor/RealtimeEvents.vue";
import TraceList from "../features/monitor/TraceList.vue";
import { listMonitorAudit, listMonitorEvents, listMonitorTraceEvents } from "../features/monitor/api";
import type { AuditLog, ModelUsageQuery, MonitorTraceEvent, PlatformResource } from "../features/monitor/types";

const route = useRoute();
const router = useRouter();
const initialTab = String(route.query.tab || "audit");
const tab = ref(["audit", "events", "traces", "logs"].includes(initialTab) ? initialTab : "audit");
const auditRows = ref<AuditLog[]>([]);
const events = ref<PlatformResource[]>([]);
const traces = ref<MonitorTraceEvent[]>([]);
const loadingAudit = ref(false);
const loadingEvents = ref(false);
const loadingTraces = ref(false);
const error = ref("");

const filters = reactive<ModelUsageQuery>({
  range: "30d",
  limit: 50
});

const rangeOptions = [
  { label: "今日", value: "today" },
  { label: "7 天", value: "7d" },
  { label: "30 天", value: "30d" },
  { label: "本月", value: "month" }
];

const loading = computed(() => loadingAudit.value || loadingEvents.value || loadingTraces.value);

onMounted(loadAll);

watch(tab, async (value) => {
  if (!["audit", "events", "traces", "logs"].includes(value)) {
    tab.value = "audit";
    return;
  }
  await router.replace({ query: { ...route.query, tab: value } });
});

async function loadAll() {
  error.value = "";
  try {
    await Promise.all([loadAudit(), loadEvents(), loadTraces()]);
  } catch (err) {
    error.value = err instanceof Error ? err.message : "加载监控数据失败";
  }
}

async function loadAudit() {
  loadingAudit.value = true;
  try {
    auditRows.value = await listMonitorAudit(200);
  } finally {
    loadingAudit.value = false;
  }
}

async function loadEvents() {
  loadingEvents.value = true;
  try {
    events.value = await listMonitorEvents();
  } finally {
    loadingEvents.value = false;
  }
}

async function loadTraces() {
  loadingTraces.value = true;
  try {
    traces.value = await listMonitorTraceEvents({ ...filters, limit: 100 });
  } finally {
    loadingTraces.value = false;
  }
}
</script>

<style>
.monitor-page {
  min-height: 100%;
}

.monitor-hero {
  align-items: center;
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 4px 20px;
}

.monitor-kicker {
  color: #9a6a4f;
  font-size: 12px;
  font-weight: 800;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.monitor-title {
  color: #4e342e;
  font-size: clamp(28px, 4vw, 44px);
  font-weight: 900;
  letter-spacing: -0.04em;
  line-height: 1.05;
  margin: 4px 0;
}

.monitor-subtitle {
  color: #795548;
  margin: 0;
  max-width: 720px;
}

.monitor-range {
  min-width: 150px;
}

.monitor-tabs-card {
  border-radius: 18px;
  margin-bottom: 12px;
  overflow: hidden;
}

.monitor-panels {
  background: transparent;
}

.monitor-panels .q-tab-panel {
  padding: 0;
}

.monitor-card {
  border-radius: 18px;
}

.monitor-detail-card {
  border-radius: 18px;
  max-width: 880px;
  width: min(880px, 92vw);
}

.monitor-trace-dialog {
  background: #fefdf5;
}

.monitor-json {
  background: rgba(31, 41, 55, 0.94);
  border-radius: 14px;
  color: #d1fae5;
  font-size: 12px;
  line-height: 1.6;
  margin: 0;
  max-height: 60vh;
  overflow: auto;
  padding: 14px;
  white-space: pre-wrap;
  word-break: break-word;
}

.monitor-code {
  background: rgba(121, 85, 72, 0.08);
  border-radius: 6px;
  padding: 2px 6px;
}

.monitor-empty {
  align-items: center;
  color: #8d6e63;
  display: grid;
  gap: 8px;
  justify-items: center;
  min-height: 180px;
}

.monitor-event-item {
  border-radius: 14px;
  margin-bottom: 4px;
}

.monitor-log-banner {
  background: rgba(255, 248, 231, 0.88);
  color: #5d4037;
}

.monitor-log-console {
  background: #111827;
  border-radius: 16px;
  color: #d1d5db;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  min-height: 320px;
  overflow: auto;
  padding: 14px;
}

.monitor-log-line {
  display: flex;
  gap: 10px;
  line-height: 1.7;
  min-width: max-content;
}

.monitor-log-time {
  color: #93c5fd;
}

.monitor-log-level {
  color: #c4b5fd;
  font-weight: 700;
}

.monitor-log-line--warn .monitor-log-level {
  color: #fbbf24;
}

.monitor-log-line--error .monitor-log-level {
  color: #f87171;
}

.monitor-log-empty {
  color: #9ca3af;
  display: grid;
  min-height: 220px;
  place-items: center;
}

body.body--dark .monitor-title,
body.body--dark .monitor-kicker,
body.body--dark .monitor-subtitle {
  color: inherit;
}

body.body--dark .monitor-trace-dialog {
  background: #121212;
}

@media (max-width: 720px) {
  .monitor-hero {
    align-items: stretch;
    flex-direction: column;
  }

  .monitor-range {
    min-width: 100%;
  }
}
</style>
