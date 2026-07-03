import { onBeforeUnmount, ref, watch, type Ref } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Activity } from '../activityTypes';
import { useEventFilter } from './useEventFilter';
import type { InspectorEvent } from '../eventFilter';

const MAX_EVENTS = 2000;

export type ChatEventInspectorStreamDeps = {
  ownerKind?: 'agent' | 'team';
  /**
   * Phase 5 Blocker A: register a callback fired when the WS transport
   * reconnects for the inspected session. The inspector uses this to
   * re-fetch historical entities via the v2 store (fetchSessionHistory),
   * replacing the legacy server-side replay (event.Buffer → replayEvents →
   * Envelope). Returns an unsubscribe function.
   */
  onReconnect?: (handler: () => void) => () => void;
};

/**
 * useChatEventInspector provides a unified view of historical Activities and
 * live events for the session event inspector dialog.
 *
 * Phase 3b-D: the historical data source has been migrated from the v1
 * ListActivities RPC to the v2 entity read API (Tasks/Turns/Steps). The v2
 * entities are loaded into the shared `useChatActivityStore` via
 * `fetchSessionHistory`. The legacy `activities` ref is retained as an empty
 * array for backward compatibility with the existing inspector panel UI;
 * it will be removed together with the v1 Activity types in Task 15.
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
      // v2 history fetch populates the shared activity store (tasks/turns/steps
      // Maps). The legacy `activities` ref stays empty until Task 15 replaces
      // the inspector panel with a v2-backed view.
      const store = useChatActivityStore();
      await store.fetchSessionHistory(id);
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load activities';
    } finally {
      loading.value = false;
    }
  }

  function connectStream(id: string): void {
    disconnectStream();
    // Phase 5 Blocker A: on WS reconnect, re-fetch v2 history via the shared
    // activity store to backfill any events missed during the disconnection
    // window.
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
