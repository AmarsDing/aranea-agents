// features/companion/__tests__/launchParticles.spec.ts
import { describe, it, expect } from 'vitest';

import { makeBurstParticles, BURST_PARTICLE_COUNT } from '../launchParticles';

/** 确定性 rng 序列（LCG），测试粒子参数稳定性。 */
function seqRng(seed = 1): () => number {
  let s = seed;
  return () => {
    s = (s * 48271) % 2147483647;
    return s / 2147483647;
  };
}

describe('makeBurstParticles', () => {
  it('emits the requested count with default when omitted', () => {
    expect(makeBurstParticles(24, seqRng())).toHaveLength(24);
    expect(makeBurstParticles(undefined, seqRng())).toHaveLength(BURST_PARTICLE_COUNT);
  });

  it('keeps every parameter within its animation budget', () => {
    const ps = makeBurstParticles(200, seqRng(7));
    for (const p of ps) {
      // 发射方向偏上（-180°..0°，即向上半球）
      expect(p.angleDeg).toBeGreaterThanOrEqual(-180);
      expect(p.angleDeg).toBeLessThanOrEqual(0);
      expect(p.distance).toBeGreaterThan(40);
      expect(p.distance).toBeLessThanOrEqual(220);
      expect(p.durationMs).toBeGreaterThanOrEqual(450);
      expect(p.durationMs).toBeLessThanOrEqual(950);
      expect(p.delayMs).toBeGreaterThanOrEqual(0);
      expect(p.delayMs).toBeLessThanOrEqual(120);
      expect(p.size).toBeGreaterThanOrEqual(2);
      expect(p.size).toBeLessThanOrEqual(6);
    }
  });

  it('is deterministic under an injected rng (same seed → same params)', () => {
    const a = makeBurstParticles(10, seqRng(42));
    const b = makeBurstParticles(10, seqRng(42));
    expect(a).toEqual(b);
  });
});
