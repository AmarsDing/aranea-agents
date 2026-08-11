import { describe, expect, it } from 'vitest';
import { normalizeUsdPer1M } from '../providerRuntimeOverlay';

// 上游 models.dev 价格偶发 float32 加宽噪声（如 0.14000000059604645），
// 加载进编辑表单前必须归一化，否则 number input 原样显示长小数。
describe('normalizeUsdPer1M', () => {
  it('strips float32-widening noise from upstream catalog prices', () => {
    expect(normalizeUsdPer1M(0.14000000059604645)).toBe(0.14);
  });

  it('keeps clean values untouched', () => {
    expect(normalizeUsdPer1M(2.5)).toBe(2.5);
    expect(normalizeUsdPer1M(0.075)).toBe(0.075);
  });

  it('preserves precision up to 6 decimals', () => {
    expect(normalizeUsdPer1M(0.000125)).toBe(0.000125);
  });

  it('maps non-positive / non-finite to 0', () => {
    expect(normalizeUsdPer1M(0)).toBe(0);
    expect(normalizeUsdPer1M(-3)).toBe(0);
    expect(normalizeUsdPer1M(Number.NaN)).toBe(0);
  });
});
