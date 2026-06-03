import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useEvaluationStore } from '../evaluation';

vi.mock('../../features/evaluation/api', () => ({
  listDatasets: vi.fn().mockResolvedValue({ items: [{ id: 'ds-1', name: 'Test', case_count: 0 }], total: 1 }),
  createDataset: vi.fn().mockResolvedValue({ id: 'ds-2', name: 'New', case_count: 0 }),
  deleteDataset: vi.fn().mockResolvedValue(undefined),
  listRuns: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  runEvaluation: vi.fn().mockResolvedValue({ id: 'run-1', status: 'running' }),
  getRun: vi.fn(),
  getRunResults: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  uploadCases: vi.fn(),
}));

describe('useEvaluationStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loads datasets into state', async () => {
    const store = useEvaluationStore();
    await store.loadDatasets({ limit: 10 });
    expect(store.datasets).toHaveLength(1);
    expect(store.datasets[0].id).toBe('ds-1');
    expect(store.datasetsTotal).toBe(1);
  });

  it('adds a dataset to the front of the list', async () => {
    const store = useEvaluationStore();
    await store.addDataset({ name: 'New', description: '' });
    expect(store.datasets[0].id).toBe('ds-2');
    expect(store.datasetsTotal).toBe(1);
  });
});
