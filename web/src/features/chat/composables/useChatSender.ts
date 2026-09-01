// TECH-DEBT resolved: moved sendMessage to runtimeStore.send — chat optimization
// TECH-DEBT: WS send (deps.sendChatViaWs + sendCommand) bypasses Store action —
// see aranea-frontend-guide §3.1 (API → Store → Composable → Page).
import { ref, reactive, computed, type Ref, type ComputedRef } from 'vue';
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
  CHAT_STALL_NOTIFY_DURATION_MS,
  CHAT_FIRST_BYTE_NOTIFY_DURATION_MS,
  firstByteNotifyThresholdMs,
} from '../../constants/timeouts';

import type { RunStatusValue } from '../types';

// 单条用户纯文本输入硬上限（字符数）——与后端 biz.UserInputHardLimitChars 保持一致。
export const USER_INPUT_HARD_LIMIT_CHARS = 200000;

type SessionStore = ReturnType<typeof useChatSessionStore>;
type MessageStore = ReturnType<typeof useChatMessageStore>;

type SendStrategy = {
  resolveSessionId: () => string | undefined;
  ensureSession: (title: string) => Promise<void>;
  resolveProviderModel: () => { provider: string; model: string };
  buildWsPayload: (
    sessionId: string,
    pendingUserId: string,
    content: string,
    provider: string,
    model: string,
  ) => WsUpstream;
  ensureStream: (sessionId: string) => WsSessionStream;
  httpFallbackKeys: { agentKey: string | undefined; teamId: string | undefined };
  errorLabel: string;
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
  onNewSession: (title?: string) => Promise<void>;
  makeSessionTitle: (content: string) => string;
  refreshRunStatus: () => Promise<void>;
  setRunStatus: (status: RunStatusValue) => void;
  submitAwaitingReply: () => Promise<void>;
  submitToolConfirm: (approved: boolean) => Promise<void>;
  refreshPendingMessages?: () => Promise<void>;
};

export function useChatSender(deps: SenderDeps) {
  const $q = useQuasar();
  const router = useRouter();
  const { t } = useI18n();
  const runtime = useChatRuntimeStore();
  const activityStore = useChatActivityStore();

  const sending = ref(false);

  // T1.5: No-Timeout principle — removed sendingTimeout, turnAckTimeout.
  // Tasks run until completion or user cancel. The stall check and first-byte
  // notice are notification-only; they never mark messages as failed.

  let firstByteNoticeTimeout: ReturnType<typeof setTimeout> | null = null;
  let stallNotified = false;

  let lastRunEventAt = 0;
  let stallCheckInterval: ReturnType<typeof setInterval> | null = null;

  const failedPendingIds = reactive(new Map<string, string>());

  function clearStallCheck() {
    if (stallCheckInterval != null) {
      clearInterval(stallCheckInterval);
      stallCheckInterval = null;
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
    }, CHAT_RUN_STALL_CHECK_INTERVAL_MS);
  }

  function markSending(_sessionId?: string) {
    sending.value = true;
    startStallCheck();
  }

  function markSendingDone() {
    sending.value = false;
    clearFirstByteNotice();
    clearStallCheck();
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

  function touchRunActivity() {
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
        ? t(
            'chat.backendUnavailableDev',
            '后端服务不可用，请确认后端已启动',
          )
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
    return sending.value;
  });

  async function onSend() {
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
    if (!isActiveRun() && sending.value) {
      return;
    }

    if (deps.isAwaitingUser.value) {
      if (deps.awaitKind.value === AWAIT_KIND_TOOL_CONFIRM) {
        $q.notify({ type: 'info', message: t('chat.toolConfirmUseButtons', '请使用批准或拒绝按钮来确认工具调用') });
        return;
      }
      await submitAwaitingReply();
      return;
    }

    if (deps.sessionStore.entityKind === 'agent') {
      await sendAgentMessage(content);
    } else if (deps.sessionStore.entityKind === 'team' && deps.sessionStore.selectedTeamId) {
      await sendTeamMessage(content);
    } else {
      const reason = !deps.sessionStore.entityKind
        ? t('chat.noEntitySelected', '请先选择一个 Agent 或团队')
        : deps.sessionStore.entityKind === 'team'
          ? t('chat.noTeamSelected', '请先选择一个团队')
          : t('chat.unsupportedEntity', '不支持的实体类型');
      $q.notify({ type: 'warning', message: reason });
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
        const res = await runtime.enqueue(sid, failed.content_markdown);
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

  async function enqueueDuringRun(sessionId: string, content: string, pendingUserId: string) {
    const runtime = useChatRuntimeStore();
    try {
      const res = await runtime.enqueue(sessionId, content);
      if (res.accepted) {
        dropPendingUserRow(sessionId, pendingUserId);
        // T5.3: Clear input only after successful enqueue (matches Path B
        // onEnqueueWhileRunning behavior). For follow-up sends, input was
        // intentionally not cleared in sendUserContent so the user can retry
        // if enqueue fails.
        deps.inputText.value = '';
        $q.notify({
          type: 'positive',
          message: res.queued
            ? t('chat.enqueueQueued', 'Message queued for after the current run')
            : t('chat.enqueueAccepted', 'Message will be injected at the next tool boundary'),
        });
        await deps.refreshPendingMessages?.();
        return;
      }
      dropPendingUserRow(sessionId, pendingUserId);
      deps.setRunStatus('idle');
      await sendUserContent(deps.sessionStore.entityKind, content);
    } catch (err: unknown) {
      dropPendingUserRow(sessionId, pendingUserId);
      const errMessage = err instanceof Error ? err.message : '';
      if (errMessage.includes('CHAT_RUN_ENDED')) {
        deps.setRunStatus('idle');
        await sendUserContent(deps.sessionStore.entityKind, content);
      } else if (isChatQueueFullError(err)) {
        $q.notify({ type: 'warning', message: t('chat.enqueueQueueFull', '排队消息已满，请稍后再试') });
      } else {
        $q.notify({
          type: 'negative',
          message: err instanceof Error ? err.message : t('chat.enqueueRejected', 'Could not enqueue message'),
        });
      }
    }
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
    await sendCommand({
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
  }

  function notifyUnsupportedImageModel() {
    $q.notify({
      type: 'warning',
      message: t('chat.imageModelUnsupported', '当前模型不支持图片理解，请移除图片附件或切换到支持视觉的模型'),
    });
  }

  function getAgentStrategy(): SendStrategy {
    return {
      resolveSessionId: () => deps.sessionStore.selectedSession?.id,
      ensureSession: async (title) => {
        if (!deps.sessionStore.selectedSession) await deps.onNewSession(title);
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
      buildWsPayload: (sessionId, pendingUserId, content, provider, model) => ({
        direction: 'client_to_server',
        channel: 'chat',
        type: 'user_message',
        request_id: pendingUserId,
        payload: {
          session_id: sessionId,
          agent_key: deps.appStore.selectedAgent?.agent_key,
          content,
          options: {
            dialog_mode: deps.dialogMode.value,
            provider,
            model,
            attachments: deps.attachments.value.map((item) => ({ id: item.id })),
          },
        },
      }),
      ensureStream: deps.ensureChatStream,
      httpFallbackKeys: { agentKey: deps.appStore.selectedAgent?.agent_key, teamId: undefined },
      errorLabel: t('chat.sendFailed', '发送失败，请稍后重试'),
    };
  }

  function getTeamStrategy(): SendStrategy {
    return {
      resolveSessionId: () => deps.sessionStore.teamSelectedSessionId ?? undefined,
      ensureSession: async (title) => {
        if (!deps.sessionStore.teamSelectedSessionId) await deps.onNewSession(title);
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
      buildWsPayload: (sessionId, pendingUserId, content, provider, model) => ({
        direction: 'client_to_server',
        channel: 'chat',
        type: 'user_message',
        request_id: pendingUserId,
        payload: {
          session_id: sessionId,
          team_id: deps.sessionStore.selectedTeamId,
          content,
          options: {
            dialog_mode: deps.dialogMode.value,
            provider,
            model,
            attachments: deps.attachments.value.map((item) => ({ id: item.id })),
          },
        },
      }),
      ensureStream: deps.ensureTeamStream,
      httpFallbackKeys: { agentKey: undefined, teamId: deps.sessionStore.selectedTeamId ?? undefined },
      errorLabel: t('chat.teamSendFailed', 'Team 发送失败'),
    };
  }

  async function sendUserContent(entityKind: ChatEntityKind, content: string, reusePendingId?: string) {
    const strategy = entityKind === 'team' ? getTeamStrategy() : getAgentStrategy();
    const text = content.trim();
    if (!text) return;
    const followUp = isActiveRun();
    // S-03: Do not call markSending() before session is resolved — it starts
    // the stall check and dispatch timeout prematurely. The second call below
    // (after ensureSession) is the correct one.
    let clearAttachments = false;
    try {
      await strategy.ensureSession(deps.makeSessionTitle(text));
      const sessionId = strategy.resolveSessionId();
      if (!sessionId) {
        $q.notify({ type: 'negative', message: t('chat.sessionCreateFailed', '未创建会话或会话无效，请重试') });
        if (!followUp) markSendingDone();
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
      // only as the WS request_id for idempotency. Input is cleared here for
      // the primary send path; the follow-up (enqueue) path clears input
      // after a successful enqueue inside enqueueDuringRun.
      if (!reusePendingId && !followUp) {
        deps.inputText.value = '';
      }

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

      const { provider, model } = strategy.resolveProviderModel();
      const selectedModel = deps.selectedProviderModel.value;
      const blockReason = shouldBlockAttachmentsForModel(
        { provider, model, capabilities: selectedModel?.capabilities },
        deps.attachments.value,
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
        if (!reusePendingId) dropPendingUserRow(sessionId, pendingUserId);
        if (!followUp) markSendingDone();
        return;
      }

      if (!(await checkBackendAvailability())) {
        markPendingUserFailed(sessionId, pendingUserId, t('chat.sendFailedBackend', '后端不可用'));
        if (!followUp) markSendingDone();
        return;
      }

      if (followUp) {
        await enqueueDuringRun(sessionId, text, pendingUserId);
        return;
      }

      try {
        const stream = strategy.ensureStream(sessionId);
        deps.sendChatViaWs(stream, strategy.buildWsPayload(sessionId, pendingUserId, text, provider, model));
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
              dialogMode: deps.dialogMode.value,
              provider,
              model,
              attachments: deps.attachments.value,
            },
            pendingUserId,
          );
          // B2: HTTP command channel returns ACK only — no loadMessages.
          // The WS data channel pushes the persisted message and subsequent
          // streaming/state events. markSendingDone so the sending state
          // doesn't stay true; the WS data channel will deliver the message.
          if (!followUp) markSendingDone();
          clearAttachments = true;
          // E2E-P1-07: Keep the optimistic task. HTTP ACK is not a delivery
          // guarantee for task.created, and WS replay was removed. Reconcile
          // via REST snapshot; upsertTask clears pending-user-* when the real
          // task arrives (or on reconnect hydrate).
          void activityStore.fetchSessionHistory(sessionId).catch(() => undefined);
        } catch {
          markPendingUserFailed(sessionId, pendingUserId, t('chat.sendFailedRetry', '发送失败，请点击重试'));
          if (!followUp) markSendingDone();
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
        if (!followUp) markSendingDone();
      }
    } finally {
      if (clearAttachments) {
        deps.attachments.value = [];
      }
    }
  }

  async function sendAgentUserContent(content: string, reusePendingId?: string) {
    await sendUserContent('agent', content, reusePendingId);
  }

  async function sendAgentMessage(content: string) {
    await sendAgentUserContent(content);
  }

  async function sendTeamMessage(content: string, reusePendingId?: string) {
    await sendUserContent('team', content, reusePendingId);
  }

  return {
    sending,
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
    clearFailedPendingForSession,
    failedPendingIds,
  };
}
