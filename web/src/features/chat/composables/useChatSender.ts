import { ref, computed, type Ref, type ComputedRef } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { stopGeneration } from "../api";
import { useChatRuntimeStore } from "../../../stores/chat/runtimeStore";
import { useAuthStore } from "../../../stores/auth";
import { useAppStore } from "../../../stores/app";
import { useChatStore } from "../../../stores/chat";
import { checkBackendHealth, getServerHeartbeatState } from "../../heartbeat/useServerHeartbeat";
import type { ChatAttachment, ChatEntityKind } from "../../../components/chat/types";
import type { UseEnvelopeStreamReturn } from "../useEnvelopeStream";
import type { WsUpstream } from "../envelope";
import { createPlaceholderMessage } from "../streamHandlers";

export type SenderDeps = {
  appStore: ReturnType<typeof useAppStore>;
  chatStore: ReturnType<typeof useChatStore>;
  inputText: Ref<string>;
  dialogMode: Ref<string>;
  attachments: Ref<ChatAttachment[]>;
  isAwaitingUser: Ref<boolean>;
  awaitingRunId: Ref<string>;
  runStatus: Ref<string>;
  selectedProviderModel: ComputedRef<{ provider: string; model: string } | undefined>;
  selectedKnowledgeBases: Ref<string[]>;
  ensureChatStream: (sessionId: string) => UseEnvelopeStreamReturn;
  ensureTeamStream: (sessionId: string) => UseEnvelopeStreamReturn;
  sendChatViaWs: (stream: UseEnvelopeStreamReturn, upstream: WsUpstream) => void;
  onNewSession: (title?: string) => Promise<void>;
  makeSessionTitle: (content: string) => string;
  refreshRunStatus: () => Promise<void>;
  submitAwaitingReply: () => Promise<void>;
  submitToolConfirm: (approved: boolean) => Promise<void>;
  refreshPendingMessages?: () => Promise<void>;
};

export function useChatSender(deps: SenderDeps) {
  const $q = useQuasar();
  const router = useRouter();
  const { t } = useI18n();

  const sending = ref(false);
  let sendingTimeout: ReturnType<typeof setTimeout> | null = null;
  const SENDING_TIMEOUT_MS = 120_000;

  function markSending(sessionId?: string) {
    sending.value = true;
    clearSendingTimeout();
    sendingTimeout = setTimeout(() => {
      if (sending.value) {
        sending.value = false;
        if (sessionId) {
          stopGeneration(sessionId);
        }
        $q.notify({ type: "warning", message: "响应超时，已自动取消生成" });
      }
    }, SENDING_TIMEOUT_MS);
  }

  function markSendingDone() {
    sending.value = false;
    clearSendingTimeout();
  }

  function clearSendingTimeout() {
    if (sendingTimeout != null) {
      clearTimeout(sendingTimeout);
      sendingTimeout = null;
    }
  }

  function stopStreaming(sessionId?: string) {
    if (sessionId) {
      stopGeneration(sessionId);
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
        ? "后端不可用，请确认 admin 是否在 :8000 运行（页面应使用 http://localhost:9001）"
        : "后端服务不可用，请重新登录";
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

    if (deps.isAwaitingUser.value) {
      await submitAwaitingReply();
      return;
    }

    if (deps.chatStore.entityKind === "agent") {
      await sendAgentMessage(content);
    } else if (deps.chatStore.entityKind === "team" && deps.chatStore.selectedTeamId) {
      await sendTeamMessage(content);
    }
  }

  function dropPendingUserRow(sessionId: string, pendingUserId: string) {
    deps.chatStore.setMessages(
      sessionId,
      deps.chatStore.getMessages(sessionId).filter((m) => m.id !== pendingUserId)
    );
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
      dropPendingUserRow(sessionId, pendingUserId);
      $q.notify({ type: "warning", message: t("chat.enqueueRejected", "Could not enqueue message") });
    } catch (err) {
      dropPendingUserRow(sessionId, pendingUserId);
      $q.notify({
        type: "negative",
        message: err instanceof Error ? err.message : t("chat.enqueueRejected", "Could not enqueue message"),
      });
    }
  }

  /** Send user text (or A2UI userAction JSONL) on the active agent session via WS. */
  async function sendAgentUserContent(content: string) {
    const text = content.trim();
    if (!text) return;
    const followUp = isActiveRun();
    if (!followUp) {
      markSending();
    }
    try {
      if (!deps.chatStore.selectedSession) await deps.onNewSession(deps.makeSessionTitle(text));
      if (!deps.chatStore.selectedSession) {
        $q.notify({ type: "negative", message: "未创建会话或会话无效，请重试" });
        if (!followUp) markSendingDone();
        return;
      }
      const sessionId = deps.chatStore.selectedSession.id;
      if (!followUp) {
        markSending(sessionId);
      }
      const selectedModel = deps.selectedProviderModel.value;

      const pendingUserId = `pending-user-${crypto.randomUUID()}`;
      deps.chatStore.setMessages(sessionId, [
        ...deps.chatStore.getMessages(sessionId),
        createPlaceholderMessage(pendingUserId, sessionId, "user", text),
      ]);

      if (!(await checkBackendAvailability())) {
        deps.chatStore.setMessages(
          sessionId,
          deps.chatStore.getMessages(sessionId).filter((m) => !String(m.id).startsWith("pending-user-"))
        );
        if (!followUp) markSendingDone();
        return;
      }

      if (followUp) {
        await enqueueDuringRun(sessionId, text, pendingUserId);
        return;
      }

      const stream = deps.ensureChatStream(sessionId);
      deps.sendChatViaWs(stream, {
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
            provider:
              selectedModel?.provider ||
              deps.chatStore.selectedSession.provider ||
              deps.appStore.selectedAgent?.provider ||
              "",
            model:
              selectedModel?.model ||
              deps.chatStore.selectedSession.model ||
              deps.appStore.selectedAgent?.model ||
              "",
            attachments: deps.attachments.value.map((item) => ({ id: item.id })),
            knowledge_bases: deps.selectedKnowledgeBases.value,
          },
        },
      });
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        $q.notify({
          type: "negative",
          message: error instanceof Error ? error.message : "发送失败，请稍后重试",
        });
        const sid = deps.chatStore.selectedSession?.id;
        if (sid) {
          deps.chatStore.setMessages(
            sid,
            deps.chatStore.getMessages(sid).filter((m) => !String(m.id).startsWith("pending-user-"))
          );
          try {
            await deps.chatStore.loadMessages({ sessionId: sid });
            const agentId = deps.appStore.selectedAgent?.id;
            if (agentId) await deps.chatStore.loadAgentSessions(agentId, { refreshOnly: true });
          } catch {
            /* ignore reload failure */
          }
        }
        if (!followUp) markSendingDone();
      }
    } finally {
      deps.attachments.value = [];
    }
  }

  async function sendAgentMessage(content: string) {
    deps.inputText.value = "";
    await sendAgentUserContent(content);
  }

  async function sendTeamMessage(content: string) {
    const followUp = isActiveRun();
    if (!followUp) {
      markSending();
    }
    let sessionIdForCatch = "";
    try {
      if (!deps.chatStore.teamSelectedSessionId) await deps.onNewSession(deps.makeSessionTitle(content));
      const sessionId = deps.chatStore.teamSelectedSessionId;
      if (!sessionId) {
        $q.notify({ type: "negative", message: "未创建会话或会话无效，请重试" });
        if (!followUp) markSendingDone();
        return;
      }
      sessionIdForCatch = sessionId;
      if (!followUp) {
        markSending(sessionId);
      }
      const teamId = deps.chatStore.selectedTeamId!;
      const session = deps.chatStore.teamSessions[teamId]?.find((item) => item.id === sessionId);
      const selectedModel = deps.selectedProviderModel.value;
      deps.inputText.value = "";

      const pendingUserId = `pending-user-${crypto.randomUUID()}`;
      deps.chatStore.setMessages(sessionId, [
        ...deps.chatStore.getMessages(sessionId),
        createPlaceholderMessage(pendingUserId, sessionId, "user", content),
      ]);

      if (!(await checkBackendAvailability())) {
        deps.chatStore.setMessages(
          sessionId,
          deps.chatStore.getMessages(sessionId).filter((item) => !String(item.id).startsWith("pending-user-"))
        );
        if (!followUp) markSendingDone();
        return;
      }

      if (followUp) {
        await enqueueDuringRun(sessionId, content, pendingUserId);
        return;
      }

      const stream = deps.ensureTeamStream(sessionId);
      deps.sendChatViaWs(stream, {
        direction: "client_to_server",
        channel: "chat",
        type: "user_message",
        request_id: pendingUserId,
        payload: {
          session_id: sessionId,
          team_id: deps.chatStore.selectedTeamId,
          content,
          options: {
            dialog_mode: deps.dialogMode.value,
            provider: selectedModel?.provider || session?.provider || "",
            model: selectedModel?.model || session?.model || "",
            attachments: deps.attachments.value.map((item) => ({ id: item.id })),
            knowledge_bases: deps.selectedKnowledgeBases.value,
          },
        },
      });
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        $q.notify({ type: "negative", message: error instanceof Error ? error.message : "Team 发送失败" });
      }
      if (sessionIdForCatch) {
        deps.chatStore.setMessages(
          sessionIdForCatch,
          deps.chatStore.getMessages(sessionIdForCatch).filter((item) => !String(item.id).startsWith("pending-user-"))
        );
        try {
          await deps.chatStore.loadMessages({ sessionId: sessionIdForCatch });
        } catch {
          /* ignore */
        }
      }
    } finally {
      deps.attachments.value = [];
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
  };
}
