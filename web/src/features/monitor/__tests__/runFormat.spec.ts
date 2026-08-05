import { describe, expect, it } from 'vitest';
import { formatCompactInt, formatCostUsd } from '../runFormat';

describe('formatCompactInt', () => {
  it('0 与非法值归零', () => {
    expect(formatCompactInt(0)).toBe('0');
    expect(formatCompactInt(-5)).toBe('0');
    expect(formatCompactInt(NaN)).toBe('0');
  });

  it('<10000 千分位原样', () => {
    expect(formatCompactInt(999)).toBe('999');
    expect(formatCompactInt(1234)).toBe('1,234');
    expect(formatCompactInt(9999)).toBe('9,999');
  });

  it('≥10000 缩写 k', () => {
    expect(formatCompactInt(10000)).toBe('10k');
    expect(formatCompactInt(156779)).toBe('156.8k');
    expect(formatCompactInt(999499)).toBe('999.5k');
  });

  it('≥1M 缩写 M', () => {
    expect(formatCompactInt(1_000_000)).toBe('1M');
    expect(formatCompactInt(2_340_000)).toBe('2.3M');
  });
});

describe('formatCostUsd', () => {
  it('0 与非法值', () => {
    expect(formatCostUsd(0)).toBe('$0.00');
    expect(formatCostUsd(-1)).toBe('$0.00');
  });

  it('极小成本', () => {
    expect(formatCostUsd(0.0004)).toBe('<$0.01');
  });

  it('<1 四位小数', () => {
    expect(formatCostUsd(0.021999)).toBe('$0.0220');
    expect(formatCostUsd(0.5)).toBe('$0.5000');
  });

  it('≥1 两位小数', () => {
    expect(formatCostUsd(1)).toBe('$1.00');
    expect(formatCostUsd(12.345)).toBe('$12.35');
  });
});
