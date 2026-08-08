import { computed, ref, watch, type ComputedRef, type Ref } from 'vue';
import type { SessionView } from '../../components/chat/types';
import { readCachedMobileSessions, writeCachedMobileSessions } from './offlineCache';
import { useNetworkStatus } from './useNetworkStatus';

/**
 * P3.2c: session list with offline fallback for the mobile sessions page.
 *
 * The chat session store only assigns its list after a successful load, so
 * any non-empty live list is known-good and gets persisted to the offline
 * cache. When the live list is empty *and* connectivity is down (offline
 * event, or a failed load on a weak network surfaced via `loadError`), the
 * last cached list is shown instead and flagged via `showingCache` so the UI
 * can label it as stale.
 */
export function useOfflineSessionList(args: {
  live: Ref<readonly SessionView[]>;
  agentId: Ref<string>;
  loadError: Ref<string | null>;
}): {
  online: Ref<boolean>;
  displaySessions: ComputedRef<readonly SessionView[]>;
  showingCache: ComputedRef<boolean>;
} {
  const { online } = useNetworkStatus();
  const cached = ref<readonly SessionView[]>([]);
  let cachedFor = '';

  watch(
    args.agentId,
    (id) => {
      const key = id.trim();
      cachedFor = key;
      cached.value = key ? readCachedMobileSessions(key) : [];
    },
    { immediate: true },
  );

  // Persist every non-empty live list — the store only mutates it on
  // successful loads, so this is always fresh server truth.
  watch(args.live, (list) => {
    const key = args.agentId.value.trim();
    if (!key || list.length === 0) return;
    writeCachedMobileSessions(key, list);
    if (cachedFor === key) cached.value = list;
  });

  const showingCache = computed(
    () => args.live.value.length === 0 && cached.value.length > 0 && (!online.value || !!args.loadError.value),
  );

  const displaySessions = computed<readonly SessionView[]>(() => (showingCache.value ? cached.value : args.live.value));

  return { online, displaySessions, showingCache };
}
