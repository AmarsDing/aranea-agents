import { describe, expect, it } from 'vitest';
import { hasPricingConfigured, shouldWarnZeroCost } from '../pricingWarning';

describe('pricingWarning', () => {
  it('detects missing pricing', () => {
    expect(hasPricingConfigured({})).toBe(false);
    expect(hasPricingConfigured({ inputPrice: 0, outputPrice: 0 })).toBe(false);
    expect(hasPricingConfigured({ inputPrice: 1 })).toBe(true);
  });

  it('warns when tokens consumed but cost zero', () => {
    expect(shouldWarnZeroCost({ totalTokens: 100, totalCostMicroUsd: 0 })).toBe(true);
    expect(shouldWarnZeroCost({ totalTokens: 0, totalCostMicroUsd: 0 })).toBe(false);
    expect(shouldWarnZeroCost({ totalTokens: 10, totalCostMicroUsd: 5 })).toBe(false);
  });
});
