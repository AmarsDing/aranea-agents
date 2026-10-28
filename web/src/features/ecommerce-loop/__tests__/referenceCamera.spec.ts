import { describe, expect, it } from 'vitest';
import { cameraTransform, referenceTarget } from '../referenceCamera';

describe('reference camera', () => {
  it('uses a uniform transform with the measured panel at the source target', () => {
    const target = referenceTarget(3, null, 'suggestion')!;
    const result = cameraTransform({ x: 1025, y: 648, width: 820 }, target);
    const numbers = result.match(/-?[\d.]+/g)!.map(Number);
    expect(numbers[0]).toBe(numbers[3]);
    expect(1025 * numbers[0]! + numbers[4]!).toBeCloseTo(target.x);
    expect(648 * numbers[3]! + numbers[5]!).toBeCloseTo(target.y);
    expect(820 * numbers[0]!).toBeCloseTo(target.width);
  });

  it('uses different cameras for the same step on the two flow pages', () => {
    expect(referenceTarget(2, 4, null)).not.toEqual(referenceTarget(4, 4, null));
    expect(referenceTarget(3, 4, 'ceo-detail')).toBeNull();
    expect(referenceTarget(3, null, 'funnel-overview')).toBeNull();
  });
});
