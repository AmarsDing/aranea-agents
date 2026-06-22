import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useToolsStore } from '../tools';

vi.mock('../../features/tools/api', () => ({
  listTools: vi.fn().mockResolvedValue({
    items: [],
    total: 0,
    summary: { total_tools: 0, enabled_tools: 0, high_risk_enabled: 0, calls_24h: 0, failure_rate_24h: 0 },
  }),
  getTool: vi.fn(),
  createTool: vi.fn(),
  updateTool: vi.fn(),
  deleteTool: vi.fn().mockResolvedValue(undefined),
  toggleToolEnabled: vi.fn(),
  updateToolConfig: vi.fn(),
  getAgentEffectiveTools: vi.fn(),
  listToolAgentOverrides: vi.fn().mockResolvedValue([]),
  listToolAgentOverridesByAgent: vi.fn().mockResolvedValue([]),
  upsertToolAgentOverride: vi.fn(),
  deleteToolAgentOverride: vi.fn().mockResolvedValue(undefined),
  listToolRunsForTool: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listToolRuns: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  listToolInvocationAudits: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  testTool: vi.fn(),
}));

describe('useToolsStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('loadTools populates tools, total, summary and sets loading to false', async () => {
    const { listTools } = await import('../../features/tools/api');
    const mockSummary = {
      total_tools: 5,
      enabled_tools: 3,
      high_risk_enabled: 1,
      calls_24h: 100,
      failure_rate_24h: 0.02,
    };
    const mockTools = [
      { id: 't1', display_name: 'Tool 1', enabled: true },
      { id: 't2', display_name: 'Tool 2', enabled: false },
    ];
    (listTools as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      items: mockTools,
      total: 2,
      summary: mockSummary,
    });

    const store = useToolsStore();
    expect(store.loading).toBe(false);

    const result = await store.loadTools();

    expect(store.tools).toHaveLength(2);
    expect(store.tools[0].id).toBe('t1');
    expect(store.total).toBe(2);
    expect(store.summary).toEqual(mockSummary);
    expect(store.loading).toBe(false);
    expect(result.items).toHaveLength(2);
  });

  it('loadTools sets loading to false even on error', async () => {
    const { listTools } = await import('../../features/tools/api');
    (listTools as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error('network'));

    const store = useToolsStore();
    await expect(store.loadTools()).rejects.toThrow('network');
    expect(store.loading).toBe(false);
  });

  it('addTool prepends created tool to the list', async () => {
    const { createTool } = await import('../../features/tools/api');
    const created = { id: 't-new', display_name: 'New Tool', enabled: true };
    (createTool as ReturnType<typeof vi.fn>).mockResolvedValueOnce(created);

    const store = useToolsStore();
    store.tools = [{ id: 't1', display_name: 'Existing', enabled: true }] as any;

    const result = await store.addTool({ display_name: 'New Tool' } as any);

    expect(result.id).toBe('t-new');
    expect(store.tools[0].id).toBe('t-new');
    expect(store.tools).toHaveLength(2);
  });

  it('editTool updates the matching item in the list', async () => {
    const { updateTool } = await import('../../features/tools/api');
    const updated = { id: 't1', display_name: 'Updated Tool', enabled: true };
    (updateTool as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

    const store = useToolsStore();
    store.tools = [
      { id: 't1', display_name: 'Old Tool', enabled: false },
      { id: 't2', display_name: 'Other', enabled: true },
    ] as any;

    const result = await store.editTool('t1', { display_name: 'Updated Tool' } as any);

    expect(result.display_name).toBe('Updated Tool');
    expect(store.tools[0].display_name).toBe('Updated Tool');
    expect(store.tools[1].display_name).toBe('Other');
  });

  it('editTool also updates activeTool when it matches', async () => {
    const { updateTool } = await import('../../features/tools/api');
    const updated = { id: 't1', display_name: 'Updated', enabled: true };
    (updateTool as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

    const store = useToolsStore();
    store.tools = [{ id: 't1', display_name: 'Old', enabled: false }] as any;
    store.activeTool = { id: 't1', display_name: 'Old', enabled: false } as any;

    await store.editTool('t1', { display_name: 'Updated' } as any);

    expect(store.activeTool?.display_name).toBe('Updated');
  });

  it('remove deletes tool from list', async () => {
    const store = useToolsStore();
    store.tools = [
      { id: 't1', display_name: 'Tool 1', enabled: true },
      { id: 't2', display_name: 'Tool 2', enabled: false },
    ] as any;

    await store.remove('t1');

    expect(store.tools).toHaveLength(1);
    expect(store.tools[0].id).toBe('t2');
  });

  it('remove clears activeTool when it matches', async () => {
    const store = useToolsStore();
    store.tools = [{ id: 't1', display_name: 'Tool 1', enabled: true }] as any;
    store.activeTool = { id: 't1', display_name: 'Tool 1', enabled: true } as any;

    await store.remove('t1');

    expect(store.tools).toHaveLength(0);
    expect(store.activeTool).toBeNull();
  });

  it('toggle updates enabled state in the list', async () => {
    const { toggleToolEnabled } = await import('../../features/tools/api');
    const updated = { id: 't1', display_name: 'Tool 1', enabled: true };
    (toggleToolEnabled as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);

    const store = useToolsStore();
    store.tools = [{ id: 't1', display_name: 'Tool 1', enabled: false }] as any;

    const result = await store.toggle('t1', true);

    expect(result.enabled).toBe(true);
    expect(store.tools[0].enabled).toBe(true);
  });
});
