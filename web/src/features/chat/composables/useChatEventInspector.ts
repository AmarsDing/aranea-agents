import { onBeforeUnmount, ref, watch, type Ref } from 'vue';
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer } from '../../../realtime/globalWsHub';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import { listActivities } from '../../session/api';
import type { Activity } from '../activityTypes';

const MAX_LIVE_ACTIVITIES = 500;

export type ChatEventInspectorStreamDeps = {
  ownerKind?: 'agent' | 'team';
  /**
   * Phase 5 Blocker A: register a callback fired when the WS transport
   * reconnects for the inspected session. The inspector uses this to
   * re-fetch historical activities via listActivities RPC after a WS
   * disconnection window. Returns an unsubscribe function.
   */
  onReconnect?: (handler: () => void) => () => void;
};

/**
 * useChatEventInspector provides a unified view of historical Activities
 * (loaded via ListActivities RPC) and live Activities (received via WS
 * activity_event messages) for the session event inspector dialog.
 *
 * Data sources:
 *   - History: GET /v1/sessions/{id}/activities (listActivities) — reads
 *     from the activities/steps_v2 table, returns all Activities for the
 *     session sorted by turn_id + parent_activity_id + timestamp.
 *   - Live: WS activity_event messages — the global WS hub dispatches
 *     ActivityEvent payloads (full Activity snapshot) to all consumers.
 *     We filter by session_id and upsert into the liveActivities ref.
 *
 * Both refs use the unified Activity domain model (kind/status/agent_name/
 * tool_name/content/reasoning) so the inspector UI renders business-semantic
 * information instead of transport-layer envelope fields.
 */
export function useChatEventInspector(
  sessionId: Ref<string | null | undefined>,
  active: Ref<boolean>,
  streamDeps?: ChatEventInspectorStreamDeps,
) {
  const activities = ref<Activity[]>([]);
  const liveActivities = ref<Activity[]>([]);
  const paused = ref(false);
  const loading = ref(false);
  const error = ref('');

  let wsConsumerId: string | null = null;
  let unsubReconnect: (() => void) | null = null;

  /** Upserts an Activity into the live stream (dedup by id, cap at MAX_LIVE_ACTIVITIES). */
  function upsertLiveActivity(act: Activity): void {
    if (paused.value) return;
    const idx = liveActivities.value.findIndex((a) => a.id === act.id);
    if (idx >= 0) {
      // Replace existing snapshot (later events carry updated status/content).
      const next = [...liveActivities.value];
      next[idx] = act;
      liveActivities.value = next;
      return;
    }
    liveActivities.value = [act, ...liveActivities.value].slice(0, MAX_LIVE_ACTIVITIES);
  }

  /** Handles an ActivityEvent from the WS hub: filters by session and upserts. */
  function handleActivityEvent(ev: ActivityEvent): void {
    const sid = sessionId.value;
    if (!sid) return;
    if (ev.activity.session_id !== sid) return;
    upsertLiveActivity(ev.activity as unknown as Activity);
  }

  async function loadHistory(id: string): Promise<void> {
    loading.value = true;
    error.value = '';
    try {
      activities.value = await listActivities(id);
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load activities';
      activities.value = [];
    } finally {
      loading.value = false;
    }
  }

  function connectStream(id: string): void {
    disconnectStream();
    // Subscribe to the global WS hub for activity_event messages.
    // The hub dispatches ActivityEvent payloads to all consumers; we filter
    // by session_id inside handleActivityEvent.
    wsConsumerId = acquireGlobalWsConsumer({
      channels: ['chat'],
      logEnabled: false,
      onActivityEvent: handleActivityEvent,
    });
    // On WS reconnect, re-fetch history to backfill any events missed
    // during the disconnection window.
    unsubReconnect =
      streamDeps?.onReconnect?.(() => {
        void loadHistory(id);
      }) ?? null;
  }

  function disconnectStream(): void {
    if (wsConsumerId) {
      releaseGlobalWsConsumer(wsConsumerId);
      wsConsumerId = null;
    }
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
    liveActivities.value = [];
  }

  return {
    /** Historical activities loaded from the backend (ListActivities RPC). */
    activities,
    /** Live activities received via WS activity_event messages. */
    liveActivities,
    paused,
    loading,
    error,
    clearEvents,
    reload: () => {
      const id = sessionId.value;
      if (id) void loadHistory(id);
    },
  };
}
