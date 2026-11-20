import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useAgentsPageStore } from '../agents';

vi.mock('../../features/agents/api', () => ({
  listAgentsPaged: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  deleteAgent: vi.fn().mockResolvedValue(undefined),
  toggleAgentFavorite: vi.fn(),
  checkAgentKey: vi.fn(),
  listAgentTemplates: vi.fn().mockResolvedValue([]),
  duplicateAgent: vi.fn(),
  updateAgent: vi.fn(),
}));

vi.mock('../../features/platform/api', () => ({
  listPlatformResources: vi.fn().mockResolvedValue([]),
  listPlatformResourceTree: vi.fn().mockResolvedValue([]),
  validateModel: vi.fn().mockResolvedValue({ ok: true }),
}));

vi.mock('../avatar', () => ({
  useAvatarCatalogStore: vi.fn().mockReturnValue({ ensureAgentsCatalog: vi.fn().mockResolvedValue(undefined) }),
}));

vi.mock('../sessionMutationBus', () => ({
  emitSessionMutation: vi.fn(),
}));

describe('useAgentsPageStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loadAgentList populates agents and total', async () => {
    const { listAgentsPaged } = await import('../../features/agents/api');
    const mockAgents = [
      { id: 'a1', display_name: 'Agent 1', is_favorite: false },
      { id: 'a2', display_name: 'Agent 2', is_favorite: true },
    ];
    (listAgentsPaged as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ items: mockAgents, total: 2 });

    const store = useAgentsPageStore();
    await store.loadAgentList();

    expect(store.agents).toHaveLength(2);
    expect(store.agents[0].id).toBe('a1');
    expect(store.total).toBe(2);
    expect(store.listLoading).toBe(false);
    expect(listAgentsPaged).toHaveBeenCalledWith({ limit: 1000, offset: 0 });
  });

  it('loadAgentList sets listLoading to false even on error', async () => {
    const { listAgentsPaged } = await import('../../features/agents/api');
    (listAgentsPaged as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network'));

    const store = useAgentsPageStore();
    await expect(store.loadAgentList()).rejects.toThrow('network');
    expect(store.listLoading).toBe(false);
  });

  it('loadAgentList records listError and clears it on a successful retry', async () => {
    const { listAgentsPaged } = await import('../../features/agents/api');
    const store = useAgentsPageStore();

    (listAgentsPaged as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network down'));
    await expect(store.loadAgentList()).rejects.toThrow('network down');
    expect(store.listError).toBe('network down');

    (listAgentsPaged as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ items: [], total: 0 });
    await store.loadAgentList();
    expect(store.listError).toBe('');
  });

  it('toggleAgentFavorite updates list and emits event on success', async () => {
    const { toggleAgentFavorite } = await import('../../features/agents/api');
    const { emitSessionMutation } = await import('../sessionMutationBus');

    const store = useAgentsPageStore();
    store.agents = [{ id: 'a1', display_name: 'Agent 1', is_favorite: false }] as any;

    const updatedAgent = { id: 'a1', display_name: 'Agent 1', is_favorite: true };
    (toggleAgentFavorite as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updatedAgent);

    await store.toggleAgentFavorite('a1');

    expect(store.agents[0].is_favorite).toBe(true);
    expect(emitSessionMutation).toHaveBeenCalledWith({ type: 'agent_updated', agent: updatedAgent });
  });

  it('toggleAgentFavorite rolls back on failure', async () => {
    const { toggleAgentFavorite } = await import('../../features/agents/api');

    const store = useAgentsPageStore();
    store.agents = [{ id: 'a1', display_name: 'Agent 1', is_favorite: false }] as any;

    (toggleAgentFavorite as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('fail'));

    await expect(store.toggleAgentFavorite('a1')).rejects.toThrow('fail');
    expect(store.agents[0].is_favorite).toBe(false);
  });

  it('removeListedAgent deletes then reloads list', async () => {
    const api = await import('../../features/agents/api');
    const { listAgentsPaged } = api;

    (listAgentsPaged as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ items: [], total: 0 });

    const store = useAgentsPageStore();
    await store.removeListedAgent('a1');

    expect(api.deleteAgent).toHaveBeenCalledWith('a1');
    expect(listAgentsPaged).toHaveBeenCalled();
  });
});
