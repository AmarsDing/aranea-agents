import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useHooksStore } from '../hooks';

vi.mock('../../features/hooks/api', () => ({
  listHooks: vi.fn().mockResolvedValue([]),
  createHook: vi.fn(),
  updateHook: vi.fn(),
  deleteHook: vi.fn().mockResolvedValue(undefined),
}));

vi.mock('../../features/hooks/deliveries', () => ({
  listHookDeliveries: vi.fn().mockResolvedValue({ items: [], total: 0 }),
}));

const mockHook = (overrides: Record<string, unknown> = {}): any => ({
  id: 'h1',
  key: 'test-hook',
  name: 'Test Hook',
  description: '',
  status: 'active',
  enabled: true,
  sort_order: 0,
  config_json: '{}',
  metadata_json: '{}',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  ...overrides,
});

describe('useHooksStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('initialises with empty hooks and loading false', () => {
    const store = useHooksStore();
    expect(store.hooks).toEqual([]);
    expect(store.loading).toBe(false);
    expect(store.deliveries).toEqual([]);
    expect(store.deliveriesTotal).toBe(0);
    expect(store.deliveriesLoading).toBe(false);
  });

  it('loadHooks populates hooks and sets loading to false', async () => {
    const { listHooks } = await import('../../features/hooks/api');
    const h1 = mockHook({ id: 'h1' });
    const h2 = mockHook({ id: 'h2' });
    (listHooks as ReturnType<typeof vi.fn>).mockResolvedValueOnce([h1, h2]);

    const store = useHooksStore();
    const result = await store.loadHooks();

    expect(store.hooks).toHaveLength(2);
    expect(store.hooks[0].id).toBe('h1');
    expect(store.loading).toBe(false);
    expect(result).toHaveLength(2);
  });

  it('loadHooks sets loading to false even on error', async () => {
    const { listHooks } = await import('../../features/hooks/api');
    (listHooks as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network'));

    const store = useHooksStore();
    await expect(store.loadHooks()).rejects.toThrow('network');
    expect(store.loading).toBe(false);
  });

  it('addHook prepends created hook to list', async () => {
    const { createHook } = await import('../../features/hooks/api');
    const created = mockHook({ id: 'h-new', key: 'new-hook', name: 'New Hook' });
    (createHook as ReturnType<typeof vi.fn>).mockResolvedValueOnce(created);

    const store = useHooksStore();
    store.hooks = [mockHook({ id: 'h1' })] as any;

    const result = await store.addHook({
      key: 'new-hook',
      name: 'New Hook',
      rule: { event: 'session.created', action: 'webhook', config: {} } as any,
    });

    expect(result.id).toBe('h-new');
    expect(store.hooks[0].id).toBe('h-new');
    expect(store.hooks).toHaveLength(2);
  });

  it('saveHook updates matching hook in list', async () => {
    const { updateHook } = await import('../../features/hooks/api');
    const updated = mockHook({ id: 'h1', name: 'Updated Hook' });
    (updateHook as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

    const store = useHooksStore();
    store.hooks = [mockHook({ id: 'h1', name: 'Old Hook' }), mockHook({ id: 'h2' })] as any;

    const result = await store.saveHook('h1', { name: 'Updated Hook' });

    expect(result.name).toBe('Updated Hook');
    expect(store.hooks[0].name).toBe('Updated Hook');
    expect(store.hooks[1].id).toBe('h2');
  });

  it('removeHook deletes hook from list', async () => {
    const { deleteHook } = await import('../../features/hooks/api');

    const store = useHooksStore();
    store.hooks = [mockHook({ id: 'h1' }), mockHook({ id: 'h2' })] as any;

    await store.removeHook('h1');

    expect(deleteHook).toHaveBeenCalledWith('h1');
    expect(store.hooks).toHaveLength(1);
    expect(store.hooks[0].id).toBe('h2');
  });

  it('loadDeliveries populates deliveries and total', async () => {
    const { listHookDeliveries } = await import('../../features/hooks/deliveries');
    const d1 = { id: 'd1', hook_key: 'test', hook_id: 'h1', status: 'delivered' };
    (listHookDeliveries as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      items: [d1],
      total: 1,
    });

    const store = useHooksStore();
    const result = await store.loadDeliveries({ hook_key: 'test' });

    expect(store.deliveries).toHaveLength(1);
    expect(store.deliveriesTotal).toBe(1);
    expect(store.deliveriesLoading).toBe(false);
    expect(result.items).toHaveLength(1);
    expect(result.total).toBe(1);
  });

  it('loadDeliveries sets loading to false even on error', async () => {
    const { listHookDeliveries } = await import('../../features/hooks/deliveries');
    (listHookDeliveries as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network'));

    const store = useHooksStore();
    await expect(store.loadDeliveries()).rejects.toThrow('network');
    expect(store.deliveriesLoading).toBe(false);
  });

  it('addHook deduplicates by id', async () => {
    const { createHook } = await import('../../features/hooks/api');
    const created = mockHook({ id: 'h1', name: 'Recreated' });
    (createHook as ReturnType<typeof vi.fn>).mockResolvedValueOnce(created);

    const store = useHooksStore();
    store.hooks = [mockHook({ id: 'h1', name: 'Old' })] as any;

    await store.addHook({
      key: 'test-hook',
      name: 'Recreated',
      rule: { event: 'session.created', action: 'webhook', config: {} } as any,
    });

    expect(store.hooks).toHaveLength(1);
    expect(store.hooks[0].name).toBe('Recreated');
  });
});
