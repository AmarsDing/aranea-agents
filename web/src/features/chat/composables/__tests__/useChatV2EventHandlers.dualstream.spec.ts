/**
 * useChatV2EventHandlers 双流隔离契约测试（2026-10-26 二轮评审 U 桶）。
 *
 * 背景：chat 流 + team 流双 WS 常驻（spirit 面板拓扑）。此前终态帧
 * （task.completed/failed/cancelled）、send_failed、run_status 副作用全部
 * 无视 envelope.session_id——异会话的帧会掐灭本会话发送态（90s 对账
 * watchdog 同死）、把空闲会话 composer 顶成运行态、乐观 Task 卡永悬。
 *
 * 钉的行为：
 *  U-1a 异会话 task.* 终态帧不得复位本会话 sending；同会话/无 sid（旧服务端）保持复位
 *  U-1b 异会话 send_failed 不得复位本会话 sending
 *  U-2  异会话 run_status 跳过 composer/run 侧效应，但 MemberSession 卡补丁
 *       （按 chat_session_id 精确寻址）仍然落
 *  U-3  send_failed 按 meta.request_id 移除乐观 pending-user-* Task 卡；
 *       缺 request_id（旧服务端）不触 removeTask
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ref } from 'vue';

const notifyCreate = vi.hoisted(() => vi.fn());
vi.mock('quasar', () => ({
  Notify: { create: notifyCreate },
}));
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

const sessionStoreMock = vi.hoisted(() => ({
  entityKind: 'agent',
  fetchAndReconcileSession: vi.fn(),
  patchSessionMetricsLocal: vi.fn(),
}));
vi.mock('../../../../stores/chat/sessionStore', () => ({
  useChatSessionStore: () => sessionStoreMock,
}));

const messageStoreMock = vi.hoisted(() => ({
  getMessages: vi.fn(() => []),
  setMessages: vi.fn(),
}));
vi.mock('../../../../stores/chat/messageStore', () => ({
  useChatMessageStore: () => messageStoreMock,
}));

const runtimeStoreMock = vi.hoisted(() => ({ setSkillCatalog: vi.fn() }));
vi.mock('../../../../stores/chat/runtimeStore', () => ({
  useChatRuntimeStore: () => runtimeStoreMock,
}));

const spiritStoreMock = vi.hoisted(() => ({
  upsertTeamFromV2TeamStage: vi.fn(),
  updateMemberFromV2MemberSession: vi.fn(),
  handleSystemNotice: vi.fn(),
}));
vi.mock('../../../../stores/spirit', () => ({
  useSpiritTeamStore: () => spiritStoreMock,
}));

const activityStoreMock = vi.hoisted(() => ({
  noteContractMismatch: vi.fn(),
  removeTask: vi.fn(),
  patchMemberSessionStatusByChatSession: vi.fn(),
}));
vi.mock('../../../../stores/chat/activityV2Store', () => ({
  useChatActivityStore: () => activityStoreMock,
}));

const llmRetryStoreMock = vi.hoisted(() => ({
  clear: vi.fn(),
  clearTransient: vi.fn(),
  noteRetry: vi.fn(),
  noteAlert: vi.fn(),
}));
vi.mock('../../../../stores/chat/llmRetryStore', () => ({
  useLlmRetryStore: () => llmRetryStoreMock,
}));

const eventRouterMock = vi.hoisted(() => ({ dispatch: vi.fn() }));
vi.mock('../useChatEventRouter', () => ({
  useChatEventRouter: () => eventRouterMock,
}));

vi.mock('../../activityToolCall', () => ({
  cancelRunningToolMessages: (msgs: unknown) => msgs,
}));
vi.mock('../../../../stores/sessionMutationBus', () => ({
  emitSessionMutation: vi.fn(),
}));
vi.mock('../../channelWsCursor', () => ({
  noteChannelWsEnvelope: vi.fn(),
}));

import { useChatV2EventHandlers } from '../useChatV2EventHandlers';
import type { V2WsEnvelope } from '../../v2Types';

const SELF = 'sess-1';
const OTHER = 'sess-2';

function makeDeps(opts?: { sending?: boolean; sendingSid?: string }) {
  const sending = ref(opts?.sending ?? true);
  const deps = {
    selectedSessionId: () => SELF,
    sender: {
      sending,
      markSendingDone: vi.fn(),
      onRunAccepted: vi.fn(),
      onFirstByteArrived: vi.fn(),
      touchRunActivity: vi.fn(),
      sendingSessionId: () => opts?.sendingSid ?? SELF,
    },
    followUp: { onRunStatusV2: vi.fn() },
    applyFromV2RunStatus: vi.fn(),
    contextualLoading: {
      onSpiritNoticeType: vi.fn(),
      onRunHeartbeat: vi.fn(),
      clearMessage: vi.fn(),
    },
    streamManager: { ensureTeamStream: vi.fn(), ensureChatStream: vi.fn() },
  };
  return { deps, ...useChatV2EventHandlers(deps) };
}

function env(kind: string, sessionId: string | undefined, payload: unknown): V2WsEnvelope {
  return { type: 'v2_event', kind: kind as never, session_id: sessionId, payload: payload as never };
}

function sendFailedNotice(sessionId: string | undefined, meta?: Record<string, unknown>): V2WsEnvelope {
  return env('system.notice', sessionId, {
    NoticeType: 'send_failed',
    Message: 'boom',
    Meta: meta ?? null,
  });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('U-1a 终态帧发送态复位隔离', () => {
  it('异会话 task.completed 不得复位本会话 sending', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(env('task.completed', OTHER, {}));
    expect(deps.sender.markSendingDone).not.toHaveBeenCalled();
  });

  it('同会话 task.completed 仍复位 sending（兜底语义不变）', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(env('task.completed', SELF, {}));
    expect(deps.sender.markSendingDone).toHaveBeenCalledTimes(1);
  });

  it('无 session_id 的终态帧（旧服务端）保持宽松复位', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(env('task.failed', undefined, {}));
    expect(deps.sender.markSendingDone).toHaveBeenCalledTimes(1);
  });
});

describe('U-1b send_failed 发送态复位隔离', () => {
  it('异会话 send_failed 不得复位本会话 sending', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(sendFailedNotice(OTHER));
    expect(deps.sender.markSendingDone).not.toHaveBeenCalled();
    // 负向 toast 仍提示（全局面），只是不复位发送态
    expect(notifyCreate).toHaveBeenCalled();
  });

  it('同会话 send_failed 复位 sending', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(sendFailedNotice(SELF));
    expect(deps.sender.markSendingDone).toHaveBeenCalledTimes(1);
  });
});

describe('U-2 run_status 侧效应按 envelope.session_id 过滤', () => {
  const runningPayload = { RunID: 'run-1', Status: 'running', Meta: null };

  it('异会话 running 帧不得驱动本会话 composer/run 侧效应', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(env('system.run_status', OTHER, runningPayload));
    expect(deps.followUp.onRunStatusV2).not.toHaveBeenCalled();
    expect(deps.applyFromV2RunStatus).not.toHaveBeenCalled();
    expect(deps.sender.onRunAccepted).not.toHaveBeenCalled();
  });

  it('同会话 running 帧照常应用侧效应', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(env('system.run_status', SELF, runningPayload));
    expect(deps.followUp.onRunStatusV2).toHaveBeenCalledTimes(1);
    expect(deps.applyFromV2RunStatus).toHaveBeenCalledTimes(1);
    expect(deps.sender.onRunAccepted).toHaveBeenCalledTimes(1);
    expect(deps.streamManager.ensureChatStream).toHaveBeenCalledWith(SELF);
  });

  it('无 session_id 的 run_status（旧服务端）保持宽松应用', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(env('system.run_status', undefined, runningPayload));
    expect(deps.followUp.onRunStatusV2).toHaveBeenCalledTimes(1);
  });

  it('异会话 member pause 帧仍按 chat_session_id 打 MemberSession 补丁，但跳过其余侧效应', () => {
    const { deps, handleV2Event } = makeDeps();
    handleV2Event(
      env('system.run_status', OTHER, {
        RunID: 'run-member',
        Status: 'paused',
        Meta: { chat_session_id: 'chat-9' },
      }),
    );
    expect(activityStoreMock.patchMemberSessionStatusByChatSession).toHaveBeenCalledWith('chat-9', 'paused');
    expect(deps.followUp.onRunStatusV2).not.toHaveBeenCalled();
  });
});

describe('U-3 send_failed 乐观 Task 卡清扫', () => {
  it('meta.request_id 存在时移除对应乐观卡', () => {
    const { handleV2Event } = makeDeps();
    handleV2Event(sendFailedNotice(SELF, { request_id: 'pending-user-abc' }));
    expect(activityStoreMock.removeTask).toHaveBeenCalledWith('pending-user-abc');
  });

  it('缺 request_id（旧服务端）不得误触 removeTask', () => {
    const { handleV2Event } = makeDeps();
    handleV2Event(sendFailedNotice(SELF, { error_type: 'send_failed' }));
    expect(activityStoreMock.removeTask).not.toHaveBeenCalled();
  });

  it('request_id 仅空白字符时同样跳过', () => {
    const { handleV2Event } = makeDeps();
    handleV2Event(sendFailedNotice(SELF, { request_id: '   ' }));
    expect(activityStoreMock.removeTask).not.toHaveBeenCalled();
  });
});
