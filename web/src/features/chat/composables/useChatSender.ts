import { ref, computed, type Ref, type ComputedRef } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { useChatRuntimeStore } from "../../../stores/chat/runtimeStore";
import { useChatSessionStore } from "../../../stores/chat/sessionStore";
import { useChatMessageStore } from "../../../stores/chat/messageStore";
import { useAuthStore } from "../../../stores/auth";
import { useAppStore } from "../../../stores/app";
import { checkBackendHealth, getServerHeartbeatState } from "../../heartbeat/useServerHeartbeat";
import type { ChatAttachment, ChatEntityKind } from "../../../components/chat/types";
import type { UseEnvelopeStreamReturn } from "../useEnvelopeStream";
import type { WsUpstream } from "../envelope";
import { createPlaceholderMessage } from "../streamHandlers";
import { shouldBlockAttachmentsForModel } from "../modelCapabilities";
import { sendMessage } from "../api";
import { AWAIT_KIND_TOOL_CONFIRM } from "../awaitConstants";

import type { RunStatusValue } from "../types";

type SessionStore = ReturnType<typeof useChatSessionStore>;
type MessageStore = ReturnType<typeof useChatMessageStore>;

export function isFailedPendingMessage(id: string, registry?: Set<string>): boolean {
  return (registry ?? emptyFailedSet).has(id);
}

const emptyFailedSet = new Set<string>();

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
  selectedProviderModel: ComputedRef<{
    provider: string;
    model: string;
    capabilities?: { vision?: boolean; image?: boolean; text_only?: boolean };
  } | undefined>;
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

  let lastRunEventAt = 0;
  const RUN_STALL_TIMEOUT_MS = 180_000;
  const RUN_STALL_CHECK_INTERVAL_MS = 30_000;
  let stallCheckInterval: ReturnType<typeof setInterval> | null = null;

  const failedPendingIds = reactive(new Set<string>());

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
          type: "warning",
          message: t("chat.runStallWarning", "响应时间较长，请耐心等待或停止生成"),
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
        $q.notify({ type: "warning", message: t("chat.sendDispatchTimeout", "消息发送超时，请检查网络连接") });
      }
    }, SEND_DISPATCH_TIMEOUT_MS);
    startStallCheck();
  }

  function markSendingDone() {
    sending.value = false;
    clearSendingTimeout();
    clearStallCheck();
  }

  function clearSendingTimeout() {
    if (sendingTimeout != null) {
      clearTimeout(sendingTimeout);
      sendingTimeout = null;
    }
  }

  function touchRunActivity() {
    lastRunEventAt = Date.now();
  }

  function clearFailedPendingForSession() {
    failedPendingIds.clear();
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
        ? t("chat.backendUnavailableDev", "后端不可用，请确认 admin 是否在 :8000 运行（页面应使用 http://localhost:9001）")
        : t("chat.backendUnavailable", "后端服务不可用，请重新登录");
      $q.notify({ type: "negative", message: msg, timeout: import.meta.env.DEV ? 8000 : 0 });
      if (!import.meta.env.DEV) {
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: "login" });
      }
      return false;
    }
    return true;
  }

  function isActiveRun(): boolean {
    const s = deps.runStatus.value;
    return s === "running" || s === "pending";
  }

  const inputDisabled = computed(() => sending.value && !isActiveRun());

  async function onSend() {
    const content = deps.inputText.value.trim();
    if (!content) return;
    if (!isActiveRun() && sending.value) return;

    // P1-2: Distinguish await reply vs tool confirm
    if (deps.isAwaitingUser.value) {
      if (deps.awaitKind.value === AWAIT_KIND_TOOL_CONFIRM) {
        // Tool confirm should only use approve/deny buttons, ignore text input
        $q.notify({ type: "info", message: t("chat.toolConfirmUseButtons", "请使用批准或拒绝按钮来确认工具调用") });
        return;
      }
      await submitAwaitingReply();
      return;
    }

    if (deps.sessionStore.entityKind === "agent") {
      await sendAgentMessage(content);
    } else if (deps.sessionStore.entityKind === "team" && deps.sessionStore.selectedTeamId) {
      await sendTeamMessage(content);
    }
  }

  function dropPendingUserRow(sessionId: string, pendingUserId: string) {
    failedPendingIds.delete(pendingUserId);
    deps.messageStore.setMessages(
      sessionId,
      deps.messageStore.getMessages(sessionId).filter((m) => m.id !== pendingUserId)
    );
  }

  /** Mark a pending user message as failed (preserves the bubble for retry). */
  function markPendingUserFailed(sessionId: string, pendingUserId: string, errorMsg: string) {
    failedPendingIds.add(pendingUserId);
    const msgs = deps.messageStore.getMessages(sessionId);
    deps.messageStore.setMessages(
      sessionId,
      msgs.map((m) =>
        m.id === pendingUserId
          ? { ...m, status: "failed", error_message: errorMsg }
          : m
      )
    );
  }

  async function retryFailedMessage(pendingUserId: string) {
    const sid = deps.sessionStore.selectedSession?.id ?? deps.sessionStore.teamSelectedSessionId;
    if (!sid) return;
    const msgs = deps.messageStore.getMessages(sid);
    const failed = msgs.find((m) => m.id === pendingUserId);
    if (!failed || failed.status !== "failed") return;
    failedPendingIds.delete(pendingUserId);
    deps.messageStore.setMessages(
      sid,
      msgs.map((m) =>
        m.id === pendingUserId ? { ...m, status: "ok", error_message: "" } : m
      )
    );
    const entityKind = deps.sessionStore.entityKind;
    if (entityKind === "team") {
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
          type: "positive",
          message: res.queued
            ? t("chat.enqueueQueued", "Message queued for after the current run")
            : t("chat.enqueueAccepted", "Message will be injected at the next tool boundary"),
        });
        await deps.refreshPendingMessages?.();
        return;
      }
      // Backend rejected enqueue — likely no active run. Reset status and retry as new turn.
      dropPendingUserRow(sessionId, pendingUserId);
      deps.setRunStatus("idle");
      await sendAgentUserContent(content);
    } catch (err: unknown) {
      dropPendingUserRow(sessionId, pendingUserId);
      const errMessage = err instanceof Error ? err.message : "";
      // P1-3: Map backend error codes to user-friendly messages
      if (errMessage.includes("CHAT_RUN_ENDED")) {
        // Backend says run ended — reset status and retry as new turn
        deps.setRunStatus("idle");
        await sendAgentUserContent(content);
      } else if (errMessage.includes("CHAT_QUEUE_FULL")) {
        $q.notify({ type: "warning", message: t("chat.enqueueQueueFull", "排队消息已满，请稍后再试") });
      } else {
        $q.notify({
          type: "negative",
          message: err instanceof Error ? err.message : t("chat.enqueueRejected", "Could not enqueue message"),
        });
      }
    }
  }

  /** Send via HTTP as fallback when WS is disconnected. */
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
    }
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
      type: "warning",
      message: t("chat.imageModelUnsupported", "当前模型不支持图片理解，请移除图片附件或切换到支持视觉的模型"),
    });
  }

  async function sendAgentUserContent(content: string, reusePendingId?: string) {
    const text = content.trim();
    if (!text) return;
    const followUp = isActiveRun();
    if (!followUp) {
      markSending();
    }
    let clearAttachments = true;
    try {
      if (!deps.sessionStore.selectedSession) await deps.onNewSession(deps.makeSessionTitle(text));
      if (!deps.sessionStore.selectedSession) {
        $q.notify({ type: "negative", message: t("chat.sessionCreateFailed", "未创建会话或会话无效，请重试") });
        if (!followUp) markSendingDone();
        return;
      }
      const sessionId = deps.sessionStore.selectedSession.id;
      if (!followUp) {
        markSending(sessionId);
      }

      // Optimistic UI: clear input and show placeholder immediately so the user
      // sees their message without waiting for backend availability checks.
      // turn_id="" means "unassigned" — groupMessagesByTurn groups this with
      // the ws-stream row via turn_id until the server returns a persisted message.
      const pendingUserId = reusePendingId ?? `pending-user-${crypto.randomUUID()}`;
      if (!reusePendingId) {
        deps.inputText.value = "";
        deps.messageStore.setMessages(sessionId, [
          ...deps.messageStore.getMessages(sessionId),
          createPlaceholderMessage(pendingUserId, sessionId, "user", text),
        ]);
      }

      const selectedModel = deps.selectedProviderModel.value;
      const provider =
        selectedModel?.provider ||
        deps.sessionStore.selectedSession.provider ||
        deps.appStore.selectedAgent?.provider ||
        "";
      const model =
        selectedModel?.model ||
        deps.sessionStore.selectedSession.model ||
        deps.appStore.selectedAgent?.model ||
        "";
      const blockReason = shouldBlockAttachmentsForModel({ provider, model, capabilities: selectedModel?.capabilities }, deps.attachments.value);
      if (blockReason) {
        clearAttachments = false;
        if (blockReason === "ATTACHMENT_UNSUPPORTED") {
          $q.notify({ type: "warning", message: t("chat.fileModelUnsupported", "当前模型不支持文件附件，请移除附件或切换模型") });
        } else {
          notifyUnsupportedImageModel();
        }
        // Remove the optimistic placeholder since we're aborting the send.
        if (!reusePendingId) dropPendingUserRow(sessionId, pendingUserId);
        if (!followUp) markSendingDone();
        return;
      }

      if (!(await checkBackendAvailability())) {
        // P0-1: Mark as failed instead of removing
        markPendingUserFailed(sessionId, pendingUserId, t("chat.sendFailedBackend", "后端不可用"));
        if (!followUp) markSendingDone();
        return;
      }

      if (followUp) {
        await enqueueDuringRun(sessionId, text, pendingUserId);
        return;
      }

      const wsPayload: WsUpstream = {
        direction: "client_to_server",
        channel: "chat",
        type: "user_message",
        request_id: pendingUserId,
        payload: {
          session_id: sessionId,
          agent_key: deps.appStore.selectedAgent?.agent_key,
          content: text,
          options: {
            dialog_mode: deps.dialogMode.value,
            provider,
            model,
            attachments: deps.attachments.value.map((item) => ({ id: item.id })),
            knowledge_bases: deps.selectedKnowledgeBases.value,
          },
        },
      };

      // P0-2: Try WS first, fallback to HTTP if disconnected
      try {
        const stream = deps.ensureChatStream(sessionId);
        deps.sendChatViaWs(stream, wsPayload);
      } catch (wsError) {
        // WS unavailable — fallback to HTTP
        $q.notify({ type: "info", message: t("chat.wsFallbackHttp", "WebSocket 不可用，正在通过 HTTP 发送…") });
        try {
          await sendViaHttpFallback(
            sessionId,
            text,
            deps.appStore.selectedAgent?.agent_key,
            undefined,
            {
              dialogMode: deps.dialogMode.value,
              provider,
              model,
              attachments: deps.attachments.value,
              knowledgeBases: deps.selectedKnowledgeBases.value,
            }
          );
          // HTTP success — reload messages to get the persisted version
          dropPendingUserRow(sessionId, pendingUserId);
          await deps.messageStore.loadMessages({ sessionId });
        } catch (httpError) {
          markPendingUserFailed(sessionId, pendingUserId, t("chat.sendFailedRetry", "发送失败，请点击重试"));
          if (!followUp) markSendingDone();
        }
      }
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        const sid = deps.sessionStore.selectedSession?.id;
        if (sid) {
          // P0-1: Mark as failed instead of removing — user can retry
          const pendingId = sid && deps.messageStore.getMessages(sid).find((m) => m.content_markdown === text && m.id.startsWith("pending-user-"))?.id;
          if (pendingId) {
            markPendingUserFailed(sid, pendingId, t("chat.sendFailedRetry", "发送失败，请点击重试"));
          }
        }
        $q.notify({
          type: "negative",
          message: error instanceof Error ? error.message : t("chat.sendFailed", "发送失败，请稍后重试"),
        });
        if (!followUp) markSendingDone();
      }
    } finally {
      if (clearAttachments) {
        deps.attachments.value = [];
      }
    }
  }

  async function sendAgentMessage(content: string) {
    await sendAgentUserContent(content);
  }

  async function sendTeamMessage(content: string, reusePendingId?: string) {
    const followUp = isActiveRun();
    if (!followUp) {
      markSending();
    }
    let sessionIdForCatch = "";
    let clearAttachments = true;
    try {
      if (!deps.sessionStore.teamSelectedSessionId) await deps.onNewSession(deps.makeSessionTitle(content));
      const sessionId = deps.sessionStore.teamSelectedSessionId;
      if (!sessionId) {
        $q.notify({ type: "negative", message: t("chat.sessionCreateFailed", "未创建会话或会话无效，请重试") });
        if (!followUp) markSendingDone();
        return;
      }
      sessionIdForCatch = sessionId;
      if (!followUp) {
        markSending(sessionId);
      }

      const pendingUserId = reusePendingId ?? `pending-user-${crypto.randomUUID()}`;
      if (!reusePendingId) {
        deps.inputText.value = "";
        deps.messageStore.setMessages(sessionId, [
          ...deps.messageStore.getMessages(sessionId),
          createPlaceholderMessage(pendingUserId, sessionId, "user", content),
        ]);
      }

      const teamId = deps.sessionStore.selectedTeamId!;
      const session = deps.sessionStore.teamSessions[teamId]?.find((item) => item.id === sessionId);
      const selectedModel = deps.selectedProviderModel.value;
      const provider = selectedModel?.provider || session?.provider || "";
      const model = selectedModel?.model || session?.model || "";
      const blockReason = shouldBlockAttachmentsForModel({ provider, model, capabilities: selectedModel?.capabilities }, deps.attachments.value);
      if (blockReason) {
        clearAttachments = false;
        if (blockReason === "ATTACHMENT_UNSUPPORTED") {
          $q.notify({ type: "warning", message: t("chat.fileModelUnsupported", "当前模型不支持文件附件，请移除附件或切换模型") });
        } else {
          notifyUnsupportedImageModel();
        }
        // Remove the optimistic placeholder since we're aborting the send.
        dropPendingUserRow(sessionId, pendingUserId);
        if (!followUp) markSendingDone();
        return;
      }

      if (!(await checkBackendAvailability())) {
        // P0-1: Mark as failed instead of removing
        markPendingUserFailed(sessionId, pendingUserId, t("chat.sendFailedBackend", "后端不可用"));
        if (!followUp) markSendingDone();
        return;
      }

      if (followUp) {
        await enqueueDuringRun(sessionId, content, pendingUserId);
        return;
      }

      // P0-2: Try WS first, fallback to HTTP
      try {
        const stream = deps.ensureTeamStream(sessionId);
        deps.sendChatViaWs(stream, {
          direction: "client_to_server",
          channel: "chat",
          type: "user_message",
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
        });
      } catch (wsError) {
        $q.notify({ type: "info", message: t("chat.wsFallbackHttp", "WebSocket 不可用，正在通过 HTTP 发送…") });
        try {
          await sendViaHttpFallback(
            sessionId,
            content,
            undefined,
            deps.sessionStore.selectedTeamId ?? undefined,
            {
              dialogMode: deps.dialogMode.value,
              provider,
              model,
              attachments: deps.attachments.value,
              knowledgeBases: deps.selectedKnowledgeBases.value,
            }
          );
          dropPendingUserRow(sessionId, pendingUserId);
          await deps.messageStore.loadMessages({ sessionId });
        } catch (httpError) {
          markPendingUserFailed(sessionId, pendingUserId, t("chat.sendFailedRetry", "发送失败，请点击重试"));
          if (!followUp) markSendingDone();
        }
      }
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        $q.notify({ type: "negative", message: error instanceof Error ? error.message : t("chat.teamSendFailed", "Team 发送失败") });
        if (sessionIdForCatch) {
          const pendingId = deps.messageStore.getMessages(sessionIdForCatch).find(
            (m) => m.content_markdown === content && m.id.startsWith("pending-user-")
          )?.id;
          if (pendingId) {
            markPendingUserFailed(sessionIdForCatch, pendingId, t("chat.sendFailedRetry", "发送失败，请点击重试"));
          }
        }
      }
    } finally {
      if (clearAttachments) {
        deps.attachments.value = [];
      }
    }
  }

  return {
    sending,
    inputDisabled,
    markSending,
    markSendingDone,
    clearSendingTimeout,
    stopStreaming,
    submitAwaitingReply,
    submitToolConfirm: deps.submitToolConfirm,
    onSend,
    sendAgentUserContent,
    retryFailedMessage,
    touchRunActivity,
    clearFailedPendingForSession,
    failedPendingIds,
  };
}
