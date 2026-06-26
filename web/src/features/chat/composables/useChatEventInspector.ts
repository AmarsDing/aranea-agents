import { onBeforeUnmount, ref, watch, type Ref } from 'vue';
import { listActivities } from '../../session/api';
import type { Activity } from '../activityTypes';
import { useEventFilter } from './useEventFilter';
import type { InspectorEvent } from '../eventFilter';

const LIVE_TYPES: string[] = [
  'text_delta',
  'text_done',
  'tool_call',
  'tool_result',
  'state_delta',
  'transfer',
  'runner_completion',
  'context_usage',
  'run_status',
  'error',
  'graph_node_start',
  'graph_node_end',
  'graph_node_error',
  'graph_node_custom',
  'graph_step',
  'graph_execution_done',
  'checkpoint',
  'intent_pass',
  'member_message_start',
  'member_delta',
  'member_message_done',
  'team_run_started',
  'team_run_finished',
  'team_run_failed',
  'team_step_started',
  'team_step_finished',
  'team_summary',
  'knowledge_ingest',
];

const MAX_EVENTS = 2000;

export type ChatEventInspectorStreamDeps = {
  ownerKind?: 'agent' | 'team';
  subscribe?: (
    sessionId: string,
    ownerKind: 'agent' | 'team',
    types: string[],
    handler: (env: InspectorEvent) => void,
  ) => () => void;
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

  let unsubLive: (() => void) | null = null;
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
    const ownerKind = streamDeps?.ownerKind ?? 'agent';
    if (streamDeps?.subscribe) {
      unsubLive = streamDeps.subscribe(id, ownerKind, LIVE_TYPES, (env) => {
        if (paused.value) return;
        upsertEvent(env);
      });
    }
    // Phase 5 Blocker A: on WS reconnect, re-fetch Activities via ListActivities
    // RPC to backfill any events missed during the disconnection window.
    unsubReconnect =
      streamDeps?.onReconnect?.(() => {
        void loadHistory(id);
      }) ?? null;
  }

  function disconnectStream(): void {
    unsubLive?.();
    unsubLive = null;
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
