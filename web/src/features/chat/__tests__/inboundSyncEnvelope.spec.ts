import { describe, expect, it } from 'vitest';
import type { ActivityEvent, Activity } from '../../../realtime/activityEvent';
import {
  activitySessionRevision,
  activitySource,
  isSessionRevisionSyncActivity,
  isTurnCompleteActivity,
} from '../inboundSyncEnvelope';
import { SESSION_RUN_STATUS } from '../sessionRunStatus';

function makeActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    kind: 'task',
    status: 'running',
    session_id: 'sess-1',
    turn_id: 'turn-1',
    parent_activity_id: '',
    timestamp: '',
    duration_ms: 0,
    seq: 1,
    prompt_tokens: 0,
    completion_tokens: 0,
    content: '',
    reasoning: '',
    tool_name: '',
    tool_category: 'other',
    tool_call_id: '',
    tool_arguments: '',
    tool_result: '',
    tool_duration_ms: 0,
    tool_error_code: '',
    stage: 'run_status',
    child_board_id: '',
    spirit_session_id: '',
    team_id: '',
    dag_node_id: '',
    depends_on: [],
    agent_key: '',
    agent_name: '',
    collapsed: false,
    label: '',
    meta: {},
    ...overrides,
  };
}

function activityEvent(meta: Record<string, unknown>, overrides: Partial<Activity> = {}): ActivityEvent {
  return {
    event: 'updated',
    activity: makeActivity({ meta, ...overrides }),
  };
}

describe('inboundSyncEnvelope (DECO-01 / M55-SYNC) — ActivityEvent', () => {
  it('reads session_revision from activity meta', () => {
    expect(activitySessionRevision(activityEvent({ session_revision: 3 }))).toBe(3);
    expect(activitySessionRevision(activityEvent({ revision: 5 }))).toBe(5);
    expect(activitySessionRevision(activityEvent({}))).toBe(0);
  });

  it('reads channel source for cross-plane routing', () => {
    expect(activitySource(activityEvent({ source: 'channel' }))).toBe('channel');
    expect(activitySource(activityEvent({}))).toBe('');
  });

  it('treats sync run_status as hydrate-only, not turn complete', () => {
    const syncEv = activityEvent(
      { source: 'channel', status: SESSION_RUN_STATUS.SYNC, run_id: 'run-1', session_revision: 2 },
      { stage: 'run_status' },
    );
    expect(isSessionRevisionSyncActivity(syncEv)).toBe(true);
    expect(isTurnCompleteActivity(syncEv)).toBe(false);
  });

  it('treats completed channel turn as turn complete', () => {
    const done = activityEvent(
      { source: 'channel', status: SESSION_RUN_STATUS.COMPLETED, run_id: 'run-1', session_revision: 3 },
      { stage: 'run_status' },
    );
    expect(isSessionRevisionSyncActivity(done)).toBe(false);
    expect(isTurnCompleteActivity(done)).toBe(true);
  });

  it('runner_completion always completes turn', () => {
    expect(isTurnCompleteActivity(activityEvent({}, { stage: 'runner_completion' }))).toBe(true);
  });

  it('treats failed events as turn complete', () => {
    const ev: ActivityEvent = {
      event: 'failed',
      activity: makeActivity({ stage: 'run_status' }),
    };
    expect(isTurnCompleteActivity(ev)).toBe(true);
  });
});
