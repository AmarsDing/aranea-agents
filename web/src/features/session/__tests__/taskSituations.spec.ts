import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';

const { listMock } = vi.hoisted(() => ({ listMock: vi.fn() }));
vi.mock('../../../services/index', () => ({
  createSessionService: () => ({}),
  createSessionV2Service: () => ({ ListTaskSituations: listMock }),
}));

import { listTaskSituations } from '../api';
import { useTaskSituationStore } from '../../../stores/session/taskSituations';

beforeEach(() => {
  setActivePinia(createPinia());
  listMock.mockReset();
});

describe('task situation API', () => {
  it('preserves optional absence and authoritative zero values', async () => {
    listMock.mockResolvedValue({
      items: [
        { taskId: 'task-1', sessionId: 'sess-1', status: 'waiting' },
        { taskId: 'task-2', sessionId: 'sess-2', status: 'completed', artifactCount: 0, retryAvailable: false },
      ],
      total: 35,
      limit: 20,
      offset: 0,
    });
    const page = await listTaskSituations({ status: 'waiting', keyword: 'release', limit: 20, offset: 0 });
    expect(listMock).toHaveBeenCalledWith({
      sessionId: undefined,
      status: 'waiting',
      keyword: 'release',
      limit: 20,
      offset: 0,
      sortBy: 'updated_at',
      sortOrder: 'desc',
    });
    expect(page.items[0].title).toBeUndefined();
    expect(page.items[0].artifact_count).toBeUndefined();
    expect(page.items[0].updated_at).toBeUndefined();
    expect(page.items[1].artifact_count).toBe(0);
    expect(page.items[1].retry_available).toBe(false);
  });
});

describe('task situation store', () => {
  it('keeps only the server page and retains the last snapshot on failure', async () => {
    listMock.mockResolvedValueOnce({
      items: [{ taskId: 'task-1', sessionId: 'sess-1', status: 'running', title: 'first page' }],
      total: 35,
      limit: 20,
      offset: 0,
    });
    const store = useTaskSituationStore();
    await store.load({ limit: 20, offset: 0 });
    expect(store.items.map((item) => item.task_id)).toEqual(['task-1']);
    expect(store.total).toBe(35);

    listMock.mockResolvedValueOnce({
      items: [{ taskId: 'task-21', sessionId: 'sess-2', status: 'waiting', title: 'second page' }],
      total: 35,
      limit: 20,
      offset: 20,
    });
    await store.load({ limit: 20, offset: 20 });
    expect(store.items.map((item) => item.task_id)).toEqual(['task-21']);
    expect(listMock).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 20, offset: 20 }));

    const snapshot = store.items;
    listMock.mockRejectedValueOnce(new Error('offline'));
    await store.load({ limit: 20, offset: 0 });
    expect(store.items).toBe(snapshot);
    expect(store.items[0].task_id).toBe('task-21');
    expect(store.error).toBe('offline');
  });
});
