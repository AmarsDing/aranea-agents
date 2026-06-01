import { computed, onMounted, onUnmounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import { useMonitorStore } from "../../stores/monitor";
import type { ModelUsageQuery, MonitorTraceEvent } from "./types";

const VALID_TABS = ["usage", "alerts", "audit", "events", "traces", "logs"] as const;

export function useMonitorPage() {
  const route = useRoute();
  const router = useRouter();
  const monitorStore = useMonitorStore();
  const { auditLogs, events } = storeToRefs(monitorStore);
  const initialTab = String(route.query.tab || "usage");
  const tab = ref(VALID_TABS.includes(initialTab as (typeof VALID_TABS)[number]) ? initialTab : "usage");
  const highlightUsageEventId = ref(String(route.query.usage_event_id || "").trim());
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

  const autoRefreshMs = 30_000;
  let refreshTimer: ReturnType<typeof setInterval> | null = null;

  function refreshActiveTab() {
    if (tab.value === "audit") void loadAudit();
    else if (tab.value === "events") void loadEvents();
    else if (tab.value === "traces") void loadTraces();
  }

  onMounted(() => {
    void loadAll();
    refreshTimer = setInterval(refreshActiveTab, autoRefreshMs);
  });

  onUnmounted(() => {
    if (refreshTimer != null) {
      clearInterval(refreshTimer);
      refreshTimer = null;
    }
  });

  watch(tab, async (value) => {
    if (!VALID_TABS.includes(value as (typeof VALID_TABS)[number])) {
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
      await monitorStore.loadAuditLogs({ limit: 200 });
    } finally {
      loadingAudit.value = false;
    }
  }

  async function loadEvents() {
    loadingEvents.value = true;
    try {
      await monitorStore.loadEvents();
    } finally {
      loadingEvents.value = false;
    }
  }

  async function loadTraces() {
    loadingTraces.value = true;
    try {
      traces.value = await monitorStore.fetchTraceEvents({ ...filters, limit: 100 });
    } finally {
      loadingTraces.value = false;
    }
  }

  return {
    tab,
    highlightUsageEventId,
    auditRows: auditLogs,
    events,
    traces,
    loadingAudit,
    loadingEvents,
    loadingTraces,
    error,
    filters,
    rangeOptions,
    loading,
    loadAll,
    loadAudit,
    loadEvents,
    loadTraces
  };
}
