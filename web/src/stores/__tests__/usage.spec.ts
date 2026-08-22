import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useUsageStore } from '../usage';
import { getCacheHitRatioStats } from '../../features/usage/api';

vi.mock('../../features/usage/api', () => ({
  exportUsageEventsCsv: vi.fn(),
  getCacheHitRatioStats: vi.fn(),
  getModelUsageOverview: vi.fn(),
  listModelUsageEvents: vi.fn(),
  listModelUsageTrends: vi.fn(),
  purgeUsageEvents: vi.fn(),
}));

describe('useUsageStore.loadCacheHitStats（29-token §9.3）', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('成功时写入 stats 并清除 denied', async () => {
    vi.mocked(getCacheHitRatioStats).mockResolvedValueOnce([
      {
        provider: 'deepseek',
        model: 'deepseek-chat',
        samples: 25,
        prompt_tokens: 100000,
        cached_tokens: 60000,
        weighted_ratio: 0.6,
        p50_ratio: 0.72,
      },
    ]);
    const store = useUsageStore();
    await store.loadCacheHitStats(24);
    expect(getCacheHitRatioStats).toHaveBeenCalledWith(24);
    expect(store.cacheHitStats).toHaveLength(1);
    expect(store.cacheHitStats[0].p50_ratio).toBe(0.72);
    expect(store.cacheHitDenied).toBe(false);
    expect(store.cacheHitLoading).toBe(false);
  });

  it('失败（403/网络）时置 denied 并清空 stats——卡片静默降级', async () => {
    vi.mocked(getCacheHitRatioStats).mockRejectedValueOnce(new Error('Forbidden'));
    const store = useUsageStore();
    await store.loadCacheHitStats(1);
    expect(store.cacheHitDenied).toBe(true);
    expect(store.cacheHitStats).toEqual([]);
    expect(store.cacheHitLoading).toBe(false);
  });

  it('失败后再次成功可恢复（denied 复位）', async () => {
    vi.mocked(getCacheHitRatioStats).mockRejectedValueOnce(new Error('Forbidden'));
    const store = useUsageStore();
    await store.loadCacheHitStats(1);
    expect(store.cacheHitDenied).toBe(true);

    vi.mocked(getCacheHitRatioStats).mockResolvedValueOnce([]);
    await store.loadCacheHitStats(1);
    expect(store.cacheHitDenied).toBe(false);
  });
});
