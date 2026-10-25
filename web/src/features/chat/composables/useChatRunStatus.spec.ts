import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useChatRunStatus } from './useChatRunStatus';

vi.mock('../api', () => ({
  getRunStatus: vi.fn(),
}));

import { getRunStatus } from '../api';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import type { Activity } from '../../../realtime/activityEvent';

function makeActivity(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'act-1',
    kind: 'task',
    status: 'running',
    session_id: 'sess-1',
    turn_id: 'turn-1',
    parent_activity_id: '',
    timestamp: '2026-06-10T10:00:00.000Z',
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

describe('useChatRunStatus', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.useFakeTimers();
    vi.mocked(getRunStatus).mockReset();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('prefers WS ActivityEvent over delayed HTTP hydrate', async () => {
    vi.mocked(getRunStatus).mockResolvedValue({
      status: 'running',
      runId: 'http-run',
      errorMessage: '',
      updatedAt: '',
    });

    const applyAwaitRunStatus = vi.fn();
    const { runStatus, applyFromActivityEvent, onSessionSwitch } = useChatRunStatus({
      applyAwaitRunStatus,
    });

    onSessionSwitch('sess-1');
    applyFromActivityEvent(activityEvent({ status: 'completed', run_id: 'ws-run' }));

    expect(runStatus.value).toBe('completed');
    vi.advanceTimersByTime(500);
    await Promise.resolve();
    expect(getRunStatus).not.toHaveBeenCalled();
  });

  it('hydrates from HTTP when WS silent after session switch', async () => {
    vi.mocked(getRunStatus).mockResolvedValue({
      status: 'awaiting_user',
      runId: 'run-1',
      errorMessage: '',
      updatedAt: '',
      awaitKind: 'reply',
    });

    const applyAwaitRunStatus = vi.fn();
    const { runStatus, onSessionSwitch } = useChatRunStatus({ applyAwaitRunStatus });

    onSessionSwitch('sess-2');
    await vi.advanceTimersByTimeAsync(400);

    expect(getRunStatus).toHaveBeenCalledWith('sess-2');
    expect(runStatus.value).toBe('awaiting_user');
    expect(applyAwaitRunStatus).toHaveBeenCalled();
  });

  // E2E-B3：PauseSession 取消链同 runId 连发 cancelled→paused 两帧。
  describe('E2E-B3 paused lifecycle hold', () => {
    function setup() {
      const applyAwaitRunStatus = vi.fn();
      return {
        applyAwaitRunStatus,
        ...useChatRunStatus({ applyAwaitRunStatus }),
      };
    }

    it('accepts paused frame after terminal cancelled for the same run', () => {
      const { runStatus, applyFromActivityEvent } = setup();
      applyFromActivityEvent(activityEvent({ status: 'running', run_id: 'run-1' }));
      applyFromActivityEvent(activityEvent({ status: 'cancelled', run_id: 'run-1' }));
      applyFromActivityEvent(activityEvent({ status: 'paused', run_id: 'run-1' }));
      expect(runStatus.value).toBe('paused');
    });

    it('keeps terminal stickiness against plain non-terminal regression (running after cancelled rejected)', () => {
      const { runStatus, applyFromActivityEvent } = setup();
      applyFromActivityEvent(activityEvent({ status: 'cancelled', run_id: 'run-1' }));
      applyFromActivityEvent(activityEvent({ status: 'running', run_id: 'run-1' }));
      expect(runStatus.value).toBe('cancelled');
    });

    it('accepts resume (running) after paused for the same run', () => {
      const { runStatus, applyFromActivityEvent } = setup();
      applyFromActivityEvent(activityEvent({ status: 'cancelled', run_id: 'run-1' }));
      applyFromActivityEvent(activityEvent({ status: 'paused', run_id: 'run-1' }));
      applyFromActivityEvent(activityEvent({ status: 'running', run_id: 'run-1' }));
      expect(runStatus.value).toBe('running');
    });

    it('treats paused as settled across runIds (new turn frame accepted)', () => {
      const { runStatus, applyFromActivityEvent } = setup();
      applyFromActivityEvent(activityEvent({ status: 'paused', run_id: 'run-1' }));
      applyFromActivityEvent(activityEvent({ status: 'running', run_id: 'run-2' }));
      expect(runStatus.value).toBe('running');
    });

    it('accepts terminal after paused for the same run', () => {
      const { runStatus, applyFromActivityEvent } = setup();
      applyFromActivityEvent(activityEvent({ status: 'paused', run_id: 'run-1' }));
      applyFromActivityEvent(activityEvent({ status: 'completed', run_id: 'run-1' }));
      expect(runStatus.value).toBe('completed');
    });
  });
});
