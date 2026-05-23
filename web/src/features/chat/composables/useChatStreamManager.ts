import { ref, type Ref } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { useAuthStore } from "../../../stores/auth";
import { useAppStore } from "../../../stores/app";
import {
  createChatStream,
  createTeamStream,
  type UseEnvelopeStreamReturn,
} from "../useEnvelopeStream";
import type { Envelope, EnvelopeType, WsUpstream } from "../envelope";
import { upsertToolMessage, cancelRunningToolMessages, finalizeOrphanToolMessages } from "../envelopeToolCall";
import { dropPendingUserPlaceholders } from "../mergeSessionMessages";
import { createMessageBatchWriter } from "../messageStoreBatch";
import { runStatusFromEnvelope } from "../envelopeRunStatus";
import { patchStreamingMessage } from "../streamContentPatch";
import type { RunStatusValue } from "../types";
import type { Message } from "../../../components/chat/types";
import type { TeamDefinition } from "../../teams/types";
import type { TeamRow } from "../../../components/chat/types";

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

export type StreamManagerDeps = {
  store: ReturnType<typeof useAppStore>;
  teamMessages: Ref<Record<string, Message[]>>;
  teamSelectedSessionId: Ref<string | null>;
  selectedTeamId: Ref<string | null>;
  displayTeams: Ref<TeamRow[]>;
  markSendingDone: () => void;
  onRunStatus: (env: Envelope) => void;
};

export function useChatStreamManager(deps: StreamManagerDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const router = useRouter();

  let chatStream: UseEnvelopeStreamReturn | null = null;
  let chatStreamSessionId: string | null = null;
  let teamStream: UseEnvelopeStreamReturn | null = null;
  let teamStreamSessionId: string | null = null;

  const wsReplaying = ref(false);
  const agentMessageWriter = createMessageBatchWriter(
    () => deps.store.messages,
    (rows) => {
      deps.store.messages = rows;
    }
  );

  function patchAgentMessagesBatched(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    agentMessageWriter.update((cur) => {
      const exists = cur.some((m) => m.id === streamId);
      let next = cur;
      if (!exists) {
        next = [
          ...cur,
          { ...createPlaceholderMessage(streamId, sessionId, "assistant", ""), status: "streaming" },
        ];
      }
      return patchStreamingMessage(next, streamId, {
        text: isDone ? undefined : env.content?.text,
        reasoning: isDone ? undefined : env.content?.reasoning,
        replaceText: isDone ? env.content?.text : undefined,
        replaceReasoning: isDone ? env.content?.reasoning : undefined,
        status: isDone ? "ok" : "streaming",
      });
    });
  }

  function ensureChatStream(sessionId: string) {
    if (chatStream && chatStream.transport.value && chatStreamSessionId === sessionId) {
      return chatStream;
    }
    chatStream?.disconnect();
    chatStream = createChatStream(sessionId, {
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

    chatStream.onType("text_delta", (env: Envelope) => {
      const sid = env.session_id || sessionId;
      if (sid !== sessionId || (!env.content?.text && !env.content?.reasoning)) return;
      patchAgentMessagesBatched(sid, `ws-stream-${sid}`, env, false);
    });
    chatStream.onType("text_done", (env: Envelope) => {
      const sid = env.session_id || sessionId;
      if (sid !== sessionId) return;
      patchAgentMessagesBatched(sid, `ws-stream-${sid}`, env, true);
    });
    chatStream.onType("tool_call", (env: Envelope) => {
      const sid = env.session_id || sessionId;
      if (sid !== sessionId || !env.tool_call) return;
      agentMessageWriter.update((cur) => upsertToolMessage(cur, sid, env, "before"));
    });
    chatStream.onType("tool_result", (env: Envelope) => {
      const sid = env.session_id || sessionId;
      if (sid !== sessionId || !env.tool_call) return;
      agentMessageWriter.update((cur) => upsertToolMessage(cur, sid, env, "after"));
    });
    chatStream.onType("run_status", (env: Envelope) => {
      if (env.session_id && env.session_id !== sessionId) return;
      deps.onRunStatus(env);
    });
    chatStream.onType("runner_completion", async () => {
      deps.markSendingDone();
      agentMessageWriter.flushSync();
      const sid = deps.store.selectedSession?.id;
      if (sid) {
        deps.store.messages = dropPendingUserPlaceholders(finalizeOrphanToolMessages(deps.store.messages));
        try {
          await deps.store.loadMessages();
          await deps.store.loadSessions();
        } catch { /* ignore */ }
      }
    });
    chatStream.onType("error", (env: Envelope) => {
      const errType = env.error?.type ?? "";
      if (errType.startsWith("flow_")) return;
      const msg = env.error?.message ?? "stream failed";
      $q.notify({ type: "negative", message: msg });
      if (env.request_id?.startsWith("pending-user-")) {
        deps.store.messages = deps.store.messages.filter((m) => m.id !== env.request_id);
      }
      deps.markSendingDone();
    });
    chatStream.onType("intent_pass", (env: Envelope) => {
      deps.store.lastIntentPass = env.metadata as any;
    });

    chatStream.connect();
    chatStreamSessionId = sessionId;
    return chatStream;
  }

  function ensureTeamStream(sessionId: string) {
    if (teamStream && teamStream.transport.value && teamStreamSessionId === sessionId) {
      return teamStream;
    }
    teamStream?.disconnect();
    teamStream = createTeamStream(sessionId, {
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

    teamStream.onType("text_delta", (env: Envelope) => {
      const sid = deps.teamSelectedSessionId.value;
      if (!sid || (!env.content?.text && !env.content?.reasoning)) return;
      patchTeamMessages(sid, `ws-team-stream-${sid}`, env, false);
    });
    teamStream.onType("text_done", (env: Envelope) => {
      const sid = deps.teamSelectedSessionId.value;
      if (!sid) return;
      patchTeamMessages(sid, `ws-team-stream-${sid}`, env, true);
    });
    teamStream.onType("tool_call", (env: Envelope) => {
      const sid = deps.teamSelectedSessionId.value;
      if (!sid || !env.tool_call) return;
      deps.teamMessages.value[sid] = upsertToolMessage(deps.teamMessages.value[sid] ?? [], sid, env, "before");
    });
    teamStream.onType("tool_result", (env: Envelope) => {
      const sid = deps.teamSelectedSessionId.value;
      if (!sid || !env.tool_call) return;
      deps.teamMessages.value[sid] = upsertToolMessage(deps.teamMessages.value[sid] ?? [], sid, env, "after");
    });
    teamStream.onType("run_status", (env: Envelope) => {
      if (env.session_id && env.session_id !== sessionId) return;
      deps.onRunStatus(env);
    });
    teamStream.onType("member_message_start", (env: Envelope) => {
      if (env.author && deps.teamSelectedSessionId.value) {
        const sid = deps.teamSelectedSessionId.value;
        const msgId = `member-${env.author}`;
        const cur = deps.teamMessages.value[sid] ?? [];
        if (!cur.some((m) => m.id === msgId)) {
          const meta = resolveTeamMemberMeta(env.author);
          deps.teamMessages.value[sid] = [
            ...cur,
            {
              ...createPlaceholderMessage(msgId, sid, "assistant", ""),
              status: "streaming",
              model_name: `team/${meta.role || "member"}`,
              options_json: JSON.stringify({ team_member: meta }),
            },
          ];
        }
      }
    });
    teamStream.onType("member_delta", (env: Envelope) => {
      if (env.author && deps.teamSelectedSessionId.value) {
        const sid = deps.teamSelectedSessionId.value;
        const msgId = `member-${env.author}`;
        deps.teamMessages.value[sid] = patchStreamingMessage(deps.teamMessages.value[sid] ?? [], msgId, {
          text: env.content?.text,
          reasoning: env.content?.reasoning,
        });
      }
    });
    teamStream.onType("member_message_done", (env: Envelope) => {
      if (env.author && deps.teamSelectedSessionId.value) {
        const sid = deps.teamSelectedSessionId.value;
        const msgId = `member-${env.author}`;
        deps.teamMessages.value[sid] = patchStreamingMessage(deps.teamMessages.value[sid] ?? [], msgId, {
          replaceText: env.content?.text,
          replaceReasoning: env.content?.reasoning,
          status: "ok",
        });
      }
    });
    teamStream.onType("runner_completion", async () => {
      deps.markSendingDone();
      if (deps.teamSelectedSessionId.value) {
        const sid = deps.teamSelectedSessionId.value;
        deps.teamMessages.value[sid] = dropPendingUserPlaceholders(
          finalizeOrphanToolMessages(deps.teamMessages.value[sid] ?? [])
        );
        try {
          const { listSessionChatMessages: listMessages } = await import("../../session/api");
          deps.teamMessages.value[sid] = await listMessages(sid);
        } catch { /* keep assembled rows */ }
      }
    });
    teamStream.onType("error", (env: Envelope) => {
      const errType = env.error?.type ?? "";
      if (errType.startsWith("flow_")) return;
      const msg = env.error?.message ?? "stream failed";
      $q.notify({ type: "negative", message: msg });
      const sid = deps.teamSelectedSessionId.value;
      if (sid && env.request_id?.startsWith("pending-user-")) {
        deps.teamMessages.value[sid] = (deps.teamMessages.value[sid] ?? []).filter((m) => m.id !== env.request_id);
      }
      deps.markSendingDone();
    });
    teamStream.onType("intent_pass", (env: Envelope) => {
      deps.store.lastIntentPass = env.metadata as any;
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
    chatStream?.disconnect();
    chatStream = null;
    chatStreamSessionId = null;
  }

  function disconnectTeamStream() {
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
    const exists = deps.store.messages.some((m) => m.id === streamId);
    if (!exists) {
      deps.store.messages = [
        ...deps.store.messages,
        { ...createPlaceholderMessage(streamId, sessionId, "assistant", ""), status: "streaming" },
      ];
    }
    deps.store.messages = patchStreamingMessage(deps.store.messages, streamId, {
      text: isDone ? undefined : env.content?.text,
      reasoning: isDone ? undefined : env.content?.reasoning,
      replaceText: isDone ? env.content?.text : undefined,
      replaceReasoning: isDone ? env.content?.reasoning : undefined,
      status: isDone ? "ok" : "streaming",
    });
  }

  function patchTeamMessages(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    const cur = deps.teamMessages.value[sessionId] ?? [];
    const exists = cur.some((m) => m.id === streamId);
    if (!exists) {
      deps.teamMessages.value[sessionId] = [
        ...cur,
        { ...createPlaceholderMessage(streamId, sessionId, "assistant", ""), status: "streaming" },
      ];
    }
    deps.teamMessages.value[sessionId] = patchStreamingMessage(deps.teamMessages.value[sessionId] ?? [], streamId, {
      text: isDone ? undefined : env.content?.text,
      reasoning: isDone ? undefined : env.content?.reasoning,
      replaceText: isDone ? env.content?.text : undefined,
      replaceReasoning: isDone ? env.content?.reasoning : undefined,
      status: isDone ? "ok" : "streaming",
    });
  }

  function resolveTeamMemberMeta(agentKey: string) {
    const team = deps.displayTeams.value.find((row) => row.id === deps.selectedTeamId.value);
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
