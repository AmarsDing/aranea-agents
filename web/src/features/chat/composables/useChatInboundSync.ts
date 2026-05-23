import { onMounted, onUnmounted, type Ref } from "vue";
import {
  acquireGlobalWsConsumer,
  releaseGlobalWsConsumer,
} from "../globalWsHub";
import type { Envelope } from "../envelope";
import type { UseEnvelopeStreamReturn } from "../useEnvelopeStream";
import type { useAppStore } from "../../../stores/app";
import type { useChatStore } from "../../../stores/chat";
import { useChatStreamingSnapshots } from "../../../stores/chatStreamingSnapshots";
import { runStatusFromEnvelope } from "../envelopeRunStatus";
import {
  upsertToolMessage,
  finalizeOrphanToolMessages,
} from "../envelopeToolCall";
import { dropPendingUserPlaceholders } from "../mergeSessionMessages";
import { createMessageBatchWriter } from "../messageStoreBatch";
import { refreshAgentSessionsForChannel } from "../channelInboundSessionRefresh";

export type ChatInboundSyncDeps = {
  appStore: ReturnType<typeof useAppStore>;
  chatStore: ReturnType<typeof useChatStore>;
  selectedAgentId: Ref<string | undefined>;
  selectedSessionId: Ref<string | undefined>;
  wsReplaying?: Ref<boolean>;
  isChatRoute?: () => boolean;
  shouldAutoFocusChannel?: () => boolean;
  onTurnComplete?: (sessionId: string) => void;
  focusChannelSession?: (sessionId: string, agentId: string) => void | Promise<void>;
  ensureChatStream: (sessionId: string) => UseEnvelopeStreamReturn;
  ensureTeamStream: (sessionId: string) => UseEnvelopeStreamReturn;
  patchAgentMessages: (sessionId: string, streamId: string, env: Envelope, isDone: boolean) => void;
  patchTeamMessages: (sessionId: string, streamId: string, env: Envelope, isDone: boolean) => void;
  loadTeamSessions?: (teamId: string) => Promise<void>;
};

const HYDRATE_DEBOUNCE_MS = 200;

function envelopeSessionRevision(env: Envelope): number {
  if (typeof env.session_revision === "number" && env.session_revision > 0) {
    return env.session_revision;
  }
  const md = env.metadata as Record<string, unknown> | undefined;
  const fromMeta = md?.session_revision;
  if (typeof fromMeta === "number" && fromMeta > 0) return fromMeta;
  return 0;
}

function envelopeSource(env: Envelope): string {
  const direct = (env.source ?? "").trim();
  if (direct) return direct;
  const md = env.metadata as Record<string, unknown> | undefined;
  return typeof md?.source === "string" ? md.source.trim() : "";
}

function isTurnCompleteEnvelope(env: Envelope): boolean {
  if (env.type === "runner_completion") return true;
  if (env.type !== "run_status") return false;
  const status = runStatusFromEnvelope(env)?.status;
  return status === "completed" || status === "failed" || status === "cancelled";
}

/**
 * Subscribes to global WS (`session_id=*`) so Channel/Cron inbound turns update the Chat UI
 * when the matching agent session is open or newly created.
 * Header bell notifications are handled by useGlobalInboundNotifications (MainLayout).
 */
export function useChatInboundSync(deps: ChatInboundSyncDeps) {
  const streamingSnapshots = useChatStreamingSnapshots();
  let hubId: string | null = null;
  let hydrateTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingHydrateSessionId = "";

  const inboundMessageWriter = createMessageBatchWriter(
    () => {
      const sid = deps.selectedSessionId.value;
      return sid ? deps.chatStore.getMessages(sid) : [];
    },
    (rows) => {
      const sid = deps.selectedSessionId.value;
      if (sid) deps.chatStore.setMessages(sid, rows);
    }
  );

  function agentIdFromEnvelope(env: Envelope): string {
    const md = env.metadata as Record<string, unknown> | undefined;
    const fromMeta = typeof md?.agent_id === "string" ? md.agent_id.trim() : "";
    if (fromMeta) return fromMeta;
    const sid = (env.session_id ?? "").trim();
    if (!sid) return "";
    const sess =
      deps.chatStore.sessions.find((s) => s.id === sid) ??
      (deps.chatStore.selectedSession?.id === sid ? deps.chatStore.selectedSession : null);
    return sess?.agent_id?.trim() ?? "";
  }

  function teamIdFromEnvelope(env: Envelope): string {
    if (env.team_id?.trim()) return env.team_id.trim();
    const sid = (env.session_id ?? "").trim();
    if (!sid) return "";
    const sess = deps.chatStore.sessions.find((s) => s.id === sid);
    return sess?.team_id?.trim() ?? "";
  }

  function matchesSelectedEntity(env: Envelope): boolean {
    if (deps.chatStore.entityKind === "team") {
      const tid = deps.chatStore.selectedTeamId?.trim();
      return !!tid && teamIdFromEnvelope(env) === tid;
    }
    const aid = deps.selectedAgentId.value?.trim();
    return !!aid && agentIdFromEnvelope(env) === aid;
  }

  async function refreshSessionsAfterTurn(sessionId: string) {
    if (deps.chatStore.entityKind === "agent") {
      const aid = deps.selectedAgentId.value?.trim();
      if (aid) await deps.chatStore.loadAgentSessions(aid, { refreshOnly: true });
      const sessAgent = agentIdFromEnvelope({ session_id: sessionId } as Envelope);
      if (sessAgent && sessAgent !== aid) {
        await deps.chatStore.loadAgentSessions(sessAgent, { refreshOnly: true });
      }
    } else if (deps.chatStore.entityKind === "team") {
      const tid = deps.chatStore.selectedTeamId?.trim();
      if (tid && deps.loadTeamSessions) {
        await deps.loadTeamSessions(tid);
      }
    }
  }

  function scheduleHydrate(sessionId: string, dropStaleInFlight = false) {
    if (deps.wsReplaying?.value) {
      return;
    }
    pendingHydrateSessionId = sessionId;
    if (hydrateTimer) clearTimeout(hydrateTimer);
    hydrateTimer = setTimeout(() => {
      hydrateTimer = null;
      void hydrateCurrentSession(pendingHydrateSessionId, dropStaleInFlight);
    }, HYDRATE_DEBOUNCE_MS);
  }

  async function hydrateCurrentSession(sessionId: string, dropStaleInFlight = false) {
    if (deps.chatStore.entityKind === "team") {
      deps.ensureTeamStream(sessionId);
    } else {
      deps.ensureChatStream(sessionId);
    }
    inboundMessageWriter.flushSync();
    if (dropStaleInFlight) {
      const finalized = dropPendingUserPlaceholders(
        finalizeOrphanToolMessages(deps.chatStore.getMessages(sessionId))
      );
      deps.chatStore.setMessages(sessionId, finalized);
    }
    const localRev = deps.chatStore.sessionRevisionBySession[sessionId] ?? 0;
    try {
      if (localRev > 0) {
        await deps.chatStore.loadMessages({ sessionId, afterRevision: localRev, dropStaleInFlight });
      } else {
        await deps.chatStore.loadMessages({ sessionId, dropStaleInFlight });
      }
      streamingSnapshots.clear(sessionId);
    } catch {
      /* ignore background hydrate */
    }
  }

  async function handleInboundEnvelope(env: Envelope) {
    const sessionId = (env.session_id ?? "").trim();
    if (!sessionId) return;

    const envRev = envelopeSessionRevision(env);
    const localRev = deps.chatStore.sessionRevisionBySession[sessionId] ?? 0;

    const isCurrent = deps.selectedSessionId.value === sessionId;
    const entityMatch = matchesSelectedEntity(env);
    const inboundSource = envelopeSource(env);

    if (inboundSource === "channel") {
      const agentId = agentIdFromEnvelope(env);
      if (agentId) {
        void refreshAgentSessionsForChannel(deps.chatStore, agentId, {
          entityKind: deps.chatStore.entityKind,
          activeAgentId: deps.selectedAgentId.value,
        });
      }
    }

    if (
      entityMatch &&
      inboundSource === "channel" &&
      env.type === "run_status" &&
      runStatusFromEnvelope(env)?.status === "running"
    ) {
      const agentId = agentIdFromEnvelope(env);
      if (
        agentId &&
        (deps.isChatRoute?.() ?? false) &&
        !isCurrent &&
        deps.focusChannelSession &&
        (deps.shouldAutoFocusChannel?.() ?? false)
      ) {
        await deps.focusChannelSession(sessionId, agentId);
      }
    }

    if (isCurrent && env.type === "run_status") {
      const rs = runStatusFromEnvelope(env);
      if (rs?.status === "running") {
        if (deps.chatStore.entityKind === "team") {
          deps.ensureTeamStream(sessionId);
        } else {
          deps.ensureChatStream(sessionId);
          if (inboundSource === "channel") {
            const localRevNow = deps.chatStore.sessionRevisionBySession[sessionId] ?? 0;
            if (localRevNow > 0) {
              void deps.chatStore.loadMessages({ sessionId, afterRevision: localRevNow });
            } else {
              void deps.chatStore.loadMessages({ sessionId });
            }
          }
        }
      }
    }

    if (!isCurrent && !entityMatch) return;

    if (isCurrent && deps.chatStore.entityKind === "agent") {
      deps.ensureChatStream(sessionId);
      if (env.type === "text_delta" && (env.content?.text || env.content?.reasoning)) {
        streamingSnapshots.put(sessionId, {
          reasoning: env.content?.reasoning,
          partialText: env.content?.text,
        });
        deps.patchAgentMessages(sessionId, `ws-stream-${sessionId}`, env, false);
        return;
      }
      if (env.type === "text_done") {
        streamingSnapshots.put(sessionId, {
          reasoning: env.content?.reasoning,
          partialText: env.content?.text,
          replace: true,
        });
        deps.patchAgentMessages(sessionId, `ws-stream-${sessionId}`, env, true);
        return;
      }
      if (env.type === "tool_call" && env.tool_call) {
        inboundMessageWriter.update((cur) => upsertToolMessage(cur, sessionId, env, "before"));
        return;
      }
      if (env.type === "tool_result" && env.tool_call) {
        inboundMessageWriter.update((cur) => upsertToolMessage(cur, sessionId, env, "after"));
        return;
      }
    }

    if (!isTurnCompleteEnvelope(env)) return;

    if (entityMatch || isCurrent) {
      await refreshSessionsAfterTurn(sessionId);
    }

    if (isCurrent) {
      if (envRev === 0 || envRev > localRev) {
        scheduleHydrate(sessionId, true);
      } else {
        const finalized = dropPendingUserPlaceholders(
          finalizeOrphanToolMessages(deps.chatStore.getMessages(sessionId))
        );
        deps.chatStore.setMessages(sessionId, finalized);
        streamingSnapshots.clear(sessionId);
      }
    }
    deps.onTurnComplete?.(sessionId);
  }

  onMounted(() => {
    hubId = acquireGlobalWsConsumer({
      channels: ["chat"],
      logEnabled: false,
      onEnvelope: (env) => {
        if (env.channel !== "chat") return;
        void handleInboundEnvelope(env);
      },
    });
  });

  onUnmounted(() => {
    if (hydrateTimer) clearTimeout(hydrateTimer);
    if (hubId) {
      releaseGlobalWsConsumer(hubId);
      hubId = null;
    }
  });
}
