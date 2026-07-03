import type { ActivityEvent as AFActivityEvent } from '../../../realtime/activityEvent';

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
      // Team/graph/plan/session orchestration events are published with Domain=chat
      // so they render as Activity timeline cards, but they also drive the spirit
      // store (team status, progress, plan state). Route them through the inbound
      // pipeline AND keep them on the timeline.
      if (isOrchestrationActivityEvent(ev)) return true;
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
   * Team/graph/plan/session orchestration events need both inbound-sync state
   * updates (spirit store) and Activity timeline rendering.
   */
  function isOrchestrationActivityEvent(ev: AFActivityEvent): boolean {
    const { kind, stage } = ev.activity;
    if (kind === 'team_stage') return true;
    if (kind === 'graph_stage') return true;
    if (kind === 'plan') return true;
    if (kind === 'session') {
      return (
        stage === 'plan_created' ||
        stage === 'allocation_created' ||
        stage === 'orchestration_started' ||
        stage === 'orchestration_checkpoint' ||
        stage === 'orchestration_interrupted' ||
        stage === 'orchestration_completed' ||
        stage === 'orchestration_failed' ||
        stage === 'synthesis_completed'
      );
    }
    return false;
  }

  /**
   * Process a system/control ActivityEvent:
   *   - run_status → apply run-status updates (follow-up queue, runStatus ref,
   *     tool-message cancellation on terminal statuses).
   *   - all other system events → route to the inbound-sync pipeline
   *     (useChatInboundSync.handleInboundActivityEvent) when bound.
   *
   * Rendering of system events (notices, team/graph/plan cards) is now
   * handled by the v2 event pipeline (eventRouter → activityStore); this
   * handler only drives state side-effects (run status, inbound sync).
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
    }
    // No fallback: the inbound handler is bound by useChatWorkspace before
    // any ActivityEvent is dispatched. v1 timeline rendering is removed;
    // notice/orchestration events are rendered via the v2 pipeline.
    return true;
  }

  return {
    isSystemEvent,
    handleSystemEvent,
  };
}
