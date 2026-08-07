import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ref } from 'vue';

// Mock onUnmounted since we're not in a component context
vi.mock('vue', async () => {
  const actual = await vi.importActual<typeof import('vue')>('vue');
  return {
    ...actual,
    onUnmounted: vi.fn(),
  };
});

import { hasActiveRuns, useEvalRunPolling } from '../useEvalRunPolling';

describe('hasActiveRuns', () => {
  it('returns false for empty list', () => {
    expect(hasActiveRuns([])).toBe(false);
  });

  it('returns true when any run is pending or running', () => {
    expect(hasActiveRuns([{ status: 'pending' }])).toBe(true);
    expect(hasActiveRuns([{ status: 'running' }])).toBe(true);
    expect(hasActiveRuns([{ status: 'completed' }, { status: 'running' }])).toBe(true);
  });

  it('returns false when all runs are terminal', () => {
    expect(hasActiveRuns([{ status: 'completed' }, { status: 'failed' }])).toBe(false);
  });
});

describe('useEvalRunPolling', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not poll when no active runs', () => {
    const runs = ref([{ status: 'completed' }]);
    const reload = vi.fn();
    useEvalRunPolling(runs, reload, 3000);
    vi.advanceTimersByTime(10000);
    expect(reload).not.toHaveBeenCalled();
  });

  it('polls while a run is pending and stops when it becomes terminal', async () => {
    const runs = ref([{ status: 'pending' }]);
    const reload = vi.fn().mockImplementation(() => {
      // Second poll observes terminal state.
      runs.value = [{ status: 'completed' }];
      return Promise.resolve();
    });
    useEvalRunPolling(runs, reload, 3000);

    await vi.advanceTimersByTimeAsync(3000);
    expect(reload).toHaveBeenCalledTimes(1);

    // Terminal state reached → no more polling.
    await vi.advanceTimersByTimeAsync(9000);
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it('does not overlap reload calls when a poll is still in flight', async () => {
    const runs = ref([{ status: 'running' }]);
    let resolveReload: () => void = () => {};
    const reload = vi.fn().mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveReload = resolve;
        }),
    );
    useEvalRunPolling(runs, reload, 3000);

    await vi.advanceTimersByTimeAsync(3000);
    expect(reload).toHaveBeenCalledTimes(1);

    // Next tick fires while first reload still pending → skipped.
    await vi.advanceTimersByTimeAsync(3000);
    expect(reload).toHaveBeenCalledTimes(1);

    resolveReload();
    await vi.advanceTimersByTimeAsync(3000);
    expect(reload).toHaveBeenCalledTimes(2);
  });
});
