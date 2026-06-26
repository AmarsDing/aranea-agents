import type { ActivityEvent } from '../../realtime/activityEvent';
import { runStatusFromActivityEvent } from './activityRunStatus';
import { SESSION_RUN_STATUS } from './sessionRunStatus';

// ── ActivityEvent-based helpers ────────────────────────────────────────
// The backend sends ALL chat/system events as ActivityEvent on the WS
// chat channel.

/**
 * Read session revision from an ActivityEvent.
 *
 * Field mapping (envelope → activity):
 *   env.session_revision                → ev.activity.meta.session_revision
 *   env.metadata.session_revision       → ev.activity.meta.session_revision
 *   env.metadata.revision               → ev.activity.meta.revision (fallback)
 */
export function activitySessionRevision(ev: ActivityEvent): number {
  const meta = ev.activity.meta ?? {};
  if (typeof meta.session_revision === 'number' && meta.session_revision > 0) {
    return meta.session_revision;
  }
  if (typeof meta.revision === 'number' && meta.revision > 0) {
    return meta.revision;
  }
  return 0;
}

/**
 * Resolve the inbound source (channel / cron / a2a / durable / ws / web) from
 * an ActivityEvent.
 *
 * Field mapping (envelope → activity):
 *   env.source                          → ev.activity.meta.source
 *   env.metadata.source                 → ev.activity.meta.source
 */
export function activitySource(ev: ActivityEvent): string {
  const meta = ev.activity.meta ?? {};
  const direct = typeof meta.source === 'string' ? meta.source.trim() : '';
  return direct;
}

/** M55 session_revision sync — incremental hydrate only, not turn complete. */
export function isSessionRevisionSyncActivity(ev: ActivityEvent): boolean {
  if (ev.activity.stage !== 'run_status') return false;
  return runStatusFromActivityEvent(ev)?.status === SESSION_RUN_STATUS.SYNC;
}

/** Check skip_hydrate meta flag — backend sets this on WS-originated sync events
 *  to prevent redundant full hydrate that causes duplicate messages. */
export function shouldSkipHydrateActivity(ev: ActivityEvent): boolean {
  return ev.activity.meta?.skip_hydrate === true;
}

/**
 * Detect a turn-complete ActivityEvent.
 *
 * Mapping (envelope → activity):
 *   env.type === 'runner_completion'    → ev.activity.stage === 'runner_completion'
 *                                         OR (ev.event === 'completed' && kind === 'task')
 *   env.type === 'error'                → ev.event === 'failed'
 *   env.type === 'run_status'           → ev.activity.stage === 'run_status' &&
 *     status completed/failed/cancelled (excluding 'sync')
 */
export function isTurnCompleteActivity(ev: ActivityEvent): boolean {
  if (ev.activity.stage === 'runner_completion') return true;
  if (ev.event === 'failed') return true;
  if (ev.activity.stage !== 'run_status') return false;
  const status = runStatusFromActivityEvent(ev)?.status;
  if (status === SESSION_RUN_STATUS.SYNC) return false;
  return (
    status === SESSION_RUN_STATUS.COMPLETED ||
    status === SESSION_RUN_STATUS.FAILED ||
    status === SESSION_RUN_STATUS.CANCELLED
  );
}
