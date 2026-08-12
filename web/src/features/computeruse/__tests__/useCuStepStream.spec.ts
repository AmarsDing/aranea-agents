// web/src/features/computeruse/__tests__/useCuStepStream.spec.ts
// 75 M1.4 任务 4：computeruse.step MonitorEvent → CuStep 视图模型映射 + 去重排序。
import { describe, it, expect } from 'vitest';
import { cuStepFromMonitorEvent, upsertCuStep, cuSessionIdFromSteps, type CuStep } from '../useCuStepStream';
import type { MonitorEvent } from '../../../realtime/monitorEvent';
import type { Step } from '../../chat/v2Types';

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
