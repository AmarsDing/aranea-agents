import { onMounted, onUnmounted, type Ref } from 'vue';
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer } from '../globalWsHub';
import type { Envelope } from '../envelope';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import type { UseEnvelopeStreamReturn } from '../useEnvelopeStream';
import type { useAppStore } from '../../../stores/app';
import type { useChatSessionStore } from '../../../stores/chat/sessionStore';
import type { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useChatConversationStore } from '../../../stores/chat/conversationStore';
import { useChatStreamingSnapshots } from '../../../stores/chatStreamingSnapshots';
import { runStatusFromEnvelope } from '../envelopeRunStatus';
import { SESSION_RUN_STATUS } from '../sessionRunStatus';
import {
  envelopeSessionRevision,
  envelopeSource,
  isSessionRevisionSyncEnvelope,
  isTurnCompleteEnvelope,
  shouldSkipHydrate,
} from '../inboundSyncEnvelope';
import {
  shouldGlobalHubFinalizeTurn,
  shouldGlobalHubHandleStream,
  shouldScheduleChannelFocus,
  shouldSkipMessageReloadOnChannelFocus,
  isStreamEnvelopeType,
  type ChannelFocusOptions,
} from '../inboundSyncRouting';
import { isSessionCompressNotice, sessionContextPatchFromEnvelope } from '../sessionContextPatch';
import { createMessageBatchWriter } from '../messageStoreBatch';
import { patchStreamingEnvelope } from '../streamHandlers';
import { upsertToolMessage } from '../envelopeToolCall';
import { refreshAgentSessionsForChannel } from '../channelInboundSessionRefresh';
import { isChannelInboundSession, resolveInboundAgentId } from '../channelInboundSession';
import { noteChannelWsEnvelope } from '../channelWsCursor';
import { projectConversationEnvelope } from '../conversationEventDispatcher';
import { emitSessionMutation } from '../../../stores/sessionSync';
import { useSpiritTeamStore } from '../../../stores/spirit';
import { CHAT_HYDRATE_DEBOUNCE_MS } from '../../constants/timeouts';

export type ChatInboundSyncDeps = {
  appStore: ReturnType<typeof useAppStore>;
  sessionStore: ReturnType<typeof useChatSessionStore>;
  messageStore: ReturnType<typeof useChatMessageStore>;
  spiritStore: ReturnType<typeof useSpiritTeamStore>;
  selectedAgentId: Ref<string | undefined>;
  selectedSessionId: Ref<string | undefined>;
  wsReplaying?: Ref<boolean>;
  onSpiritEnvelope?: (envelope: Envelope) => void;
  onActivityEnvelope?: (envelope: Envelope) => void;
  /** Activity-First (AF): route new ActivityEvent messages to the timeline. */
  onActivityEvent?: (ev: ActivityEvent) => void;
  isChatRoute?: () => boolean;
  shouldAutoFocusChannel?: () => boolean;
  onTurnComplete?: (sessionId: string) => void;
  onHydrateError?: (sessionId: string, message: string) => void;
  focusChannelSession?: (sessionId: string, agentId: string, options?: ChannelFocusOptions) => void | Promise<void>;
  ensureChatStream: (sessionId: string) => UseEnvelopeStreamReturn;
  ensureTeamStream: (sessionId: string) => UseEnvelopeStreamReturn;
  loadTeamSessions?: (teamId: string) => Promise<void>;
};

type TurnStreamSeal = { revision: number };

function inboundStreamRowId(sessionId: string): string {
  return `ws-stream-${sessionId}`;
}

/**
 * Global WS consumer for channel/cron inbound. Session-scoped WS (ensureChatStream) handles
 * web-initiated turns; channel inbound uses this path for incremental stream + hydrate.
 */
export function useChatInboundSync(deps: ChatInboundSyncDeps) {
  const streamingSnapshots = useChatStreamingSnapshots();
  const conversationStore = useChatConversationStore();
  let hubId: string | null = null;
  let hydrateTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingHydrateSessionId = '';
  const sealedTurnBySession = new Map<string, TurnStreamSeal>();
  const inboundWriters = new Map<string, ReturnType<typeof createMessageBatchWriter>>();
  const hydrateInFlight = new Map<string, Promise<void>>();
  const focusInFlight = new Set<string>();

  function inboundWriter(sessionId: string) {
    let writer = inboundWriters.get(sessionId);
    if (!writer) {
      writer = createMessageBatchWriter(
        () => deps.messageStore.getMessages(sessionId),
        (rows) => deps.messageStore.setMessages(sessionId, rows),
      );
      inboundWriters.set(sessionId, writer);
    }
    return writer;
  }

  function flushInboundWriter(sessionId: string) {
    inboundWriters.get(sessionId)?.flushSync();
  }

  function sealTurnStream(sessionId: string, env: Envelope, envRev: number) {
    const localRev = deps.messageStore.sessionRevisionBySession[sessionId] ?? 0;
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
    return env.type === 'text_delta' || env.type === 'text_done';
  }

  function agentIdFromEnvelope(env: Envelope): string {
    const md = env.metadata as Record<string, unknown> | undefined;
    const fromMeta = typeof md?.agent_id === 'string' ? md.agent_id.trim() : '';
    if (fromMeta) return fromMeta;
    const sid = (env.session_id ?? '').trim();
    if (!sid) return '';
    const sess =
      deps.sessionStore.sessions.find((s) => s.id === sid) ??
      (deps.sessionStore.selectedSession?.id === sid ? deps.sessionStore.selectedSession : null);
    return sess?.agent_id?.trim() ?? '';
  }

  function teamIdFromEnvelope(env: Envelope): string {
    if (env.team_id?.trim()) return env.team_id.trim();
    const sid = (env.session_id ?? '').trim();
    if (!sid) return '';
    const sess = deps.sessionStore.sessions.find((s) => s.id === sid);
    return sess?.team_id?.trim() ?? '';
  }

  function matchesSelectedEntity(env: Envelope): boolean {
    if (deps.sessionStore.entityKind === 'team') {
      const tid = deps.sessionStore.selectedTeamId?.trim();
      return !!tid && teamIdFromEnvelope(env) === tid;
    }
    const aid = deps.selectedAgentId.value?.trim();
    return !!aid && agentIdFromEnvelope(env) === aid;
  }

  function isViewingSession(sessionId: string, agentId: string): boolean {
    if (deps.selectedSessionId.value !== sessionId) return false;
    const selectedAgent = deps.selectedAgentId.value?.trim() ?? '';
    return !agentId || selectedAgent === agentId.trim();
  }

  async function refreshSessionsAfterTurn(sessionId: string) {
    if (deps.sessionStore.entityKind === 'agent') {
      const aid = deps.selectedAgentId.value?.trim();
      if (aid) await deps.sessionStore.loadAgentSessions(aid, { refreshOnly: true });
      const sessAgent = agentIdFromEnvelope({ session_id: sessionId } as Envelope);
      if (sessAgent && sessAgent !== aid) {
        await deps.sessionStore.loadAgentSessions(sessAgent, { refreshOnly: true });
      }
    } else if (deps.sessionStore.entityKind === 'team') {
      const tid = deps.sessionStore.selectedTeamId?.trim();
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
    }, CHAT_HYDRATE_DEBOUNCE_MS);
  }

  async function hydrateCurrentSession(sessionId: string, dropStaleInFlight = false, clearStreaming = true) {
    const inFlight = hydrateInFlight.get(sessionId);
    if (inFlight) {
      await inFlight;
      return;
    }
    const task = (async () => {
      if (deps.sessionStore.entityKind === 'team') {
        deps.ensureTeamStream(sessionId);
      } else {
        deps.ensureChatStream(sessionId);
      }
      flushInboundWriter(sessionId);
      try {
        const afterRevision = deps.messageStore.sessionRevisionBySession[sessionId] ?? 0;
        await deps.messageStore.loadMessages({
          sessionId,
          afterRevision: afterRevision > 0 ? afterRevision : undefined,
          dropStaleInFlight,
        });
        if (clearStreaming) {
          streamingSnapshots.clear(sessionId);
        }
        deps.onHydrateError?.(sessionId, '');
      } catch (err) {
        const message = err instanceof Error ? err.message : 'hydrate failed';
        deps.onHydrateError?.(sessionId, message);
      }
    })();
    hydrateInFlight.set(sessionId, task);
    try {
      await task;
    } finally {
      if (hydrateInFlight.get(sessionId) === task) {
        hydrateInFlight.delete(sessionId);
      }
    }
  }

  async function finalizeTurn(sessionId: string, env: Envelope, envRev: number) {
    sealTurnStream(sessionId, env, envRev);
    flushInboundWriter(sessionId);
    // Always load persisted messages on turn complete — ephemeral ws-stream rows are not durable.
    await hydrateCurrentSession(sessionId, true, true);
  }

  function scheduleChannelFocus(sessionId: string, agentId: string, skipMessageReload: boolean) {
    if (!deps.focusChannelSession || focusInFlight.has(sessionId)) return;
    flushInboundWriter(sessionId);
    focusInFlight.add(sessionId);
    void Promise.resolve(deps.focusChannelSession(sessionId, agentId, { skipMessageReload }))
      .catch((err) => {
        console.warn('[chat] channel focus failed:', sessionId, err);
      })
      .finally(() => {
        focusInFlight.delete(sessionId);
      });
  }

  function patchChannelStreamEnvelope(sessionId: string, env: Envelope, channelInbound: boolean): boolean {
    if (!shouldGlobalHubHandleStream(channelInbound, deps.sessionStore.entityKind, env)) {
      return false;
    }
    const streamId = inboundStreamRowId(sessionId);
    const writer = inboundWriter(sessionId);
    if (env.type === 'text_delta' && (env.content?.text || env.content?.reasoning)) {
      if (isStaleStreamEnvelope(sessionId, env)) return true;
      streamingSnapshots.put(sessionId, {
        reasoning: env.content?.reasoning,
        partialText: env.content?.text,
      });
      writer.batchPatch((msgs) => patchStreamingEnvelope(msgs, sessionId, streamId, env, false));
      return true;
    }
    if (env.type === 'text_done') {
      if (isStaleStreamEnvelope(sessionId, env)) return true;
      streamingSnapshots.put(sessionId, {
        reasoning: env.content?.reasoning,
        partialText: env.content?.text,
        replace: true,
      });
      writer.batchPatch((msgs) => patchStreamingEnvelope(msgs, sessionId, streamId, env, true));
      return true;
    }
    if (env.type === 'tool_call' && env.tool_call) {
      writer.batchPatch((msgs) => upsertToolMessage(msgs, sessionId, env, 'before'));
      return true;
    }
    if (env.type === 'tool_result' && env.tool_call) {
      writer.batchPatch((msgs) => upsertToolMessage(msgs, sessionId, env, 'after'));
      return true;
    }
    return false;
  }

  async function handleInboundEnvelope(env: Envelope) {
    const sessionId = (env.session_id ?? '').trim();
    if (!sessionId) return;
    const projection = projectConversationEnvelope(env, {
      currentSessionId: deps.selectedSessionId.value,
    });
    if (projection) {
      conversationStore.applyProjection(projection);
    }

    if (env.id) {
      noteChannelWsEnvelope(sessionId, env.id);
    }

    if (isSessionCompressNotice(env)) {
      const prev = deps.sessionStore.findSessionById(sessionId);
      const patch = sessionContextPatchFromEnvelope(env, prev);
      if (patch) {
        deps.sessionStore.patchSessionMetricsLocal(sessionId, patch);
      }
    }

    if (env.type === 'context_usage' && env.usage) {
      const patch = sessionContextPatchFromEnvelope(env);
      if (patch) {
        deps.sessionStore.patchSessionMetricsLocal(sessionId, patch);
      }
    }

    if (env.type === 'session.status_changed' && env.metadata) {
      const md = env.metadata as Record<string, unknown>;
      const status = typeof md.status === 'string' ? md.status : '';
      const statusReason = typeof md.status_reason === 'string' ? md.status_reason : '';
      const statusChangedAt = typeof md.status_changed_at === 'string' ? md.status_changed_at : '';
      if (status) {
        emitSessionMutation({ type: 'status_changed', id: sessionId, status, statusReason, statusChangedAt });
      }
    }

    if (env.type === 'metrics_updated') {
      deps.sessionStore.fetchAndReconcileSession(sessionId);
    }

    if (env.type.startsWith('spirit_')) {
      deps.spiritStore.handleSpiritEnvelope(env);
      deps.onSpiritEnvelope?.(env);
    }

    // P1: Register default handlers for previously-unhandled event types
    if (env.type === 'mcp.session.reconnect' || env.type === 'mcp.health.alert') {
      console.info(`[envelope] ${env.type} received`, { sessionId, requestId: env.request_id });
    }
    if (env.type === 'user_feedback') {
      console.info('[envelope] user_feedback received', { sessionId, author: env.author });
    }
    if (env.type === 'alert.notify') {
      console.warn('[envelope] alert.notify received', { sessionId, metadata: env.metadata });
    }
    if (env.type.startsWith('butler.orchestration.')) {
      deps.spiritStore.handleSpiritEnvelope(env);
      deps.onSpiritEnvelope?.(env);
    }
    if (env.type === 'skill.health_changed' || env.type === 'skill.evolution_proposed') {
      console.info(`[envelope] ${env.type} received`, { sessionId, metadata: env.metadata });
    }
    if (env.type === 'token_usage' && env.token_usage) {
      console.info('[envelope] token_usage received', {
        sessionId,
        model: env.token_usage.model_display_name,
        totalTokens: env.token_usage.total_tokens,
      });
    }
    if (env.type === 'monitor.auto_healed' || env.type === 'monitor.self_check_completed') {
      console.info(`[envelope] ${env.type} received`, { sessionId, metadata: env.metadata });
    }

    const envRev = projection?.revision || envelopeSessionRevision(env);
    const localRev = deps.messageStore.sessionRevisionBySession[sessionId] ?? 0;
    const inboundSource = projection?.source ?? envelopeSource(env);
    const channelInbound =
      inboundSource === 'channel' || (await isChannelInboundSession(sessionId, inboundSource, deps.sessionStore));

    let channelAgentId = '';
    let channelRunStatus = '';
    if (channelInbound) {
      channelAgentId = await resolveInboundAgentId(sessionId, env, deps.sessionStore);
      const shouldRefreshSessions =
        env.type === 'run_status' || isSessionRevisionSyncEnvelope(env) || isTurnCompleteEnvelope(env);
      if (channelAgentId && shouldRefreshSessions) {
        void refreshAgentSessionsForChannel(deps.sessionStore, channelAgentId, {
          entityKind: deps.sessionStore.entityKind,
          activeAgentId: deps.selectedAgentId.value,
        });
      }

      channelRunStatus = env.type === 'run_status' ? (runStatusFromEnvelope(env)?.status ?? '') : '';
      if (channelRunStatus === SESSION_RUN_STATUS.RUNNING) {
        unsealTurnStream(sessionId);
      }
    }

    const isCurrent = deps.selectedSessionId.value === sessionId;

    // AF-GAP-02: Route Activity events to useActivityTimeline handler for
    // ALL sessions (including current). The handleActivityEnvelope in
    // useChatWorkspace deduplicates by envelope ID, so forwarding from both
    // the session WS streamHandlers and the global hub inbound sync is safe —
    // the first path to arrive processes the event, the second is a no-op.
    // This ensures activity events are not lost during WS reconnection gaps
    // or when the session WS stream is not yet established.
    if (env.type.startsWith('activity_')) {
      deps.onActivityEnvelope?.(env);
    }

    const entityMatch = matchesSelectedEntity(env);
    const turnComplete = isTurnCompleteEnvelope(env);
    const ownsEnvelope = isCurrent || entityMatch || (channelInbound && (isStreamEnvelopeType(env) || turnComplete));

    if (ownsEnvelope && patchChannelStreamEnvelope(sessionId, env, channelInbound)) {
      // OBS-02: Route tool_call/tool_result to contextual loading message handler
      // before returning, so agent-level loading messages are displayed.
      if (env.type === 'tool_call' || env.type === 'tool_result') {
        deps.onSpiritEnvelope?.(env);
      }
      return;
    }

    if (channelInbound && channelAgentId) {
      const focusTrigger = channelRunStatus === SESSION_RUN_STATUS.RUNNING || isSessionRevisionSyncEnvelope(env);
      if (
        shouldScheduleChannelFocus({
          channelInbound,
          channelAgentId,
          focusTrigger,
          isChatRoute: deps.isChatRoute?.() ?? false,
          isViewingSession: isViewingSession(sessionId, channelAgentId),
          shouldAutoFocus: deps.shouldAutoFocusChannel?.() ?? false,
          hasFocusHandler: Boolean(deps.focusChannelSession),
        })
      ) {
        scheduleChannelFocus(sessionId, channelAgentId, shouldSkipMessageReloadOnChannelFocus(channelRunStatus));
      }
    }

    if (isCurrent && env.type === 'run_status') {
      const rs = runStatusFromEnvelope(env);
      if (rs?.status === SESSION_RUN_STATUS.RUNNING) {
        unsealTurnStream(sessionId);
        if (deps.sessionStore.entityKind === 'team') {
          deps.ensureTeamStream(sessionId);
        } else {
          deps.ensureChatStream(sessionId);
        }
      }
    }

    if (!ownsEnvelope) return;

    if (isCurrent && isSessionRevisionSyncEnvelope(env)) {
      if (shouldSkipHydrate(env)) {
        if (envRev > localRev) {
          deps.messageStore.sessionRevisionBySession[sessionId] = envRev;
        }
        return;
      }
      if (envRev > localRev) {
        scheduleHydrate(sessionId, false, false);
      }
      return;
    }

    if (!turnComplete) return;

    if (entityMatch || isCurrent) {
      await refreshSessionsAfterTurn(sessionId);
    }

    if (shouldGlobalHubFinalizeTurn(channelInbound, isCurrent, turnComplete)) {
      await finalizeTurn(sessionId, env, envRev);
    }

    deps.onTurnComplete?.(sessionId);
  }

  onMounted(() => {
    hubId = acquireGlobalWsConsumer({
      channels: ['chat'],
      logEnabled: false,
      onEnvelope: (env) => {
        if (env.channel !== 'chat') return;
        void handleInboundEnvelope(env);
      },
      onActivityEvent: deps.onActivityEvent ? (ev) => deps.onActivityEvent!(ev) : undefined,
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
