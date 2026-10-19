import { describe, expect, it, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useChatActivityStore } from '../activityV2Store';
import type { Task } from '../../../features/chat/v2Types';

function pendingTask(id: string, sessionId: string, msg: string): Task {
  return {
    ID: id,
    SessionID: sessionId,
    UserMessage: msg,
    Status: 'pending',
    Seq: 1,
    Version: 0,
    CreatedAt: new Date().toISOString(),
    UpdatedAt: new Date().toISOString(),
    CompletedAt: null,
  };
}

describe('activityV2Store.adoptOptimisticTask', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('remaps pending-user id onto real task id without dual cards', () => {
    const store = useChatActivityStore();
    store.upsertTask(pendingTask('pending-user-abc', 's1', 'hello'));
    store.adoptOptimisticTask('pending-user-abc', 'real-task-1');
    expect(store.tasks.get('pending-user-abc')).toBeUndefined();
    expect(store.tasks.get('real-task-1')?.UserMessage).toBe('hello');
  });

  it('no-ops when ids are equal', () => {
    const store = useChatActivityStore();
    store.upsertTask(pendingTask('pending-user-abc', 's1', 'hello'));
    store.adoptOptimisticTask('pending-user-abc', 'pending-user-abc');
    expect(store.tasks.get('pending-user-abc')?.UserMessage).toBe('hello');
  });
});
