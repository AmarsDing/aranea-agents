// TECH-DEBT resolved: moved sendMessage to runtimeStore.send — chat optimization
// TECH-DEBT: WS send (deps.sendChatViaWs + sendCommand) bypasses Store action —
// see aranea-frontend-guide §3.1 (API → Store → Composable → Page).
import { ref, reactive, computed, getCurrentScope, onScopeDispose, type Ref, type ComputedRef } from 'vue';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { randomUUID } from '../../../utils/uuid';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useAuthStore } from '../../../stores/auth';
import { useAppStore } from '../../../stores/app';
import type { Task } from '../v2Types';
import { checkBackendHealth, getServerHeartbeatState } from '../../heartbeat/useServerHeartbeat';
import type { ChatAttachment, ChatEntityKind } from '../../../components/chat/types';
import type { WsSessionStream } from '../../../realtime/createWsSessionStream';
import type { WsUpstream } from '../../../realtime/ws-transport';
import { shouldBlockAttachmentsForModel } from '../modelCapabilities';
import { AWAIT_KIND_TOOL_CONFIRM } from '../awaitConstants';
import { isChatQueueFullError } from '../api';
import { sendCommand } from '../../../realtime/command_channel';
import {
  CHAT_RUN_STALL_CHECK_INTERVAL_MS,
  CHAT_RUN_STALL_NOTIFY_THRESHOLD_MS,
  CHAT_RUN_RECONCILE_THRESHOLD_MS,
  CHAT_STALL_NOTIFY_DURATION_MS,
  CHAT_FIRST_BYTE_NOTIFY_DURATION_MS,
  CHAT_RUN_ORPHAN_RECONCILE_DELAY_MS,
  firstByteNotifyThresholdMs,
} from '../../constants/timeouts';

import type { RunStatusValue } from '../types';

// 单条用户纯文本输入硬上限（字符数）——与后端 biz.UserInputHardLimitChars 保持一致。
export const USER_INPUT_HARD_LIMIT_CHARS = 200000;

type SessionStore = ReturnType<typeof useChatSessionStore>;
type MessageStore = ReturnType<typeof useChatMessageStore>;

type SendStrategy = {
  resolveSessionId: () => string | undefined;
  ensureSession: (title: string) => Promise<void | string>;
  resolveProviderModel: () => { provider: string; model: string };
  buildWsPayload: (
    sessionId: string,
    pendingUserId: string,
    content: string,
    provider: string,
    model: string,
    options: { dialogMode: string; attachments: ChatAttachment[] },
  ) => WsUpstream;
  ensureStream: (sessionId: string) => WsSessionStream;
  httpFallbackKeys: { agentKey: string | undefined; teamId: string | undefined };
  errorLabel: string;
};

type SendSnapshot = {
  entityKind: ChatEntityKind;
  entityId: string | undefined;
  sessionId: string | undefined;
  strategy: SendStrategy;
  options: {
    dialogMode: string;
    provider: string;
    model: string;
    capabilities: NonNullable<SenderDeps['selectedProviderModel']['value']>['capabilities'];
    attachments: ChatAttachment[];
  };
  attachmentRows: ChatAttachment[];
  attachmentRefs: ChatAttachment[];
  consumeDraft: () => void;
  onSessionReady?: (sessionId: string | undefined) => void;
};

export type SenderDeps = {
  appStore: ReturnType<typeof useAppStore>;
  sessionStore: SessionStore;
  messageStore: MessageStore;
  inputText: Ref<string>;
  dialogMode: Ref<string>;
  attachments: Ref<ChatAttachment[]>;
  isAwaitingUser: Ref<boolean>;
  awaitingRunId: Ref<string>;
  awaitKind: Ref<string>;
  runStatus: Ref<string>;
  selectedProviderModel: ComputedRef<
    | {
        provider: string;
        model: string;
        capabilities?: { vision?: boolean; image?: boolean; text_only?: boolean };
        config_json?: string;
      }
    | undefined
  >;
  ensureChatStream: (sessionId: string) => WsSessionStream;
  ensureTeamStream: (sessionId: string) => WsSessionStream;
  sendChatViaWs: (stream: WsSessionStream, upstream: WsUpstream) => void;
  onNewSession: (title?: string) => Promise<void | string>;
  makeSessionTitle: (content: string) => string;
  refreshRunStatus: () => Promise<void>;
  // 主动对账（watchdog 用）：绕过 WS 权威守卫直接 REST 核对 run 状态。
  // 返回本次对账刚取回并应用的后端权威状态；null = 未获后端确认（无选中会话，
  // 或响应被会话过期守卫丢弃）。网络失败会抛错。
  reconcileRunStatus: () => Promise<string | null>;
  setRunStatus: (status: RunStatusValue) => void;
  submitAwaitingReply: () => Promise<void>;
  submitToolConfirm: (approved: boolean) => Promise<void>;
  refreshPendingMessages?: () => Promise<void>;
  captureDraft?: () => () => void;
};

export function useChatSender(deps: SenderDeps) {
  const $q = useQuasar();
  const router = useRouter();
  const { t } = useI18n();
  const runtime = useChatRuntimeStore();
  const activityStore = useChatActivityStore();

  const sending = ref(false);
  const submissions = reactive<
    Array<{
      entityKind: ChatEntityKind;
      entityId: string | undefined;
      sessionId: string | undefined;
      creating: boolean;
    }>
  >([]);
  const submitting = computed(() =>
    submissions.some(
      (command) =>
        command.entityKind === deps.sessionStore.entityKind &&
        command.entityId === currentEntityId(command.entityKind) &&
        (command.creating || command.sessionId === currentSessionId(command.entityKind)),
    ),
  );
  let active = true;

  // T1.5: No-Timeout principle — removed sendingTimeout, turnAckTimeout.
  // Tasks run until completion or user cancel. The stall check and first-byte
  // notice are notification-only; they never mark messages as failed.

  let firstByteNoticeTimeout: ReturnType<typeof setTimeout> | null = null;
  let stallNotified = false;

  let lastRunEventAt = 0;
  let stallCheckInterval: ReturnType<typeof setInterval> | null = null;
  let watchdogReconciling = false;
  // D9-19 孤儿态自愈：markSendingDone 后 runStatus 仍非终态时（终态 WS/eWS 帧丢失
  // 而 task.completed 已停表），延迟 CHAT_RUN_ORPHAN_RECONCILE_DELAY_MS 单次触发
  // 既有对账通道的定时器。与 watchdog 共用 watchdogReconciling 重入锁、不发恢复通知。
  let orphanReconcileTimer: ReturnType<typeof setTimeout> | null = null;
  // 当前发送会话 id（final-review REG-3）：touchRunActivity 只统计本会话的活动帧，
  // 其他会话/团队流的事件不得顺延本会话的 90s 对账窗口。
  let sendingSessionId: string | undefined;

  const failedPendingIds = reactive(new Map<string, string>());

  function clearStallCheck() {
    if (stallCheckInterval != null) {
      clearInterval(stallCheckInterval);
      stallCheckInterval = null;
    }
  }

  // 终态集合：后端权威确认为这些状态时，watchdog 可安全复位 sending。
  // awaiting_user/pending/running/sync/durable 均不算终态（run 仍存活或待用户作答）。
  function isTerminalRunStatus(s: string): boolean {
    return s === 'idle' || s === 'completed' || s === 'failed' || s === 'cancelled';
  }

  // 发送态对账 watchdog：长时间无任何 run 活动事件时（终态 WS 帧丢失的残余场景），
  // 主动 REST 核对后端权威状态；仅当后端确认终态才复位。
  // 网络失败不复位（下个轮询周期重试）；活动中的 run 绝不打断。
  async function reconcileRunState() {
    if (watchdogReconciling) return;
    watchdogReconciling = true;
    try {
      const freshStatus = await deps.reconcileRunStatus();
      if (!sending.value) return;
      // 终态判定只认本次对账刚取回的后端状态（final-review REG-2）。null = 未获
      // 后端确认——本地遗留的 runStatus 值（可能是初始 'idle' 默认值）绝不能冒充
      // 后端确认，否则会无凭据复位 sending 并误报「已同步最新运行状态」。
      if (freshStatus != null && isTerminalRunStatus(freshStatus)) {
        markSendingDone();
        $q.notify({
          type: 'info',
          message: t('chat.runStateRecovered', '已同步最新运行状态'),
          timeout: 3000,
        });
      }
    } catch {
      // 网络失败：保持 sending，下个轮询周期重试。
    } finally {
      watchdogReconciling = false;
    }
  }

  function startStallCheck() {
    clearStallCheck();
    stallNotified = false;
    stallCheckInterval = setInterval(() => {
      if (!sending.value || lastRunEventAt === 0) return;
      // T1.5: notification-only — show a "please wait" notice after the
      // threshold, but never mark the message as failed or interrupt the run.
      if (!stallNotified && Date.now() - lastRunEventAt > CHAT_RUN_STALL_NOTIFY_THRESHOLD_MS) {
        stallNotified = true;
        $q.notify({
          type: 'info',
          message: t('chat.runStallWarning', '响应时间较长，模型仍在处理中，请耐心等待'),
          timeout: CHAT_STALL_NOTIFY_DURATION_MS,
        });
      }
      if (Date.now() - lastRunEventAt > CHAT_RUN_RECONCILE_THRESHOLD_MS) {
        void reconcileRunState();
      }
    }, CHAT_RUN_STALL_CHECK_INTERVAL_MS);
  }

  function clearOrphanReconcileTimer() {
    if (orphanReconcileTimer != null) {
      clearTimeout(orphanReconcileTimer);
      orphanReconcileTimer = null;
    }
  }

  // D9-19 孤儿对账：复用 watchdogReconciling 重入锁；静默（不发恢复通知）、
  // 不重排期（网络失败留待下次发送/切会话自然带上新对账）。
  async function orphanReconcileRunState() {
    if (watchdogReconciling) return;
    watchdogReconciling = true;
    try {
      await deps.reconcileRunStatus();
    } catch {
      // 网络失败：放弃本次自愈节拍。
    } finally {
      watchdogReconciling = false;
    }
  }

  // markSendingDone 收表时 runStatus 仍非终态（idle/completed/failed/cancelled）→
  // 延迟短节拍恰好一次孤儿对账；sending=true 期间由 90s watchdog 负责，不叠加。
  function scheduleOrphanReconcile() {
    clearOrphanReconcileTimer();
    if (isTerminalRunStatus(deps.runStatus.value)) return;
    orphanReconcileTimer = setTimeout(() => {
      orphanReconcileTimer = null;
      // 窗口内可能已由权威帧落定终态 —— 再查一次，避免无谓对账。
      if (isTerminalRunStatus(deps.runStatus.value)) return;
      void orphanReconcileRunState();
    }, CHAT_RUN_ORPHAN_RECONCILE_DELAY_MS);
  }

  function markSending(sessionId?: string) {
    // 新一轮发送：挂起的孤儿对账定时器作废（窗口由本轮 markSendingDone 重计）。
    clearOrphanReconcileTimer();
    sending.value = true;
    sendingSessionId = sessionId;
    // 发送时刻即 run 活动基线（final-review #2）：『发送后全程无事件』恰是最需要
    // 对账的场景——若保持 lastRunEventAt===0，stall 检查的首行早退会让对账永不可达。
    // WS 事件到达时由 touchRunActivity 顺延该基线（见 useChatV2EventHandlers）。
    lastRunEventAt = Date.now();
    startStallCheck();
  }

  function markSendingDone() {
    sending.value = false;
    sendingSessionId = undefined;
    clearFirstByteNotice();
    clearStallCheck();
    scheduleOrphanReconcile();
  }

  function clearFirstByteNotice() {
    if (firstByteNoticeTimeout != null) {
      clearTimeout(firstByteNoticeTimeout);
      firstByteNoticeTimeout = null;
    }
  }

  // T1.5: turnAckTimeout removed — no client-side turn-ack timeout.
  // The backend will process the message; if it fails, an error event arrives.

  function onFirstByteArrived() {
    clearFirstByteNotice();
  }

  function onRunAccepted() {
    clearFirstByteNotice();
    // T1.5: notification-only — show a "model thinking" notice after the
    // threshold, but never mark the message as failed.
    firstByteNoticeTimeout = setTimeout(() => {
      if (sending.value) {
        $q.notify({
          type: 'info',
          message: t('chat.firstByteTimeout', '模型正在思考中，请耐心等待'),
          timeout: CHAT_FIRST_BYTE_NOTIFY_DURATION_MS,
        });
        clearFirstByteNotice();
      }
    }, firstByteNotifyThresholdMs(deps.selectedProviderModel.value?.config_json));
  }

  function touchRunActivity(eventSessionId?: string) {
    // final-review REG-3：基线按发送会话隔离——其他会话/团队流的 run 活动不得
    // 顺延本会话的 90s 对账窗口（那会持续掩盖本会话终态帧丢失）。会话未知的帧
    // 保持宽松刷新（兼容无 session_id 的事件与旧调用方，如 stall watchdog 包装器）。
    if (eventSessionId && sendingSessionId && eventSessionId !== sendingSessionId) return;
    lastRunEventAt = Date.now();
  }

  function clearFailedPendingForSession(sessionId?: string) {
    if (!sessionId) {
      failedPendingIds.clear();
      return;
    }
    for (const [pendingId, sid] of failedPendingIds) {
      if (sid === sessionId) failedPendingIds.delete(pendingId);
    }
  }

  function stopStreaming(sessionId?: string) {
    if (sessionId) {
      runtime.stop(sessionId);
    }
    markSendingDone();
  }

  async function submitAwaitingReply() {
    await deps.submitAwaitingReply();
  }

  async function checkBackendAvailability(): Promise<boolean> {
    const heartbeat = getServerHeartbeatState();
    let backendUp = heartbeat.isAlive.value;
    if (!backendUp) {
      backendUp = await checkBackendHealth();
      if (backendUp) {
        heartbeat.isAlive.value = true;
      }
    }
    if (!backendUp) {
      const msg = import.meta.env.DEV
        ? t('chat.backendUnavailableDev', '后端服务不可用，请确认后端已启动')
        : t('chat.backendUnavailable', '后端服务不可用，请重新登录');
      $q.notify({ type: 'negative', message: msg, timeout: import.meta.env.DEV ? 8000 : 0 });
      if (!import.meta.env.DEV) {
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: 'login' });
      }
      return false;
    }
    return true;
  }

  function isActiveRun(): boolean {
    const s = deps.runStatus.value;
    return s === 'running' || s === 'pending';
  }

  const inputDisabled = computed(() => {
    if (isActiveRun()) return false;
    return sending.value && sendingSessionId === currentSessionId(deps.sessionStore.entityKind);
  });

  async function onSend() {
    if (submitting.value) return;
    const content = deps.inputText.value.trim();
    if (!content) {
      return;
    }
    // 单条输入硬上限（与后端 biz.UserInputHardLimitChars 对齐，2026-07-27）：
    // 超上限直接拒绝并提示改走附件通道；后端同样兜底校验。
    if (content.length > USER_INPUT_HARD_LIMIT_CHARS) {
      $q.notify({
        type: 'warning',
        message: t('chat.inputTooLong', { limit: USER_INPUT_HARD_LIMIT_CHARS }),
      });
      return;
    }
    if (!isActiveRun() && sending.value && sendingSessionId === currentSessionId(deps.sessionStore.entityKind)) {
      return;
    }

    const command = reactive({
      entityKind: deps.sessionStore.entityKind,
      entityId: currentEntityId(deps.sessionStore.entityKind),
      sessionId: currentSessionId(deps.sessionStore.entityKind),
      creating: !currentSessionId(deps.sessionStore.entityKind),
    });
    submissions.push(command);
    try {
      if (deps.isAwaitingUser.value) {
        if (deps.awaitKind.value === AWAIT_KIND_TOOL_CONFIRM) {
          $q.notify({ type: 'info', message: t('chat.toolConfirmUseButtons', '请使用批准或拒绝按钮来确认工具调用') });
          return;
        }
        await submitAwaitingReply();
        return;
      }

      if (
        deps.sessionStore.entityKind === 'agent' ||
        (deps.sessionStore.entityKind === 'team' && deps.sessionStore.selectedTeamId)
      ) {
        const snapshot = captureSendSnapshot(deps.sessionStore.entityKind, content);
        snapshot.onSessionReady = (sessionId) => {
          command.sessionId = sessionId;
          command.creating = false;
        };
        await sendUserContent(deps.sessionStore.entityKind, content, undefined, snapshot);
      } else {
        const reason = !deps.sessionStore.entityKind
          ? t('chat.noEntitySelected', '请先选择一个 Agent 或团队')
          : deps.sessionStore.entityKind === 'team'
            ? t('chat.noTeamSelected', '请先选择一个团队')
            : t('chat.unsupportedEntity', '不支持的实体类型');
        $q.notify({ type: 'warning', message: reason });
      }
    } finally {
      const index = submissions.indexOf(command);
      if (index >= 0) submissions.splice(index, 1);
    }
  }

  function dropPendingUserRow(sessionId: string, pendingUserId: string) {
    failedPendingIds.delete(pendingUserId);
    deps.messageStore.setMessages(
      sessionId,
      deps.messageStore.getMessages(sessionId).filter((m) => m.id !== pendingUserId),
    );
  }

  function markPendingUserFailed(sessionId: string, pendingUserId: string, errorMsg: string) {
    failedPendingIds.set(pendingUserId, sessionId);
    // T6: placeholder mechanism removed — no local placeholder row to mark
    // as failed. Surface the error to the user via a notification instead.
    $q.notify({ type: 'negative', message: errorMsg });
    activityStore.removeTask(pendingUserId);
  }

  async function retryFailedMessage(pendingUserId: string) {
    const sid = deps.sessionStore.selectedSession?.id ?? deps.sessionStore.teamSelectedSessionId;
    if (!sid) return;
    const msgs = deps.messageStore.getMessages(sid);
    const failed = msgs.find((m) => m.id === pendingUserId);
    if (!failed || failed.status !== 'failed') return;

    // 问题 4 修复：活跃 run 期间重试失败消息时，直接入队而非走 sendUserContent。
    // sendUserContent 在 followUp=true 时会进入 enqueueDuringRun 路径并 dropPendingUserRow，
    // 但此前已将状态重置为 'ok'，用户既看不到失败标记也看不到排队反馈。
    // 正确做法：检测到活跃 run 时直接 enqueue，成功后移除失败行并提示已加入队列，
    // 消息通过 ChatPendingQueue 展示，提供清晰的重试反馈。
    if (isActiveRun()) {
      const runtime = useChatRuntimeStore();
      try {
        const res = await runtime.enqueue(sid, failed.content_markdown, pendingUserId);
        if (res.accepted) {
          dropPendingUserRow(sid, pendingUserId);
          $q.notify({
            type: 'info',
            message: t('chat.retryQueuedDuringRun', '当前有任务执行中，已加入队列'),
          });
          await deps.refreshPendingMessages?.();
          return;
        }
        $q.notify({
          type: 'warning',
          message: t('chat.retryEnqueueRejected', '重试失败，请稍后再试'),
        });
      } catch (err: unknown) {
        const errMessage = err instanceof Error ? err.message : '';
        if (isChatQueueFullError(err)) {
          $q.notify({ type: 'warning', message: t('chat.enqueueQueueFull', '排队消息已满，请稍后再试') });
        } else {
          $q.notify({
            type: 'negative',
            message: err instanceof Error ? err.message : t('chat.retryEnqueueRejected', '重试失败，请稍后再试'),
          });
        }
      }
      return;
    }

    // 无活跃 run：重置状态并重新发送（复用 pendingId 避免占位闪烁）
    failedPendingIds.delete(pendingUserId);
    deps.messageStore.setMessages(
      sid,
      msgs.map((m) => (m.id === pendingUserId ? { ...m, status: 'ok', error_message: '' } : m)),
    );
    const entityKind = deps.sessionStore.entityKind;
    if (entityKind === 'team') {
      await sendTeamMessage(failed.content_markdown, pendingUserId);
    } else {
      await sendAgentUserContent(failed.content_markdown, pendingUserId);
    }
  }

  async function enqueueDuringRun(
    sessionId: string,
    content: string,
    pendingUserId: string,
    snapshot: SendSnapshot,
  ): Promise<boolean> {
    const runtime = useChatRuntimeStore();
    try {
      const res = await runtime.enqueue(sessionId, content, pendingUserId);
      if (res.accepted) {
        dropPendingUserRow(sessionId, pendingUserId);
        // The caller consumes the originating draft revision after this ACK.
        $q.notify({
          type: 'positive',
          message: res.queued
            ? t('chat.enqueueQueued', 'Message queued for after the current run')
            : t('chat.enqueueAccepted', 'Message will be injected at the next tool boundary'),
        });
        if (matchesSnapshot(snapshot)) await deps.refreshPendingMessages?.();
        return true;
      }
      dropPendingUserRow(sessionId, pendingUserId);
      if (!matchesSnapshot(snapshot)) return false;
      deps.setRunStatus('idle');
      await sendUserContent(snapshot.entityKind, content, undefined, snapshot);
    } catch (err: unknown) {
      dropPendingUserRow(sessionId, pendingUserId);
      const errMessage = err instanceof Error ? err.message : '';
      if (errMessage.includes('CHAT_RUN_ENDED') && matchesSnapshot(snapshot)) {
        deps.setRunStatus('idle');
        await sendUserContent(snapshot.entityKind, content, undefined, snapshot);
      } else if (isChatQueueFullError(err)) {
        $q.notify({ type: 'warning', message: t('chat.enqueueQueueFull', '排队消息已满，请稍后再试') });
      } else {
        $q.notify({
          type: 'negative',
          message: err instanceof Error ? err.message : t('chat.enqueueRejected', 'Could not enqueue message'),
        });
      }
    }
    return false;
  }

  async function sendViaHttpFallback(
    sessionId: string,
    content: string,
    agentKey: string | undefined,
    teamId: string | undefined,
    options: {
      dialogMode: string;
      provider: string;
      model: string;
      attachments: ChatAttachment[];
    },
    requestId?: string,
  ): Promise<void> {
    // B2: HTTP command channel — submit message and get ACK only.
    // Full message/state data arrives via the WS data channel.
    // Do NOT call loadMessages after HTTP success — the WS data channel
    // is the single source of truth for message data.
    // US-14：不再随消息发送 knowledge_bases（留空 = 后端全库智能路由）。
    const ack = await sendCommand({
      session_id: sessionId,
      agent_key: agentKey,
      team_id: teamId,
      content,
      // P3：HTTP fallback 与 WS 共用同一幂等键（pendingUserId），
      // WS 发出后结果不明时的重试不会重复执行。
      request_id: requestId,
      options: {
        dialog_mode: options.dialogMode,
        provider: options.provider,
        model: options.model,
        attachments: options.attachments.map((a) => ({ id: a.id })),
      },
    });
    // SP-1e: when ACK messageId differs from optimistic pending id, remap in
    // place so task.created merges without dual-source double-render.
    const realId = String(ack?.messageId || ack?.turnId || '').trim();
    const pending = String(requestId || '').trim();
    if (pending && realId && pending !== realId) {
      activityStore.adoptOptimisticTask(pending, realId);
    }
  }

  function notifyUnsupportedImageModel() {
    $q.notify({
      type: 'warning',
      message: t('chat.imageModelUnsupported', '当前模型不支持图片理解，请移除图片附件或切换到支持视觉的模型'),
    });
  }

  function getAgentStrategy(): SendStrategy {
    const agentKey = deps.appStore.selectedAgent?.agent_key;
    return {
      resolveSessionId: () => deps.sessionStore.selectedSession?.id,
      ensureSession: async (title) => {
        if (!deps.sessionStore.selectedSession) return deps.onNewSession(title);
      },
      resolveProviderModel: () => {
        const selectedModel = deps.selectedProviderModel.value;
        const provider =
          selectedModel?.provider ||
          deps.sessionStore.selectedSession?.provider ||
          deps.appStore.selectedAgent?.provider ||
          '';
        const model =
          selectedModel?.model || deps.sessionStore.selectedSession?.model || deps.appStore.selectedAgent?.model || '';
        return { provider, model };
      },
      buildWsPayload: (sessionId, pendingUserId, content, provider, model, options) => ({
        direction: 'client_to_server',
        channel: 'chat',
        type: 'user_message',
        request_id: pendingUserId,
        payload: {
          session_id: sessionId,
          agent_key: agentKey,
          content,
          options: {
            dialog_mode: options.dialogMode,
            provider,
            model,
            attachments: options.attachments.map((item) => ({ id: item.id })),
          },
        },
      }),
      ensureStream: deps.ensureChatStream,
      httpFallbackKeys: { agentKey: deps.appStore.selectedAgent?.agent_key, teamId: undefined },
      errorLabel: t('chat.sendFailed', '发送失败，请稍后重试'),
    };
  }

  function getTeamStrategy(): SendStrategy {
    const teamId = deps.sessionStore.selectedTeamId;
    return {
      resolveSessionId: () => deps.sessionStore.teamSelectedSessionId ?? undefined,
      ensureSession: async (title) => {
        if (!deps.sessionStore.teamSelectedSessionId) return deps.onNewSession(title);
      },
      resolveProviderModel: () => {
        const selectedModel = deps.selectedProviderModel.value;
        const teamId = deps.sessionStore.selectedTeamId!;
        const session = deps.sessionStore.teamSessions[teamId]?.find(
          (item) => item.id === deps.sessionStore.teamSelectedSessionId,
        );
        const provider = selectedModel?.provider || session?.provider || '';
        const model = selectedModel?.model || session?.model || '';
        return { provider, model };
      },
      buildWsPayload: (sessionId, pendingUserId, content, provider, model, options) => ({
        direction: 'client_to_server',
        channel: 'chat',
        type: 'user_message',
        request_id: pendingUserId,
        payload: {
          session_id: sessionId,
          team_id: teamId,
          content,
          options: {
            dialog_mode: options.dialogMode,
            provider,
            model,
            attachments: options.attachments.map((item) => ({ id: item.id })),
          },
        },
      }),
      ensureStream: deps.ensureTeamStream,
      httpFallbackKeys: { agentKey: undefined, teamId: deps.sessionStore.selectedTeamId ?? undefined },
      errorLabel: t('chat.teamSendFailed', 'Team 发送失败'),
    };
  }

  function currentEntityId(kind: ChatEntityKind): string | undefined {
    return kind === 'team' ? (deps.sessionStore.selectedTeamId ?? undefined) : deps.appStore.selectedAgent?.id;
  }

  function currentSessionId(kind: ChatEntityKind): string | undefined {
    return kind === 'team'
      ? (deps.sessionStore.teamSelectedSessionId ?? undefined)
      : deps.sessionStore.selectedSession?.id;
  }

  function matchesSnapshot(snapshot: SendSnapshot): boolean {
    return (
      active &&
      deps.sessionStore.entityKind === snapshot.entityKind &&
      currentEntityId(snapshot.entityKind) === snapshot.entityId &&
      (!snapshot.sessionId || snapshot.strategy.resolveSessionId() === snapshot.sessionId)
    );
  }

  function captureSendSnapshot(entityKind: ChatEntityKind, text: string): SendSnapshot {
    const strategy = entityKind === 'team' ? getTeamStrategy() : getAgentStrategy();
    const draftText = deps.inputText.value;
    const usesDraft = draftText.trim() === text;
    const consume = usesDraft ? deps.captureDraft?.() : undefined;
    const selectedModel = deps.selectedProviderModel.value;
    const rows = deps.attachments.value;
    const snapshot: SendSnapshot = {
      entityKind,
      entityId: currentEntityId(entityKind),
      sessionId: strategy.resolveSessionId(),
      strategy,
      options: {
        dialogMode: deps.dialogMode.value,
        ...strategy.resolveProviderModel(),
        capabilities: selectedModel?.capabilities ? { ...selectedModel.capabilities } : undefined,
        attachments: rows.map((item) => ({ ...item })),
      },
      attachmentRows: rows,
      attachmentRefs: [...rows],
      consumeDraft:
        consume ??
        (() => {
          if (usesDraft && matchesSnapshot(snapshot) && deps.inputText.value === draftText) deps.inputText.value = '';
        }),
    };
    return snapshot;
  }

  async function sendUserContent(
    entityKind: ChatEntityKind,
    content: string,
    reusePendingId?: string,
    inheritedSnapshot?: SendSnapshot,
  ) {
    const text = content.trim();
    if (!text) return;
    const snapshot = inheritedSnapshot ?? captureSendSnapshot(entityKind, text);
    const strategy = snapshot.strategy;
    if (!matchesSnapshot(snapshot)) return;
    const finishSending = () => {
      if (active && sendingSessionId === snapshot.sessionId) markSendingDone();
    };
    const followUp = isActiveRun();
    // S-03: Do not call markSending() before session is resolved — it starts
    // the stall check and dispatch timeout prematurely. The second call below
    // (after ensureSession) is the correct one.
    let clearAttachments = false;
    try {
      const createdId = await strategy.ensureSession(deps.makeSessionTitle(text));
      snapshot.sessionId ??= createdId || strategy.resolveSessionId();
      snapshot.onSessionReady?.(snapshot.sessionId);
      if (!matchesSnapshot(snapshot)) return;
      const sessionId = snapshot.sessionId;
      if (!sessionId) {
        $q.notify({ type: 'negative', message: t('chat.sessionCreateFailed', '未创建会话或会话无效，请重试') });
        if (!followUp) finishSending();
        return;
      }
      // If the session was pre-created (e.g. user clicked "新会话") with the
      // default title "新对话", rename it to the first message content.
      // ensureSession is a no-op when a session already exists, so without
      // this check the title would stay "新对话" forever.
      const defaultTitle = t('chat.untitledSession');
      const currentTitle =
        entityKind === 'team'
          ? deps.sessionStore.teamSessions[deps.sessionStore.selectedTeamId ?? '']?.find((s) => s.id === sessionId)
              ?.title
          : deps.sessionStore.selectedSession?.title;
      if (currentTitle === defaultTitle || !currentTitle) {
        const newTitle = deps.makeSessionTitle(text);
        if (newTitle && newTitle !== defaultTitle) {
          void deps.sessionStore.renameSessionByKind(sessionId, newTitle);
        }
      }
      if (!followUp) {
        markSending(sessionId);
      }

      const pendingUserId = reusePendingId ?? `pending-user-${randomUUID()}`;
      // T6: placeholder mechanism removed — user messages are now rendered
      // from task activity events via ActivityStream. pendingUserId is kept
      // only as the WS request_id for idempotency. Drafts are consumed only
      // after successful dispatch or the HTTP/queue ACK, never before preflight.

      // 真实 task.created 事件到达时，store.upsertTask 会自动清理同 sessionId
      // 下所有 'pending-user-' 开头的乐观 Task。
      const optimisticTask: Task = {
        ID: pendingUserId,
        SessionID: sessionId,
        UserMessage: text,
        Status: 'pending',
        Seq: Date.now(),
        Version: 0,
        CreatedAt: new Date().toISOString(),
        UpdatedAt: new Date().toISOString(),
        CompletedAt: null,
      };
      activityStore.upsertTask(optimisticTask);

      const { provider, model, capabilities } = snapshot.options;
      const blockReason = shouldBlockAttachmentsForModel(
        { provider, model, capabilities },
        snapshot.options.attachments,
      );
      if (blockReason) {
        if (blockReason === 'ATTACHMENT_UNSUPPORTED') {
          $q.notify({
            type: 'warning',
            message: t('chat.fileModelUnsupported', '当前模型不支持文件附件，请移除附件或切换模型'),
          });
        } else {
          notifyUnsupportedImageModel();
        }
        if (!reusePendingId) {
          dropPendingUserRow(sessionId, pendingUserId);
          activityStore.removeTask(pendingUserId);
        }
        if (!followUp) finishSending();
        return;
      }

      if (!(await checkBackendAvailability())) {
        markPendingUserFailed(sessionId, pendingUserId, t('chat.sendFailedBackend', '后端不可用'));
        if (!followUp) finishSending();
        return;
      }

      if (!matchesSnapshot(snapshot)) {
        if (!reusePendingId) activityStore.removeTask(pendingUserId);
        finishSending();
        return;
      }

      if (followUp) {
        clearAttachments = await enqueueDuringRun(sessionId, text, pendingUserId, snapshot);
        return;
      }

      try {
        const stream = strategy.ensureStream(sessionId);
        deps.sendChatViaWs(
          stream,
          strategy.buildWsPayload(sessionId, pendingUserId, text, provider, model, snapshot.options),
        );
        // T1.5: No turn-ack timeout — the backend processes the message and
        // sends run.status=running as an early ack. If it fails, an error
        // event arrives. No client-side timeout marks the message as failed.
        // Clear attachments only after WS send is dispatched successfully.
        // If the backend fails, the error envelope will arrive later and
        // the user can retry with the same attachments still attached.
        clearAttachments = true;
      } catch {
        $q.notify({ type: 'info', message: t('chat.wsFallbackHttp', 'WebSocket 不可用，正在通过 HTTP 发送…') });
        try {
          await sendViaHttpFallback(
            sessionId,
            text,
            strategy.httpFallbackKeys.agentKey,
            strategy.httpFallbackKeys.teamId,
            {
              dialogMode: snapshot.options.dialogMode,
              provider,
              model,
              attachments: snapshot.options.attachments,
            },
            pendingUserId,
          );
          // B2: HTTP command channel returns ACK only — no loadMessages.
          // The WS data channel pushes the persisted message and subsequent
          // streaming/state events. markSendingDone so the sending state
          // doesn't stay true; the WS data channel will deliver the message.
          if (!followUp) finishSending();
          clearAttachments = true;
          // E2E-P1-07: Keep the optimistic task. HTTP ACK is not a delivery
          // guarantee for task.created, and WS replay was removed. Reconcile
          // via REST snapshot; upsertTask clears pending-user-* when the real
          // task arrives (or on reconnect hydrate).
          void activityStore.fetchSessionHistory(sessionId).catch(() => undefined);
        } catch {
          markPendingUserFailed(sessionId, pendingUserId, t('chat.sendFailedRetry', '发送失败，请点击重试'));
          if (!followUp) finishSending();
        }
      }
    } catch (error) {
      if (!(error instanceof DOMException && error.name === 'AbortError')) {
        // T6: placeholder mechanism removed — no local placeholder row to
        // mark as failed. Surface the error directly via a notification.
        $q.notify({
          type: 'negative',
          message: error instanceof Error ? error.message : strategy.errorLabel,
        });
        if (!followUp) finishSending();
      }
    } finally {
      if (clearAttachments) {
        snapshot.consumeDraft();
        const consumed = new Set(snapshot.attachmentRefs);
        for (let index = snapshot.attachmentRows.length - 1; index >= 0; index--) {
          if (consumed.has(snapshot.attachmentRows[index]!)) snapshot.attachmentRows.splice(index, 1);
        }
      }
    }
  }

  async function sendAgentUserContent(content: string, reusePendingId?: string) {
    await sendUserContent('agent', content, reusePendingId);
  }

  async function sendTeamMessage(content: string, reusePendingId?: string) {
    await sendUserContent('team', content, reusePendingId);
  }

  // 组件销毁路径：挂起的孤儿对账定时器随作用域回收清理（单测在组件外调用，
  // onScopeDispose 需作用域守卫）。
  if (getCurrentScope()) {
    onScopeDispose(() => {
      active = false;
      clearOrphanReconcileTimer();
    });
  }

  return {
    sending,
    submitting,
    inputDisabled,
    markSending,
    markSendingDone,
    onRunAccepted,
    onFirstByteArrived,
    stopStreaming,
    submitAwaitingReply,
    submitToolConfirm: deps.submitToolConfirm,
    onSend,
    sendAgentUserContent,
    sendTeamMessage,
    retryFailedMessage,
    touchRunActivity,
    // U-1 双流隔离：终态帧/send_failed 复位 sending 前按它比对帧来源会话。
    sendingSessionId: () => sendingSessionId,
    clearFailedPendingForSession,
    failedPendingIds,
  };
}
