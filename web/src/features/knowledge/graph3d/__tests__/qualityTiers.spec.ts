/**
 * qualityTiers.spec：G5 渲染管线 v2 自适应画质契约（万级节点流畅兜底）。
 *
 * - 初始分级按节点数：<2500 HIGH / <8000 MID / 其余 LOW
 * - 运行期 governor：连续低帧降档（不超低档），连续高帧升档（不超初始档顶）
 */
import { describe, expect, it } from 'vitest';
import {
  GOVERN_DOWN_FPS,
  GOVERN_DOWN_FRAMES,
  GOVERN_UP_FPS,
  GOVERN_UP_FRAMES,
  QUALITY_HIGH,
  QUALITY_LOW,
  QUALITY_MED,
  QUALITY_SPECS,
  TIER_LOW_MIN_NODES,
  TIER_MED_MIN_NODES,
  governTier,
  initialTier,
} from '../qualityTiers';

describe('initialTier', () => {
  it('节点数 < 2500 → HIGH', () => {
    expect(initialTier(0)).toBe(QUALITY_HIGH);
    expect(initialTier(TIER_MED_MIN_NODES - 1)).toBe(QUALITY_HIGH);
  });

  it('2500 ≤ 节点数 < 8000 → MID', () => {
    expect(initialTier(TIER_MED_MIN_NODES)).toBe(QUALITY_MED);
    expect(initialTier(TIER_LOW_MIN_NODES - 1)).toBe(QUALITY_MED);
  });

  it('节点数 ≥ 8000 → LOW', () => {
    expect(initialTier(TIER_LOW_MIN_NODES)).toBe(QUALITY_LOW);
    expect(initialTier(50000)).toBe(QUALITY_LOW);
  });
});

describe('QUALITY_SPECS 契约', () => {
  it('HIGH：bloom 开 + pixelRatio≤2 + 200 标签候选', () => {
    const s = QUALITY_SPECS[QUALITY_HIGH];
    expect(s.bloom).toBe(true);
    expect(s.maxPixelRatio).toBe(2);
    expect(s.labelCandidates).toBe(200);
    expect(s.label).toBe('HIGH');
  });

  it('LOW：bloom 关 + pixelRatio=1 + 40 标签候选', () => {
    const s = QUALITY_SPECS[QUALITY_LOW];
    expect(s.bloom).toBe(false);
    expect(s.maxPixelRatio).toBe(1);
    expect(s.labelCandidates).toBe(40);
    expect(s.label).toBe('LOW');
  });
});

describe('governTier', () => {
  it('连续低帧 ≥ DOWN_FRAMES → 降一档（不超低档）', () => {
    expect(governTier(QUALITY_HIGH, GOVERN_DOWN_FRAMES, 0, QUALITY_HIGH)).toBe(QUALITY_MED);
    expect(governTier(QUALITY_MED, GOVERN_DOWN_FRAMES + 10, 0, QUALITY_MED)).toBe(QUALITY_LOW);
    expect(governTier(QUALITY_LOW, GOVERN_DOWN_FRAMES + 100, 0, QUALITY_LOW)).toBe(QUALITY_LOW);
  });

  it('连续高帧 ≥ UP_FRAMES → 升一档（不超初始档顶）', () => {
    expect(governTier(QUALITY_LOW, 0, GOVERN_UP_FRAMES, QUALITY_MED)).toBe(QUALITY_MED);
    // 初始档顶是 MID：LOW→MID 后不得继续升 HIGH
    expect(governTier(QUALITY_MED, 0, GOVERN_UP_FRAMES + 100, QUALITY_MED)).toBe(QUALITY_MED);
    expect(governTier(QUALITY_HIGH, 0, GOVERN_UP_FRAMES + 100, QUALITY_HIGH)).toBe(QUALITY_HIGH);
  });

  it('未达帧数阈值 → 保持原档', () => {
    expect(governTier(QUALITY_HIGH, GOVERN_DOWN_FRAMES - 1, 0, QUALITY_HIGH)).toBe(QUALITY_HIGH);
    expect(governTier(QUALITY_MED, 0, GOVERN_UP_FRAMES - 1, QUALITY_MED)).toBe(QUALITY_MED);
    expect(governTier(QUALITY_MED, 10, 10, QUALITY_MED)).toBe(QUALITY_MED);
  });

  it('降档优先于升档（低帧与高帧计数不会同时成立，防御分支）', () => {
    expect(governTier(QUALITY_HIGH, GOVERN_DOWN_FRAMES, GOVERN_UP_FRAMES, QUALITY_HIGH)).toBe(QUALITY_MED);
  });
});

describe('governor 常量契约', () => {
  it('低帧阈值 45fps/90 帧；高帧阈值 57fps/600 帧', () => {
    expect(GOVERN_DOWN_FPS).toBe(45);
    expect(GOVERN_UP_FPS).toBe(57);
    expect(GOVERN_DOWN_FRAMES).toBe(90);
    expect(GOVERN_UP_FRAMES).toBe(600);
  });
});
