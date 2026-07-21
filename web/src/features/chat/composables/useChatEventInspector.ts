import { onBeforeUnmount, ref, watch, type Ref } from 'vue';
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer } from '../../../realtime/globalWsHub';
import { listStepsV2 } from '../../session/v2Api';
import type { Activity } from '../activityTypes';
import type { Step, StepCreatedPayload, StepStreamingPayload, StepUpdatedPayload, V2WsEnvelope } from '../v2Types';
import { stepToActivitySnapshot } from '../stepToActivitySnapshot';

const MAX_LIVE_ACTIVITIES = 500;

const STEP_KINDS = new Set(['step.created', 'step.updated', 'step.completed', 'step.failed', 'step.streaming']);

export type ChatEventInspectorStreamDeps = {
  ownerKind?: 'agent' | 'team';
  /**
   * Register a callback fired when the WS transport reconnects for the
   * inspected session. Re-fetches steps via listStepsV2 after a disconnect.
   */
  onReconnect?: (handler: () => void) => () => void;
};

/**
 * Unified historical + live Activity view for SessionEventInspector.
 *
 * - History: GET /v2/sessions/{id}/steps → stepToActivitySnapshot
 * - Live: v2 `step.*` WS events mapped via stepToActivitySnapshot
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

  function upsertLiveActivity(act: Activity): void {
    if (paused.value) return;
    const idx = liveActivities.value.findIndex((a) => a.id === act.id);
    if (idx >= 0) {
      const next = [...liveActivities.value];
      next[idx] = act;
      liveActivities.value = next;
      return;
    }
    liveActivities.value = [act, ...liveActivities.value].slice(0, MAX_LIVE_ACTIVITIES);
  }

  function sessionMatches(stepSession: string, spiritSession: string): boolean {
    const sid = sessionId.value?.trim() ?? '';
    if (!sid) return false;
    return stepSession === sid || spiritSession === sid;
  }

  function handleStepSnapshot(step: Step): void {
    if (!sessionMatches(step.SessionID ?? '', step.SpiritSessionID ?? '')) return;
    upsertLiveActivity(stepToActivitySnapshot(step));
  }

  function handleStepStreaming(payload: StepStreamingPayload): void {
    if (paused.value) return;
    const stepId = payload.StepID;
    if (!stepId) return;
    const idx = liveActivities.value.findIndex((a) => a.id === stepId);
    if (idx < 0) return;
    // Streaming deltas only update an already-seen step; no session filter
    // needed beyond id match (created event already scoped the row).
    const prev = liveActivities.value[idx];
    const field = payload.DeltaField;
    const chunk = payload.DeltaChunk ?? '';
    const next: Activity = { ...prev };
    if (field === 'reasoning') {
      next.reasoning = `${prev.reasoning ?? ''}${chunk}`;
    } else {
      next.content = `${prev.content ?? ''}${chunk}`;
    }
    const copy = [...liveActivities.value];
    copy[idx] = next;
    liveActivities.value = copy;
  }

  function handleV2Event(envelope: V2WsEnvelope): void {
    if (!STEP_KINDS.has(envelope.kind)) return;

    switch (envelope.kind) {
      case 'step.created':
      case 'step.updated':
      case 'step.completed':
      case 'step.failed': {
        const step = (envelope.payload as StepCreatedPayload | StepUpdatedPayload).Step;
        if (!step) return;
        handleStepSnapshot(step);
        break;
      }
      case 'step.streaming':
        handleStepStreaming(envelope.payload as StepStreamingPayload);
        break;
      default:
        break;
    }
  }

  async function loadHistory(id: string): Promise<void> {
    loading.value = true;
    error.value = '';
    try {
      const steps = await listStepsV2(id);
      activities.value = steps.map(stepToActivitySnapshot);
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'Failed to load steps';
      activities.value = [];
    } finally {
      loading.value = false;
    }
  }

  function connectStream(id: string): void {
    disconnectStream();
    wsConsumerId = acquireGlobalWsConsumer({
      channels: ['chat'],
      logEnabled: false,
      onV2Event: handleV2Event,
    });
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
    activities,
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
