import { onBeforeUnmount, ref, watch, type Ref } from 'vue';
import { listActivities } from '../../session/api';
import type { Activity } from '../activityTypes';
import { useEventFilter } from './useEventFilter';
import type { InspectorEvent } from '../eventFilter';

const MAX_EVENTS = 2000;

export type ChatEventInspectorStreamDeps = {
  ownerKind?: 'agent' | 'team';
  /**
   * Phase 5 Blocker A: register a callback fired when the WS transport
   * reconnects for the inspected session. The inspector uses this to
   * re-fetch historical Activities via ListActivities RPC, replacing the
   * legacy server-side replay (event.Buffer → replayEvents → Envelope).
   * Returns an unsubscribe function.
   */
  onReconnect?: (handler: () => void) => () => void;
};

/**
 * useChatEventInspector provides a unified view of historical Activities and
 * live events for the session event inspector dialog.
 *
 * Phase 5 Blocker A: the legacy WS replay path (event.Buffer → replayEvents →
 * wsDownstream.Envelope) has been removed. Historical data is sourced from
 * Activity records via the ListActivities RPC. On WS reconnect, the inspector
 * re-fetches Activities via onReconnect (provided by the caller) instead of
 * relying on server-side replay.
 *
 * AF: Live events are stored as InspectorEvent (a minimal local type that
 * captures only the fields the inspector UI accesses). The underlying WS
 * stream still delivers Envelope objects, but they are structurally
 * compatible with InspectorEvent and adapted at the subscribe boundary.
 */
export function useChatEventInspector(
  sessionId: Ref<string | null | undefined>,
  active: Ref<boolean>,
  streamDeps?: ChatEventInspectorStreamDeps,
) {
  const events = ref<InspectorEvent[]>([]);
  const activities = ref<Activity[]>([]);
  const paused = ref(false);
  const loading = ref(false);
  const error = ref('');
  const selectedInvocationId = ref<string | null>(null);

  const { filters, filteredEvents, branchTree, resetFilters } = useEventFilter(events);

  let unsubReconnect: (() => void) | null = null;

  function upsertEvent(env: InspectorEvent): void {
    const idx = events.value.findIndex((e) => e.id === env.id);
    if (idx >= 0) {
      const next = [...events.value];
      next[idx] = env;
      events.value = next;
      return;
    }
    events.value = [env, ...events.value].slice(0, MAX_EVENTS);
  }

  async function loadHistory(id: string): Promise<void> {
    loading.value = true;
    error.value = '';
    try {
      const items = await listActivities(id);
      activities.value = items;
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load activities';
    } finally {
      loading.value = false;
    }
  }

  function connectStream(id: string): void {
    disconnectStream();
    // Phase 5 Blocker A: on WS reconnect, re-fetch Activities via ListActivities
    // RPC to backfill any events missed during the disconnection window.
    unsubReconnect =
      streamDeps?.onReconnect?.(() => {
        void loadHistory(id);
      }) ?? null;
  }

  function disconnectStream(): void {
    unsubReconnect?.();
    unsubReconnect = null;
  }

  watch(
    () => [active.value, sessionId.value] as const,
    ([isActive, id]) => {
      if (!isActive || !id) {
        disconnectStream();
        return;
      }
      void loadHistory(id);
      connectStream(id);
    },
    { immediate: true },
  );

  onBeforeUnmount(() => {
    disconnectStream();
  });

  function clearEvents(): void {
    events.value = [];
    activities.value = [];
  }

  return {
    events,
    activities,
    paused,
    loading,
    error,
    filters,
    filteredEvents,
    branchTree,
    selectedInvocationId,
    resetFilters,
    clearEvents,
    reload: () => {
      const id = sessionId.value;
      if (id) void loadHistory(id);
    },
  };
}
