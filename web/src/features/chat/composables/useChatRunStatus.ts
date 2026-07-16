import { ref, type Ref } from 'vue';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import type { RunStatus, RunStatusValue } from '../types';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import type { RunStatusEventPayload } from '../v2Types';
import {
  runStatusFromActivityEvent,
  messageQueuedFromActivityEvent,
  runStatusFromV2Payload,
  messageQueuedFromV2Payload,
  type RunStatusFromWs,
} from '../activityRunStatus';

const HYDRATE_DELAY_MS = 400;

type UseChatRunStatusDeps = {
  applyAwaitRunStatus: (rs: RunStatus) => void;
};

/**
 * WS-primary run status: envelopes update state immediately; HTTP hydrates only
 * when WS has not yet reported for the current session (e.g. after session switch).
 */
export function useChatRunStatus(deps: UseChatRunStatusDeps) {
  const runStatus: Ref<RunStatusValue> = ref('idle');
  const runMeta: Ref<RunStatus | null> = ref(null);

  let wsAuthoritative = false;
  let hydrateTimer: ReturnType<typeof setTimeout> | null = null;
  let hydrateSessionId = '';

  function clearHydrateTimer() {
    if (hydrateTimer) {
      clearTimeout(hydrateTimer);
      hydrateTimer = null;
    }
  }

  function resetRunStatus() {
    clearHydrateTimer();
    wsAuthoritative = false;
    hydrateSessionId = '';
    runStatus.value = 'idle';
    runMeta.value = null;
  }

  function applyRunStatusFields(rs: RunStatusFromWs) {
    wsAuthoritative = true;
    clearHydrateTimer();
    runStatus.value = rs.status;
    const meta: RunStatus = {
      status: rs.status,
      runId: rs.runId,
      errorMessage: rs.errorMessage,
      awaitKind: rs.awaitKind,
      awaitToolKey: rs.awaitToolKey,
      awaitToolCallId: rs.awaitToolCallId,
      updatedAt: '',
    };
    runMeta.value = meta;
    deps.applyAwaitRunStatus(meta);
  }

  /** Apply run status from a v2 system.run_status payload. */
  function applyFromV2RunStatus(payload: RunStatusEventPayload) {
    if (messageQueuedFromV2Payload(payload)) return;
    const rs = runStatusFromV2Payload(payload);
    if (!rs) return;
    applyRunStatusFields(rs);
  }

  /** Legacy: apply run status from an ActivityEvent (stage=run_status). */
  function applyFromActivityEvent(ev: ActivityEvent) {
    if (messageQueuedFromActivityEvent(ev)) return;
    const rs = runStatusFromActivityEvent(ev);
    if (!rs) return;
    applyRunStatusFields(rs);
  }

  function scheduleHttpHydrate(sessionId: string) {
    hydrateSessionId = sessionId;
    wsAuthoritative = false;
    clearHydrateTimer();
    hydrateTimer = setTimeout(() => {
      void hydrateFromHttpIfNeeded(sessionId);
    }, HYDRATE_DELAY_MS);
  }

  async function hydrateFromHttpIfNeeded(sessionId: string) {
    if (!sessionId || sessionId !== hydrateSessionId || wsAuthoritative) return;
    try {
      const runtime = useChatRuntimeStore();
      const rs = await runtime.fetchRunStatus(sessionId);
      if (sessionId !== hydrateSessionId || wsAuthoritative) return;
      runStatus.value = rs.status;
      runMeta.value = rs;
      deps.applyAwaitRunStatus(rs);
    } catch {
      /* ignore transient hydrate failures */
    }
  }

  /** Fallback HTTP refresh; skipped when WS already authoritative for this session. */
  async function refreshRunStatus(sessionId?: string) {
    if (!sessionId) {
      resetRunStatus();
      return;
    }
    await hydrateFromHttpIfNeeded(sessionId);
  }

  function onSessionSwitch(sessionId: string | undefined) {
    if (!sessionId) {
      resetRunStatus();
      return;
    }
    // Don't reset to "idle" immediately — keep previous value until HTTP hydrate
    // completes to avoid a brief incorrect "idle" flash during session switch.
    // The hydrate will set the correct status within HYDRATE_DELAY_MS.
    scheduleHttpHydrate(sessionId);
  }

  /** Force-set run status (used when backend rejects enqueue due to stale state). */
  function forceSetRunStatus(status: RunStatusValue) {
    wsAuthoritative = true;
    clearHydrateTimer();
    runStatus.value = status;
    if (status === 'idle') {
      runMeta.value = null;
    }
  }

  return {
    runStatus,
    runMeta,
    applyFromV2RunStatus,
    applyFromActivityEvent,
    onSessionSwitch,
    refreshRunStatus,
    forceSetRunStatus,
  };
}
