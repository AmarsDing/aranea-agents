import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Activity as AFActivity } from '../../../realtime/activityEvent';

// Mock the listActivities import to avoid real API calls.
// Individual tests can override the mock implementation via vi.mocked(listActivities).
vi.mock('../../../session/api', () => ({
  listActivities: vi.fn().mockResolvedValue([]),
}));

import { listActivities } from '../../../session/api';
import { useActivityTimeline } from '../useActivityTimeline';

/** Builds an AF (Activity-First) Activity snapshot for handleActivityEvent tests. */
function makeAFActivity(
  overrides: Partial<AFActivity> & Pick<AFActivity, 'id' | 'kind'>,
): AFActivity {
  return {
    status: 'running',
    session_id: 'sess-1',
    turn_id: 'turn-1',
    parent_activity_id: '',
    timestamp: '2026-06-13T00:00:00Z',
    duration_ms: 0,
    seq: 0,
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
    stage: '',
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

describe('useActivityTimeline', () => {
  let tl: ReturnType<typeof useActivityTimeline>;

  beforeEach(() => {
    vi.mocked(listActivities).mockReset();
    vi.mocked(listActivities).mockResolvedValue([]);
    tl = useActivityTimeline();
    // Phase 3: activities are isolated per session. Set the current session
    // so the public computed properties (activities / activityTree / etc.)
    // reflect events for the default session used by tests.
    tl.setCurrentSession('sess-1');
  });

  it('starts with empty activities', () => {
    expect(tl.activities.value).toEqual([]);
    expect(tl.activityTree.value).toEqual([]);
    expect(tl.sortedActivities.value).toEqual([]);
  });

  it('handleActivityEvent (created) adds an activity', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({
        id: 'act-1',
        kind: 'task',
        agent_key: 'agent-a',
        agent_name: 'Agent A',
      }),
    });

    expect(tl.activities.value).toHaveLength(1);
    expect(tl.activities.value[0].id).toBe('act-1');
    expect(tl.activities.value[0].kind).toBe('task');
    expect(tl.rootActivityId.value).toBe('act-1');
  });

  it('handleActivityEvent (created) tracks root activity (no parent)', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'root', kind: 'task', parent_activity_id: '' }),
    });
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'child', kind: 'thinking', parent_activity_id: 'root' }),
    });

    expect(tl.rootActivityId.value).toBe('root');
  });

  it('handleActivityEvent (streaming) updates reasoning content', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'think-1', kind: 'thinking' }),
    });

    tl.handleActivityEvent({
      event: 'streaming',
      activity: makeAFActivity({ id: 'think-1', kind: 'thinking' }),
      delta_field: 'reasoning',
      delta_chunk: 'Hello ',
    });
    tl.handleActivityEvent({
      event: 'streaming',
      activity: makeAFActivity({ id: 'think-1', kind: 'thinking' }),
      delta_field: 'reasoning',
      delta_chunk: 'World',
    });

    const activity = tl.activities.value.find((a) => a.id === 'think-1');
    expect(activity?.reasoning).toBe('Hello World');
  });

  it('handleActivityEvent (streaming) updates content field', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'reply-1', kind: 'reply' }),
    });

    tl.handleActivityEvent({
      event: 'streaming',
      activity: makeAFActivity({ id: 'reply-1', kind: 'reply' }),
      delta_field: 'content',
      delta_chunk: 'Hi',
    });

    const activity = tl.activities.value.find((a) => a.id === 'reply-1');
    expect(activity?.content).toBe('Hi');
  });

  it('handleActivityEvent (streaming) accumulates tool_arguments for action activities', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({
        id: 'action-1',
        kind: 'action',
        tool_call_id: 'call-1',
        tool_name: 'read_file',
        tool_arguments: '{"path": "',
      }),
    });

    tl.handleActivityEvent({
      event: 'streaming',
      activity: makeAFActivity({ id: 'action-1', kind: 'action', status: 'tool_running' }),
      delta_field: 'tool_arguments',
      delta_chunk: 'README.md',
    });
    tl.handleActivityEvent({
      event: 'streaming',
      activity: makeAFActivity({ id: 'action-1', kind: 'action', status: 'tool_running' }),
      delta_field: 'tool_arguments',
      delta_chunk: '"}',
    });

    const activity = tl.activities.value.find((a) => a.id === 'action-1');
    expect(activity?.toolArguments).toBe('{"path": "README.md"}');
  });

  it('handleActivityEvent (completed) updates status and fields', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'act-1', kind: 'thinking' }),
    });

    tl.handleActivityEvent({
      event: 'completed',
      activity: makeAFActivity({
        id: 'act-1',
        kind: 'thinking',
        status: 'completed',
        duration_ms: 1500,
        collapsed: true,
        content: 'full content',
        reasoning: 'full reasoning',
      }),
    });

    const activity = tl.activities.value.find((a) => a.id === 'act-1');
    expect(activity?.status).toBe('completed');
    expect(activity?.durationMs).toBe(1500);
    expect(activity?.collapsed).toBe(true);
    expect(activity?.content).toBe('full content');
    expect(activity?.reasoning).toBe('full reasoning');
  });

  it('handleActivityEvent (created) creates new object for reactivity on streaming', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'reply-1', kind: 'reply' }),
    });

    const before = tl.activities.value.find((a) => a.id === 'reply-1');
    const beforeRef = before;

    tl.handleActivityEvent({
      event: 'streaming',
      activity: makeAFActivity({ id: 'reply-1', kind: 'reply' }),
      delta_field: 'content',
      delta_chunk: 'updated',
    });

    const after = tl.activities.value.find((a) => a.id === 'reply-1');
    // The object reference should be different (new object created for reactivity)
    expect(after).not.toBe(beforeRef);
    expect(after?.content).toBe('updated');
  });

  it('reset clears all activities', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'act-1', kind: 'task' }),
    });
    expect(tl.activities.value).toHaveLength(1);

    tl.reset();
    expect(tl.activities.value).toHaveLength(0);
    expect(tl.rootActivityId.value).toBeNull();
  });

  it('loadActivities replaces all activities for the given session', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'old', kind: 'task' }),
    });

    // Phase 3: loadActivities scopes the replacement to a session.
    // Pass 'sess-1' explicitly so the new data lands in the current session
    // (matching the test's beforeEach setCurrentSession('sess-1')).
    tl.loadActivities(
      [
        {
          id: 'new-1',
          kind: 'task',
          status: 'completed',
          sessionId: 'sess-1',
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
      ],
      'sess-1',
    );

    expect(tl.activities.value).toHaveLength(1);
    expect(tl.activities.value[0].id).toBe('new-1');
    expect(tl.rootActivityId.value).toBe('new-1');
  });

  it('Phase 3: isolates activities per session_id', () => {
    // Session A
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'a-1', kind: 'task', session_id: 'sess-a' }),
    });
    // Session B
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'b-1', kind: 'task', session_id: 'sess-b' }),
    });

    // sess-a view
    tl.setCurrentSession('sess-a');
    expect(tl.activities.value.map((a) => a.id)).toEqual(['a-1']);
    expect(tl.rootActivityId.value).toBe('a-1');

    // sess-b view
    tl.setCurrentSession('sess-b');
    expect(tl.activities.value.map((a) => a.id)).toEqual(['b-1']);
    expect(tl.rootActivityId.value).toBe('b-1');

    // Switching back to sess-a keeps its data (no reset needed)
    tl.setCurrentSession('sess-a');
    expect(tl.activities.value.map((a) => a.id)).toEqual(['a-1']);
  });

  it('activityTree builds parent-child relationships', () => {
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'root', kind: 'task', parent_activity_id: '' }),
    });
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'child-1', kind: 'thinking', parent_activity_id: 'root' }),
    });
    tl.handleActivityEvent({
      event: 'created',
      activity: makeAFActivity({ id: 'child-2', kind: 'reply', parent_activity_id: 'root' }),
    });

    const tree = tl.activityTree.value;
    expect(tree).toHaveLength(1); // one root
    expect(tree[0].id).toBe('root');
    expect(tree[0].children).toHaveLength(2);
    expect(tree[0].children[0].id).toBe('child-1');
    expect(tree[0].children[1].id).toBe('child-2');
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

  // --- §9.1.3: ensureActivitiesLoaded cache-skip behavior ---

  it('ensureActivitiesLoaded skips API call when session is already cached', async () => {
    vi.mocked(listActivities).mockResolvedValue([]);
    // Prime the cache for sess-1 via loadActivities (no API call).
    tl.loadActivities([], 'sess-1');
    const callCountBefore = vi.mocked(listActivities).mock.calls.length;

    await tl.ensureActivitiesLoaded('sess-1');

    // No additional API call should have been made.
    expect(vi.mocked(listActivities).mock.calls.length).toBe(callCountBefore);
  });

  it('ensureActivitiesLoaded loads from API when session is not cached', async () => {
    vi.mocked(listActivities).mockResolvedValue([
      {
        id: 'act-lazy',
        kind: 'task',
        status: 'completed',
        sessionId: 'sess-lazy',
        turnId: 'turn-1',
        parentActivityId: null,
        timestamp: '2026-06-13T00:00:00Z',
        durationMs: 0,
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

    tl.setCurrentSession('sess-lazy');
    await tl.ensureActivitiesLoaded('sess-lazy');

    expect(listActivities).toHaveBeenCalledWith('sess-lazy', undefined);
    expect(tl.activities.value).toHaveLength(1);
    expect(tl.activities.value[0].id).toBe('act-lazy');
  });

  it('ensureActivitiesLoaded retries on next call after a failed load (cache not populated)', async () => {
    vi.useFakeTimers();
    tl.setCurrentSession('sess-fail');
    // First call: all retries fail → cache NOT populated.
    vi.mocked(listActivities).mockRejectedValue(new Error('down'));
    let promise = tl.ensureActivitiesLoaded('sess-fail');
    await vi.advanceTimersByTimeAsync(8000);
    await promise;
    expect(tl.loadError.value).toBeTruthy();
    expect(vi.mocked(listActivities)).toHaveBeenCalledTimes(5);

    // Second call: cache still absent → should retry the API.
    vi.mocked(listActivities).mockResolvedValue([]);
    vi.mocked(listActivities).mockClear();
    promise = tl.ensureActivitiesLoaded('sess-fail');
    await promise;
    expect(listActivities).toHaveBeenCalledWith('sess-fail', undefined);
    expect(tl.loadError.value).toBeNull();
    vi.useRealTimers();
  });

  describe('sortedActivities', () => {
    it('returns flat list including task, sorted by timestamp', () => {
      tl.setCurrentSession('s1');
      // 创建 task + thinking + reply
      tl.handleActivityEvent({
        event: 'created',
        activity: makeAFActivity({
          id: 't1',
          kind: 'task',
          session_id: 's1',
          timestamp: '2026-01-01T00:00:00Z',
          content: 'hi',
        }),
      });
      tl.handleActivityEvent({
        event: 'created',
        activity: makeAFActivity({
          id: 'th1',
          kind: 'thinking',
          session_id: 's1',
          timestamp: '2026-01-01T00:00:01Z',
          parent_activity_id: 't1',
        }),
      });
      tl.handleActivityEvent({
        event: 'created',
        activity: makeAFActivity({
          id: 'r1',
          kind: 'reply',
          session_id: 's1',
          timestamp: '2026-01-01T00:00:02Z',
          parent_activity_id: 't1',
        }),
      });

      const ids = tl.sortedActivities.value.map((a) => a.id);
      expect(ids).toEqual(['t1', 'th1', 'r1']); // 含 task，按 timestamp 排序
    });
  });
});
