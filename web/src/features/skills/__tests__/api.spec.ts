/**
 * skills api.ts 映射测试（R-1 回归：getSkillHealth 曾漏映射
 * routed_count_7d/loaded_count_7d/routed_count_30d/loaded_count_30d——
 * proto 字段 13-16 已返回，mapper 漏抄导致运行时 undefined，
 * SkillHealthCard「路由命中率」恒显示 "-"）。
 */
import { describe, expect, it, vi, beforeEach } from 'vitest';

const mockGetSkillHealth = vi.fn();
vi.mock('../../../services', () => ({
  createSkillService: () => ({ GetSkillHealth: mockGetSkillHealth }),
  createSkillIntelligenceService: () => ({}),
  createSkillEvolutionSuggestionService: () => ({}),
  kratosApi: { get: vi.fn(), post: vi.fn() },
}));

import { getSkillHealth } from '../api';

describe('getSkillHealth', () => {
  beforeEach(() => {
    mockGetSkillHealth.mockReset();
  });

  it('映射全部 7d/30d 统计字段，含路由命中分子/分母（snake_case）', async () => {
    mockGetSkillHealth.mockResolvedValue({
      skill_id: 's1',
      total_invocations_7d: 10,
      success_count_7d: 8,
      success_rate_7d: 0.8,
      p95_duration_ms_7d: 120,
      total_invocations_30d: 40,
      success_count_30d: 30,
      success_rate_30d: 0.75,
      p95_duration_ms_30d: 150,
      route_hit_rate_7d: 0.5,
      route_hit_rate_30d: 0.4,
      routed_count_7d: 5,
      loaded_count_7d: 3,
      routed_count_30d: 16,
      loaded_count_30d: 12,
      daily_metrics: [],
    });
    const out = await getSkillHealth('s1');
    expect(mockGetSkillHealth).toHaveBeenCalledTimes(1);
    expect(mockGetSkillHealth).toHaveBeenCalledWith({ skillId: 's1' });
    expect(out).toMatchObject({
      skill_id: 's1',
      route_hit_rate_7d: 0.5,
      route_hit_rate_30d: 0.4,
      routed_count_7d: 5,
      loaded_count_7d: 3,
      routed_count_30d: 16,
      loaded_count_30d: 12,
    });
  });

  it('兼容 camelCase 响应键', async () => {
    mockGetSkillHealth.mockResolvedValue({
      skillId: 's2',
      routeHitRate7d: 0.6,
      routedCount7d: 7,
      loadedCount7d: 4,
      routedCount30d: 20,
      loadedCount30d: 9,
    });
    const out = await getSkillHealth('s2');
    expect(out).toMatchObject({
      skill_id: 's2',
      route_hit_rate_7d: 0.6,
      routed_count_7d: 7,
      loaded_count_7d: 4,
      routed_count_30d: 20,
      loaded_count_30d: 9,
    });
  });

  it('路由统计字段缺失时兜底 0（不为 undefined）', async () => {
    mockGetSkillHealth.mockResolvedValue({ skill_id: 's3' });
    const out = await getSkillHealth('s3');
    expect(out.routed_count_7d).toBe(0);
    expect(out.loaded_count_7d).toBe(0);
    expect(out.routed_count_30d).toBe(0);
    expect(out.loaded_count_30d).toBe(0);
    expect(out.daily_metrics).toEqual([]);
  });
});
