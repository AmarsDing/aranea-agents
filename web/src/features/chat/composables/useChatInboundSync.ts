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
import { SESSION_RUN_STATUS } from "../sessionRunStatus";
import {
  envelopeSessionRevision,
  envelopeSource,
  isSessionRevisionSyncEnvelope,
  isTurnCompleteEnvelope,
} from "../inboundSyncEnvelope";
import {
  upsertToolMessage,
} from "../envelopeToolCall";
import { createMessageBatchWriter } from "../messageStoreBatch";
import { patchStreamingEnvelope } from "../streamHandlers";
import { refreshAgentSessionsForChannel } from "../channelInboundSessionRefresh";
import {
  isChannelInboundSession,
  resolveInboundAgentId,
} from "../channelInboundSession";
import { noteChannelWsEnvelope } from "../channelWsCursor";

export type ChatInboundSyncDeps = {
  appStore: ReturnType<typeof useAppStore>;
  chatStore: ReturnType<typeof useChatStore>;
  selectedAgentId: Ref<string | undefined>;
  selectedSessionId: Ref<string | undefined>;
  wsReplaying?: Ref<boolean>;
  isChatRoute?: () => boolean;
  shouldAutoFocusChannel?: () => boolean;
  onTurnComplete?: (sessionId: string) => void;
  onHydrateError?: (sessionId: string, message: string) => void;
  focusChannelSession?: (sessionId: string, agentId: string) => void | Promise<void>;
  ensureChatStream: (sessionId: string) => UseEnvelopeStreamReturn;
  ensureTeamStream: (sessionId: string) => UseEnvelopeStreamReturn;
  patchAgentMessages: (sessionId: string, streamId: string, env: Envelope, isDone: boolean) => void;
  patchTeamMessages: (sessionId: string, streamId: string, env: Envelope, isDone: boolean) => void;
  loadTeamSessions?: (teamId: string) => Promise<void>;
};

const HYDRATE_DEBOUNCE_MS = 200;

type TurnStreamSeal = { revision: number };

function inboundStreamRowId(sessionId: string): string {
  return `ws-stream-${sessionId}`;
}

function isStreamEnvelope(env: Envelope): boolean {
  return (
    env.type === "text_delta" ||
    env.type === "text_done" ||
    env.type === "tool_call" ||
    env.type === "tool_result"
  );
}

/**
 * Global WS consumer for channel/cron inbound. Session-scoped WS (ensureChatStream) handles
 * web-initiated turns; channel inbound uses this path for incremental stream + hydrate.
 */
export function useChatInboundSync(deps: ChatInboundSyncDeps) {
  const streamingSnapshots = useChatStreamingSnapshots();
  let hubId: string | null = null;
  let hydrateTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingHydrateSessionId = "";
  const sealedTurnBySession = new Map<string, TurnStreamSeal>();
  const inboundWriters = new Map<
    string,
    ReturnType<typeof createMessageBatchWriter>
  >();

  function inboundWriter(sessionId: string) {
    let writer = inboundWriters.get(sessionId);
    if (!writer) {
      writer = createMessageBatchWriter(
        () => deps.chatStore.getMessages(sessionId),
        (rows) => deps.chatStore.setMessages(sessionId, rows)
      );
      inboundWriters.set(sessionId, writer);
    }
    return writer;
  }

  function flushInboundWriter(sessionId: string) {
    inboundWriters.get(sessionId)?.flushSync();
  }

  function sealTurnStream(sessionId: string, env: Envelope, envRev: number) {
    const localRev = deps.chatStore.sessionRevisionBySession[sessionId] ?? 0;
    flushInboundWriter(sessionId);
    sealedTurnBySession.set(sessionId, {
      revision: Math.max(envRev, localRev),
    });
  }

  function unsealTurnStream(sessionId: string) {
    sealedTurnBySession.delete(sessionId);
  }

  function isStaleStreamEnvelope(sessionId: string, env: Envelope): boolean {
    if (!sealedTurnBySession.has(sessionId)) return false;
    return env.type === "text_delta" || env.type === "text_done";
  }

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

  function isViewingSession(sessionId: string, agentId: string): boolean {
    if (deps.selectedSessionId.value !== sessionId) return false;
    const selectedAgent = deps.selectedAgentId.value?.trim() ?? "";
    return !agentId || selectedAgent === agentId.trim();
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

  function scheduleHydrate(sessionId: string, dropStaleInFlight = false, clearStreaming = true) {
    if (deps.wsReplaying?.value) {
      return;
    }
    pendingHydrateSessionId = sessionId;
    if (hydrateTimer) clearTimeout(hydrateTimer);
    hydrateTimer = setTimeout(() => {
      hydrateTimer = null;
      void hydrateCurrentSession(pendingHydrateSessionId, dropStaleInFlight, clearStreaming);
    }, HYDRATE_DEBOUNCE_MS);
  }

  async function hydrateCurrentSession(
    sessionId: string,
    dropStaleInFlight = false,
    clearStreaming = true
  ) {
    if (deps.chatStore.entityKind === "team") {
      deps.ensureTeamStream(sessionId);
    } else {
      deps.ensureChatStream(sessionId);
    }
    flushInboundWriter(sessionId);
    try {
      await deps.chatStore.loadMessages({ sessionId, dropStaleInFlight });
      if (clearStreaming) {
        streamingSnapshots.clear(sessionId);
      }
      deps.onHydrateError?.(sessionId, "");
    } catch (err) {
      const message = err instanceof Error ? err.message : "hydrate failed";
      deps.onHydrateError?.(sessionId, message);
    }
  }

  async function finalizeTurn(sessionId: string, env: Envelope, envRev: number) {
    sealTurnStream(sessionId, env, envRev);
    flushInboundWriter(sessionId);
    // Always load persisted messages on turn complete — ephemeral ws-stream rows are not durable.
    await hydrateCurrentSession(sessionId, true, true);
  }

  async function handleInboundEnvelope(env: Envelope) {
    const sessionId = (env.session_id ?? "").trim();
    if (!sessionId) return;

    if (env.id) {
      noteChannelWsEnvelope(sessionId, env.id);
    }

    const envRev = envelopeSessionRevision(env);
    const localRev = deps.chatStore.sessionRevisionBySession[sessionId] ?? 0;
    const inboundSource = envelopeSource(env);
    const channelInbound =
      inboundSource === "channel" ||
      (await isChannelInboundSession(sessionId, inboundSource, deps.chatStore));

    let channelAgentId = "";
    if (channelInbound) {
      channelAgentId = await resolveInboundAgentId(sessionId, env, deps.chatStore);
      if (channelAgentId) {
        void refreshAgentSessionsForChannel(deps.chatStore, channelAgentId, {
          entityKind: deps.chatStore.entityKind,
          activeAgentId: deps.selectedAgentId.value,
        });
      }

      const rs = env.type === "run_status" ? runStatusFromEnvelope(env)?.status : "";
      if (rs === SESSION_RUN_STATUS.RUNNING) {
        unsealTurnStream(sessionId);
      }

      const focusTrigger =
        rs === SESSION_RUN_STATUS.RUNNING || isSessionRevisionSyncEnvelope(env);
      if (
        focusTrigger &&
        channelAgentId &&
        (deps.isChatRoute?.() ?? false) &&
        !isViewingSession(sessionId, channelAgentId) &&
        deps.focusChannelSession &&
        (deps.shouldAutoFocusChannel?.() ?? false)
      ) {
        await deps.focusChannelSession(sessionId, channelAgentId);
      }
    }

    const isCurrent = deps.selectedSessionId.value === sessionId;
    const entityMatch = matchesSelectedEntity(env);
    const turnComplete = isTurnCompleteEnvelope(env);
    const ownsEnvelope =
      isCurrent ||
      entityMatch ||
      (channelInbound && (isStreamEnvelope(env) || turnComplete));

    if (isCurrent && env.type === "run_status") {
      const rs = runStatusFromEnvelope(env);
      if (rs?.status === SESSION_RUN_STATUS.RUNNING) {
        unsealTurnStream(sessionId);
        if (deps.chatStore.entityKind === "team") {
          deps.ensureTeamStream(sessionId);
        } else {
          deps.ensureChatStream(sessionId);
        }
      }
    }

    if (!ownsEnvelope) return;

    if (isCurrent && isSessionRevisionSyncEnvelope(env)) {
      if (envRev > localRev) {
        scheduleHydrate(sessionId, false, false);
      }
      return;
    }

    if (deps.chatStore.entityKind === "agent" && isStreamEnvelope(env)) {
      const streamId = inboundStreamRowId(sessionId);
      const writer = inboundWriter(sessionId);
      if (env.type === "text_delta" && (env.content?.text || env.content?.reasoning)) {
        if (isStaleStreamEnvelope(sessionId, env)) return;
        streamingSnapshots.put(sessionId, {
          reasoning: env.content?.reasoning,
          partialText: env.content?.text,
        });
        writer.update((cur) =>
          patchStreamingEnvelope(cur, sessionId, streamId, env, false)
        );
        return;
      }
      if (env.type === "text_done") {
        if (isStaleStreamEnvelope(sessionId, env)) return;
        streamingSnapshots.put(sessionId, {
          reasoning: env.content?.reasoning,
          partialText: env.content?.text,
          replace: true,
        });
        writer.update((cur) =>
          patchStreamingEnvelope(cur, sessionId, streamId, env, true)
        );
        return;
      }
      if (env.type === "tool_call" && env.tool_call) {
        writer.update((cur) => upsertToolMessage(cur, sessionId, env, "before"));
        return;
      }
      if (env.type === "tool_result" && env.tool_call) {
        writer.update((cur) => upsertToolMessage(cur, sessionId, env, "after"));
        return;
      }
    }

    if (!turnComplete) return;

    if (entityMatch || isCurrent) {
      await refreshSessionsAfterTurn(sessionId);
    }

    if (channelInbound || isCurrent || entityMatch) {
      await finalizeTurn(sessionId, env, envRev);
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
