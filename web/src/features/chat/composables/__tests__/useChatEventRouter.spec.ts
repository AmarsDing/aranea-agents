// web/src/features/chat/composables/__tests__/useChatEventRouter.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import { useChatEventRouter } from '../useChatEventRouter';
import type { V2WsEnvelope, Task, Step } from '../../v2Types';

function mkTask(over: Partial<Task> = {}): Task {
  return {
    ID: 't1', SessionID: 's1', UserMessage: 'hi', Status: 'running',
    Seq: 1, Version: 1, CreatedAt: '', UpdatedAt: '', CompletedAt: null, ...over,
  };
}

function mkStep(over: Partial<Step> = {}): Step {
  return {
    ID: 's1', TurnID: 't1', TaskID: 'tk1', SessionID: 's1',
    SpiritSessionID: 's1', Kind: 'reply', AuthorAgentKey: 'a1',
    Seq: 1, Version: 1, Content: '', Reasoning: '',
    ToolName: '', ToolCallID: '', ToolArgs: null, ToolResult: null,
    ToolDurationMs: 0, ToolErrorCode: '', Status: 'running',
    IsFinal: false, StartedAt: '', CompletedAt: null, ...over,
  };
}

describe('useChatEventRouter', () => {
  let store: ReturnType<typeof useChatActivityStore>;
  let router: ReturnType<typeof useChatEventRouter>;

  beforeEach(() => {
    setActivePinia(createPinia());
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
      type: 'v2_event', kind: 'step.streaming',
      payload: { StepID: 's1', DeltaField: 'content', DeltaChunk: 'hello' },
    });
    expect(store.steps.get('s1')?.Content).toBe('hello');
  });

  it('handles step.streaming reasoning', () => {
    store.upsertStep(mkStep({ ID: 's1', Reasoning: '' }));
    router.dispatch({
      type: 'v2_event', kind: 'step.streaming',
      payload: { StepID: 's1', DeltaField: 'reasoning', DeltaChunk: 'think' },
    });
    expect(store.steps.get('s1')?.Reasoning).toBe('think');
  });

  it('handles task.completed with version guard', () => {
    store.upsertTask(mkTask({ ID: 't1', Version: 1, Status: 'running' }));
    router.dispatch({
      type: 'v2_event', kind: 'task.completed',
      payload: { Task: mkTask({ ID: 't1', Version: 2, Status: 'completed' }) },
    });
    expect(store.tasks.get('t1')?.Status).toBe('completed');
  });

  it('ignores unknown event kinds', () => {
    router.dispatch({ type: 'v2_event', kind: 'unknown.kind' as never, payload: {} as never });
    expect(store.tasks.size).toBe(0);
  });
});
