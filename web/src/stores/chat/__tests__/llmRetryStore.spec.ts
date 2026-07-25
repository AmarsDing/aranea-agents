import { describe, expect, it, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useLlmRetryStore } from '../llmRetryStore';

describe('useLlmRetryStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('returns null when no retry was recorded for the session', () => {
    const store = useLlmRetryStore();
    expect(store.retryFor('s1')).toBeNull();
  });

  it('noteRetry stores attempt/delay/error per session', () => {
    const store = useLlmRetryStore();
    store.noteRetry('s1', { attempt: 2, max_retries: -1, delay_ms: 2000, error: 'connection reset' });
    const state = store.retryFor('s1');
    expect(state).not.toBeNull();
    expect(state?.attempt).toBe(2);
    expect(state?.maxRetries).toBe(-1);
    expect(state?.delayMs).toBe(2000);
    expect(state?.error).toBe('connection reset');
  });

  it('noteRetry latest event wins for the same session', () => {
    const store = useLlmRetryStore();
    store.noteRetry('s1', { attempt: 1, max_retries: -1, delay_ms: 1000, error: 'e1' });
    store.noteRetry('s1', { attempt: 3, max_retries: -1, delay_ms: 4000, error: 'e3' });
    const state = store.retryFor('s1');
    expect(state?.attempt).toBe(3);
    expect(state?.delayMs).toBe(4000);
    expect(state?.error).toBe('e3');
  });

  it('noteRetry tracks sessions independently', () => {
    const store = useLlmRetryStore();
    store.noteRetry('s1', { attempt: 1, max_retries: -1, delay_ms: 1000, error: 'e1' });
    store.noteRetry('s2', { attempt: 5, max_retries: 10, delay_ms: 8000, error: 'e2' });
    expect(store.retryFor('s1')?.attempt).toBe(1);
    expect(store.retryFor('s2')?.attempt).toBe(5);
    expect(store.retryFor('s2')?.maxRetries).toBe(10);
  });

  it('noteRetry ignores empty session id', () => {
    const store = useLlmRetryStore();
    store.noteRetry('', { attempt: 1, max_retries: -1, delay_ms: 1000, error: 'e' });
    store.noteRetry('   ', { attempt: 1, max_retries: -1, delay_ms: 1000, error: 'e' });
    expect(store.retryFor('')).toBeNull();
  });

  it('noteRetry tolerates missing meta fields with sane defaults', () => {
    const store = useLlmRetryStore();
    store.noteRetry('s1', {});
    const state = store.retryFor('s1');
    expect(state?.attempt).toBe(1);
    expect(state?.maxRetries).toBe(-1);
    expect(state?.delayMs).toBe(0);
    expect(state?.error).toBe('');
  });

  it('clear removes only the target session state', () => {
    const store = useLlmRetryStore();
    store.noteRetry('s1', { attempt: 1, max_retries: -1, delay_ms: 1000, error: 'e1' });
    store.noteRetry('s2', { attempt: 2, max_retries: -1, delay_ms: 2000, error: 'e2' });
    store.clear('s1');
    expect(store.retryFor('s1')).toBeNull();
    expect(store.retryFor('s2')).not.toBeNull();
  });

  it('clearAll removes every session state', () => {
    const store = useLlmRetryStore();
    store.noteRetry('s1', { attempt: 1, max_retries: -1, delay_ms: 1000, error: 'e1' });
    store.noteRetry('s2', { attempt: 2, max_retries: -1, delay_ms: 2000, error: 'e2' });
    store.clearAll();
    expect(store.retryFor('s1')).toBeNull();
    expect(store.retryFor('s2')).toBeNull();
  });
});
