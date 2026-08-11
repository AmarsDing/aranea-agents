import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useAgentHooksPanel } from '../useAgentHooksPanel';
import { listHooks, createHook, updateHook, deleteHook } from '../../hooks/api';

const notify = vi.fn();
const dialogOnOk = vi.fn((cb: () => void | Promise<void>) => cb());

vi.mock('quasar', () => ({
  useQuasar: () => ({
    notify,
    dialog: vi.fn(() => ({ onOk: dialogOnOk })),
  }),
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock('../../hooks/api', () => ({
  listHooks: vi.fn().mockResolvedValue([]),
  listHooksPaged: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 }),
  createHook: vi.fn(),
  updateHook: vi.fn(),
  deleteHook: vi.fn().mockResolvedValue(undefined),
}));

vi.mock('../../hooks/deliveries', () => ({
  listHookDeliveries: vi.fn().mockResolvedValue({ items: [], total: 0 }),
}));

type MockHookOverrides = Record<string, unknown>;

function mockHook(overrides: MockHookOverrides = {}) {
  return {
    id: 'h1',
    key: 'test-hook',
    name: 'Test Hook',
    description: '',
    status: 'active',
    enabled: true,
    sort_order: 0,
    config_json: JSON.stringify({
      callback_point: 'before_tool',
      condition: { agent_id: 'a1' },
      action: { type: 'log', log_level: 'info' },
    }),
    metadata_json: '{}',
    created_at: '2025-01-01T00:00:00Z',
    updated_at: '2025-01-01T00:00:00Z',
    ...overrides,
  };
}

function hookWithAgentId(id: string, agentId: string) {
  return mockHook({
    id,
    key: `hook-${id}`,
    config_json: JSON.stringify({
      callback_point: 'before_tool',
      condition: { agent_id: agentId },
      action: { type: 'log' },
    }),
  });
}

describe('useAgentHooksPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    (listHooks as ReturnType<typeof vi.fn>).mockResolvedValue([]);
    (deleteHook as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  });

  it('scopedRows 只保留 condition.agent_id 匹配当前 Agent ID 或 Key 的行', async () => {
    (listHooks as ReturnType<typeof vi.fn>).mockResolvedValue([
      hookWithAgentId('h1', 'a1'),
      hookWithAgentId('h2', 'agent-key'),
      hookWithAgentId('h3', 'other-agent'),
      hookWithAgentId('h4', ''),
    ]);
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();

    expect(panel.scopedRows.value.map((r) => r.id)).toEqual(['h1', 'h2']);
  });

  it('globalRows 只保留 condition.agent_id 为空的行（对本 Agent 生效的全局规则）', async () => {
    (listHooks as ReturnType<typeof vi.fn>).mockResolvedValue([
      hookWithAgentId('h1', 'a1'),
      hookWithAgentId('h4', ''),
      hookWithAgentId('h5', '  '),
    ]);
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();

    expect(panel.globalRows.value.map((r) => r.id)).toEqual(['h4', 'h5']);
  });

  it('createScopedHook 强制覆盖 condition.agent_id 为当前 Agent（防止清空后创建出全局规则）', async () => {
    const created = hookWithAgentId('h-new', 'a1');
    (createHook as ReturnType<typeof vi.fn>).mockResolvedValue(created);
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();

    panel.draftRule.value.condition.agent_id = '';
    await panel.createScopedHook();

    expect(createHook).toHaveBeenCalledTimes(1);
    const payload = (createHook as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(payload.rule.condition.agent_id).toBe('a1');
  });

  it('createScopedHook 使用用户输入的名称与启用状态；名称留空时回退默认名', async () => {
    (createHook as ReturnType<typeof vi.fn>).mockImplementation((input) =>
      Promise.resolve(mockHook({ id: 'h-new', key: input.key, name: input.name, enabled: input.enabled })),
    );
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();

    panel.draftName.value = '  审计 Webhook  ';
    panel.draftEnabled.value = false;
    await panel.createScopedHook();

    let payload = (createHook as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(payload.name).toBe('审计 Webhook');
    expect(payload.enabled).toBe(false);

    panel.draftName.value = '   ';
    await panel.createScopedHook();
    payload = (createHook as ReturnType<typeof vi.fn>).mock.calls[1][0];
    expect(payload.name).toBe('hooksPage.agentPanel.defaultHookName');
  });

  it('createScopedHook 创建成功后本地插入行，不再全量重拉', async () => {
    const created = hookWithAgentId('h-new', 'a1');
    (createHook as ReturnType<typeof vi.fn>).mockResolvedValue(created);
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();
    const loadsAfterInit = (listHooks as ReturnType<typeof vi.fn>).mock.calls.length;

    await panel.createScopedHook();

    expect(panel.scopedRows.value.some((r) => r.id === 'h-new')).toBe(true);
    expect((listHooks as ReturnType<typeof vi.fn>).mock.calls.length).toBe(loadsAfterInit);
  });

  it('saveEdit 提交名称/启用/排序/规则并本地替换行', async () => {
    const row = hookWithAgentId('h1', 'a1');
    (listHooks as ReturnType<typeof vi.fn>).mockResolvedValue([row]);
    const updated = { ...row, name: '改名', enabled: false, sort_order: 5 };
    (updateHook as ReturnType<typeof vi.fn>).mockResolvedValue(updated);
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();

    panel.openEdit(row);
    panel.editName.value = '改名';
    panel.editEnabled.value = false;
    panel.editSort.value = 5;
    await panel.saveEdit();

    const payload = (updateHook as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(payload.name).toBe('改名');
    expect(payload.enabled).toBe(false);
    expect(payload.sort_order).toBe(5);
    expect(payload.rule).toBeDefined();
    expect(panel.scopedRows.value[0].name).toBe('改名');
    expect(panel.editOpen.value).toBe(false);
  });

  it('saveEdit 强制保持 condition.agent_id 为当前 Agent', async () => {
    const row = hookWithAgentId('h1', 'a1');
    (listHooks as ReturnType<typeof vi.fn>).mockResolvedValue([row]);
    (updateHook as ReturnType<typeof vi.fn>).mockImplementation((id, patch) =>
      Promise.resolve({ ...row, config_json: JSON.stringify(patch.rule) }),
    );
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();

    panel.openEdit(row);
    panel.editRule.value.condition.agent_id = 'other-agent';
    await panel.saveEdit();

    const payload = (updateHook as ReturnType<typeof vi.fn>).mock.calls[0][1];
    expect(payload.rule.condition.agent_id).toBe('a1');
  });

  it('toggleEnabled 只 patch enabled 并本地更新行', async () => {
    const row = hookWithAgentId('h1', 'a1');
    (listHooks as ReturnType<typeof vi.fn>).mockResolvedValue([row]);
    (updateHook as ReturnType<typeof vi.fn>).mockResolvedValue({ ...row, enabled: false });
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();

    await panel.toggleEnabled(row, false);

    expect(updateHook).toHaveBeenCalledWith('h1', { enabled: false });
    expect(panel.scopedRows.value[0].enabled).toBe(false);
  });

  it('confirmRemove 经确认对话框后删除并本地移除行', async () => {
    const row = hookWithAgentId('h1', 'a1');
    (listHooks as ReturnType<typeof vi.fn>).mockResolvedValue([row, hookWithAgentId('h2', 'a1')]);
    const panel = useAgentHooksPanel(
      () => 'a1',
      () => 'agent-key',
    );
    await panel.reload();

    await panel.confirmRemove(row);

    expect(dialogOnOk).toHaveBeenCalled();
    expect(deleteHook).toHaveBeenCalledWith('h1');
    expect(panel.scopedRows.value.map((r) => r.id)).toEqual(['h2']);
  });

  it('agentId 与 agentKey 均为空时创建 key 以前缀 hook 兜底', async () => {
    (createHook as ReturnType<typeof vi.fn>).mockImplementation((input) =>
      Promise.resolve(mockHook({ id: 'h-new', key: input.key })),
    );
    const panel = useAgentHooksPanel(
      () => '',
      () => '',
    );
    await panel.reload();

    await panel.createScopedHook();

    const payload = (createHook as ReturnType<typeof vi.fn>).mock.calls[0][0];
    expect(payload.key.startsWith('hook-hook-')).toBe(true);
  });
});
