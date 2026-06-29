import type { ActivityEvent as AFActivityEvent } from '../../../realtime/activityEvent';
import type { useActivityTimeline } from './useActivityTimeline';

export interface UseSystemEventNotificationDeps {
  /**
   * Apply run-status updates from an ActivityEvent (stage=run_status).
   * Updates the follow-up queue, runStatus ref, and cancels tool messages
   * on terminal statuses.
   */
  applyRunStatusFromActivityEvent: (ev: AFActivityEvent) => void;
  /**
   * Lazy accessor for the inbound sync handler (useChatInboundSync.
   * handleInboundActivityEvent). The handler is bound after the inbound
   * sync composable is initialized in useChatWorkspace, so callers must
   * fetch it lazily at event-dispatch time.
   */
  getInboundActivityEventHandler: () => ((ev: AFActivityEvent) => void | Promise<void>) | null;
  /** The Activity timeline, used as a fallback when no inbound handler is bound yet. */
  activityTimeline: ReturnType<typeof useActivityTimeline>;
}

/**
 * useSystemEventNotification — Phase 3 / ADR-03 D6.
 *
 * Handles `Domain=system` ActivityEvents (run_status, error, team_*,
 * graph_*, spirit_*, session.status_changed, metrics_updated, …) by routing
 * them to the inbound-sync pipeline. Chat-rendering events (kind ∈ {task
 * streaming/created/completed, thinking, action, reply, confirm}) are NOT
 * processed here — they belong to the Activity timeline.
 *
 * Event classification uses `ev.domain` when present (preferred, ADR-03 D1),
 * and falls back to `kind + stage` heuristics for older backends that do
 * not set the `domain` field.
 */
export function useSystemEventNotification(deps: UseSystemEventNotificationDeps) {
  /**
   * Determine whether an ActivityEvent is a system/control event (vs. a
   * chat-rendering event that drives the Activity timeline).
   *
   * Preferred signal: `ev.domain === 'system'` (ADR-03 D1).
   * Fallback for older backends (no `domain` field): inspect kind/event.
   *
   * task.failed falls through to the system path: it carries an error
   * payload that handleInboundActivityEvent processes (mirrors the prior
   * logic in useChatWorkspace.handleActivityEvent).
   */
  function isSystemEvent(ev: AFActivityEvent): boolean {
    if (ev.domain === 'system') return true;
    if (ev.domain === 'chat') {
      // Notice events are system notifications (run_status, session_status_changed,
      // pre_planning_gate, user_feedback, …) even when Domain=chat. They need to be
      // routed through the inbound-sync pipeline for state updates (e.g. session
      // status mutation) and then rendered as NoticeBlock in the Activity timeline.
      if (ev.activity.kind === 'notice') return true;
      return false;
    }
    // Legacy fallback: kind + event heuristic.
    const { kind } = ev.activity;
    const isChatRendering =
      (kind === 'task' && (ev.event === 'streaming' || ev.event === 'created' || ev.event === 'completed')) ||
      kind === 'thinking' ||
      kind === 'action' ||
      kind === 'reply' ||
      kind === 'confirm';
    return !isChatRendering;
  }

  /**
   * Process a system/control ActivityEvent:
   *   - run_status → apply run-status updates (follow-up queue, runStatus ref,
   *     tool-message cancellation on terminal statuses).
   *   - all other system events → route to the inbound-sync pipeline
   *     (useChatInboundSync.handleInboundActivityEvent). Falls back to the
   *     Activity timeline if the inbound handler is not bound yet.
   *
   * Returns true if the event was handled as a system event; false if it
   * should be treated as a chat-rendering event by the caller.
   */
  function handleSystemEvent(ev: AFActivityEvent): boolean {
    if (!isSystemEvent(ev)) return false;

    const { stage } = ev.activity;
    if (stage === 'run_status') {
      deps.applyRunStatusFromActivityEvent(ev);
    }

    const inboundHandler = deps.getInboundActivityEventHandler();
    if (inboundHandler) {
      void inboundHandler(ev);
    } else {
      // Fallback: no inbound handler bound yet — pass to the timeline so
      // nothing is dropped before useChatInboundSync wires up.
      deps.activityTimeline.handleActivityEvent(ev);
    }

    // Notice events (run_status, session_status_changed, pre_planning_gate, …)
    // are system notifications that should ALSO render as NoticeBlock in the
    // Activity timeline when they carry a user-facing message. Passing them here
    // ensures chat-domain notices are visible while still receiving inbound-sync
    // state updates above. Empty notices (e.g. background_job_refresh) stay off
    // the timeline and are handled purely by inbound-sync / store side effects.
    if (ev.activity.kind === 'notice' && (ev.activity.content || '').trim()) {
      deps.activityTimeline.handleActivityEvent(ev);
    }
    return true;
  }

  return {
    isSystemEvent,
    handleSystemEvent,
  };
}
