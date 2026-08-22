import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  exportUsageEventsCsv,
  getCacheHitRatioStats,
  getModelUsageOverview,
  listModelUsageEvents,
  listModelUsageTrends,
  purgeUsageEvents,
} from '../../features/usage/api';
import type {
  CacheHitRatioStat,
  ModelTokenUsageEvent,
  ModelUsageOverview,
  ModelUsageQuery,
  ModelUsageTrendPoint,
} from '../../features/usage/types';

export const useUsageStore = defineStore('usage', () => {
  const overview = ref<ModelUsageOverview | null>(null);
  const trends = ref<ModelUsageTrendPoint[]>([]);
  const events = ref<ModelTokenUsageEvent[]>([]);
  const eventsTotal = ref(0);
  const loading = ref(false);
  const error = ref('');
  const eventsLoading = ref(false);
  const eventsError = ref('');
  const exporting = ref(false);

  async function loadOverview(query: ModelUsageQuery = {}, trendGranularity: 'day' | 'hour' = 'day') {
    loading.value = true;
    error.value = '';
    try {
      const o = await getModelUsageOverview(query);
      if (trendGranularity === 'hour') {
        o.trends = await listModelUsageTrends({ ...query, granularity: 'hour' });
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
    eventsError.value = '';
    try {
      const result = await listModelUsageEvents(query);
      events.value = result.items;
      eventsTotal.value = result.total;
      return result;
    } catch (e) {
      eventsError.value = e instanceof Error ? e.message : String(e);
      events.value = [];
      eventsTotal.value = 0;
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

  async function purgeEvents(retainDays: number): Promise<number> {
    const result = await purgeUsageEvents(retainDays);
    return result.deleted_count;
  }

  // --- 29-token §9.3 prompt 缓存命中率（平台级门禁 RPC） ---
  const cacheHitStats = ref<CacheHitRatioStat[]>([]);
  const cacheHitLoading = ref(false);
  // 非管理员调用 403 时置位，卡片据此静默隐藏（观测性降级，不弹错误）。
  const cacheHitDenied = ref(false);

  async function loadCacheHitStats(windowHours = 24) {
    cacheHitLoading.value = true;
    try {
      cacheHitStats.value = await getCacheHitRatioStats(windowHours);
      cacheHitDenied.value = false;
    } catch {
      cacheHitDenied.value = true;
      cacheHitStats.value = [];
    } finally {
      cacheHitLoading.value = false;
    }
  }

  return {
    overview,
    trends,
    events,
    eventsTotal,
    loading,
    error,
    eventsLoading,
    eventsError,
    exporting,
    cacheHitStats,
    cacheHitLoading,
    cacheHitDenied,
    loadOverview,
    fetchOverview,
    loadTrends,
    loadEvents,
    exportEventsCsv,
    purgeEvents,
    loadCacheHitStats,
  };
});
