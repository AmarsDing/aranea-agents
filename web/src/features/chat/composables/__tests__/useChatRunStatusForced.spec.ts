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

  it('accepts the new session run while the previous session is still running', async () => {
    const ctrl = useChatRunStatus({ applyAwaitRunStatus: vi.fn() });
    ctrl.onSessionSwitch('A');
    ctrl.applyFromV2RunStatus({ RunID: 'run-A', Status: 'running', Meta: null });
    ctrl.onSessionSwitch('B');
    runtimeStoreMock.fetchRunStatus.mockResolvedValue({ ...makeRs('awaiting_user'), runId: 'run-B' });
    expect(await ctrl.refreshRunStatusForced('B')).toBe('awaiting_user');
    expect(ctrl.runMeta.value?.runId).toBe('run-B');
  });

  it('rejects an old response after A to B to A even though the session ID matches again', async () => {
    const ctrl = useChatRunStatus({ applyAwaitRunStatus: vi.fn() });
    ctrl.onSessionSwitch('A');
    let resolve!: (value: RunStatus) => void;
    runtimeStoreMock.fetchRunStatus.mockReturnValueOnce(
      new Promise<RunStatus>((r) => {
        resolve = r;
      }),
    );
    const old = ctrl.refreshRunStatusForced('A');
    ctrl.onSessionSwitch('B');
    ctrl.onSessionSwitch('A');
    runtimeStoreMock.fetchRunStatus.mockResolvedValueOnce({ ...makeRs('running'), runId: 'new-run-A' });
    expect(await ctrl.refreshRunStatusForced('A')).toBe('running');
    resolve({ ...makeRs('completed'), runId: 'old-run-A' });
    expect(await old).toBeNull();
    expect(ctrl.runMeta.value?.runId).toBe('new-run-A');
    expect(ctrl.runStatus.value).toBe('running');
  });
});

/**
 * useChatRunStatus 新鲜度守卫契约（D9-19）：
 *  R1 终态粘性 —— 同 runId 已接受 terminal 后拒绝非终态回落（idle 除外）
 *  R2 runId 绑定 —— 跨 runId 仅在上个 run 已 terminal/idle/未知时接受
 *  R3 REST 单调 —— 同 runId 且双方带服务端时间时拒绝更旧快照
 * 复位：forceSetRunStatus 后同 run 的迟到回放帧不得复活旧状态。
 */
describe('useChatRunStatus staleness guard (D9-19)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  function makeGuardRs(status: string, runId: string, updatedAt = ''): RunStatus {
    return { runId, status: status as RunStatusValue, errorMessage: '', updatedAt };
  }

  function wsFrame(status: string, runId: string) {
    return { RunID: runId, Status: status, Meta: null };
  }

  it('① WS terminal(run1) 后迟到 REST running(run1,T_old) → 丢弃且 runStatus 保持 completed', async () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    ctrl.applyFromV2RunStatus(wsFrame('completed', 'run-1'));
    expect(ctrl.runStatus.value).toBe('completed');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(1);

    runtimeStoreMock.fetchRunStatus.mockResolvedValue(makeGuardRs('running', 'run-1', '2026-10-21T00:00:00Z'));
    const applied = await ctrl.refreshRunStatusForced('sess-A');

    expect(applied).toBeNull();
    expect(ctrl.runStatus.value).toBe('completed');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(1);
  });

  it('② WS running(run1) 后 REST completed(run1) → 接受并返回 completed', async () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    ctrl.applyFromV2RunStatus(wsFrame('running', 'run-1'));
    expect(ctrl.runStatus.value).toBe('running');

    runtimeStoreMock.fetchRunStatus.mockResolvedValue(makeGuardRs('completed', 'run-1', '2026-10-21T00:05:00Z'));
    const applied = await ctrl.refreshRunStatusForced('sess-A');

    expect(applied).toBe('completed');
    expect(ctrl.runStatus.value).toBe('completed');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(2);
  });

  it('③ REST running(T2) 后 REST running(T1<T2) → 第二次返回 null（服务端时间单调）', async () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    runtimeStoreMock.fetchRunStatus.mockResolvedValueOnce(makeGuardRs('running', 'run-1', '2026-10-21T02:00:00Z'));
    const first = await ctrl.refreshRunStatusForced('sess-A');
    expect(first).toBe('running');

    runtimeStoreMock.fetchRunStatus.mockResolvedValueOnce(makeGuardRs('running', 'run-1', '2026-10-21T01:00:00Z'));
    const second = await ctrl.refreshRunStatusForced('sess-A');

    expect(second).toBeNull();
    expect(ctrl.runStatus.value).toBe('running');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(1);
  });

  it('④ WS completed(run1) 后 running(run2) → 接受（新 run）', () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    ctrl.applyFromV2RunStatus(wsFrame('completed', 'run-1'));
    ctrl.applyFromV2RunStatus(wsFrame('running', 'run-2'));

    expect(ctrl.runStatus.value).toBe('running');
    expect(ctrl.runMeta.value?.runId).toBe('run-2');
  });

  it('⑤ WS running(run1) 后 running(run2) → 丢弃（上个 run 未终态）', () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    ctrl.applyFromV2RunStatus(wsFrame('running', 'run-1'));
    ctrl.applyFromV2RunStatus(wsFrame('running', 'run-2'));

    expect(ctrl.runStatus.value).toBe('running');
    expect(ctrl.runMeta.value?.runId).toBe('run-1');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(1);
  });

  it('⑥ WS completed(run1) 后 idle 接受（reset 语义）', () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    ctrl.applyFromV2RunStatus(wsFrame('completed', 'run-1'));
    ctrl.applyFromV2RunStatus(wsFrame('idle', 'run-1'));

    expect(ctrl.runStatus.value).toBe('idle');
  });

  it('⑦ WS completed 后 WS running（回放/乱序兜底）→ 丢弃（终态粘性）', () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    ctrl.applyFromV2RunStatus(wsFrame('completed', 'run-1'));
    ctrl.applyFromV2RunStatus(wsFrame('running', 'run-1'));

    expect(ctrl.runStatus.value).toBe('completed');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(1);
  });

  it('⑧ forceSetRunStatus(idle) 后旧 runId 的 terminal 帧不复活旧状态', () => {
    const applyAwaitRunStatus = vi.fn();
    const ctrl = useChatRunStatus({ applyAwaitRunStatus });
    ctrl.onSessionSwitch('sess-A');
    ctrl.applyFromV2RunStatus(wsFrame('completed', 'run-1'));
    ctrl.forceSetRunStatus('idle');
    expect(ctrl.runStatus.value).toBe('idle');

    ctrl.applyFromV2RunStatus(wsFrame('completed', 'run-1'));

    expect(ctrl.runStatus.value).toBe('idle');
    expect(applyAwaitRunStatus).toHaveBeenCalledTimes(1);
  });
});
