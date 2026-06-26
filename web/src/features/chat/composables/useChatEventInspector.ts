import { onBeforeUnmount, ref, shallowRef, watch, type Ref } from 'vue';
import { listActivities } from '../../session/api';
import type { Activity } from '../activityTypes';
import type { Envelope, EnvelopeType } from '../envelope';
import { createEnvelopeStream, type UseEnvelopeStreamReturn } from '../useEnvelopeStream';
import { useEventFilter } from './useEventFilter';
import type { InspectorEvent } from '../eventFilter';

const LIVE_TYPES: EnvelopeType[] = [
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
    types: EnvelopeType[],
    handler: (env: Envelope) => void,
  ) => () => void;
};

/**
 * useChatEventInspector provides a unified view of historical Activities and
 * live Envelopes for the session event inspector dialog.
 *
 * Phase 1c-2: history is now sourced from Activity records via the
 * ListActivities RPC. Live events continue to flow through the WS envelope
 * stream. The two collections are exposed separately so the UI can render
 * Activity-derived history (with kind/status/tool fields) distinct from
 * raw real-time envelopes.
 *
 * AF: Live events are stored as InspectorEvent (a minimal local type that
 * captures only the fields the inspector UI accesses). Full Envelope objects
 * from the WS stream are structurally compatible and assigned directly.
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

  const stream = shallowRef<UseEnvelopeStreamReturn | null>(null);
  let unsubLive: (() => void) | null = null;
  let ownsStream = false;

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
      return;
    }
    ownsStream = true;
    const s = createEnvelopeStream({
      sessionId: id,
      channels: ['chat', 'team', 'graph', 'system'],
      autoConnect: true,
    });
    unsubLive = s.onType(LIVE_TYPES, (env) => {
      if (paused.value) return;
      upsertEvent(env);
    });
    stream.value = s;
  }

  function disconnectStream(): void {
    unsubLive?.();
    unsubLive = null;
    if (ownsStream) {
      stream.value?.disconnect();
      stream.value = null;
      ownsStream = false;
    }
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
