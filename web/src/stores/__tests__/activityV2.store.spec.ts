// web/src/stores/__tests__/activityV2.store.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../chat/activityV2Store';
import type { Task, Turn, Step } from '../../features/chat/v2Types';

function makeTask(over: Partial<Task> = {}): Task {
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

describe('useChatActivityStore', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('starts empty', () => {
    const s = useChatActivityStore();
    expect(s.tasks.size).toBe(0);
    expect(s.turns.size).toBe(0);
    expect(s.steps.size).toBe(0);
  });

  it('upsertTask adds a task', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1' }));
    expect(s.tasks.get('t1')?.UserMessage).toBe('hi');
  });

  it('upsertTask replaces with higher version', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', Version: 1, Status: 'running' }));
    s.upsertTask(makeTask({ ID: 't1', Version: 2, Status: 'completed' }));
    expect(s.tasks.get('t1')?.Status).toBe('completed');
  });

  it('upsertTask ignores lower version', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', Version: 2, Status: 'completed' }));
    s.upsertTask(makeTask({ ID: 't1', Version: 1, Status: 'running' }));
    expect(s.tasks.get('t1')?.Status).toBe('completed');
  });

  it('upsertStep merges streaming content', () => {
    const s = useChatActivityStore();
    const step: Step = {
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
    };
    s.upsertStep(step);
    s.appendStepDelta('s1', 'content', 'hello ');
    s.appendStepDelta('s1', 'content', 'world');
    expect(s.steps.get('s1')?.Content).toBe('hello world');
  });

  it('getSessionTasks returns tasks sorted by seq', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't2', Seq: 2 }));
    s.upsertTask(makeTask({ ID: 't1', Seq: 1 }));
    const tasks = s.getSessionTasks('s1');
    expect(tasks.map((t) => t.ID)).toEqual(['t1', 't2']);
  });

  it('clearSession removes entities by spirit session id', () => {
    const s = useChatActivityStore();
    s.upsertTask(makeTask({ ID: 't1', SessionID: 's1' }));
    s.upsertTask(makeTask({ ID: 't2', SessionID: 's2' }));
    s.clearSession('s1');
    expect(s.tasks.has('t1')).toBe(false);
    expect(s.tasks.has('t2')).toBe(true);
  });
});
