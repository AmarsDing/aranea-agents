import { ref, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { useChatStreamingSnapshots } from "../../../stores/chatStreamingSnapshots";
import { useAuthStore } from "../../../stores/auth";
import { useChatStore } from "../../../stores/chat";
import {
  createChatStream,
  createTeamStream,
  type UseEnvelopeStreamReturn,
} from "../useEnvelopeStream";
import type { Envelope, EnvelopeType, WsUpstream } from "../envelope";
import { bindStreamHandlers, patchStreamingEnvelope } from "../streamHandlers";
import type { TeamRow } from "../../../components/chat/types";
import type { TeamDefinition } from "../../teams/types";

export type StreamManagerDeps = {
  chatStore: ReturnType<typeof useChatStore>;
  displayTeams: Ref<TeamRow[]>;
  resolveAgentId: () => string | undefined;
  markSendingDone: () => void;
  onRunStatus: (env: Envelope) => void;
};

export function useChatStreamManager(deps: StreamManagerDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const router = useRouter();
  const streamingSnapshots = useChatStreamingSnapshots();

  let chatStream: UseEnvelopeStreamReturn | null = null;
  let chatStreamSessionId: string | null = null;
  let teamStream: UseEnvelopeStreamReturn | null = null;
  let teamStreamSessionId: string | null = null;

  const wsReplaying = ref(false);

  function notifyError(message: string) {
    $q.notify({ type: "negative", message });
  }

  function notifyOrchestration(message: string) {
    $q.notify({ type: "info", message, timeout: 4000, group: false });
  }

  async function reloadAgentAfterCompletion(sessionId: string) {
    try {
      const rev = deps.chatStore.sessionRevisionBySession[sessionId] ?? 0;
      if (rev > 0) {
        await deps.chatStore.loadMessages({ sessionId, afterRevision: rev, dropStaleInFlight: true });
      } else {
        await deps.chatStore.loadMessages({ sessionId, replace: true, dropStaleInFlight: true });
      }
      streamingSnapshots.clear(sessionId);
      if (deps.chatStore.entityKind === "agent") {
        const agentId = deps.resolveAgentId();
        if (agentId) await deps.chatStore.loadAgentSessions(agentId, { refreshOnly: true });
      }
    } catch (err) {
      notifyError(err instanceof Error ? err.message : t("chat.loadMessagesFailed", "加载消息失败"));
    }
  }

  async function reloadTeamAfterCompletion(sessionId: string) {
    try {
      const rev = deps.chatStore.sessionRevisionBySession[sessionId] ?? 0;
      if (rev > 0) {
        await deps.chatStore.loadMessages({ sessionId, afterRevision: rev });
      } else {
        await deps.chatStore.loadMessages({ sessionId, replace: true });
      }
    } catch (err) {
      notifyError(err instanceof Error ? err.message : t("chat.loadMessagesFailed", "加载消息失败"));
    }
  }

  function resolveTeamMemberMeta(agentKey: string) {
    const team = deps.displayTeams.value.find((row) => row.id === deps.chatStore.selectedTeamId);
    let def: TeamDefinition | null = null;
    try {
      def = team?.definition_json ? (JSON.parse(team.definition_json) as TeamDefinition) : null;
    } catch {
      def = null;
    }
    const member = def?.members?.find((m) => m.agent_id === agentKey || m.name === agentKey);
    return {
      agent_key: agentKey,
      name: member?.name || agentKey,
      role: member?.role || "",
    };
  }

  function ensureChatStream(sessionId: string) {
    if (chatStream && chatStream.transport.value && chatStreamSessionId === sessionId) {
      deps.chatStore.setWsConnected(sessionId, chatStream.connected.value);
      return chatStream;
    }
    chatStream?.disconnect();
    deps.chatStore.setWsConnected(sessionId, false);

    chatStream = createChatStream(sessionId, {
      onConnected: () => {
        deps.chatStore.setWsConnected(sessionId, true);
      },
      onDisconnected: () => {
        deps.chatStore.setWsConnected(sessionId, false);
      },
      onServerShutdown: () => {
        $q.notify({
          type: "warning",
          message: t("chat.serverShutdown", "服务器已关闭，请重新登录"),
          timeout: 0,
          actions: [{ label: t("chat.relogin", "重新登录"), color: "white", handler: () => {} }],
        });
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: "login" });
      },
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
      onReconnectFailed: () => {
        $q.notify({
          type: "negative",
          message: t("chat.reconnectFailed", "连接已断开，请刷新页面重试"),
          timeout: 0,
          actions: [{ label: t("chat.refresh", "刷新页面"), color: "white", handler: () => window.location.reload() }],
        });
      },
    });

    bindStreamHandlers(
      chatStream,
      {
        sessionId,
        resolveActiveSessionId: () => deps.chatStore.selectedSession?.id ?? null,
        getMessages: (sid) => deps.chatStore.getMessages(sid),
        setMessages: (sid, rows) => deps.chatStore.setMessages(sid, rows),
        markSendingDone: deps.markSendingDone,
        onRunStatus: deps.onRunStatus,
        onErrorNotify: notifyError,
        onOrchestrationNotice: notifyOrchestration,
        onReloadAfterCompletion: reloadAgentAfterCompletion,
        setLastIntentPass: (value) => {
          deps.chatStore.lastIntentPass = value;
        },
        onStreamingPatch: (sid, patch) => {
          if (patch.done) {
            streamingSnapshots.put(sid, {
              reasoning: patch.reasoning,
              partialText: patch.partialText,
              replace: true,
            });
            return;
          }
          streamingSnapshots.put(sid, {
            reasoning: patch.reasoning,
            partialText: patch.partialText,
          });
        },
      },
      { batched: true }
    );

    chatStream.connect();
    chatStreamSessionId = sessionId;
    return chatStream;
  }

  function ensureTeamStream(sessionId: string) {
    if (teamStream && teamStream.transport.value && teamStreamSessionId === sessionId) {
      deps.chatStore.setWsConnected(sessionId, teamStream.connected.value);
      return teamStream;
    }
    teamStream?.disconnect();
    deps.chatStore.setWsConnected(sessionId, false);

    teamStream = createTeamStream(sessionId, {
      onConnected: () => {
        deps.chatStore.setWsConnected(sessionId, true);
      },
      onDisconnected: () => {
        deps.chatStore.setWsConnected(sessionId, false);
      },
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
      onReconnectFailed: () => {
        $q.notify({
          type: "negative",
          message: t("chat.reconnectFailed", "连接已断开，请刷新页面重试"),
          timeout: 0,
          actions: [{ label: t("chat.refresh", "刷新页面"), color: "white", handler: () => window.location.reload() }],
        });
      },
    });

    bindStreamHandlers(teamStream, {
      sessionId,
      streamIdPrefix: "ws-team-stream",
      resolveActiveSessionId: () => deps.chatStore.teamSelectedSessionId,
      getMessages: (sid) => deps.chatStore.getMessages(sid),
      setMessages: (sid, rows) => deps.chatStore.setMessages(sid, rows),
      markSendingDone: deps.markSendingDone,
      onRunStatus: deps.onRunStatus,
      onErrorNotify: notifyError,
      onOrchestrationNotice: notifyOrchestration,
      onReloadAfterCompletion: reloadTeamAfterCompletion,
      setLastIntentPass: (value) => {
        deps.chatStore.lastIntentPass = value;
      },
      resolveMemberMeta: resolveTeamMemberMeta,
    });

    teamStream.connect();
    teamStreamSessionId = sessionId;
    return teamStream;
  }

  function sendChatViaWs(stream: UseEnvelopeStreamReturn, upstream: WsUpstream): void {
    stream.connect();
    const transport = stream.transport.value;
    if (!transport) {
      throw new Error("WebSocket transport unavailable");
    }
    transport.send(upstream);
  }

  function disconnectChatStream() {
    if (chatStreamSessionId) {
      deps.chatStore.setWsConnected(chatStreamSessionId, false);
    }
    chatStream?.disconnect();
    chatStream = null;
    chatStreamSessionId = null;
  }

  function disconnectTeamStream() {
    if (teamStreamSessionId) {
      deps.chatStore.setWsConnected(teamStreamSessionId, false);
    }
    teamStream?.disconnect();
    teamStream = null;
    teamStreamSessionId = null;
  }

  function disconnectAll() {
    disconnectChatStream();
    disconnectTeamStream();
  }

  function cancelActiveStream() {
    chatStream?.cancel();
    teamStream?.cancel();
  }

  function patchAgentMessages(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    deps.chatStore.setMessages(
      sessionId,
      patchStreamingEnvelope(deps.chatStore.getMessages(sessionId), sessionId, streamId, env, isDone)
    );
  }

  function patchTeamMessages(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    patchAgentMessages(sessionId, streamId, env, isDone);
  }

  function subscribeSessionStream(
    sessionId: string,
    ownerKind: "agent" | "team",
    types: EnvelopeType[],
    handler: (env: Envelope) => void,
  ): () => void {
    const stream = ownerKind === "team" ? ensureTeamStream(sessionId) : ensureChatStream(sessionId);
    return stream.onType(types, handler);
  }

  return {
    wsReplaying,
    ensureChatStream,
    ensureTeamStream,
    subscribeSessionStream,
    sendChatViaWs,
    disconnectChatStream,
    disconnectTeamStream,
    disconnectAll,
    cancelActiveStream,
    patchAgentMessages,
    patchTeamMessages,
    resolveTeamMemberMeta,
  };
}
