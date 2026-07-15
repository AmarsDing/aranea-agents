import { describe, expect, it } from 'vitest';
import type { ActivityEvent, Activity } from '../../../realtime/activityEvent';
import { conversationSourceFromActivity, projectConversationActivityEvent } from '../conversationEventDispatcher';

function makeActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    kind: 'task',
    status: 'running',
    session_id: 'sess-1',
    turn_id: '',
    parent_activity_id: '',
    timestamp: '2026-05-27T00:00:00Z',
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

describe('conversationEventDispatcher — ActivityEvent', () => {
  it('projects current-session channel completion into a hydrating turn event', () => {
    const projected = projectConversationActivityEvent(
      activityEvent(
        {
          source: 'channel',
          status: 'completed',
          delivery_status: 'sent',
          channel_id: 'ch-1',
          platform: 'slack',
          peer_id: 'peer-1',
          session_revision: 7,
        },
        { turn_id: 'turn-1', session_id: 'sess-1', stage: 'run_status' },
      ),
      { currentSessionId: 'sess-1' },
    );

    expect(projected).toMatchObject({
      scope: 'current-session',
      sessionId: 'sess-1',
      turnId: 'turn-1',
      source: 'channel',
      revision: 7,
      status: 'completed',
      hydrate: true,
      stream: false,
      delivery: {
        kind: 'channel',
        channelId: 'ch-1',
        platform: 'slack',
        recipientId: 'peer-1',
        status: 'delivered',
      },
    });
  });

  it('routes non-current session events to inbox', () => {
    const projected = projectConversationActivityEvent(activityEvent({}, { session_id: 'sess-2' }), {
      currentSessionId: 'sess-1',
    });
    expect(projected?.scope).toBe('inbox');
  });

  it('normalizes background source aliases', () => {
    expect(conversationSourceFromActivity(activityEvent({ source: 'job' }))).toBe('durable');
    expect(conversationSourceFromActivity(activityEvent({ source: 'background' }))).toBe('durable');
  });

  it('projects failed events as failed hydrating turn events', () => {
    const ev: ActivityEvent = {
      event: 'failed',
      activity: makeActivity({
        session_id: 'sess-1',
        meta: { request_id: 'pending-user-1' },
      }),
    };
    const projected = projectConversationActivityEvent(ev, { currentSessionId: 'sess-1' });

    expect(projected).toMatchObject({
      scope: 'current-session',
      turnId: 'pending-user-1',
      status: 'failed',
      hydrate: true,
    });
  });

  // 2026-07-04 修复：子 session（team/member agent session）的事件不应该
  // 进入 inbox 列表。子 session 的展示由 activityV2Store + team panel 负责。
  // 判断依据：activity.spirit_session_id 非空且 ≠ session_id 说明这是子 session
  // 事件（spirit session 自身的 spirit_session_id 为空或等于自身 ID）。
  it('filters out child session events (spirit_session_id !== session_id)', () => {
    // member agent session 事件：session_id=member-sess, spirit_session_id=spirit-sess
    const memberEv = activityEvent({}, { session_id: 'member-sess', spirit_session_id: 'spirit-sess' });
    expect(projectConversationActivityEvent(memberEv, { currentSessionId: 'spirit-sess' })).toBeNull();

    // team session 事件：session_id=team-sess, spirit_session_id=spirit-sess
    const teamEv = activityEvent({}, { session_id: 'team-sess', spirit_session_id: 'spirit-sess' });
    expect(projectConversationActivityEvent(teamEv, { currentSessionId: 'spirit-sess' })).toBeNull();

    // spirit session 自身事件：spirit_session_id 空或 === session_id → 正常 projection
    const spiritEv1 = activityEvent({}, { session_id: 'spirit-sess', spirit_session_id: '' });
    expect(projectConversationActivityEvent(spiritEv1, { currentSessionId: 'spirit-sess' })?.scope).toBe(
      'current-session',
    );

    const spiritEv2 = activityEvent({}, { session_id: 'spirit-sess', spirit_session_id: 'spirit-sess' });
    expect(projectConversationActivityEvent(spiritEv2, { currentSessionId: 'spirit-sess' })?.scope).toBe(
      'current-session',
    );

    // channel inbound 事件：spirit_session_id 空且 session_id ≠ currentSessionId → 正常进 inbox
    const channelEv = activityEvent({}, { session_id: 'ch-sess', spirit_session_id: '' });
    expect(projectConversationActivityEvent(channelEv, { currentSessionId: 'spirit-sess' })?.scope).toBe('inbox');
  });
});
