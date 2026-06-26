import { describe, expect, it } from 'vitest';
import type { ActivityEvent, Activity } from '../../../realtime/activityEvent';
import { shouldChannelInboundCompleteToastActivity } from '../channelInboundSession';
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

describe('shouldChannelInboundCompleteToastActivity', () => {
  it('toasts on runner_completion', () => {
    expect(shouldChannelInboundCompleteToastActivity(activityEvent({}, { stage: 'runner_completion' }))).toBe(true);
  });

  it('toasts on channel revision completed (M55 primary path)', () => {
    expect(
      shouldChannelInboundCompleteToastActivity(
        activityEvent({ source: 'channel', status: SESSION_RUN_STATUS.COMPLETED }),
      ),
    ).toBe(true);
  });

  it('does not toast on generic run_status completed without channel source', () => {
    expect(
      shouldChannelInboundCompleteToastActivity(activityEvent({ status: SESSION_RUN_STATUS.COMPLETED })),
    ).toBe(false);
  });

  it('toasts on failed/cancelled channel turns', () => {
    expect(
      shouldChannelInboundCompleteToastActivity(activityEvent({ status: SESSION_RUN_STATUS.FAILED })),
    ).toBe(true);
  });
});
