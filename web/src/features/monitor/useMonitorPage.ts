import { computed, onMounted, onUnmounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { listMonitorAudit, listMonitorEvents, listMonitorTraceEvents } from "./api";
import type { AuditLog, ModelUsageQuery, MonitorTraceEvent, PlatformResource } from "./types";

const VALID_TABS = ["usage", "alerts", "audit", "events", "traces", "logs"] as const;

export function useMonitorPage() {
  const route = useRoute();
  const router = useRouter();
  const initialTab = String(route.query.tab || "usage");
  const tab = ref(VALID_TABS.includes(initialTab as (typeof VALID_TABS)[number]) ? initialTab : "usage");
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

  return {
    tab,
    highlightUsageEventId,
    auditRows,
    auditTotal,
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
