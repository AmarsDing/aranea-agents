/**
 * useChatSender 发送态对账 watchdog 契约测试（final-review #14，配套 #2 接线修复）。
 *
 * 覆盖恢复栈第 4 层（前端自愈）的行为契约：
 *  ① 后端确认终态（idle/completed/failed/cancelled）→ sending 复位 + 恢复提示
 *  ② 网络失败 / reconcile reject → sending 保持，下轮重试
 *  ③ 非终态（running/awaiting_user）→ 绝不打断进行中的 run
 *  ④ 90s 静默阈值之前不触发对账
 *  ⑤ 对账在途时不重入
 *  ⑥ touchRunActivity 刷新活动基线 → 对账窗口顺延（接 WS 事件接线）
 *
 * 计时口径（features/constants/timeouts.ts）：
 *  CHAT_RUN_STALL_CHECK_INTERVAL_MS = 30s（检查节拍）
 *  CHAT_RUN_STALL_NOTIFY_THRESHOLD_MS = 60s（停滞提醒，跟本套件断言无关）
 *  CHAT_RUN_RECONCILE_THRESHOLD_MS = 90s（严格大于才触发），
 *  因此 markSending 基线下首个真正触发对账的节拍是 T+120s。
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { computed, ref, type Ref } from 'vue';

const notify = vi.fn();
vi.mock('quasar', () => ({
  useQuasar: () => ({ notify }),
}));
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
}));
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>();
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
  };
});

const runtimeStoreMock = vi.hoisted(() => ({
  stop: vi.fn(),
  enqueue: vi.fn(async () => ({ accepted: true, queued: true })),
}));
vi.mock('../../../../stores/chat/runtimeStore', () => ({
  useChatRuntimeStore: () => runtimeStoreMock,
}));

const activityStoreMock = vi.hoisted(() => ({
  upsertTask: vi.fn(),
  removeTask: vi.fn(),
  fetchSessionHistory: vi.fn(async () => undefined),
}));
vi.mock('../../../../stores/chat/activityV2Store', () => ({
  useChatActivityStore: () => activityStoreMock,
}));

import { useChatSender, type SenderDeps } from '../useChatSender';

const RECOVERED_MESSAGE = '已同步最新运行状态';

function recoveredNotified(): boolean {
  return notify.mock.calls.some((call) => (call[0] as { message?: string } | undefined)?.message === RECOVERED_MESSAGE);
}

function setup(overrides: Partial<SenderDeps> = {}) {
  const runStatus = ref('running') as Ref<string>;
  const defaultReconcile = vi.fn(async () => {
    runStatus.value = 'completed';
    return 'completed';
  });
  const deps: SenderDeps = {
    appStore: {} as unknown as SenderDeps['appStore'],
    sessionStore: {
      entityKind: 'agent',
      selectedSession: { id: 'sess-1' },
      teamSelectedSessionId: null,
    } as unknown as SenderDeps['sessionStore'],
    messageStore: {
      getMessages: () => [],
      setMessages: vi.fn(),
    } as unknown as SenderDeps['messageStore'],
    inputText: ref(''),
    dialogMode: ref('chat'),
    attachments: ref([]),
    isAwaitingUser: ref(false),
    awaitingRunId: ref(''),
    awaitKind: ref(''),
    runStatus,
    selectedProviderModel: computed(() => undefined),
    ensureChatStream: vi.fn() as unknown as SenderDeps['ensureChatStream'],
    ensureTeamStream: vi.fn() as unknown as SenderDeps['ensureTeamStream'],
    sendChatViaWs: vi.fn(),
    onNewSession: vi.fn(async () => undefined),
    makeSessionTitle: (s: string) => s.slice(0, 22),
    refreshRunStatus: vi.fn(async () => undefined),
    reconcileRunStatus: defaultReconcile,
    setRunStatus: vi.fn(),
    submitAwaitingReply: vi.fn(async () => undefined),
    submitToolConfirm: vi.fn(async () => undefined),
    ...overrides,
  };
  const sender = useChatSender(deps);
  // 返回 deps 实际使用的对账桩（调用方可能以 overrides 覆盖默认实现）。
  const reconcileRunStatus = deps.reconcileRunStatus as ReturnType<typeof vi.fn>;
  return { sender, deps, runStatus, reconcileRunStatus };
}

describe('useChatSender watchdog reconcile', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it.each(['completed', 'failed', 'cancelled', 'idle'])(
    'resets sending and notifies when the backend confirms terminal status %s',
    async (status) => {
      const { sender, runStatus, reconcileRunStatus } = setup({
        reconcileRunStatus: vi.fn(async () => {
          runStatus.value = status;
          return status;
        }),
      });
      sender.markSending('sess-1');
      expect(sender.sending.value).toBe(true);

      await vi.advanceTimersByTimeAsync(120_000);

      expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
      expect(sender.sending.value).toBe(false);
      expect(recoveredNotified()).toBe(true);
    },
  );

  it('keeps sending when reconcile rejects (network failure) and retries next cycle', async () => {
    const { sender, reconcileRunStatus } = setup({
      reconcileRunStatus: vi.fn(async () => {
        throw new Error('network down');
      }),
    });
    sender.markSending('sess-1');

    await vi.advanceTimersByTimeAsync(150_000);

    // T120/T150 两个节拍都发起了对账，网络失败均不复位 sending、不弹恢复提示。
    expect(reconcileRunStatus.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(sender.sending.value).toBe(true);
    expect(recoveredNotified()).toBe(false);
  });

  it.each(['running', 'awaiting_user'])('never interrupts a live run in status %s', async (status) => {
    const { sender, runStatus, reconcileRunStatus } = setup({
      reconcileRunStatus: vi.fn(async () => {
        runStatus.value = status;
        return status;
      }),
    });
    sender.markSending('sess-1');

    await vi.advanceTimersByTimeAsync(240_000);

    // 后端权威值不是终态：无论对账多少次都不得复位 sending。
    expect(reconcileRunStatus.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(sender.sending.value).toBe(true);
    expect(recoveredNotified()).toBe(false);
  });

  it('does not reconcile before the 90s silence threshold', async () => {
    const { sender, reconcileRunStatus } = setup();
    sender.markSending('sess-1');

    // T30/T60 节拍：未达 90s 阈值，不应对账。
    await vi.advanceTimersByTimeAsync(89_999);
    expect(reconcileRunStatus).not.toHaveBeenCalled();

    // 越过阈值后的节拍（T120）才首次触发对账并复位。
    await vi.advanceTimersByTimeAsync(30_001);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
    expect(sender.sending.value).toBe(false);
  });

  it('does not re-enter while a reconcile is in flight', async () => {
    let resolveReconcile: (() => void) | undefined;
    const reconcileRunStatus = vi.fn(
      () =>
        new Promise<string | null>((resolve) => {
          resolveReconcile = () => resolve(null);
        }),
    );
    const { sender } = setup({ reconcileRunStatus });
    sender.markSending('sess-1');

    // T120 首次触发（Promise 挂起不返回），后续节拍必须被防重入守卫挡掉。
    await vi.advanceTimersByTimeAsync(120_000);
    await vi.advanceTimersByTimeAsync(120_000);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);

    resolveReconcile?.();
    await vi.advanceTimersByTimeAsync(0);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
  });

  it('touchRunActivity refreshes the activity baseline and defers reconcile', async () => {
    const { sender, reconcileRunStatus } = setup({
      reconcileRunStatus: vi.fn(async () => null),
    });
    sender.markSending('sess-1');

    await vi.advanceTimersByTimeAsync(80_000);
    // WS run 活动事件到达（#2 接线点）：基线顺延，下一次 90s 窗口从 T80 重新起算。
    sender.touchRunActivity();

    // T90~T165 的节拍：距基线均不足 90s，不触发对账。
    await vi.advanceTimersByTimeAsync(85_000);
    expect(reconcileRunStatus).not.toHaveBeenCalled();

    // 距基线 T80 超过 90s 后的下一个节拍（T180）触发对账。
    await vi.advanceTimersByTimeAsync(35_000);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
    // 未获后端确认（reconcile 返回 null）：不复位 sending。
    expect(sender.sending.value).toBe(true);
  });

  // REG-2（final-review 2026-10-19）：null = 未经后端确认的本地遗留状态
  //（无选中会话 / 响应被会话过期守卫丢弃）——绝不能以本地 runStatus 默认值
  //（如初始 idle）冒充后端确认而复位 sending 并误报「已同步」。
  it('keeps sending when reconcile returns null even if local runStatus defaults to idle', async () => {
    const { sender, runStatus, reconcileRunStatus } = setup({
      reconcileRunStatus: vi.fn(async () => null),
    });
    // 复现回归路径：本地 runStatus 停留在默认值 idle（如初始态），
    // 旧实现会读到它并误判终态复位。
    runStatus.value = 'idle';
    sender.markSending('sess-1');

    await vi.advanceTimersByTimeAsync(150_000);

    expect(reconcileRunStatus.mock.calls.length).toBeGreaterThanOrEqual(2);
    expect(sender.sending.value).toBe(true);
    expect(recoveredNotified()).toBe(false);
  });

  // REG-3（final-review 2026-10-19）：活动基线按发送会话隔离——其他会话/团队流的
  // run 活动不得顺延本会话的 90s 对账窗口。
  it('touchRunActivity from a foreign session does not defer reconcile', async () => {
    const { sender, reconcileRunStatus } = setup({
      reconcileRunStatus: vi.fn(async () => null),
    });
    sender.markSending('sess-1');

    await vi.advanceTimersByTimeAsync(80_000);
    // 其他会话的 run 活动事件：不得顺延本会话（sess-1）的对账窗口。
    sender.touchRunActivity('sess-OTHER');

    // T120 节拍距发送基线（T0）已超 90s——对账照常触发，不被外部活动掩盖。
    await vi.advanceTimersByTimeAsync(40_000);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
  });

  it('touchRunActivity from the sending session defers reconcile', async () => {
    const { sender, reconcileRunStatus } = setup({
      reconcileRunStatus: vi.fn(async () => null),
    });
    sender.markSending('sess-1');

    await vi.advanceTimersByTimeAsync(80_000);
    // 本会话的 run 活动事件：正常顺延，90s 窗口从 T80 重新起算。
    sender.touchRunActivity('sess-1');

    // T165 距基线 T80 不足 90s：不触发对账。
    await vi.advanceTimersByTimeAsync(85_000);
    expect(reconcileRunStatus).not.toHaveBeenCalled();
  });
});

/**
 * 孤儿态自愈对账契约（D9-19）：终态 WS/eWS 帧丢失而 task.completed 已停表时，
 * markSendingDone 若见 runStatus 仍非终态，延迟 10s 单次触发既有 reconcile 通道
 *（复用 watchdogReconciling 重入锁，不发「已同步最新运行状态」通知）。
 * 计时口径：CHAT_RUN_ORPHAN_RECONCILE_DELAY_MS = 10s（features/constants/timeouts.ts）。
 */
describe('useChatSender orphan reconcile (D9-19)', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('⑨ 收表时 runStatus=running → 10s 后恰好一次对账；终态落定后不再触发、无恢复通知', async () => {
    const { sender, reconcileRunStatus } = setup();
    sender.markSending('sess-1');
    sender.markSendingDone();

    await vi.advanceTimersByTimeAsync(9_999);
    expect(reconcileRunStatus).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
    expect(recoveredNotified()).toBe(false);

    // 默认桩已把 runStatus 置为 completed 并返回终态 → 后续推进不再触发。
    await vi.advanceTimersByTimeAsync(60_000);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
    expect(recoveredNotified()).toBe(false);
  });

  it.each(['idle', 'completed', 'failed', 'cancelled'])(
    '⑩ markSendingDone 时 runStatus 已终态（%s）→ 不登记定时器、不对账',
    async (status) => {
      const { sender, runStatus, reconcileRunStatus } = setup();
      runStatus.value = status;
      sender.markSending('sess-1');
      sender.markSendingDone();

      await vi.advanceTimersByTimeAsync(60_000);
      expect(reconcileRunStatus).not.toHaveBeenCalled();
    },
  );

  it('⑪ orphan 定时器挂起时再 markSending → 旧定时器清除、窗口重计', async () => {
    const { sender, reconcileRunStatus } = setup();
    sender.markSending('sess-1');
    sender.markSendingDone();

    await vi.advanceTimersByTimeAsync(5_000);
    // 新一轮发送：旧 orphan 定时器清除；此 10s 内不得对账。
    sender.markSending('sess-1');
    await vi.advanceTimersByTimeAsync(10_000);
    expect(reconcileRunStatus).not.toHaveBeenCalled();

    // 再次收表：窗口重计，10s 后恰好一次。
    sender.markSendingDone();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
  });

  it('⑫ watchdog 对账在途时 orphan 对账被拒入（watchdogReconciling 复用）', async () => {
    let resolveReconcile: (() => void) | undefined;
    const reconcileRunStatus = vi.fn(
      () =>
        new Promise<string | null>((resolve) => {
          resolveReconcile = () => resolve(null);
        }),
    );
    const { sender } = setup({ reconcileRunStatus });
    sender.markSending('sess-1');

    // T120：watchdog 对账触发且挂起（Promise 未决），重入锁持有。
    await vi.advanceTimersByTimeAsync(120_000);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);

    // 发送侧收表（task.completed 兜底路径）→ orphan 定时器登记；
    // 10s 后 orphan 触发但被共用的 watchdogReconciling 锁拒入。
    sender.markSendingDone();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);

    resolveReconcile?.();
    await vi.advanceTimersByTimeAsync(0);
    expect(reconcileRunStatus).toHaveBeenCalledTimes(1);
  });
});
