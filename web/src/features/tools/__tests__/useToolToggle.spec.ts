import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useToolToggle } from '../useToolToggle';
import type { Tool } from '../types';

const notify = vi.fn();
const confirmCalls = vi.hoisted(() => ({
  opts: null as Record<string, unknown> | null,
  resolve: null as null | ((value: boolean) => void),
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify }),
}));

// 确认框改走 useAppConfirm 单例：这里捕获入参并手动 resolve。
vi.mock('../../../composables/useAppConfirm', () => ({
  confirm: (opts: Record<string, unknown>) => {
    confirmCalls.opts = opts;
    return new Promise<boolean>((resolve) => {
      confirmCalls.resolve = resolve;
    });
  },
}));

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>();
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) };
});

const toggle = vi.fn();
const remove = vi.fn();

vi.mock('../../../stores/tools', () => ({
  useToolsStore: () => ({ toggle, remove }),
}));

function makeTool(overrides: Partial<Tool> = {}): Tool {
  return {
    id: 't1',
    key: 'shell_exec',
    display_name: 'Shell Exec',
    enabled: false,
    risk_level: 'low',
    readonly: false,
    ...overrides,
  } as Tool;
}

describe('useToolToggle', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    confirmCalls.opts = null;
    confirmCalls.resolve = null;
  });

  it('低风险工具直接切换，不弹确认框，不带确认意图', async () => {
    const onChanged = vi.fn();
    const { toggleEnabled } = useToolToggle(onChanged);
    await toggleEnabled(makeTool(), true);

    expect(confirmCalls.opts).toBeNull();
    expect(toggle).toHaveBeenCalledWith('t1', true);
    expect(onChanged).toHaveBeenCalled();
  });

  it('id 缺失时回退用 key 调用 toggle', async () => {
    const { toggleEnabled } = useToolToggle(vi.fn());
    await toggleEnabled(makeTool({ id: '' }), true);
    expect(toggle).toHaveBeenCalledWith('shell_exec', true);
  });

  it('高风险工具启用先弹确认框，确认前不调用 store', async () => {
    const onChanged = vi.fn();
    const { toggleEnabled } = useToolToggle(onChanged);
    void toggleEnabled(makeTool({ risk_level: 'high' }), true);
    await Promise.resolve();

    expect(confirmCalls.opts).not.toBeNull();
    expect(confirmCalls.opts?.tone).toBe('danger');
    expect(toggle).not.toHaveBeenCalled();
    expect(onChanged).not.toHaveBeenCalled();
  });

  it('高风险确认后携带 I_UNDERSTAND_RISK 调用 toggle', async () => {
    const onChanged = vi.fn();
    const { toggleEnabled, busyId } = useToolToggle(onChanged);
    void toggleEnabled(makeTool({ risk_level: 'high' }), true);
    await Promise.resolve();
    confirmCalls.resolve!(true);

    await vi.waitFor(() => expect(toggle).toHaveBeenCalledWith('t1', true, 'I_UNDERSTAND_RISK'));
    expect(onChanged).toHaveBeenCalled();
    expect(busyId.value).toBe('');
  });

  it('critical 风险等级同样走确认流程', async () => {
    const { toggleEnabled } = useToolToggle(vi.fn());
    void toggleEnabled(makeTool({ risk_level: 'critical' }), true);
    await Promise.resolve();

    expect(confirmCalls.opts).not.toBeNull();
    confirmCalls.resolve!(true);
    await vi.waitFor(() => expect(toggle).toHaveBeenCalledWith('t1', true, 'I_UNDERSTAND_RISK'));
  });

  it('高风险确认框取消则不切换', async () => {
    const { toggleEnabled } = useToolToggle(vi.fn());
    void toggleEnabled(makeTool({ risk_level: 'high' }), true);
    await Promise.resolve();
    confirmCalls.resolve!(false);

    await Promise.resolve();
    expect(toggle).not.toHaveBeenCalled();
  });

  it('禁用高风险工具不弹确认框（仅启用需要确认）', async () => {
    const { toggleEnabled } = useToolToggle(vi.fn());
    await toggleEnabled(makeTool({ risk_level: 'high', enabled: true }), false);

    expect(confirmCalls.opts).toBeNull();
    expect(toggle).toHaveBeenCalledWith('t1', false);
  });

  it('切换失败弹错误通知并复位 busyId', async () => {
    toggle.mockRejectedValueOnce(new Error('network down'));
    const { toggleEnabled, busyId } = useToolToggle(vi.fn());
    await toggleEnabled(makeTool(), true);

    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    expect(busyId.value).toBe('');
  });

  it('readonly 工具删除被拦截：仅警告、不弹框、不执行', () => {
    const { removeTool } = useToolToggle(vi.fn());
    void removeTool(makeTool({ readonly: true }));

    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'warning' }));
    expect(confirmCalls.opts).toBeNull();
    expect(remove).not.toHaveBeenCalled();
  });

  it('非 readonly 删除需确认后执行 remove', async () => {
    const onChanged = vi.fn();
    const { removeTool, busyId } = useToolToggle(onChanged);
    void removeTool(makeTool());
    await Promise.resolve();

    expect(confirmCalls.opts).not.toBeNull();
    expect(confirmCalls.opts?.tone).toBe('danger');
    expect(remove).not.toHaveBeenCalled();

    confirmCalls.resolve!(true);
    await vi.waitFor(() => expect(remove).toHaveBeenCalledWith('t1'));
    expect(onChanged).toHaveBeenCalled();
    expect(busyId.value).toBe('');
  });
});
