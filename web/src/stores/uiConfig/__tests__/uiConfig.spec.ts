import { describe, expect, it, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useUiConfigStore } from '../index';

describe('useUiConfigStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    localStorage.clear();
  });

  it('defaults showToolCalls to true when localStorage is empty', () => {
    const store = useUiConfigStore();
    expect(store.showToolCalls).toBe(true);
  });

  it('reads showToolCalls from localStorage when set to false', () => {
    localStorage.setItem('chat.ui.showToolCalls', 'false');
    const store = useUiConfigStore();
    expect(store.showToolCalls).toBe(false);
  });

  it('reads showToolCalls from localStorage when set to true', () => {
    localStorage.setItem('chat.ui.showToolCalls', 'true');
    const store = useUiConfigStore();
    expect(store.showToolCalls).toBe(true);
  });

  it('setShowToolCalls updates state and localStorage', () => {
    const store = useUiConfigStore();
    expect(store.showToolCalls).toBe(true);

    store.setShowToolCalls(false);
    expect(store.showToolCalls).toBe(false);
    expect(localStorage.getItem('chat.ui.showToolCalls')).toBe('false');

    store.setShowToolCalls(true);
    expect(store.showToolCalls).toBe(true);
    expect(localStorage.getItem('chat.ui.showToolCalls')).toBe('true');
  });

  it('persists across store recreation', () => {
    const store1 = useUiConfigStore();
    store1.setShowToolCalls(false);

    // Create new pinia instance (simulates page reload)
    setActivePinia(createPinia());
    const store2 = useUiConfigStore();
    expect(store2.showToolCalls).toBe(false);
  });
});
