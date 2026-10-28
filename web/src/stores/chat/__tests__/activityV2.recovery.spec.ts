import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useChatActivityStore } from '../activityV2Store';
import * as api from '../../../features/session/v2Api';
import type { Step, Task } from '../../../features/chat/v2Types';
vi.mock('../../../features/session/v2Api', () => ({
  listTasksV2: vi.fn(),
  listTurnsV2: vi.fn(),
  listStepsV2: vi.fn(),
  listTeamStagesV2: vi.fn(),
  listTeamRunsV2: vi.fn(),
  listMemberSessionsV2: vi.fn(),
  listOrphanMemberSessionsV2: vi.fn(),
  listPlanBoardsV2: vi.fn(),
  listPlanStepsV2: vi.fn(),
  listGraphStagesV2: vi.fn(),
  listGraphNodesV2: vi.fn(),
}));
const task = (id = 'T'): Task => ({
  ID: id,
  SessionID: 'S',
  UserMessage: 'work',
  Status: 'running',
  Seq: 1,
  Version: 1,
  CreatedAt: '',
  UpdatedAt: '',
  CompletedAt: null,
});
const step = (version: number, content: string): Step =>
  ({
    ID: 'reply',
    TaskID: 'T',
    TurnID: 'turn',
    SessionID: 'S',
    SpiritSessionID: 'S',
    Kind: 'reply',
    Status: 'completed',
    Content: content,
    Version: version,
    Seq: 1,
  }) as Step;
beforeEach(() => {
  setActivePinia(createPinia());
  for (const fn of Object.values(api)) if (vi.isMockFunction(fn)) fn.mockReset().mockResolvedValue([]);
});

describe('authoritative reconnect snapshots', () => {
  it('refreshes an already hydrated subtree outside the recent steps window', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task());
    vi.mocked(api.listStepsV2).mockResolvedValue([step(1, 'old')]);
    await store.hydrateTask('T');
    expect(store.hydratedTaskIds.has('T')).toBe(true);
    vi.mocked(api.listTasksV2).mockResolvedValue([task()]);
    vi.mocked(api.listStepsV2).mockImplementation(async (_sid, opts) =>
      opts?.taskId ? [step(2, 'delivered after disconnect')] : [],
    );
    await store.fetchSessionHistory('S', { reconcile: true });
    expect(store.steps.get('reply')?.Content).toBe('delivered after disconnect');
    expect(api.listTurnsV2).toHaveBeenCalledTimes(2);
    expect(store.hydratedTaskIds.has('T')).toBe(true);
  });
  it('does not announce successful reconciliation when a subtree failed, then recovers on retry', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task());
    await store.hydrateTask('T');
    vi.mocked(api.listTasksV2).mockResolvedValue([task()]);
    vi.mocked(api.listTurnsV2).mockRejectedValueOnce(new Error('network'));
    await expect(store.fetchSessionHistory('S', { reconcile: true })).rejects.toThrow('incomplete');
    expect(store.taskHydration.get('T')).toBe('error');
    expect(store.hydratedTaskIds.has('T')).toBe(true);
    await store.fetchSessionHistory('S', { reconcile: true });
    expect(store.taskHydration.has('T')).toBe(false);
  });
  it('isolates simultaneous task failures', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task('A'));
    store.upsertTask(task('B'));
    vi.mocked(api.listTurnsV2).mockImplementation(async (id) => {
      if (id === 'A') throw new Error('only A failed');
      return [];
    });
    await Promise.all([store.hydrateTask('A'), store.hydrateTask('B')]);
    expect(store.taskHydration.get('A')).toBe('error');
    expect(store.hydratedTaskIds.has('B')).toBe(true);
  });
  it('does not resurrect cleared entities from a late response', async () => {
    const store = useChatActivityStore();
    store.upsertTask(task());
    let resolve!: (rows: Step[]) => void;
    vi.mocked(api.listStepsV2).mockReturnValueOnce(
      new Promise((r) => {
        resolve = r;
      }),
    );
    const pending = store.hydrateTask('T');
    store.clearSession('S');
    resolve([step(1, 'late')]);
    await pending;
    expect(store.steps.has('reply')).toBe(false);
    expect(store.hydratedTaskIds.has('T')).toBe(false);
  });
});
