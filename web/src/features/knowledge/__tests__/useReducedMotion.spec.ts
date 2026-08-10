import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { useReducedMotion } from '../useReducedMotion';

function stubMatchMedia(matches: boolean) {
  const listeners = new Set<(e: MediaQueryListEvent) => void>();
  const mql = {
    matches,
    media: '(prefers-reduced-motion: reduce)',
    addEventListener: (_: string, cb: (e: MediaQueryListEvent) => void) => listeners.add(cb),
    removeEventListener: (_: string, cb: (e: MediaQueryListEvent) => void) => listeners.delete(cb),
    dispatch(matchesNext: boolean) {
      const evt = { matches: matchesNext } as MediaQueryListEvent;
      listeners.forEach((cb) => cb(evt));
    },
  };
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation(() => mql),
  );
  return mql;
}

describe('useReducedMotion', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reflects initial media query state', () => {
    stubMatchMedia(true);
    const { reducedMotion } = useReducedMotion();
    expect(reducedMotion.value).toBe(true);
  });

  it('defaults to false when user has no preference', () => {
    stubMatchMedia(false);
    const { reducedMotion } = useReducedMotion();
    expect(reducedMotion.value).toBe(false);
  });

  it('tracks live changes of the media query', () => {
    const mql = stubMatchMedia(false);
    const { reducedMotion } = useReducedMotion();
    expect(reducedMotion.value).toBe(false);
    mql.dispatch(true);
    expect(reducedMotion.value).toBe(true);
  });

  it('degrades to false when matchMedia is unavailable', () => {
    vi.stubGlobal('matchMedia', undefined);
    const { reducedMotion } = useReducedMotion();
    expect(reducedMotion.value).toBe(false);
  });
});
