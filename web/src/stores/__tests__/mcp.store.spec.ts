import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useMcpStore } from '../mcp';

vi.mock('../../features/mcp/api', () => ({
  listMcpServers: vi.fn().mockResolvedValue([]),
  listMcpServersPaged: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 }),
  createMcpServer: vi.fn(),
  updateMcpServer: vi.fn(),
  deleteMcpServer: vi.fn().mockResolvedValue(undefined),
  testMcpServer: vi.fn(),
  validateMcpServer: vi.fn(),
  listMcpUserCredentials: vi.fn().mockResolvedValue([]),
  upsertMcpUserCredential: vi.fn(),
  deleteMcpUserCredential: vi.fn().mockResolvedValue(undefined),
}));

const mockServer = (overrides: Record<string, unknown> = {}): any => ({
  id: 'mcp-1',
  resource: 'mcp-servers',
  key: 'test-server',
  name: 'Test Server',
  description: '',
  status: 'active',
  enabled: true,
  sort_order: 0,
  parent_id: '',
  level: '',
  agent_id: '',
  provider: '',
  model: '',
  is_system: false,
  config_json: '{}',
  metadata_json: '{}',
  created_at: '2025-01-01T00:00:00Z',
  updated_at: '2025-01-01T00:00:00Z',
  deleted_at: '',
  ...overrides,
});

describe('useMcpStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('initialises with empty servers and loading false', () => {
    const store = useMcpStore();
    expect(store.servers).toEqual([]);
    expect(store.loading).toBe(false);
  });

  it('loadServers populates servers and sets loading to false', async () => {
    const { listMcpServers } = await import('../../features/mcp/api');
    const s1 = mockServer({ id: 'mcp-1' });
    const s2 = mockServer({ id: 'mcp-2' });
    (listMcpServers as ReturnType<typeof vi.fn>).mockResolvedValueOnce([s1, s2]);

    const store = useMcpStore();
    expect(store.loading).toBe(false);

    await store.loadServers();

    expect(store.servers).toHaveLength(2);
    expect(store.servers[0].id).toBe('mcp-1');
    expect(store.loading).toBe(false);
  });

  it('loadServers sets loading to false even on error', async () => {
    const { listMcpServers } = await import('../../features/mcp/api');
    (listMcpServers as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network'));

    const store = useMcpStore();
    await expect(store.loadServers()).rejects.toThrow('network');
    expect(store.loading).toBe(false);
  });

  it('addServer appends created server to list', async () => {
    const { createMcpServer } = await import('../../features/mcp/api');
    const created = mockServer({ id: 'mcp-new', key: 'new-server', name: 'New Server' });
    (createMcpServer as ReturnType<typeof vi.fn>).mockResolvedValueOnce(created);

    const store = useMcpStore();
    store.servers = [mockServer({ id: 'mcp-1' })] as any;

    const result = await store.addServer({ key: 'new-server', name: 'New Server', config_json: '{}' });

    expect(result.id).toBe('mcp-new');
    expect(store.servers).toHaveLength(2);
    expect(store.servers[1].id).toBe('mcp-new');
  });

  it('editServer updates matching server in list', async () => {
    const { updateMcpServer } = await import('../../features/mcp/api');
    const updated = mockServer({ id: 'mcp-1', name: 'Updated Server' });
    (updateMcpServer as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

    const store = useMcpStore();
    store.servers = [mockServer({ id: 'mcp-1', name: 'Old Server' }), mockServer({ id: 'mcp-2' })] as any;

    const result = await store.editServer('mcp-1', { name: 'Updated Server' });

    expect(result.name).toBe('Updated Server');
    expect(store.servers[0].name).toBe('Updated Server');
    expect(store.servers[1].id).toBe('mcp-2');
  });

  it('removeServer deletes server from list', async () => {
    const { deleteMcpServer } = await import('../../features/mcp/api');

    const store = useMcpStore();
    store.servers = [mockServer({ id: 'mcp-1' }), mockServer({ id: 'mcp-2' })] as any;

    await store.removeServer('mcp-1');

    expect(deleteMcpServer).toHaveBeenCalledWith('mcp-1');
    expect(store.servers).toHaveLength(1);
    expect(store.servers[0].id).toBe('mcp-2');
  });

  it('test delegates to testMcpServer', async () => {
    const { testMcpServer } = await import('../../features/mcp/api');
    const testResult = { ok: true, status: 'connected', message: 'OK', details: undefined };
    (testMcpServer as ReturnType<typeof vi.fn>).mockResolvedValueOnce(testResult);

    const store = useMcpStore();
    const result = await store.test('mcp-1');

    expect(testMcpServer).toHaveBeenCalledWith('mcp-1');
    expect(result.ok).toBe(true);
  });

  it('validate delegates to validateMcpServer', async () => {
    const { validateMcpServer } = await import('../../features/mcp/api');
    const validateResult = { ok: true, status: 'valid', message: 'OK', details: undefined };
    (validateMcpServer as ReturnType<typeof vi.fn>).mockResolvedValueOnce(validateResult);

    const store = useMcpStore();
    const result = await store.validate(true, '{"command":"node"}');

    expect(validateMcpServer).toHaveBeenCalledWith(true, '{"command":"node"}');
    expect(result.ok).toBe(true);
  });

  it('fetchUserCredentials delegates to listMcpUserCredentials', async () => {
    const { listMcpUserCredentials } = await import('../../features/mcp/api');
    const creds = [{ id: 'c1', mcp_server_id: 'mcp-1', user_id: 'u1', credential_key: 'key1' }];
    (listMcpUserCredentials as ReturnType<typeof vi.fn>).mockResolvedValueOnce(creds);

    const store = useMcpStore();
    const result = await store.fetchUserCredentials('mcp-1', 'u1');

    expect(listMcpUserCredentials).toHaveBeenCalledWith('mcp-1', 'u1');
    expect(result).toHaveLength(1);
  });

  it('saveUserCredential delegates to upsertMcpUserCredential', async () => {
    const { upsertMcpUserCredential } = await import('../../features/mcp/api');
    const saved = { id: 'c1', mcp_server_id: 'mcp-1', user_id: 'u1', credential_key: 'key1' };
    (upsertMcpUserCredential as ReturnType<typeof vi.fn>).mockResolvedValueOnce(saved);

    const store = useMcpStore();
    const result = await store.saveUserCredential('mcp-1', 'u1', { credential_key: 'key1', secret: 's' });

    expect(upsertMcpUserCredential).toHaveBeenCalledWith('mcp-1', 'u1', { credential_key: 'key1', secret: 's' });
    expect(result.credential_key).toBe('key1');
  });

  it('removeUserCredential delegates to deleteMcpUserCredential', async () => {
    const { deleteMcpUserCredential } = await import('../../features/mcp/api');

    const store = useMcpStore();
    await store.removeUserCredential('mcp-1', 'u1', 'key1');

    expect(deleteMcpUserCredential).toHaveBeenCalledWith('mcp-1', 'u1', 'key1');
  });
});
