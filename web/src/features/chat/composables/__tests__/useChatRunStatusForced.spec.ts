/**
 * useChatRunStatus.refreshRunStatusForced 契约测试（final-review #18/#19）。
 *
 *  ① 响应返回时目标会话仍是当前活跃会话 → 正常写 runStatus/runMeta 并 apply
 *  ② 对账请求在途时用户切换会话 → 旧会话迟到响应整个丢弃（不写状态、不调 apply）
 *  ③ 会话过期守卫复用 hydrate 的锚点（hydrateSessionId），
 *     resetRunStatus 语义仅限 undefined 会话的普通清空路径
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

const runtimeStoreMock = vi.hoisted(() => ({ fetchRunStatus: vi.fn() }));
vi.mock('../../../../stores/chat/runtimeStore', () => ({
  useChatRuntimeStore: () => runtimeStoreMock,
}));

import { useChatRunStatus } from '../useChatRunStatus';
import type { RunStatus, RunStatusValue } from '../../types';

function makeRs(status: RunStatusValue): RunStatus {
  return {
    runId: 'run-1',
    status,
    errorMessage: '',
    updatedAt: '',
  };
}

describe('useChatRunStatus.refreshRunStatusForced', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('applies the response when the target session is still current', async () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    runtimeStoreMock.fetchRunStatus.mockResolvedValue(makeRs('running'));

    const applied = await ctrl.refreshRunStatusForced('sess-A');

    expect(applied).toBe('running');
    expect(ctrl.runStatus.value).toBe('running');
    expect(ctrl.runMeta.value?.status).toBe('running');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(1);
  });

  it('drops the in-flight response when the user switched sessions mid-request', async () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    // 先落地一次正常对账，让当前状态非 idle，才能证明后续迟到响应被丢弃而非覆盖。
    runtimeStoreMock.fetchRunStatus.mockResolvedValueOnce(makeRs('running'));
    await ctrl.refreshRunStatusForced('sess-A');
    expect(ctrl.runStatus.value).toBe('running');

    let resolveFetch: ((v: RunStatus) => void) | undefined;
    runtimeStoreMock.fetchRunStatus.mockImplementation(
      () =>
        new Promise<RunStatus>((resolve) => {
          resolveFetch = resolve;
        }),
    );
    const pending = ctrl.refreshRunStatusForced('sess-A');
    // 对账在途时切走会话 —— 守卫锚点随 hydrate 迁到 sess-B。
    ctrl.onSessionSwitch('sess-B');
    resolveFetch?.(makeRs('completed'));
    const staleResult = await pending;

    // 旧会话响应整个丢弃：runStatus/runMeta 保持切走前状态，不重复 apply；
    // 返回值 null 表示「未获后端确认」，watchdog 不得据此复位 sending。
    expect(staleResult).toBeNull();
    expect(ctrl.runStatus.value).toBe('running');
    expect(ctrl.runMeta.value?.status).toBe('running');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(1);
  });

  it('keeps local resetRunStatus semantics only for the no-session refresh path', async () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    runtimeStoreMock.fetchRunStatus.mockResolvedValue(makeRs('running'));
    await ctrl.refreshRunStatusForced('sess-A');
    expect(ctrl.runStatus.value).toBe('running');

    const resetResult = await ctrl.refreshRunStatusForced(undefined);

    // 无会话路径：本地 reset 后返回 null（未经后端确认）。
    expect(resetResult).toBeNull();
    expect(ctrl.runStatus.value).toBe('idle');
    expect(ctrl.runMeta.value).toBeNull();
  });
});
