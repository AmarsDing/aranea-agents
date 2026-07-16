import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useWebhooksStore } from '../webhooks';

vi.mock('../../features/webhooks/api', () => ({
  listWebhooks: vi.fn().mockResolvedValue([]),
  listWebhooksPaged: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 }),
  createWebhook: vi.fn(),
  updateWebhook: vi.fn(),
  deleteWebhook: vi.fn().mockResolvedValue(undefined),
}));

const mockWebhook = (overrides: Record<string, unknown> = {}): any => ({
  id: 'wh1',
  name: 'Test Webhook',
  url: 'https://example.com/hook',
  event_types_json: '["session.created"]',
  secret: '',
  headers: {},
  enabled: true,
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  ...overrides,
});

describe('useWebhooksStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('initialises with empty webhooks and loading false', () => {
    const store = useWebhooksStore();
    expect(store.webhooks).toEqual([]);
    expect(store.loading).toBe(false);
  });

  it('loadWebhooks populates webhooks and sets loading to false', async () => {
    const { listWebhooks } = await import('../../features/webhooks/api');
    const wh1 = mockWebhook({ id: 'wh1' });
    const wh2 = mockWebhook({ id: 'wh2' });
    (listWebhooks as ReturnType<typeof vi.fn>).mockResolvedValueOnce([wh1, wh2]);

    const store = useWebhooksStore();
    const result = await store.loadWebhooks();

    expect(store.webhooks).toHaveLength(2);
    expect(store.webhooks[0].id).toBe('wh1');
    expect(store.loading).toBe(false);
    expect(result).toHaveLength(2);
  });

  it('loadWebhooks sets loading to false even on error', async () => {
    const { listWebhooks } = await import('../../features/webhooks/api');
    (listWebhooks as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network'));

    const store = useWebhooksStore();
    await expect(store.loadWebhooks()).rejects.toThrow('network');
    expect(store.loading).toBe(false);
  });

  it('addWebhook prepends created webhook to list', async () => {
    const { createWebhook } = await import('../../features/webhooks/api');
    const created = mockWebhook({ id: 'wh-new', name: 'New Webhook', url: 'https://new.example.com/hook' });
    (createWebhook as ReturnType<typeof vi.fn>).mockResolvedValueOnce(created);

    const store = useWebhooksStore();
    store.webhooks = [mockWebhook({ id: 'wh1' })] as any;

    const result = await store.addWebhook({
      name: 'New Webhook',
      url: 'https://new.example.com/hook',
    });

    expect(result.id).toBe('wh-new');
    expect(store.webhooks[0].id).toBe('wh-new');
    expect(store.webhooks).toHaveLength(2);
  });

  it('saveWebhook updates matching webhook in list', async () => {
    const { updateWebhook } = await import('../../features/webhooks/api');
    const updated = mockWebhook({ id: 'wh1', name: 'Updated Webhook' });
    (updateWebhook as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

    const store = useWebhooksStore();
    store.webhooks = [mockWebhook({ id: 'wh1', name: 'Old Webhook' }), mockWebhook({ id: 'wh2' })] as any;

    const result = await store.saveWebhook('wh1', { name: 'Updated Webhook' });

    expect(result.name).toBe('Updated Webhook');
    expect(store.webhooks[0].name).toBe('Updated Webhook');
    expect(store.webhooks[1].id).toBe('wh2');
  });

  it('removeWebhook deletes webhook from list', async () => {
    const { deleteWebhook } = await import('../../features/webhooks/api');

    const store = useWebhooksStore();
    store.webhooks = [mockWebhook({ id: 'wh1' }), mockWebhook({ id: 'wh2' })] as any;

    await store.removeWebhook('wh1');

    expect(deleteWebhook).toHaveBeenCalledWith('wh1');
    expect(store.webhooks).toHaveLength(1);
    expect(store.webhooks[0].id).toBe('wh2');
  });

  it('addWebhook deduplicates by id', async () => {
    const { createWebhook } = await import('../../features/webhooks/api');
    const created = mockWebhook({ id: 'wh1', name: 'Recreated' });
    (createWebhook as ReturnType<typeof vi.fn>).mockResolvedValueOnce(created);

    const store = useWebhooksStore();
    store.webhooks = [mockWebhook({ id: 'wh1', name: 'Old' })] as any;

    await store.addWebhook({ name: 'Recreated', url: 'https://example.com/hook' });

    expect(store.webhooks).toHaveLength(1);
    expect(store.webhooks[0].name).toBe('Recreated');
  });

  it('removeWebhook on last item leaves empty list', async () => {
    const store = useWebhooksStore();
    store.webhooks = [mockWebhook({ id: 'wh1' })] as any;

    await store.removeWebhook('wh1');

    expect(store.webhooks).toEqual([]);
  });
});
