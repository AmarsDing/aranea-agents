import { describe, expect, it } from 'vitest';
import {
  allowedGranularitiesForRange,
  defaultGranularityForRange,
  isGranularityValidForRange,
  resolveGranularityForRange,
} from '../usageGranularityLinkage';

describe('usageGranularityLinkage', () => {
  describe('allowedGranularitiesForRange', () => {
    it('returns [hour] for today (only hourly trend makes sense)', () => {
      expect(allowedGranularitiesForRange('today')).toEqual(['hour']);
    });

    it('returns [day, hour] for 7d (both make sense)', () => {
      expect(allowedGranularitiesForRange('7d')).toEqual(['day', 'hour']);
    });

    it('returns [day] for 30d (hour would have 720 points)', () => {
      expect(allowedGranularitiesForRange('30d')).toEqual(['day']);
    });

    it('returns [day] for month (same as 30d)', () => {
      expect(allowedGranularitiesForRange('month')).toEqual(['day']);
    });

    it('falls back to [day] for unknown range', () => {
      expect(allowedGranularitiesForRange('quarter')).toEqual(['day']);
      expect(allowedGranularitiesForRange('')).toEqual(['day']);
    });
  });

  describe('defaultGranularityForRange', () => {
    it('returns hour for today', () => {
      expect(defaultGranularityForRange('today')).toBe('hour');
    });

    it('returns day for 7d (day is first option, sensible default)', () => {
      expect(defaultGranularityForRange('7d')).toBe('day');
    });

    it('returns day for 30d', () => {
      expect(defaultGranularityForRange('30d')).toBe('day');
    });

    it('returns day for unknown range fallback', () => {
      expect(defaultGranularityForRange('quarter')).toBe('day');
    });
  });

  describe('isGranularityValidForRange', () => {
    it('returns true when granularity is in allowed list', () => {
      expect(isGranularityValidForRange('hour', 'today')).toBe(true);
      expect(isGranularityValidForRange('day', '7d')).toBe(true);
      expect(isGranularityValidForRange('hour', '7d')).toBe(true);
      expect(isGranularityValidForRange('day', '30d')).toBe(true);
    });

    it('returns false when granularity is not in allowed list', () => {
      expect(isGranularityValidForRange('day', 'today')).toBe(false);
      expect(isGranularityValidForRange('hour', '30d')).toBe(false);
      expect(isGranularityValidForRange('hour', 'month')).toBe(false);
    });
  });

  describe('resolveGranularityForRange', () => {
    it('keeps current granularity when valid for new range', () => {
      expect(resolveGranularityForRange('day', '7d')).toBe('day');
      expect(resolveGranularityForRange('hour', '7d')).toBe('hour');
      expect(resolveGranularityForRange('day', '30d')).toBe('day');
      expect(resolveGranularityForRange('hour', 'today')).toBe('hour');
    });

    it('falls back to default when current granularity is invalid for new range', () => {
      // today only allows hour; switching from day → today falls back to hour
      expect(resolveGranularityForRange('day', 'today')).toBe('hour');
      // 30d only allows day; switching from hour → 30d falls back to day
      expect(resolveGranularityForRange('hour', '30d')).toBe('day');
      // month only allows day; switching from hour → month falls back to day
      expect(resolveGranularityForRange('hour', 'month')).toBe('day');
    });

    it('falls back to day for unknown range', () => {
      expect(resolveGranularityForRange('hour', 'quarter')).toBe('day');
    });
  });
});
