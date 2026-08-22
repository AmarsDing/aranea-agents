import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { ref } from 'vue';
import { useAgentToolOverrides } from '../useAgentToolOverrides';
import type { AgentEffectiveTools, ToolAgentOverride } from '../../tools/types';

const notify = vi.fn();

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify }),
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

const fetchEffectiveTools = vi.fn();
const fetchOverridesByAgent = vi.fn();
const fetchCatalog = vi.fn();
const saveOverride = vi.fn();
const removeOverride = vi.fn();

vi.mock('../../../stores/tools', () => ({
  useToolsStore: () => ({
    fetchEffectiveTools,
    fetchOverridesByAgent,
    fetchCatalog,
    saveOverride,
    removeOverride,
  }),
}));

function effectiveItem(toolKey: string, overrides: Record<string, unknown> = {}) {
  return {
    tool_key: toolKey,
    display_name: overrides.display_name ?? toolKey,
    category: overrides.category ?? 'filesystem',
    source: 'builtin',
    enabled: true,
    effective_state: overrides.effective_state ?? 'allowed',
    reason: overrides.reason ?? '',
  };
}

function effective(tools: ReturnType<typeof effectiveItem>[]): AgentEffectiveTools {
  return { tools_enabled: true, profile: 'full', allow: [], deny: [], items: tools };
}

function overrideRow(toolKey: string, overrides: Partial<ToolAgentOverride> = {}): ToolAgentOverride {
  return {
    id: `ov-${toolKey}`,
    tool_id: toolKey,
    tool_key: toolKey,
    agent_id: 'a1',
    enabled: true,
    mode: 'allow',
    config_override_json: '{}',
    requires_confirmation: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

async function setup(tools: ReturnType<typeof effectiveItem>[], overrides: ToolAgentOverride[] = []) {
  fetchEffectiveTools.mockResolvedValue(effective(tools));
  fetchOverridesByAgent.mockResolvedValue(overrides);
  fetchCatalog.mockResolvedValue({ items: [], page: 1, page_size: 500, total: 0 });
  const panel = useAgentToolOverrides(ref('a1'));
  await panel.reload();
  return panel;
}

describe('useAgentToolOverrides 分组与过滤', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
  });

  it('groupedRows 按 category 分组并统计已覆盖数，空 category 归入 custom', async () => {
    const panel = await setup(
      [
        effectiveItem('read_file', { category: 'filesystem' }),
        effectiveItem('browser', { category: 'web' }),
        effectiveItem('skill_search', { category: 'skill' }),
        effectiveItem('no_cat', { category: '' }),
      ],
      [overrideRow('browser'), overrideRow('read_file')],
    );

    const groups = panel.groupedRows.value;
    expect(groups.map((g) => g.category)).toEqual(['custom', 'filesystem', 'skill', 'web']);
    const byCat = Object.fromEntries(groups.map((g) => [g.category, g]));
    expect(byCat.filesystem.rows.map((r) => r.tool_key)).toEqual(['read_file']);
    expect(byCat.web.overriddenCount).toBe(1);
    expect(byCat.skill.overriddenCount).toBe(0);
  });

  it('搜索匹配 tool_key 与 display_name（大小写不敏感），空组被丢弃', async () => {
    const panel = await setup([
      effectiveItem('read_file', { display_name: '读取文件', category: 'filesystem' }),
      effectiveItem('browser', { display_name: '浏览器自动化', category: 'web' }),
    ]);

    panel.search.value = 'READ';
    expect(panel.groupedRows.value).toHaveLength(1);
    expect(panel.groupedRows.value[0].rows[0].tool_key).toBe('read_file');

    panel.search.value = '浏览器';
    expect(panel.groupedRows.value[0].rows[0].tool_key).toBe('browser');
  });

  it('stateFilter / groupFilter / onlyOverridden 组合过滤', async () => {
    const panel = await setup(
      [
        effectiveItem('a', { category: 'filesystem', effective_state: 'allowed' }),
        effectiveItem('b', { category: 'filesystem', effective_state: 'denied' }),
        effectiveItem('c', { category: 'web', effective_state: 'denied' }),
      ],
      [overrideRow('b')],
    );

    panel.stateFilter.value = 'denied';
    expect(panel.groupedRows.value.flatMap((g) => g.rows).map((r) => r.tool_key)).toEqual(['b', 'c']);

    panel.groupFilter.value = 'web';
    expect(panel.groupedRows.value.flatMap((g) => g.rows).map((r) => r.tool_key)).toEqual(['c']);

    panel.groupFilter.value = '';
    panel.onlyOverridden.value = true;
    expect(panel.groupedRows.value.flatMap((g) => g.rows).map((r) => r.tool_key)).toEqual(['b']);
  });

  it('overriddenCount 统计有覆盖的行数', async () => {
    const panel = await setup(
      [effectiveItem('a'), effectiveItem('b'), effectiveItem('c')],
      [overrideRow('a'), overrideRow('c')],
    );
    expect(panel.overriddenCount.value).toBe(2);
  });

  it('groupOptions 返回出现过的 category（custom 兜底）', async () => {
    const panel = await setup([effectiveItem('a', { category: 'web' }), effectiveItem('b', { category: '' })]);
    expect(panel.groupOptions.value.map((o) => o.value)).toEqual(['custom', 'web']);
  });
});

describe('useAgentToolOverrides 行内快捷操作', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    saveOverride.mockResolvedValue(undefined);
    removeOverride.mockResolvedValue(undefined);
  });

  it('quickSetMode(allow)：无覆盖行 upsert 默认 confirm=false / 空 JSON', async () => {
    const panel = await setup([effectiveItem('a')]);
    const row = panel.rows.value[0];
    await panel.quickSetMode(row, 'allow');
    expect(saveOverride).toHaveBeenCalledWith({
      tool_id: 'a',
      agent_id: 'a1',
      enabled: true,
      mode: 'allow',
      requires_confirmation: false,
      config_override_json: '{}',
    });
    expect(removeOverride).not.toHaveBeenCalled();
  });

  it('quickSetMode(deny)：已有覆盖行保留 confirm 与 JSON 配置', async () => {
    const panel = await setup(
      [effectiveItem('a')],
      [overrideRow('a', { mode: 'allow', requires_confirmation: true, config_override_json: '{"x":1}' })],
    );
    const row = panel.rows.value[0];
    await panel.quickSetMode(row, 'deny');
    expect(saveOverride).toHaveBeenCalledWith(
      expect.objectContaining({ mode: 'deny', requires_confirmation: true, config_override_json: '{"x":1}' }),
    );
  });

  it('quickSetMode(inherit)：无覆盖行不调 API', async () => {
    const panel = await setup([effectiveItem('a')]);
    await panel.quickSetMode(panel.rows.value[0], 'inherit');
    expect(saveOverride).not.toHaveBeenCalled();
    expect(removeOverride).not.toHaveBeenCalled();
  });

  it('quickSetMode(inherit)：纯模式覆盖行直接删除覆盖', async () => {
    const panel = await setup([effectiveItem('a')], [overrideRow('a', { mode: 'deny' })]);
    await panel.quickSetMode(panel.rows.value[0], 'inherit');
    expect(removeOverride).toHaveBeenCalledWith('a', 'a1');
    expect(saveOverride).not.toHaveBeenCalled();
  });

  it('quickSetMode(inherit)：含确认/JSON 的覆盖行降级为 inherit 保留配置', async () => {
    const panel = await setup(
      [effectiveItem('a')],
      [overrideRow('a', { mode: 'deny', requires_confirmation: true, config_override_json: '{"x":1}' })],
    );
    await panel.quickSetMode(panel.rows.value[0], 'inherit');
    expect(saveOverride).toHaveBeenCalledWith(
      expect.objectContaining({ mode: 'inherit', requires_confirmation: true, config_override_json: '{"x":1}' }),
    );
    expect(removeOverride).not.toHaveBeenCalled();
  });

  it('quickToggleConfirm：无覆盖行创建 inherit + confirm 覆盖', async () => {
    const panel = await setup([effectiveItem('a')]);
    await panel.quickToggleConfirm(panel.rows.value[0]);
    expect(saveOverride).toHaveBeenCalledWith(
      expect.objectContaining({ mode: 'inherit', requires_confirmation: true }),
    );
  });

  it('quickToggleConfirm：已有覆盖行翻转 confirm 并保留模式', async () => {
    const panel = await setup([effectiveItem('a')], [overrideRow('a', { mode: 'deny', requires_confirmation: true })]);
    await panel.quickToggleConfirm(panel.rows.value[0]);
    expect(saveOverride).toHaveBeenCalledWith(
      expect.objectContaining({ mode: 'deny', requires_confirmation: false }),
    );
  });

  it('快捷操作期间 pendingKeys 标记行级加载，结束后清除', async () => {
    let resolveSave: (() => void) | undefined;
    saveOverride.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          resolveSave = resolve;
        }),
    );
    const panel = await setup([effectiveItem('a')]);
    const p = panel.quickSetMode(panel.rows.value[0], 'allow');
    expect(panel.pendingKeys.value.has('a')).toBe(true);
    resolveSave?.();
    await p;
    expect(panel.pendingKeys.value.has('a')).toBe(false);
  });

  it('快捷操作失败时提示错误并清除 pending 标记', async () => {
    saveOverride.mockRejectedValue(new Error('boom'));
    const panel = await setup([effectiveItem('a')]);
    await panel.quickSetMode(panel.rows.value[0], 'allow');
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    expect(panel.pendingKeys.value.has('a')).toBe(false);
  });
});
