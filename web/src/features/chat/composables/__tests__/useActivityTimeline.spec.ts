import { describe, it, expect, vi, beforeEach } from 'vitest';
import type {
  ActivityStartMeta,
  ActivityDeltaMeta,
  ActivityDoneMeta,
  ActivityChildStartMeta,
} from '../../activityTypes';

// Mock the listActivities import to avoid real API calls
vi.mock('../../../session/api', () => ({
  listActivities: vi.fn().mockResolvedValue([]),
}));

import { useActivityTimeline } from '../useActivityTimeline';

function makeStartMeta(overrides: Partial<ActivityStartMeta> & Pick<ActivityStartMeta, 'activity_id' | 'kind'>): ActivityStartMeta {
  return {
    kind: overrides.kind,
    status: 'running',
    session_id: 'sess-1',
    turn_id: 'turn-1',
    parent_activity_id: null,
    timestamp: '2026-06-13T00:00:00Z',
    duration_ms: null,
    collapsed: false,
    ...overrides,
  };
}

describe('useActivityTimeline', () => {
  let tl: ReturnType<typeof useActivityTimeline>;

  beforeEach(() => {
    tl = useActivityTimeline();
  });

  it('starts with empty activities', () => {
    expect(tl.activities.value).toEqual([]);
    expect(tl.activityTree.value).toEqual([]);
    expect(tl.streamEvents.value).toEqual([]);
  });

  it('handleActivityStart adds an activity', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'act-1',
      kind: 'task',
      agent_key: 'agent-a',
      agent_name: 'Agent A',
    }));

    expect(tl.activities.value).toHaveLength(1);
    expect(tl.activities.value[0].id).toBe('act-1');
    expect(tl.activities.value[0].kind).toBe('task');
    expect(tl.rootActivityId.value).toBe('act-1');
  });

  it('handleActivityStart tracks root activity (no parent)', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'root',
      kind: 'task',
      parent_activity_id: null,
    }));
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'child',
      kind: 'thinking',
      parent_activity_id: 'root',
    }));

    expect(tl.rootActivityId.value).toBe('root');
  });

  it('handleActivityDelta updates reasoning content', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'think-1',
      kind: 'thinking',
    }));

    tl.handleActivityDelta({
      activity_id: 'think-1',
      delta_field: 'reasoning',
      delta_chunk: 'Hello ',
    });
    tl.handleActivityDelta({
      activity_id: 'think-1',
      delta_field: 'reasoning',
      delta_chunk: 'World',
    });

    const activity = tl.activities.value.find((a) => a.id === 'think-1');
    expect(activity?.reasoning).toBe('Hello World');
  });

  it('handleActivityDelta updates content field', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'reply-1',
      kind: 'reply',
    }));

    tl.handleActivityDelta({
      activity_id: 'reply-1',
      delta_field: 'content',
      delta_chunk: 'Hi',
    });

    const activity = tl.activities.value.find((a) => a.id === 'reply-1');
    expect(activity?.content).toBe('Hi');
  });

  it('handleActivityDelta ignores unknown activity_id', () => {
    tl.handleActivityDelta({
      activity_id: 'nonexistent',
      delta_field: 'content',
      delta_chunk: 'x',
    });

    expect(tl.activities.value).toHaveLength(0);
  });

  it('handleActivityDone updates status and fields', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'act-1',
      kind: 'thinking',
    }));

    tl.handleActivityDone({
      activity_id: 'act-1',
      status: 'completed',
      duration_ms: 1500,
      collapsed: true,
      content: 'full content',
      reasoning: 'full reasoning',
    });

    const activity = tl.activities.value.find((a) => a.id === 'act-1');
    expect(activity?.status).toBe('completed');
    expect(activity?.durationMs).toBe(1500);
    expect(activity?.collapsed).toBe(true);
    expect(activity?.content).toBe('full content');
    expect(activity?.reasoning).toBe('full reasoning');
  });

  it('handleActivityDone ignores unknown activity_id', () => {
    tl.handleActivityDone({
      activity_id: 'nonexistent',
      status: 'completed',
      duration_ms: 0,
      collapsed: true,
    });

    expect(tl.activities.value).toHaveLength(0);
  });

  it('handleActivityChildStart adds child activity', () => {
    tl.handleActivityChildStart({
      activity_id: 'child-1',
      kind: 'thinking',
      status: 'running',
      parent_activity_id: 'root',
      child_board_id: null,
      team_id: null,
      spirit_session_id: null,
      dag_node_id: null,
      depends_on: null,
    });

    expect(tl.activities.value).toHaveLength(1);
    expect(tl.activities.value[0].id).toBe('child-1');
  });

  it('reset clears all activities', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'act-1',
      kind: 'task',
    }));
    expect(tl.activities.value).toHaveLength(1);

    tl.reset();
    expect(tl.activities.value).toHaveLength(0);
    expect(tl.rootActivityId.value).toBeNull();
  });

  it('loadActivities replaces all activities', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'old',
      kind: 'task',
    }));

    tl.loadActivities([
      {
        id: 'new-1',
        kind: 'task',
        status: 'completed',
        sessionId: 's1',
        turnId: 't1',
        parentActivityId: null,
        timestamp: '2026-06-13T00:00:00Z',
        durationMs: 100,
        content: null,
        reasoning: null,
        toolName: null,
        toolCallId: null,
        toolArguments: null,
        toolResult: null,
        toolDurationMs: null,
        toolErrorCode: null,
        childBoardId: null,
        spiritSessionId: null,
        teamId: null,
        dagNodeId: null,
        dependsOn: null,
        agentKey: null,
        agentName: null,
        collapsed: false,
        label: null,
      },
    ]);

    expect(tl.activities.value).toHaveLength(1);
    expect(tl.activities.value[0].id).toBe('new-1');
    expect(tl.rootActivityId.value).toBe('new-1');
  });

  it('activityTree builds parent-child relationships', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'root',
      kind: 'task',
      parent_activity_id: null,
    }));
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'child-1',
      kind: 'thinking',
      parent_activity_id: 'root',
    }));
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'child-2',
      kind: 'reply',
      parent_activity_id: 'root',
    }));

    const tree = tl.activityTree.value;
    expect(tree).toHaveLength(1); // one root
    expect(tree[0].id).toBe('root');
    expect(tree[0].children).toHaveLength(2);
    expect(tree[0].children[0].id).toBe('child-1');
    expect(tree[0].children[1].id).toBe('child-2');
  });

  it('streamEvents maps kinds correctly', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'think-1',
      kind: 'thinking',
    }));
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'act-1',
      kind: 'action',
      tool_name: 'search',
    }));
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'reply-1',
      kind: 'reply',
    }));
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'error-1',
      kind: 'error',
      content: 'something went wrong',
    }));

    const activities = tl.streamEvents.value;
    // task, delegate, sub_task_board are filtered out; thinking, action, reply, error remain
    expect(activities).toHaveLength(4);
    expect(activities[0].kind).toBe('thinking');
    expect(activities[1].kind).toBe('action');
    expect(activities[2].kind).toBe('reply');
    expect(activities[3].kind).toBe('error');
  });

  it('delta creates new object for reactivity (not mutating existing)', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'reply-1',
      kind: 'reply',
    }));

    const before = tl.activities.value.find((a) => a.id === 'reply-1');
    const beforeRef = before;

    tl.handleActivityDelta({
      activity_id: 'reply-1',
      delta_field: 'content',
      delta_chunk: 'updated',
    });

    const after = tl.activities.value.find((a) => a.id === 'reply-1');
    // The object reference should be different (new object created for reactivity)
    expect(after).not.toBe(beforeRef);
    expect(after?.content).toBe('updated');
  });

  it('done creates new object for reactivity (not mutating existing)', () => {
    tl.handleActivityStart(makeStartMeta({
      activity_id: 'think-1',
      kind: 'thinking',
    }));

    const before = tl.activities.value.find((a) => a.id === 'think-1');

    tl.handleActivityDone({
      activity_id: 'think-1',
      status: 'completed',
      duration_ms: 500,
      collapsed: true,
    });

    const after = tl.activities.value.find((a) => a.id === 'think-1');
    expect(after).not.toBe(before);
    expect(after?.status).toBe('completed');
  });
});
