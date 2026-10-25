// web/src/features/chat/composables/__tests__/useChatEventRouter.spec.ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import { useChatEventRouter } from '../useChatEventRouter';
import { clearAllStepLiveText } from '../../stepLiveTextCache';
import type { Task, Step, TeamRun } from '../../v2Types';

function mkTask(over: Partial<Task> = {}): Task {
  return {
    ID: 't1',
    SessionID: 's1',
    UserMessage: 'hi',
    Status: 'running',
    Seq: 1,
    Version: 1,
    CreatedAt: '',
    UpdatedAt: '',
    CompletedAt: null,
    ...over,
  };
}

function mkStep(over: Partial<Step> = {}): Step {
  return {
    ID: 's1',
    TurnID: 't1',
    TaskID: 'tk1',
    SessionID: 's1',
    SpiritSessionID: 's1',
    Kind: 'reply',
    AuthorAgentKey: 'a1',
    Seq: 1,
    Version: 1,
    Content: '',
    Reasoning: '',
    ToolName: '',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    Status: 'running',
    IsFinal: false,
    StartedAt: '',
    CompletedAt: null,
    ...over,
  };
}

function mkTeamRun(over: Partial<TeamRun> = {}): TeamRun {
  return {
    ID: 'run-1',
    TeamStageID: 'ts1',
    TaskID: 't1',
    SessionID: 's1',
    SpiritSessionID: 's1',
    DagNodeID: '',
    DependsOn: [],
    Status: 'running',
    StartedAt: '',
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    Error: '',
    ...over,
  };
}

describe('useChatEventRouter', () => {
  let store: ReturnType<typeof useChatActivityStore>;
  let router: ReturnType<typeof useChatEventRouter>;

  beforeEach(() => {
    setActivePinia(createPinia());
    // P3-resume: live-text 缓存是模块级单例，用例间硬编码 stepId（'s1'）
    // 复用会串缓存——每个用例前全量清理。
    clearAllStepLiveText();
    store = useChatActivityStore();
    router = useChatEventRouter(store);
  });

  it('handles task.created', () => {
    router.dispatch({ type: 'v2_event', kind: 'task.created', payload: { Task: mkTask() } });
    expect(store.tasks.get('t1')?.UserMessage).toBe('hi');
  });

  it('handles step.created', () => {
    router.dispatch({ type: 'v2_event', kind: 'step.created', payload: { Step: mkStep() } });
    expect(store.steps.get('s1')?.Kind).toBe('reply');
  });

  it('handles step.streaming by appending content', () => {
    store.upsertStep(mkStep({ ID: 's1', Content: '' }));
    router.dispatch({
      type: 'v2_event',
      kind: 'step.streaming',
      payload: { StepID: 's1', DeltaField: 'content', DeltaChunk: 'hello' },
    });
    expect(store.steps.get('s1')?.Content).toBe('hello');
  });

  it('handles step.streaming reasoning', () => {
    store.upsertStep(mkStep({ ID: 's1', Reasoning: '' }));
    router.dispatch({
      type: 'v2_event',
      kind: 'step.streaming',
      payload: { StepID: 's1', DeltaField: 'reasoning', DeltaChunk: 'think' },
    });
    expect(store.steps.get('s1')?.Reasoning).toBe('think');
  });

  // P2-06: unknown DeltaField must not corrupt Reasoning (old if/else bug).
  it('handles step.streaming with unknown DeltaField without corrupting Reasoning', () => {
    store.upsertStep(mkStep({ ID: 's1', Content: '', Reasoning: 'existing' }));
    router.dispatch({
      type: 'v2_event',
      kind: 'step.streaming',
      payload: { StepID: 's1', DeltaField: 'tool_args', DeltaChunk: '{"arg":' },
    });
    expect(store.steps.get('s1')?.Content).toBe('');
    expect(store.steps.get('s1')?.Reasoning).toBe('existing');
  });

  it('handles task.completed with version guard', () => {
    store.upsertTask(mkTask({ ID: 't1', Version: 1, Status: 'running' }));
    router.dispatch({
      type: 'v2_event',
      kind: 'task.completed',
      payload: { Task: mkTask({ ID: 't1', Version: 2, Status: 'completed' }) },
    });
    expect(store.tasks.get('t1')?.Status).toBe('completed');
  });

  // 2026-10-20: 取消终态由专用 kind 承载（后端不再复用 task.completed）。
  it('handles task.cancelled as terminal upsert', () => {
    store.upsertTask(mkTask({ ID: 't1', Version: 1, Status: 'running' }));
    router.dispatch({
      type: 'v2_event',
      kind: 'task.cancelled',
      payload: { Task: mkTask({ ID: 't1', Version: 2, Status: 'cancelled' }) },
    });
    expect(store.tasks.get('t1')?.Status).toBe('cancelled');
  });

  it('ignores unknown event kinds', () => {
    router.dispatch({ type: 'v2_event', kind: 'unknown.kind' as never, payload: {} as never });
    expect(store.tasks.size).toBe(0);
  });

  it('task.created marks the new task as hydrated (live tasks default expanded)', () => {
    router.dispatch({
      type: 'v2_event',
      kind: 'task.created',
      payload: { Task: mkTask({ ID: 't-live' }) },
    });
    expect(store.hydratedTaskIds.has('t-live')).toBe(true);
  });

  // D4-5：team_run.cancelled 与三者共用 store.upsertTeamRun（状态平面以实体 Status 为准）。
  it('handles team_run.cancelled via the shared team_run terminal upsert', () => {
    store.upsertTeamRun(mkTeamRun({ ID: 'run-1', Version: 1, Status: 'running' }));
    router.dispatch({
      type: 'v2_event',
      kind: 'team_run.cancelled',
      payload: { TeamRun: mkTeamRun({ ID: 'run-1', Version: 2, Status: 'cancelled' }) },
    });
    expect(store.teamRuns.get('run-1')?.Status).toBe('cancelled');
  });

  // D4-5：同 ID 重复投递幂等（Version 守卫拒写，实体引用不变）。
  it('team_run.cancelled re-dispatch with the same id is idempotent', () => {
    router.dispatch({
      type: 'v2_event',
      kind: 'team_run.cancelled',
      payload: { TeamRun: mkTeamRun({ ID: 'run-1', Version: 2, Status: 'cancelled' }) },
    });
    const first = store.teamRuns.get('run-1');
    router.dispatch({
      type: 'v2_event',
      kind: 'team_run.cancelled',
      payload: { TeamRun: mkTeamRun({ ID: 'run-1', Version: 2, Status: 'cancelled' }) },
    });
    expect(store.teamRuns.get('run-1')).toBe(first);
  });

  describe('unknown / non-v2 frame throttled warn (D9-22)', () => {
    beforeEach(() => {
      vi.useFakeTimers();
    });

    afterEach(() => {
      vi.useRealTimers();
    });

    it('warns once for an unknown v2 event kind and throttles repeats within 60s', () => {
      const spy = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
      router.dispatch({ type: 'v2_event', kind: 'mystery.alpha' as never, payload: {} as never });
      router.dispatch({ type: 'v2_event', kind: 'mystery.alpha' as never, payload: {} as never });
      expect(spy).toHaveBeenCalledTimes(1);
      expect(spy.mock.calls[0]?.[0]).toBe('[chat] unknown v2 event kind');
      expect(spy.mock.calls[0]?.[1]).toBe('mystery.alpha');
      spy.mockRestore();
    });

    it('warns again after the 60s throttle window', () => {
      const spy = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
      router.dispatch({ type: 'v2_event', kind: 'mystery.beta' as never, payload: {} as never });
      expect(spy).toHaveBeenCalledTimes(1);
      vi.advanceTimersByTime(60_001);
      router.dispatch({ type: 'v2_event', kind: 'mystery.beta' as never, payload: {} as never });
      expect(spy).toHaveBeenCalledTimes(2);
      spy.mockRestore();
    });

    it('known kinds never trigger warn', () => {
      const spy = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
      router.dispatch({ type: 'v2_event', kind: 'task.created', payload: { Task: mkTask() } });
      router.dispatch({ type: 'v2_event', kind: 'team_run.cancelled', payload: { TeamRun: mkTeamRun() } });
      expect(spy).not.toHaveBeenCalled();
      spy.mockRestore();
    });

    it('warns once for non-v2_event frames and does not touch the store', () => {
      const spy = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
      router.dispatch({ type: 'legacy_event', kind: 'team_run.cancelled', payload: { TeamRun: mkTeamRun() } } as never);
      router.dispatch({ type: 'legacy_event', kind: 'team_run.cancelled', payload: { TeamRun: mkTeamRun() } } as never);
      expect(spy).toHaveBeenCalledTimes(1);
      expect(spy.mock.calls[0]?.[0]).toBe('[chat] ws frame dropped: non-v2_event');
      expect(store.teamRuns.size).toBe(0);
      spy.mockRestore();
    });
  });
});
