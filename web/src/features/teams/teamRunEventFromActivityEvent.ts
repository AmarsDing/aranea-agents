import type { ActivityEvent } from '../../realtime/activityEvent';
import type { TeamRun, TeamRunEvent, TeamRunStep } from './types';

/**
 * Activity stages emitted on the chat channel for Team run observability.
 *
 * Matched against `ev.activity.stage` when `ev.activity.kind === 'team_stage'`.
 * Both the short form (e.g. `'started'`) and the legacy long form
 * (e.g. `'team_run_started'`) are accepted for backward compatibility with
 * backends that still emit the long-form stage label.
 */
export const TEAM_RUNTIME_ACTIVITY_STAGES: readonly string[] = [
  'started',
  'finished',
  'failed',
  'step_started',
  'step_finished',
  'summary',
  // Legacy long-form aliases (kept for backward compatibility):
  'team_run_started',
  'team_run_finished',
  'team_run_failed',
  'team_step_started',
  'team_step_finished',
  'team_summary',
];

function pickRun(meta: Record<string, unknown>): TeamRun | undefined {
  const run = meta.run;
  if (run && typeof run === 'object') {
    return run as TeamRun;
  }
  return undefined;
}

function pickStep(meta: Record<string, unknown>): TeamRunStep | undefined {
  const step = meta.step;
  if (step && typeof step === 'object') {
    return step as TeamRunStep;
  }
  return undefined;
}

/**
 * Normalize a `team_stage` activity stage to its canonical TeamRunEvent type.
 * Accepts both short (`'started'`) and long (`'team_run_started'`) forms.
 * Unknown stages pass through unchanged so callers can surface new stages
 * without a remap.
 */
function stageToEventType(stage: string): string {
  switch (stage) {
    case 'started':
    case 'team_run_started':
      return 'team_run_started';
    case 'finished':
    case 'team_run_finished':
      return 'team_run_finished';
    case 'failed':
    case 'team_run_failed':
      return 'team_run_failed';
    case 'step_started':
    case 'team_step_started':
      return 'team_step_started';
    case 'step_finished':
    case 'team_step_finished':
      return 'team_step_finished';
    case 'summary':
    case 'team_summary':
      return 'team_summary';
    default:
      return stage;
  }
}

/**
 * Quick predicate: does this ActivityEvent carry team-run telemetry?
 *
 * Useful for callers that want to filter on `kind === 'team_stage'` plus a
 * recognized stage before invoking the full mapper.
 */
export function isTeamRuntimeActivityEvent(ev: ActivityEvent): boolean {
  if (ev.activity.kind === 'team_stage') {
    return TEAM_RUNTIME_ACTIVITY_STAGES.includes(ev.activity.stage);
  }
  // notice (intent_pass / transfer) and session (runner_completion) are also
  // team-runtime related; the full mapper decides the canonical event type.
  return (
    (ev.activity.kind === 'notice' && (ev.activity.stage === 'intent_pass' || ev.activity.stage === 'transfer')) ||
    (ev.activity.kind === 'session' && (ev.activity.stage === 'runner_completion' || ev.event === 'completed'))
  );
}

/**
 * Map a WS {@link ActivityEvent} to a {@link TeamRunEvent}; returns `null` for
 * events that are not part of the team-run lifecycle.
 *
 * Team events arrive as `activity_event` payloads on the WS "chat" channel
 * with `activity.kind === 'team_stage'` and a `activity.stage` value drawn
 * from {@link TEAM_RUNTIME_ACTIVITY_STAGES}. The legacy `intent_pass` /
 * `transfer` notices and `runner_completion` session events are also mapped
 * here so existing TeamRunEvent consumers keep working.
 */
export function teamRunEventFromActivityEvent(ev: ActivityEvent, defaultTeamID = ''): TeamRunEvent | null {
  const activity = ev.activity;
  const meta = (activity.meta ?? {}) as Record<string, unknown>;
  const teamId = activity.team_id ?? defaultTeamID;
  const runId = String(meta.run_id ?? '');
  const sessionId = activity.session_id;
  const kind = activity.kind;
  const stage = activity.stage ?? '';

  if (kind === 'team_stage') {
    const eventType = stageToEventType(stage);
    switch (eventType) {
      case 'team_run_started':
      case 'team_run_finished':
      case 'team_run_failed':
        return {
          type: eventType,
          team_id: teamId,
          run_id: runId,
          session_id: sessionId,
          run: pickRun(meta),
          payload: meta,
        };
      case 'team_summary':
        return {
          type: 'team_summary',
          team_id: teamId,
          run_id: runId,
          session_id: sessionId,
          run: pickRun(meta),
          payload: meta,
        };
      case 'team_step_started':
      case 'team_step_finished':
        return {
          type: eventType,
          team_id: teamId,
          run_id: runId,
          session_id: sessionId,
          step: pickStep(meta),
          payload: meta,
        };
      default:
        // Other team_stage stages (assembled / progress / interrupted /
        // cancelled / etc.) — surface the raw stage so UI can react without
        // waiting for an explicit remap.
        return {
          type: eventType,
          team_id: teamId,
          run_id: runId,
          session_id: sessionId,
          payload: meta,
        };
    }
  }

  if (kind === 'notice') {
    if (stage === 'intent_pass') {
      return {
        type: 'intent_pass',
        team_id: teamId,
        run_id: runId,
        session_id: sessionId,
        payload: meta,
      };
    }
    if (stage === 'transfer') {
      const transfer = meta.transfer;
      const transferObj = transfer && typeof transfer === 'object' ? (transfer as Record<string, unknown>) : {};
      return {
        type: 'transfer',
        team_id: teamId,
        run_id: runId,
        session_id: sessionId,
        payload: {
          from_agent: transferObj.from_agent,
          to_agent: transferObj.to_agent,
        },
      };
    }
    return null;
  }

  if (kind === 'session') {
    // runner_completion is signalled either by an explicit stage label or by
    // the terminal `event === 'completed'` on a session-scoped Activity.
    if (stage === 'runner_completion' || ev.event === 'completed') {
      return {
        type: 'run_finished',
        team_id: teamId,
        run_id: runId,
        session_id: sessionId,
        payload: meta,
      };
    }
    return null;
  }

  return null;
}
