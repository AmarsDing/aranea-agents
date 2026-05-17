import { defineStore } from "pinia";
import { ref } from "vue";
import { getModelUsageOverview, listModelUsageTrends, listModelUsageEvents } from "../../features/usage/api";
import type { ModelUsageOverview, ModelUsageTrendPoint, ModelTokenUsageEvent, ModelUsageQuery } from "../../features/usage/types";

export const useUsageStore = defineStore("usage", () => {
  const overview = ref<ModelUsageOverview | null>(null);
  const trends = ref<ModelUsageTrendPoint[]>([]);
  const events = ref<ModelTokenUsageEvent[]>([]);
  const loading = ref(false);

  async function loadOverview(query?: ModelUsageQuery) {
    loading.value = true;
    try {
      overview.value = await getModelUsageOverview(query);
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

  async function loadEvents(query?: ModelUsageQuery) {
    const result = await listModelUsageEvents(query);
    events.value = Array.isArray(result) ? result : [];
    return result;
  }

  return { overview, trends, events, loading, loadOverview, fetchOverview, loadTrends, loadEvents };
});
