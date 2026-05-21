import { ref, computed, type Ref, type ComputedRef } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { stopGeneration } from "../api";
import { useAuthStore } from "../../../stores/auth";
import { useAppStore } from "../../../stores/app";
import { checkBackendHealth, getServerHeartbeatState } from "../../heartbeat/useServerHeartbeat";
import { useAwaitReply } from "./useAwaitReply";
import type { Message, ChatAttachment, ChatEntityKind } from "../../../components/chat/types";
import type { UseEnvelopeStreamReturn } from "../useEnvelopeStream";
import type { WsUpstream } from "../envelope";

function createPlaceholderMessage(id: string, sessionID: string, role: string, content: string): Message {
  return {
    id,
    session_id: sessionID,
    parent_message_id: "",
    turn_index: 1,
    role,
    content_markdown: content,
    model_name: "mock",
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: "ok",
    attachments_count: 0,
    options_json: "",
    error_message: "",
    created_at: new Date().toISOString(),
  };
}

export type SenderDeps = {
  store: ReturnType<typeof useAppStore>;
  selectedEntityKind: Ref<ChatEntityKind>;
  selectedTeamId: Ref<string | null>;
  teamSelectedSessionId: Ref<string | null>;
  teamMessages: Ref<Record<string, Message[]>>;
  inputText: Ref<string>;
  dialogMode: Ref<string>;
  attachments: Ref<ChatAttachment[]>;
  isAwaitingUser: Ref<boolean>;
  awaitingRunId: Ref<string>;
  runStatus: Ref<string>;
  selectedProviderModel: ComputedRef<{ provider: string; model: string } | undefined>;
  ensureChatStream: (sessionId: string) => UseEnvelopeStreamReturn;
  ensureTeamStream: (sessionId: string) => UseEnvelopeStreamReturn;
  sendChatViaWs: (stream: UseEnvelopeStreamReturn, upstream: WsUpstream) => void;
  onNewSession: (title?: string) => Promise<void>;
  makeSessionTitle: (content: string) => string;
  refreshRunStatus: () => Promise<void>;
  loadTeamSessions: (teamId: string) => Promise<void>;
  teamSessions: Ref<Record<string, Array<{ id: string; provider?: string; model?: string }>>>;
};

export function useChatSender(deps: SenderDeps) {
  const $q = useQuasar();
  const router = useRouter();

  const selectedSessionId = computed(() => deps.store.selectedSession?.id);

  const awaitReply = useAwaitReply({
    selectedTeamId: deps.selectedTeamId,
    teamSelectedSessionId: deps.teamSelectedSessionId,
    selectedSessionId,
    inputText: deps.inputText,
    isAwaitingUser: deps.isAwaitingUser,
    awaitingRunId: deps.awaitingRunId,
    runStatus: deps.runStatus,
    refreshRunStatus: deps.refreshRunStatus,
  });

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
    await awaitReply.submitAwaitingReply();
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

    if (deps.selectedEntityKind.value === "agent") {
      await sendAgentMessage(content);
    } else if (deps.selectedEntityKind.value === "team" && deps.selectedTeamId.value) {
      await sendTeamMessage(content);
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
      if (!deps.store.selectedSession) await deps.onNewSession(deps.makeSessionTitle(text));
      if (!deps.store.selectedSession) {
        $q.notify({ type: "negative", message: "未创建会话或会话无效，请重试" });
        if (!followUp) markSendingDone();
        return;
      }
      const sessionId = deps.store.selectedSession.id;
      if (!followUp) {
        markSending(sessionId);
      }
      const selectedModel = deps.selectedProviderModel.value;

      const pendingUserId = `pending-user-${crypto.randomUUID()}`;
      deps.store.messages = [
        ...deps.store.messages,
        createPlaceholderMessage(pendingUserId, sessionId, "user", text),
      ];

      if (!(await checkBackendAvailability())) {
        deps.store.messages = deps.store.messages.filter((m) => !String(m.id).startsWith("pending-user-"));
        if (!followUp) markSendingDone();
        return;
      }

      const stream = deps.ensureChatStream(sessionId);
      deps.sendChatViaWs(stream, {
        direction: "client_to_server",
        channel: "chat",
        type: "user_message",
        payload: {
          session_id: sessionId,
          agent_key: deps.store.selectedAgent?.agent_key,
          content: text,
          options: {
            dialog_mode: deps.dialogMode.value,
            provider: selectedModel?.provider || deps.store.selectedSession.provider || deps.store.selectedAgent?.provider || "",
            model: selectedModel?.model || deps.store.selectedSession.model || deps.store.selectedAgent?.model || "",
            attachments: deps.attachments.value.map((item) => ({ id: item.id })),
          },
        },
      });
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        $q.notify({
          type: "negative",
          message: error instanceof Error ? error.message : "发送失败，请稍后重试",
        });
        deps.store.messages = deps.store.messages.filter((m) => !String(m.id).startsWith("pending-user-"));
        if (deps.store.selectedSession) {
          try {
            await deps.store.loadMessages();
            await deps.store.loadSessions();
          } catch { /* ignore */ }
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
      if (!deps.teamSelectedSessionId.value) await deps.onNewSession(deps.makeSessionTitle(content));
      const sessionId = deps.teamSelectedSessionId.value;
      if (!sessionId) {
        $q.notify({ type: "negative", message: "未创建会话或会话无效，请重试" });
        if (!followUp) markSendingDone();
        return;
      }
      sessionIdForCatch = sessionId;
      if (!followUp) {
        markSending(sessionId);
      }
      const session = deps.teamSessions.value[deps.selectedTeamId.value!]?.find((item) => item.id === sessionId);
      const selectedModel = deps.selectedProviderModel.value;
      deps.inputText.value = "";

      const pendingUserId = `pending-user-${crypto.randomUUID()}`;
      deps.teamMessages.value[sessionId] = [
        ...(deps.teamMessages.value[sessionId] ?? []),
        createPlaceholderMessage(pendingUserId, sessionId, "user", content),
      ];

      if (!(await checkBackendAvailability())) {
        const cur = deps.teamMessages.value[sessionId] ?? [];
        deps.teamMessages.value[sessionId] = cur.filter((item) => !String(item.id).startsWith("pending-user-"));
        if (!followUp) markSendingDone();
        return;
      }

      const stream = deps.ensureTeamStream(sessionId);
      deps.sendChatViaWs(stream, {
        direction: "client_to_server",
        channel: "chat",
        type: "user_message",
        payload: {
          session_id: sessionId,
          team_id: deps.selectedTeamId.value,
          content,
          options: {
            dialog_mode: deps.dialogMode.value,
            provider: selectedModel?.provider || session?.provider || "",
            model: selectedModel?.model || session?.model || "",
            attachments: deps.attachments.value.map((item) => ({ id: item.id })),
          },
        },
      });
    } catch (error) {
      if (!(error instanceof DOMException && error.name === "AbortError")) {
        $q.notify({ type: "negative", message: error instanceof Error ? error.message : "Team 发送失败" });
      }
      if (sessionIdForCatch) {
        const cur = deps.teamMessages.value[sessionIdForCatch] ?? [];
        deps.teamMessages.value[sessionIdForCatch] = cur.filter((item) => !String(item.id).startsWith("pending-user-"));
      }
    } finally {
      if (!followUp) {
        markSendingDone();
      }
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
    submitToolConfirm: awaitReply.submitToolConfirm,
    onSend,
    sendAgentUserContent,
  };
}
