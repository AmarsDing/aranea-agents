import { describe, expect, it } from 'vitest';
import {
  formatToolArgsFirstPassRate,
  toolArgsFirstPassRate,
  toolArgsFirstPassRateColor,
} from '../toolUi';

describe('toolArgsFirstPassRate', () => {
  it('returns null when the tool has no invocations', () => {
    expect(toolArgsFirstPassRate({ invoke_count: 0, repaired_count: 0, invalid_count: 0 })).toBeNull();
  });

  it('returns 1 when no repair-guard markers were recorded', () => {
    expect(toolArgsFirstPassRate({ invoke_count: 10, repaired_count: 0, invalid_count: 0 })).toBe(1);
  });

  it('subtracts repaired and invalid shares from the invocation count', () => {
    // 10 calls, 2 repaired + 1 invalid → 0.7
    expect(toolArgsFirstPassRate({ invoke_count: 10, repaired_count: 2, invalid_count: 1 })).toBeCloseTo(0.7);
  });

  it('clamps at 0 when markers exceed the invocation window', () => {
    expect(toolArgsFirstPassRate({ invoke_count: 2, repaired_count: 3, invalid_count: 1 })).toBe(0);
  });
});

describe('formatToolArgsFirstPassRate', () => {
  it('renders a dash without invocations', () => {
    expect(formatToolArgsFirstPassRate({ invoke_count: 0, repaired_count: 0, invalid_count: 0 })).toBe('—');
  });

  it('renders a percentage with one decimal', () => {
    expect(formatToolArgsFirstPassRate({ invoke_count: 8, repaired_count: 1, invalid_count: 1 })).toBe('75.0%');
  });
});

describe('toolArgsFirstPassRateColor', () => {
  it('is grey without data, warning below threshold, positive otherwise', () => {
    expect(toolArgsFirstPassRateColor({ invoke_count: 0, repaired_count: 0, invalid_count: 0 })).toBe('grey');
    expect(toolArgsFirstPassRateColor({ invoke_count: 10, repaired_count: 1, invalid_count: 0 })).toBe('warning');
    expect(toolArgsFirstPassRateColor({ invoke_count: 10, repaired_count: 0, invalid_count: 0 })).toBe('positive');
  });
});
