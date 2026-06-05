import { ref, reactive, computed, type Ref, type ComputedRef } from 'vue';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useAuthStore } from '../../../stores/auth';
import { useAppStore } from '../../../stores/app';
import { checkBackendHealth, getServerHeartbeatState } from '../../heartbeat/useServerHeartbeat';
import type { ChatAttachment, ChatEntityKind } from '../../../components/chat/types';
import type { UseEnvelopeStreamReturn } from '../useEnvelopeStream';
import type { WsUpstream } from '../envelope';
import { createPlaceholderMessage } from '../streamHandlers';
import { shouldBlockAttachmentsForModel } from '../modelCapabilities';
// TECH-DEBT: direct API call; move to store — chat optimization
import { sendMessage } from '../api';
import { AWAIT_KIND_TOOL_CONFIRM } from '../awaitConstants';

import type { RunStatusValue } from '../types';

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
  ensureStream: (sessionId: string) => UseEnvelopeStreamReturn;
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
      }
    | undefined
  >;
  selectedKnowledgeBases: Ref<string[]>;
  ensureChatStream: (sessionId: string) => UseEnvelopeStreamReturn;
  ensureTeamStream: (sessionId: string) => UseEnvelopeStreamReturn;
  sendChatViaWs: (stream: UseEnvelopeStreamReturn, upstream: WsUpstream) => void;
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

  const sending = ref(false);
  let sendingTimeout: ReturnType<typeof setTimeout> | null = null;
  const SEND_DISPATCH_TIMEOUT_MS = 30_000;

  // 30-second client-side turn-ack timeout: if backend doesn't respond with
  // run.status=running/accepted within 30s, mark the placeholder user message
  // as failed so the user can retry immediately instead of waiting for the
  // (much longer) server-side 5min timeout.
  let turnAckTimeout: ReturnType<typeof setTimeout> | null = null;
  const TURN_ACK_TIMEOUT_MS = 30_000;
  let pendingTurnAck: { sessionId: string; pendingUserId: string } | null = null;

  let firstByteTimeout: ReturnType<typeof setTimeout> | null = null;
  const FIRST_BYTE_TIMEOUT_MS = 90_000;

  let lastRunEventAt = 0;
  const RUN_STALL_TIMEOUT_MS = 180_000;
  const RUN_STALL_CHECK_INTERVAL_MS = 30_000;
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
    stallCheckInterval = setInterval(() => {
      if (!sending.value || lastRunEventAt === 0) return;
      if (Date.now() - lastRunEventAt > RUN_STALL_TIMEOUT_MS) {
        $q.notify({
          type: 'warning',
          message: t('chat.runStallWarning', '响应时间较长，请耐心等待或停止生成'),
          timeout: 8000,
        });
        clearStallCheck();
      }
    }, RUN_STALL_CHECK_INTERVAL_MS);
  }

  function markSending(sessionId?: string) {
    sending.value = true;
    clearSendingTimeout();
    sendingTimeout = setTimeout(() => {
      if (sending.value) {
        sending.value = false;
        $q.notify({ type: 'warning', message: t('chat.sendDispatchTimeout', '消息发送超时，请检查网络连接') });
      }
    }, SEND_DISPATCH_TIMEOUT_MS);
    startStallCheck();
  }

  function markSendingDone() {
    sending.value = false;
    clearSendingTimeout();
    clearFirstByteTimeout();
    clearTurnAckTimeout();
    clearStallCheck();
  }

  function clearSendingTimeout() {
    if (sendingTimeout != null) {
      clearTimeout(sendingTimeout);
      sendingTimeout = null;
    }
  }

  function clearFirstByteTimeout() {
    if (firstByteTimeout != null) {
      clearTimeout(firstByteTimeout);
      firstByteTimeout = null;
    }
  }

  function clearTurnAckTimeout() {
    if (turnAckTimeout != null) {
      clearTimeout(turnAckTimeout);
      turnAckTimeout = null;
    }
    pendingTurnAck = null;
  }

  function startTurnAckTimeout(sessionId: string, pendingUserId: string) {
    clearTurnAckTimeout();
    pendingTurnAck = { sessionId, pendingUserId };
    turnAckTimeout = setTimeout(() => {
      turnAckTimeout = null;
      const target = pendingTurnAck;
      pendingTurnAck = null;
      if (!target) return;
      if (!sending.value) return;
      markPendingUserFailed(
        target.sessionId,
        target.pendingUserId,
        t('chat.sendTimeoutRetry', '后端 30 秒内未确认 turn，请点击重试'),
      );
      markSendingDone();
      $q.notify({
        type: 'negative',
        message: t('chat.sendTimeoutToast', '后端响应超时，消息已标记为失败，请重试'),
      });
    }, TURN_ACK_TIMEOUT_MS);
  }

  function onFirstByteArrived() {
    clearFirstByteTimeout();
  }

  function onRunAccepted() {
    clearFirstByteTimeout();
    clearTurnAckTimeout();
    firstByteTimeout = setTimeout(() => {
      if (sending.value) {
        $q.notify({
          type: 'warning',
          message: t('chat.firstByteTimeout', '响应等待时间较长，模型可能正在思考中'),
          timeout: 8000,
        });
        clearFirstByteTimeout();
      }
    }, FIRST_BYTE_TIMEOUT_MS);
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
            '后端不可用，请确认 admin 是否在 :8000 运行（页面应使用 http://localhost:9001）',
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
      // unsupported entity kind — no action
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
    const msgs = deps.messageStore.getMessages(sessionId);
    deps.messageStore.setMessages(
      sessionId,
      msgs.map((m) => (m.id === pendingUserId ? { ...m, status: 'failed', error_message: errorMsg } : m)),
    );
  }

  async function retryFailedMessage(pendingUserId: string) {
    const sid = deps.sessionStore.selectedSession?.id ?? deps.sessionStore.teamSelectedSessionId;
    if (!sid) return;
    const msgs = deps.messageStore.getMessages(sid);
    const failed = msgs.find((m) => m.id === pendingUserId);
    if (!failed || failed.status !== 'failed') return;
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
      await sendUserContent('agent', content);
    } catch (err: unknown) {
      dropPendingUserRow(sessionId, pendingUserId);
      const errMessage = err instanceof Error ? err.message : '';
      if (errMessage.includes('CHAT_RUN_ENDED')) {
        deps.setRunStatus('idle');
        await sendUserContent('agent', content);
      } else if (errMessage.includes('CHAT_QUEUE_FULL')) {
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
      knowledgeBases: string[];
    },
  ): Promise<void> {
    await sendMessage({
      session_id: sessionId,
      agent_key: agentKey,
      team_id: teamId,
      content,
      options: {
        dialog_mode: options.dialogMode,
        provider: options.provider,
        model: options.model,
        attachments: options.attachments.map((a) => ({ id: a.id })),
        knowledge_bases: options.knowledgeBases,
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
            knowledge_bases: deps.selectedKnowledgeBases.value,
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
      resolveSessionId: () => deps.sessionStore.teamSelectedSessionId,
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
            knowledge_bases: deps.selectedKnowledgeBases.value,
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
    if (!followUp) {
      markSending();
    }
    let clearAttachments = true;
    try {
      await strategy.ensureSession(deps.makeSessionTitle(text));
      const sessionId = strategy.resolveSessionId();
      if (!sessionId) {
        $q.notify({ type: 'negative', message: t('chat.sessionCreateFailed', '未创建会话或会话无效，请重试') });
        if (!followUp) markSendingDone();
        return;
      }
      if (!followUp) {
        markSending(sessionId);
      }

      const pendingUserId = reusePendingId ?? `pending-user-${crypto.randomUUID()}`;
      if (!reusePendingId) {
        deps.inputText.value = '';
        deps.messageStore.setMessages(sessionId, [
          ...deps.messageStore.getMessages(sessionId),
          createPlaceholderMessage(pendingUserId, sessionId, 'user', text),
        ]);
      }

      const { provider, model } = strategy.resolveProviderModel();
      const selectedModel = deps.selectedProviderModel.value;
      const blockReason = shouldBlockAttachmentsForModel(
        { provider, model, capabilities: selectedModel?.capabilities },
        deps.attachments.value,
      );
      if (blockReason) {
        clearAttachments = false;
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
        startTurnAckTimeout(sessionId, pendingUserId);
      } catch (wsError) {
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
              knowledgeBases: deps.selectedKnowledgeBases.value,
            },
          );
          dropPendingUserRow(sessionId, pendingUserId);
          await deps.messageStore.loadMessages({ sessionId });
        } catch (httpError) {
          markPendingUserFailed(sessionId, pendingUserId, t('chat.sendFailedRetry', '发送失败，请点击重试'));
          if (!followUp) markSendingDone();
        }
      }
    } catch (error) {
      if (!(error instanceof DOMException && error.name === 'AbortError')) {
        const sid = strategy.resolveSessionId();
        if (sid) {
          const pendingId = deps.messageStore
            .getMessages(sid)
            .find((m) => m.content_markdown === text && m.id.startsWith('pending-user-'))?.id;
          if (pendingId) {
            markPendingUserFailed(sid, pendingId, t('chat.sendFailedRetry', '发送失败，请点击重试'));
          }
        }
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
    clearSendingTimeout,
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
