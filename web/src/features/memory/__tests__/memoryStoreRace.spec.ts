import { beforeEach, describe, expect, it, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';

import { listMemoryFacts, listMemoryEntities } from '../api';
import { useMemoryStore } from '../../../stores/memory';

vi.mock('../api', () => ({
  listL0Snapshots: vi.fn(),
  listMemoryFacts: vi.fn(),
  listMemoryEntities: vi.fn(),
  listL1Tasks: vi.fn(),
  getAgentIdentity: vi.fn(),
  getAgentStrategy: vi.fn(),
  listEvolutionProposals: vi.fn(),
  listEvolutionEvents: vi.fn(),
  getEvolutionMetrics: vi.fn(),
  listCascadeProposals: vi.fn(),
  approveCascadeProposal: vi.fn(),
  rejectCascadeProposal: vi.fn(),
  previewCascadeApprove: vi.fn(),
  getCascadeSagaSteps: vi.fn(),
  retryCascadeApprove: vi.fn(),
  compensateCascadeApprove: vi.fn(),
  compositeSearchMemories: vi.fn(),
  debugMemoryRecall: vi.fn(),
  getMemoryNeighborhood: vi.fn(),
  getSpreadingActivation: vi.fn(),
  getMemoryPlatformSettings: vi.fn(),
  listMemoryDeadLetters: vi.fn(),
  updateMemoryPlatformSettings: vi.fn(),
  getMemoryWorkerStatus: vi.fn(),
  replayMemoryDeadLetter: vi.fn(),
  abandonMemoryDeadLetter: vi.fn(),
  listL1Fields: vi.fn(),
  listConflictingFacts: vi.fn(),
  upsertMemoryFact: vi.fn(),
  reviewMemoryFact: vi.fn(),
  appendEvolutionEvent: vi.fn(),
  listPIIFlaggedFacts: vi.fn(),
  reviewPIIFact: vi.fn(),
  getMemoryLayerOverview: vi.fn(),
  getMemoryEpisodes: vi.fn(),
  getUnifiedMemoryGraph: vi.fn(),
}));

const mockListFacts = vi.mocked(listMemoryFacts);
const mockListEntities = vi.mocked(listMemoryEntities);

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

describe('memory store latest-wins guards (P1-4)', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('drops a stale loadFacts response that resolves after a newer one', async () => {
    const slow = deferred<{ items: unknown[]; total: number }>();
    mockListFacts
      .mockReturnValueOnce(slow.promise as ReturnType<typeof listMemoryFacts>)
      .mockResolvedValueOnce({ items: [{ id: 'new' }], total: 1 } as never);

    const store = useMemoryStore();
    const first = store.loadFacts({ agent_id: 'old' } as never);
    await store.loadFacts({ agent_id: 'new' } as never);
    expect(store.facts).toEqual([{ id: 'new' }]);

    slow.resolve({ items: [{ id: 'stale' }], total: 1 });
    await first;
    expect(store.facts).toEqual([{ id: 'new' }]);
  });

  it('drops a stale loadEntities response', async () => {
    const slow = deferred<{ items: unknown[] }>();
    mockListEntities
      .mockReturnValueOnce(slow.promise as ReturnType<typeof listMemoryEntities>)
      .mockResolvedValueOnce({ items: [{ id: 'new-entity' }] } as never);

    const store = useMemoryStore();
    const first = store.loadEntities({ agent_id: 'old' } as never);
    await store.loadEntities({ agent_id: 'new' } as never);
    expect(store.entities).toEqual([{ id: 'new-entity' }]);

    slow.resolve({ items: [{ id: 'stale-entity' }] });
    await first;
    expect(store.entities).toEqual([{ id: 'new-entity' }]);
  });

  it('clearFacts invalidates an in-flight response', async () => {
    const slow = deferred<{ items: unknown[]; total: number }>();
    mockListFacts.mockReturnValueOnce(slow.promise as ReturnType<typeof listMemoryFacts>);

    const store = useMemoryStore();
    const pending = store.loadFacts({ agent_id: 'a' } as never);
    store.clearFacts();
    slow.resolve({ items: [{ id: 'stale' }], total: 1 });
    await pending;
    expect(store.facts).toEqual([]);
  });
});
