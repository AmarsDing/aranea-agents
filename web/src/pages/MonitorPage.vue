<template>
  <q-page class="app-page-cream monitor-page">
    <MonitorHeroSection
      kicker="Observability"
      title="运行监控"
      subtitle="审计、实时事件、模型用量总览、真实模型调用 Trace 与日志预留入口统一查看。"
    >
      <template #actions>
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
        <q-btn rounded no-caps unelevated class="monitor-primary-btn" icon="refresh" label="刷新" :loading="loading" @click="loadAll" />
      </template>
    </MonitorHeroSection>

    <MonitorErrorBanner v-if="error" :message="error" @retry="loadAll" />

    <div class="monitor-tabs-wrap">
      <MonitorGlassPanel>
        <q-tabs
          v-model="tab"
          class="monitor-tabs"
          align="left"
          no-caps
          outside-arrows
          mobile-arrows
        >
          <q-tab name="usage" icon="dashboard" label="Usage" />
          <q-tab name="alerts" icon="notifications_active" label="Alerts" />
          <q-tab name="audit" icon="fact_check" label="Audit" />
          <q-tab name="events" icon="sensors" label="Events" />
          <q-tab name="traces" icon="account_tree" label="Traces" />
          <q-tab name="logs" icon="terminal" label="Logs" />
        </q-tabs>
      </MonitorGlassPanel>
    </div>

    <q-tab-panels v-model="tab" animated class="monitor-panels" :class="{ 'monitor-panels--logs-fill': tab === 'logs' }">
      <q-tab-panel name="usage">
        <MonitorRunnerMetrics />
        <MonitorUsageDashboardLink :range="filters.range" />
      </q-tab-panel>
      <q-tab-panel name="alerts">
        <MonitorAlertRules />
      </q-tab-panel>
      <q-tab-panel name="audit">
        <AuditTable :rows="auditRows" :total="auditTotal" :loading="loadingAudit" @reload="loadAudit" />
      </q-tab-panel>
      <q-tab-panel name="events">
        <RealtimeEvents :persisted-events="events" :traces="traces" />
      </q-tab-panel>
      <q-tab-panel name="traces">
        <TraceList
          :rows="traces"
          :loading="loadingTraces"
          :highlight-usage-event-id="highlightUsageEventId"
          @reload="loadTraces"
        />
      </q-tab-panel>
      <q-tab-panel name="logs" class="monitor-logs-panel">
        <LogStreamPanel />
      </q-tab-panel>
    </q-tab-panels>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import MonitorErrorBanner from "../components/monitor/MonitorErrorBanner.vue";
import MonitorGlassPanel from "../components/monitor/MonitorGlassPanel.vue";
import MonitorHeroSection from "../components/monitor/MonitorHeroSection.vue";
import AuditTable from "../components/monitor/AuditTable.vue";
import LogStreamPanel from "../components/monitor/LogStreamPanel.vue";
import RealtimeEvents from "../components/monitor/RealtimeEvents.vue";
import TraceList from "../components/monitor/TraceList.vue";
import MonitorUsageDashboardLink from "../components/monitor/MonitorUsageDashboardLink.vue";
import MonitorRunnerMetrics from "../components/monitor/MonitorRunnerMetrics.vue";
import MonitorAlertRules from "../components/monitor/MonitorAlertRules.vue";
import { listMonitorAudit, listMonitorEvents, listMonitorTraceEvents } from "../features/monitor/api";
import type { AuditLog, ModelUsageQuery, MonitorTraceEvent, PlatformResource } from "../features/monitor/types";

const route = useRoute();
const router = useRouter();
const validTabs = ["usage", "alerts", "audit", "events", "traces", "logs"];
const initialTab = String(route.query.tab || "usage");
const tab = ref(validTabs.includes(initialTab) ? initialTab : "usage");
const highlightUsageEventId = ref(String(route.query.usage_event_id || "").trim());
const auditRows = ref<AuditLog[]>([]);
const auditTotal = ref(0);
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
  if (!validTabs.includes(value)) {
    tab.value = "usage";
    return;
  }
  await router.replace({ query: { ...route.query, tab: value } });
});

watch(
  () => route.query.usage_event_id,
  (id) => {
    highlightUsageEventId.value = String(id || "").trim();
    if (highlightUsageEventId.value && tab.value !== "traces") {
      tab.value = "traces";
    }
  }
);

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
    const result = await listMonitorAudit({ limit: 200 });
    auditRows.value = result.items;
    auditTotal.value = result.total;
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

<style scoped lang="sass">
.monitor-page
  min-height: 100%
  padding: 24px
  display: flex
  flex-direction: column
  min-height: calc(100dvh - 56px)

.monitor-panels--logs-fill
  flex: 1
  min-height: 0
  display: flex
  flex-direction: column

.monitor-panels--logs-fill :deep(.q-panel)
  flex: 1
  min-height: 0
  display: flex
  flex-direction: column

.monitor-logs-panel
  flex: 1
  min-height: 0
  display: flex
  flex-direction: column

.monitor-range
  min-width: 150px

.monitor-tabs-wrap
  margin-bottom: 12px

.monitor-panels
  background: transparent

.monitor-panels :deep(.q-tab-panel)
  padding: 0

.monitor-tabs :deep(.q-tabs__indicator)
  background: var(--color-accent)

.monitor-tabs :deep(.q-tab--active)
  color: var(--color-accent)

.monitor-tabs :deep(.q-tab:not(.q-tab--active))
  color: var(--color-text-secondary)

.monitor-tabs :deep(.q-tabs__arrow)
  color: var(--color-icon-muted)

.monitor-primary-btn
  background: var(--color-accent)
  color: var(--color-on-accent)

body:not(.body--dark) .monitor-primary-btn:hover
  background: var(--color-accent-hover)

@media (max-width: 720px)
  .monitor-range
    min-width: 100%
</style>
