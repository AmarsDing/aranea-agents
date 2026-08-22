import { describe, expect, it } from 'vitest';
import { buildL0Waterfall } from '../l0Waterfall';

describe('buildL0Waterfall', () => {
  it('orders sections system → memory → user and computes percents', () => {
    const bars = buildL0Waterfall({
      user: { token_estimate: 20, message_count: 1 },
      l3: { token_estimate: 30, message_count: 0 },
      system: { token_estimate: 50, message_count: 1 },
    });
    expect(bars.map((b) => b.section)).toEqual(['system', 'l3', 'user']);
    expect(bars[0].percent).toBe(50);
    expect(bars[1].percent).toBe(30);
    expect(bars[2].percent).toBe(20);
  });

  it('returns empty for missing segments', () => {
    expect(buildL0Waterfall(null)).toEqual([]);
    expect(buildL0Waterfall({})).toEqual([]);
  });
});
