import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { effectScope, nextTick, ref, type EffectScope } from 'vue';
import type { SessionView } from '../../../components/chat/types';
import { readCachedMobileSessions } from '../offlineCache';
import { useOfflineSessionList } from '../useOfflineSessionList';

function makeSession(id: string): SessionView {
  return { id, title: `Session ${id}`, context_used_ratio: 0, at: '2026-08-08T01:00:00Z' };
}

let onlineFlag = true;

function setup() {
  const live = ref<readonly SessionView[]>([]);
  const agentId = ref('agent-1');
  const loadError = ref<string | null>(null);
  const scope: EffectScope = effectScope();
  let list!: ReturnType<typeof useOfflineSessionList>;
  scope.run(() => {
    list = useOfflineSessionList({ live, agentId, loadError });
  });
  return { live, agentId, loadError, scope, list };
}

beforeEach(() => {
  localStorage.clear();
  onlineFlag = true;
  Object.defineProperty(window.navigator, 'onLine', {
    configurable: true,
    get: () => onlineFlag,
  });
});

afterEach(() => {
  Object.defineProperty(window.navigator, 'onLine', {
    configurable: true,
    value: true,
  });
});

describe('useOfflineSessionList', () => {
  it('passes the live list through and persists it to the cache', async () => {
    const { live, list, scope } = setup();
    live.value = [makeSession('s1'), makeSession('s2')];
    await nextTick();
    expect(list.displaySessions.value.map((s) => s.id)).toEqual(['s1', 's2']);
    expect(list.showingCache.value).toBe(false);
    expect(readCachedMobileSessions('agent-1').map((s) => s.id)).toEqual(['s1', 's2']);
    scope.stop();
  });

  it('shows the cached list when offline with an empty live list', async () => {
    const { live, list, scope } = setup();
    live.value = [makeSession('s1')];
    await nextTick();
    expect(readCachedMobileSessions('agent-1')).toHaveLength(1);
    // Simulate app restart: new scope, empty live list, offline.
    onlineFlag = false;
    window.dispatchEvent(new Event('offline'));
    const live2 = ref<readonly SessionView[]>([]);
    const scope2: EffectScope = effectScope();
    let list2!: ReturnType<typeof useOfflineSessionList>;
    scope2.run(() => {
      list2 = useOfflineSessionList({ live: live2, agentId: ref('agent-1'), loadError: ref(null) });
    });
    expect(list2.showingCache.value).toBe(true);
    expect(list2.displaySessions.value.map((s) => s.id)).toEqual(['s1']);
    expect(list.showingCache.value).toBe(false); // first scope still has live data
    scope.stop();
    scope2.stop();
  });

  it('shows the cached list when the load failed on a weak network (online but loadError)', async () => {
    const { live, list, loadError, scope } = setup();
    live.value = [makeSession('s1')];
    await nextTick();
    live.value = []; // live list stays empty after the failed reload
    loadError.value = 'network error';
    await nextTick();
    expect(list.showingCache.value).toBe(true);
    expect(list.displaySessions.value.map((s) => s.id)).toEqual(['s1']);
    scope.stop();
  });

  it('shows the empty live list when online and no error, even with cache present', async () => {
    const { live, list, scope } = setup();
    live.value = [makeSession('s1')];
    await nextTick();
    live.value = [];
    await nextTick();
    expect(list.showingCache.value).toBe(false);
    expect(list.displaySessions.value).toEqual([]);
    scope.stop();
  });

  it('switches back to live data after recovery', async () => {
    const { live, list, loadError, scope } = setup();
    live.value = [makeSession('s1')];
    await nextTick();
    live.value = [];
    loadError.value = 'boom';
    await nextTick();
    expect(list.showingCache.value).toBe(true);
    live.value = [makeSession('s1'), makeSession('s3')];
    loadError.value = null;
    await nextTick();
    expect(list.showingCache.value).toBe(false);
    expect(list.displaySessions.value.map((s) => s.id)).toEqual(['s1', 's3']);
    scope.stop();
  });

  it('does not cache when agent id is blank', async () => {
    const live = ref<readonly SessionView[]>([makeSession('s1')]);
    const scope: EffectScope = effectScope();
    let list!: ReturnType<typeof useOfflineSessionList>;
    scope.run(() => {
      list = useOfflineSessionList({ live, agentId: ref('  '), loadError: ref(null) });
    });
    await nextTick();
    expect(list.displaySessions.value).toHaveLength(1);
    expect(localStorage.length).toBe(0);
    scope.stop();
  });
});
