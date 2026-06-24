import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ActivityStartMeta } from '../../activityTypes';

// Mock the listActivities import to avoid real API calls.
// Individual tests can override the mock implementation via vi.mocked(listActivities).
vi.mock('../../../session/api', () => ({
  listActivities: vi.fn().mockResolvedValue([]),
}));

import { listActivities } from '../../../session/api';
import { useActivityTimeline } from '../useActivityTimeline';

function makeStartMeta(
  overrides: Partial<ActivityStartMeta> & Pick<ActivityStartMeta, 'activity_id' | 'kind'>,
): ActivityStartMeta {
  return {
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
    vi.mocked(listActivities).mockReset();
    vi.mocked(listActivities).mockResolvedValue([]);
    tl = useActivityTimeline();
  });

  it('starts with empty activities', () => {
    expect(tl.activities.value).toEqual([]);
    expect(tl.activityTree.value).toEqual([]);
    expect(tl.streamEvents.value).toEqual([]);
  });

  it('handleActivityStart adds an activity', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'act-1',
        kind: 'task',
        agent_key: 'agent-a',
        agent_name: 'Agent A',
      }),
    );

    expect(tl.activities.value).toHaveLength(1);
    expect(tl.activities.value[0].id).toBe('act-1');
    expect(tl.activities.value[0].kind).toBe('task');
    expect(tl.rootActivityId.value).toBe('act-1');
  });

  it('handleActivityStart tracks root activity (no parent)', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'root',
        kind: 'task',
        parent_activity_id: null,
      }),
    );
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'child',
        kind: 'thinking',
        parent_activity_id: 'root',
      }),
    );

    expect(tl.rootActivityId.value).toBe('root');
  });

  it('handleActivityDelta updates reasoning content', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'think-1',
        kind: 'thinking',
      }),
    );

    tl.handleActivityDelta({
      activity_id: 'think-1',
      kind: 'thinking',
      status: 'running',
      delta_field: 'reasoning',
      delta_chunk: 'Hello ',
    });
    tl.handleActivityDelta({
      activity_id: 'think-1',
      kind: 'thinking',
      status: 'running',
      delta_field: 'reasoning',
      delta_chunk: 'World',
    });

    const activity = tl.activities.value.find((a) => a.id === 'think-1');
    expect(activity?.reasoning).toBe('Hello World');
  });

  it('handleActivityDelta updates content field', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'reply-1',
        kind: 'reply',
      }),
    );

    tl.handleActivityDelta({
      activity_id: 'reply-1',
      kind: 'reply',
      status: 'running',
      delta_field: 'content',
      delta_chunk: 'Hi',
    });

    const activity = tl.activities.value.find((a) => a.id === 'reply-1');
    expect(activity?.content).toBe('Hi');
  });

  it('handleActivityDelta accumulates tool_arguments for action activities', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'action-1',
        kind: 'action',
        tool_call_id: 'call-1',
        tool_name: 'read_file',
        tool_arguments: '{"path": "',
      }),
    );

    tl.handleActivityDelta({
      activity_id: 'action-1',
      kind: 'action',
      status: 'tool_running',
      delta_field: 'tool_arguments',
      delta_chunk: 'README.md',
    });
    tl.handleActivityDelta({
      activity_id: 'action-1',
      kind: 'action',
      status: 'tool_running',
      delta_field: 'tool_arguments',
      delta_chunk: '"}',
    });

    const activity = tl.activities.value.find((a) => a.id === 'action-1');
    expect(activity?.toolArguments).toBe('{"path": "README.md"}');
  });

  it('handleActivityDelta ignores unknown activity_id', () => {
    tl.handleActivityDelta({
      activity_id: 'nonexistent',
      kind: 'reply',
      status: 'running',
      delta_field: 'content',
      delta_chunk: 'x',
    });

    expect(tl.activities.value).toHaveLength(0);
  });

  it('handleActivityDone updates status and fields', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'act-1',
        kind: 'thinking',
      }),
    );

    tl.handleActivityDone({
      activity_id: 'act-1',
      kind: 'thinking',
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
      kind: 'thinking',
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
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'act-1',
        kind: 'task',
      }),
    );
    expect(tl.activities.value).toHaveLength(1);

    tl.reset();
    expect(tl.activities.value).toHaveLength(0);
    expect(tl.rootActivityId.value).toBeNull();
  });

  it('loadActivities replaces all activities', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'old',
        kind: 'task',
      }),
    );

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
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'root',
        kind: 'task',
        parent_activity_id: null,
      }),
    );
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'child-1',
        kind: 'thinking',
        parent_activity_id: 'root',
      }),
    );
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'child-2',
        kind: 'reply',
        parent_activity_id: 'root',
      }),
    );

    const tree = tl.activityTree.value;
    expect(tree).toHaveLength(1); // one root
    expect(tree[0].id).toBe('root');
    expect(tree[0].children).toHaveLength(2);
    expect(tree[0].children[0].id).toBe('child-1');
    expect(tree[0].children[1].id).toBe('child-2');
  });

  it('streamEvents maps kinds correctly', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'think-1',
        kind: 'thinking',
      }),
    );
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'act-1',
        kind: 'action',
        tool_name: 'search',
      }),
    );
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'reply-1',
        kind: 'reply',
      }),
    );
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'error-1',
        kind: 'error',
        content: 'something went wrong',
      }),
    );

    const activities = tl.streamEvents.value;
    // task, delegate, sub_task_board are filtered out; thinking, action, reply, error remain
    expect(activities).toHaveLength(4);
    expect(activities[0].kind).toBe('thinking');
    expect(activities[1].kind).toBe('action');
    expect(activities[2].kind).toBe('reply');
    expect(activities[3].kind).toBe('error');
  });

  it('delta creates new object for reactivity (not mutating existing)', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'reply-1',
        kind: 'reply',
      }),
    );

    const before = tl.activities.value.find((a) => a.id === 'reply-1');
    const beforeRef = before;

    tl.handleActivityDelta({
      activity_id: 'reply-1',
      kind: 'reply',
      status: 'running',
      delta_field: 'content',
      delta_chunk: 'updated',
    });

    const after = tl.activities.value.find((a) => a.id === 'reply-1');
    // The object reference should be different (new object created for reactivity)
    expect(after).not.toBe(beforeRef);
    expect(after?.content).toBe('updated');
  });

  it('done creates new object for reactivity (not mutating existing)', () => {
    tl.handleActivityStart(
      makeStartMeta({
        activity_id: 'think-1',
        kind: 'thinking',
      }),
    );

    const before = tl.activities.value.find((a) => a.id === 'think-1');

    tl.handleActivityDone({
      activity_id: 'think-1',
      kind: 'thinking',
      status: 'completed',
      duration_ms: 500,
      collapsed: true,
    });

    const after = tl.activities.value.find((a) => a.id === 'think-1');
    expect(after).not.toBe(before);
    expect(after?.status).toBe('completed');
  });

  // --- AF-GAP-05: loadActivitiesFromAPI retry behavior ---

  it('loadActivitiesFromAPI retries 5 times then sets loadError on final failure', async () => {
    // Use fake timers to skip exponential backoff delays
    vi.useFakeTimers();
    // Mock listActivities to always reject
    vi.mocked(listActivities).mockRejectedValue(new Error('network error'));

    const promise = tl.loadActivitiesFromAPI('sess-1');
    // Advance through all retry delays (500+1000+2000+4000 = 7500ms)
    await vi.advanceTimersByTimeAsync(8000);
    await promise;

    expect(listActivities).toHaveBeenCalledTimes(5);
    expect(tl.loadError.value).toBeTruthy();
    expect(tl.loadError.value).toContain('network error');
    vi.useRealTimers();
  });

  it('loadActivitiesFromAPI succeeds within 5 retries and clears loadError', async () => {
    vi.useFakeTimers();
    // Fail 3 times, then succeed on the 4th attempt
    vi.mocked(listActivities)
      .mockRejectedValueOnce(new Error('transient error'))
      .mockRejectedValueOnce(new Error('transient error'))
      .mockRejectedValueOnce(new Error('transient error'))
      .mockResolvedValueOnce([
        {
          id: 'act-1',
          kind: 'task',
          status: 'completed',
          sessionId: 'sess-1',
          turnId: 'turn-1',
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

    const promise = tl.loadActivitiesFromAPI('sess-1');
    // Advance through retry delays (500+1000+2000 = 3500ms for 3 failures)
    await vi.advanceTimersByTimeAsync(4000);
    await promise;

    expect(listActivities).toHaveBeenCalledTimes(4);
    expect(tl.loadError.value).toBeNull();
    expect(tl.activities.value).toHaveLength(1);
    expect(tl.activities.value[0].id).toBe('act-1');
    vi.useRealTimers();
  });

  it('retryLoad clears loadError before retrying', async () => {
    vi.useFakeTimers();
    // First, set loadError by failing all retries
    vi.mocked(listActivities).mockRejectedValue(new Error('failure'));

    let promise = tl.loadActivitiesFromAPI('sess-1');
    await vi.advanceTimersByTimeAsync(8000);
    await promise;
    expect(tl.loadError.value).toBeTruthy();

    // Now mock success for retryLoad
    vi.mocked(listActivities).mockResolvedValue([]);

    promise = tl.retryLoad('sess-1');
    await promise;
    expect(tl.loadError.value).toBeNull();
    vi.useRealTimers();
  });
});
