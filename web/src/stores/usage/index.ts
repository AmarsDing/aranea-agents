import { defineStore } from "pinia";
import { ref } from "vue";
import {
  exportUsageEventsCsv,
  getModelUsageOverview,
  listModelUsageEvents,
  listModelUsageTrends
} from "../../features/usage/api";
import type { ModelTokenUsageEvent, ModelUsageOverview, ModelUsageQuery, ModelUsageTrendPoint } from "../../features/usage/types";

export const useUsageStore = defineStore("usage", () => {
  const overview = ref<ModelUsageOverview | null>(null);
  const trends = ref<ModelUsageTrendPoint[]>([]);
  const events = ref<ModelTokenUsageEvent[]>([]);
  const loading = ref(false);
  const error = ref("");
  const eventsLoading = ref(false);
  const eventsError = ref("");
  const exporting = ref(false);

  async function loadOverview(query: ModelUsageQuery = {}, trendGranularity: "day" | "hour" = "day") {
    loading.value = true;
    error.value = "";
    try {
      const o = await getModelUsageOverview(query);
      if (trendGranularity === "hour") {
        o.trends = await listModelUsageTrends({ ...query, granularity: "hour" });
      }
      overview.value = o;
      return o;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      loading.value = false;
    }
  }

  async function fetchOverview(query?: ModelUsageQuery): Promise<ModelUsageOverview> {
    return getModelUsageOverview(query);
  }

  async function loadTrends(query?: ModelUsageQuery) {
    trends.value = await listModelUsageTrends(query);
  }

  async function loadEvents(query: ModelUsageQuery = {}) {
    eventsLoading.value = true;
    eventsError.value = "";
    try {
      const result = await listModelUsageEvents(query);
      events.value = Array.isArray(result) ? result : [];
      return result;
    } catch (e) {
      eventsError.value = e instanceof Error ? e.message : String(e);
      events.value = [];
      throw e;
    } finally {
      eventsLoading.value = false;
    }
  }

  async function exportEventsCsv(query: ModelUsageQuery = {}): Promise<string> {
    exporting.value = true;
    try {
      return await exportUsageEventsCsv(query);
    } finally {
      exporting.value = false;
    }
  }

  return {
    overview,
    trends,
    events,
    loading,
    error,
    eventsLoading,
    eventsError,
    exporting,
    loadOverview,
    fetchOverview,
    loadTrends,
    loadEvents,
    exportEventsCsv
  };
});
