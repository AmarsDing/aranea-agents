import { describe, expect, it } from 'vitest';
import { formatPercent, ratioValue } from './sessionUi';

describe('sessionUi context display', () => {
  it('formatPercent shows real percentage above 100% (去钳制)', () => {
    expect(formatPercent(1.5625)).toBe('156%');
  });

  it('formatPercent keeps lower bound at 0', () => {
    expect(formatPercent(-0.2)).toBe('0%');
  });

  it('formatPercent shows normal percentage', () => {
    expect(formatPercent(0.5)).toBe('50%');
  });

  it('ratioValue stays clamped for progress bar value (0-1)', () => {
    expect(ratioValue(1.5625)).toBe(1);
    expect(ratioValue(0.5)).toBe(0.5);
    expect(ratioValue(-0.2)).toBe(0);
  });
});
