// web/src/features/computeruse/__tests__/useCuStepStream.spec.ts
// 75 M1.4 任务 4：computeruse.step MonitorEvent → CuStep 视图模型映射 + 去重排序。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { defineComponent, ref, type Ref } from 'vue';
import { mount, type VueWrapper } from '@vue/test-utils';
import {
  cuStepFromMonitorEvent,
  upsertCuStep,
  cuSessionIdFromSteps,
  useCuStepStream,
  type CuStep,
} from '../useCuStepStream';
import type { MonitorEvent } from '../../../realtime/monitorEvent';
import type { Step } from '../../chat/v2Types';
import type { UseMonitorStreamOptions } from '../../../realtime/useMonitorStream';
import { createMonitorStream } from '../../../realtime/useMonitorStream';
import { createComputerUseService } from '../../../services/index';

vi.mock('../../../realtime/useMonitorStream', () => ({
  createMonitorStream: vi.fn(),
}));

vi.mock('../../../services/index', () => ({
  createComputerUseService: vi.fn(),
}));

function mkEv(overrides: Partial<MonitorEvent>): MonitorEvent {
  return {
    id: 'ev-1',
    type: 'computeruse.step',
    timestamp: '2026-08-12T00:00:00Z',
    session_id: 'cu-sess-1',
    metadata: {
      step_index: 1,
      target: '保存菜单项',
      path: 'a11y',
      action: 'invoke',
      result: 'ok',
      duration_ms: 42,
      danger: false,
      confirmed_by: 'user-1',
    },
    ...overrides,
  };
}

describe('cuStepFromMonitorEvent', () => {
  it('maps computeruse.step metadata to CuStep', () => {
    const step = cuStepFromMonitorEvent(mkEv({}));
    expect(step).not.toBeNull();
    expect(step).toMatchObject({
      stepIndex: 1,
      target: '保存菜单项',
      path: 'a11y',
      action: 'invoke',
      result: 'ok',
      durationMs: 42,
      danger: false,
      confirmedBy: 'user-1',
      error: '',
    });
  });

  it('returns null for non-computeruse types', () => {
    expect(cuStepFromMonitorEvent(mkEv({ type: 'log' }))).toBeNull();
  });

  it('returns null when metadata missing', () => {
    expect(cuStepFromMonitorEvent(mkEv({ metadata: undefined }))).toBeNull();
  });

  it('carries danger + error for failed danger steps', () => {
    const step = cuStepFromMonitorEvent(
      mkEv({
        metadata: {
          step_index: 3,
          target: '永久删除按钮',
          path: 'vision',
          action: 'click',
          result: 'failed',
          duration_ms: 130,
          danger: true,
          error: 'element not found',
        },
      }),
    );
    expect(step).toMatchObject({ danger: true, error: 'element not found', path: 'vision', result: 'failed' });
  });
});

describe('upsertCuStep', () => {
  const s = (i: number): CuStep => ({
    stepIndex: i,
    target: `t${i}`,
    path: 'a11y',
    action: 'invoke',
    result: 'ok',
    durationMs: 1,
    danger: false,
    confirmedBy: '',
    error: '',
    timestamp: '',
  });

  it('appends and keeps ascending order by step_index', () => {
    let list: CuStep[] = [];
    list = upsertCuStep(list, s(2));
    list = upsertCuStep(list, s(1));
    list = upsertCuStep(list, s(3));
    expect(list.map((x) => x.stepIndex)).toEqual([1, 2, 3]);
  });

  it('dedupes by step_index (later wins)', () => {
    let list: CuStep[] = [s(1)];
    list = upsertCuStep(list, { ...s(1), result: 'failed', error: 'boom' });
    expect(list).toHaveLength(1);
    expect(list[0].result).toBe('failed');
  });
});

// 75 M1.4 任务 4（设计 §3.8 聊天气泡内嵌）：从 turn 的 steps 中提取
// computeruse 会话 ID——任一 computer_use_* 工具的 ToolResult.session_id。
describe('cuSessionIdFromSteps', () => {
  const mkStep = (over: Partial<Step>): Step =>
    ({
      ID: 's1',
      Kind: 'action',
      ToolName: '',
      ToolResult: null,
      ...over,
    }) as Step;

  it('extracts session_id from a computer_use tool result', () => {
    const steps = [
      mkStep({ ID: 'a1', ToolName: 'computer_use_session', ToolResult: { session_id: 'cu-1', status: 'active' } }),
    ];
    expect(cuSessionIdFromSteps(steps)).toBe('cu-1');
  });

  it('prefers the earliest step carrying a session_id', () => {
    const steps = [
      mkStep({ ID: 'a1', ToolName: 'computer_use_launch', ToolResult: { session_id: 'cu-first' } }),
      mkStep({ ID: 'a2', ToolName: 'computer_use_act', ToolResult: { session_id: 'cu-second' } }),
    ];
    expect(cuSessionIdFromSteps(steps)).toBe('cu-first');
  });

  it('ignores non-computeruse tools and results without session_id', () => {
    const steps = [
      mkStep({ ID: 'a1', ToolName: 'web_search', ToolResult: { session_id: 'nope' } }),
      mkStep({ ID: 'a2', ToolName: 'computer_use_act', ToolResult: { result: 'ok' } }),
      mkStep({ ID: 'a3', ToolName: 'computer_use_act', ToolResult: 'string-result' }),
    ];
    expect(cuSessionIdFromSteps(steps)).toBe('');
  });

  it('returns empty for empty/no-match steps', () => {
    expect(cuSessionIdFromSteps([])).toBe('');
    expect(cuSessionIdFromSteps([mkStep({ ID: 'a1', ToolName: 'computer_use_act' })])).toBe('');
  });
});

// 75 review F3：composable 本体集成测试——WS 订阅 / session 过滤 / kill 状态机 / unmount 清理。
describe('useCuStepStream composable', () => {
  let captured: UseMonitorStreamOptions;
  let disconnectSpy: ReturnType<typeof vi.fn>;
  let killMock: ReturnType<typeof vi.fn>;

  function withSetup(sessionId: Ref<string>) {
    let result!: ReturnType<typeof useCuStepStream>;
    const wrapper: VueWrapper = mount(
      defineComponent({
        setup() {
          result = useCuStepStream(sessionId);
          return () => null;
        },
      }),
    );
    return { result, wrapper };
  }

  beforeEach(() => {
    vi.clearAllMocks();
    disconnectSpy = vi.fn();
    killMock = vi.fn().mockResolvedValue({});
    vi.mocked(createMonitorStream).mockImplementation((opts) => {
      captured = opts;
      return {
        connected: ref(true),
        connect: vi.fn(),
        disconnect: disconnectSpy,
        enableLog: vi.fn(),
        subscribe: vi.fn(),
        unsubscribe: vi.fn(),
      };
    });
    vi.mocked(createComputerUseService).mockReturnValue({
      KillComputerUseSession: killMock,
    } as unknown as ReturnType<typeof createComputerUseService>);
  });

  it('appends steps on computeruse.step events for the bound session', () => {
    const { result } = withSetup(ref('cu-sess-1'));
    captured.onMonitorEvent?.(mkEv({ metadata: { step_index: 2, target: 't2', result: 'ok' } }));
    captured.onMonitorEvent?.(mkEv({ metadata: { step_index: 1, target: 't1', result: 'ok' } }));
    expect(result.steps.value.map((s) => s.stepIndex)).toEqual([1, 2]);
  });

  it('ignores events from other sessions', () => {
    const { result } = withSetup(ref('cu-sess-1'));
    captured.onMonitorEvent?.(mkEv({ session_id: 'cu-other' }));
    expect(result.steps.value).toHaveLength(0);
  });

  it('marks killed when a cancelled step event arrives', () => {
    const { result } = withSetup(ref('cu-sess-1'));
    captured.onMonitorEvent?.(mkEv({ metadata: { step_index: 1, result: 'cancelled' } }));
    expect(result.killState.value).toBe('killed');
  });

  it('kill transitions to killed on success and calls the API with session id', async () => {
    const { result } = withSetup(ref('cu-sess-1'));
    await result.kill();
    expect(killMock).toHaveBeenCalledWith({ id: 'cu-sess-1' });
    expect(result.killState.value).toBe('killed');
  });

  it('kill falls back to active on failure and rethrows', async () => {
    killMock.mockRejectedValue(new Error('boom'));
    const { result } = withSetup(ref('cu-sess-1'));
    await expect(result.kill()).rejects.toThrow('boom');
    expect(result.killState.value).toBe('active');
  });

  it('kill is a no-op when session id is empty or already killed', async () => {
    const { result } = withSetup(ref('  '));
    await result.kill();
    expect(killMock).not.toHaveBeenCalled();

    const { result: killed } = withSetup(ref('cu-sess-1'));
    captured.onMonitorEvent?.(mkEv({ metadata: { step_index: 1, result: 'cancelled' } }));
    await killed.kill();
    expect(killMock).not.toHaveBeenCalled();
  });

  it('disconnects the stream on unmount', () => {
    const { wrapper } = withSetup(ref('cu-sess-1'));
    wrapper.unmount();
    expect(disconnectSpy).toHaveBeenCalledTimes(1);
  });
});
