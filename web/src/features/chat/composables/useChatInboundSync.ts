import { onMounted, onUnmounted, type Ref } from 'vue';
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer } from '../globalWsHub';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import type { UseEnvelopeStreamReturn } from '../useEnvelopeStream';
import type { useAppStore } from '../../../stores/app';
import type { useChatSessionStore } from '../../../stores/chat/sessionStore';
import type { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useChatConversationStore } from '../../../stores/chat/conversationStore';
import { useChatStreamingSnapshots } from '../../../stores/chatStreamingSnapshots';
import { runStatusFromActivityEvent } from '../activityRunStatus';
import { SESSION_RUN_STATUS } from '../sessionRunStatus';
import {
  activitySessionRevision,
  activitySource,
  isSessionRevisionSyncActivity,
  isTurnCompleteActivity,
  shouldSkipHydrateActivity,
} from '../inboundSyncEnvelope';
import {
  shouldScheduleChannelFocus,
  shouldSkipMessageReloadOnChannelFocus,
  shouldGlobalHubFinalizeTurnActivity,
  type ChannelFocusOptions,
} from '../inboundSyncRouting';
import {
  isSessionCompressNoticeFromActivityEvent,
  sessionContextPatchFromActivityEvent,
} from '../sessionContextPatch';
import { createMessageBatchWriter } from '../messageStoreBatch';
import { refreshAgentSessionsForChannel } from '../channelInboundSessionRefresh';
import { isChannelInboundSession, resolveInboundAgentIdFromActivity } from '../channelInboundSession';
import { noteChannelWsEnvelope } from '../channelWsCursor';
import { projectConversationActivityEvent } from '../conversationEventDispatcher';
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
  onSpiritActivityEvent?: (ev: ActivityEvent) => void;
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

  function unsealTurnStream(sessionId: string) {
    sealedTurnBySession.delete(sessionId);
  }

  function agentIdFromActivity(ev: ActivityEvent): string {
    const meta = ev.activity.meta ?? {};
    const fromMeta = typeof meta.agent_id === 'string' ? meta.agent_id.trim() : '';
    if (fromMeta) return fromMeta;
    const sid = (ev.activity.session_id ?? '').trim();
    if (!sid) return '';
    const sess =
      deps.sessionStore.sessions.find((s) => s.id === sid) ??
      (deps.sessionStore.selectedSession?.id === sid ? deps.sessionStore.selectedSession : null);
    return sess?.agent_id?.trim() ?? '';
  }

  function teamIdFromActivity(ev: ActivityEvent): string {
    if (ev.activity.team_id?.trim()) return ev.activity.team_id.trim();
    const sid = (ev.activity.session_id ?? '').trim();
    if (!sid) return '';
    const sess = deps.sessionStore.sessions.find((s) => s.id === sid);
    return sess?.team_id?.trim() ?? '';
  }

  function matchesSelectedEntityActivity(ev: ActivityEvent): boolean {
    if (deps.sessionStore.entityKind === 'team') {
      const tid = deps.sessionStore.selectedTeamId?.trim();
      return !!tid && teamIdFromActivity(ev) === tid;
    }
    const aid = deps.selectedAgentId.value?.trim();
    return !!aid && agentIdFromActivity(ev) === aid;
  }

  function isViewingSession(sessionId: string, agentId: string): boolean {
    if (deps.selectedSessionId.value !== sessionId) return false;
    const selectedAgent = deps.selectedAgentId.value?.trim() ?? '';
    return !agentId || selectedAgent === agentId.trim();
  }

  /** Resolve the agent_id for a session by looking it up in the session store. */
  function agentIdFromSessionLookup(sessionId: string): string {
    const sess =
      deps.sessionStore.sessions.find((s) => s.id === sessionId) ??
      (deps.sessionStore.selectedSession?.id === sessionId ? deps.sessionStore.selectedSession : null);
    return sess?.agent_id?.trim() ?? '';
  }

  async function refreshSessionsAfterTurn(sessionId: string) {
    if (deps.sessionStore.entityKind === 'agent') {
      const aid = deps.selectedAgentId.value?.trim();
      if (aid) await deps.sessionStore.loadAgentSessions(aid, { refreshOnly: true });
      const sessAgent = agentIdFromSessionLookup(sessionId);
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

  /**
   * Detect whether an ActivityEvent is a Spirit/Team/Butler orchestration
   * event that should be routed to the spirit store. These events have
   * kind ∈ {team_stage, session, plan} and specific stage values.
   */
  function isSpiritActivityEvent(ev: ActivityEvent): boolean {
    const { kind, stage } = ev.activity;
    if (kind === 'team_stage') return true;
    if (kind === 'plan') return true;
    if (kind === 'session') {
      return (
        stage === 'plan_created' ||
        stage === 'allocation_created' ||
        stage === 'orchestration_started' ||
        stage === 'orchestration_checkpoint' ||
        stage === 'orchestration_interrupted' ||
        stage === 'orchestration_completed' ||
        stage === 'orchestration_failed' ||
        stage === 'synthesis_completed'
      );
    }
    return false;
  }

  /**
   * Activity-First: Handle an inbound ActivityEvent from the global WS hub.
   *
   * Consumes ActivityEvent directly, eliminating the ActivityEvent → Envelope
   * bridge conversion. Chat-rendering events (task streaming, action,
   * thinking, reply) are NOT processed here — they are routed to
   * useActivityTimeline by useChatWorkspace.handleActivityEvent. This handler
   * processes system/control events: run_status, error,
   * session.status_changed, metrics_updated, context_usage, runner_completion,
   * spirit/team orchestration, and channel inbound routing.
   */
  async function handleInboundActivityEvent(ev: ActivityEvent) {
    const sessionId = (ev.activity.session_id ?? '').trim();
    if (!sessionId) return;

    const projection = projectConversationActivityEvent(ev, {
      currentSessionId: deps.selectedSessionId.value,
    });
    if (projection) {
      conversationStore.applyProjection(projection);
    }

    if (ev.activity.id) {
      noteChannelWsEnvelope(sessionId, ev.activity.id);
    }

    // Session compress notice (text_done + meta.kind=system.session.compress)
    if (isSessionCompressNoticeFromActivityEvent(ev)) {
      const prev = deps.sessionStore.findSessionById(sessionId);
      const patch = sessionContextPatchFromActivityEvent(ev, prev);
      if (patch) {
        deps.sessionStore.patchSessionMetricsLocal(sessionId, patch);
      }
    }

    // Context usage (stage=context_usage)
    if (ev.activity.stage === 'context_usage') {
      const patch = sessionContextPatchFromActivityEvent(ev);
      if (patch) {
        deps.sessionStore.patchSessionMetricsLocal(sessionId, patch);
      }
    }

    // Session status changed (stage=session_status_changed)
    if (ev.activity.stage === 'session_status_changed' && ev.activity.meta) {
      const md = ev.activity.meta;
      const status = typeof md.status === 'string' ? md.status : '';
      const statusReason = typeof md.status_reason === 'string' ? md.status_reason : '';
      const statusChangedAt = typeof md.status_changed_at === 'string' ? md.status_changed_at : '';
      if (status) {
        emitSessionMutation({ type: 'status_changed', id: sessionId, status, statusReason, statusChangedAt });
      }
    }

    // Metrics updated (stage=metrics_updated)
    if (ev.activity.stage === 'metrics_updated') {
      deps.sessionStore.fetchAndReconcileSession(sessionId);
    }

    // Spirit/team/butler orchestration events
    if (isSpiritActivityEvent(ev)) {
      deps.spiritStore.handleSpiritActivityEvent(ev);
      deps.onSpiritActivityEvent?.(ev);
    }

    // P1: Register default handlers for previously-unhandled event types
    if (ev.activity.stage === 'mcp.session.reconnect' || ev.activity.stage === 'mcp.health.alert') {
      console.info(`[activity] ${ev.activity.stage} received`, { sessionId, agentKey: ev.activity.agent_key });
    }
    if (ev.activity.stage === 'user_feedback') {
      console.info('[activity] user_feedback received', { sessionId, author: ev.activity.agent_key });
    }
    if (ev.activity.stage === 'alert.notify') {
      console.warn('[activity] alert.notify received', { sessionId, meta: ev.activity.meta });
    }
    if (ev.activity.stage === 'skill.health_changed' || ev.activity.stage === 'skill.evolution_proposed') {
      console.info(`[activity] ${ev.activity.stage} received`, { sessionId, meta: ev.activity.meta });
    }
    if (ev.activity.stage === 'token_usage') {
      const meta = ev.activity.meta ?? {};
      console.info('[activity] token_usage received', {
        sessionId,
        model: meta.model_display_name,
        totalTokens: meta.total_tokens,
      });
    }
    if (ev.activity.stage === 'monitor.auto_healed' || ev.activity.stage === 'monitor.self_check_completed') {
      console.info(`[activity] ${ev.activity.stage} received`, { sessionId, meta: ev.activity.meta });
    }

    const envRev = projection?.revision || activitySessionRevision(ev);
    const localRev = deps.messageStore.sessionRevisionBySession[sessionId] ?? 0;
    const inboundSource = projection?.source ?? activitySource(ev);
    const channelInbound =
      inboundSource === 'channel' || (await isChannelInboundSession(sessionId, inboundSource, deps.sessionStore));

    let channelAgentId = '';
    let channelRunStatus = '';
    if (channelInbound) {
      channelAgentId = await resolveInboundAgentIdFromActivity(sessionId, ev, deps.sessionStore);
      const shouldRefreshSessions =
        ev.activity.stage === 'run_status' || isSessionRevisionSyncActivity(ev) || isTurnCompleteActivity(ev);
      if (channelAgentId && shouldRefreshSessions) {
        void refreshAgentSessionsForChannel(deps.sessionStore, channelAgentId, {
          entityKind: deps.sessionStore.entityKind,
          activeAgentId: deps.selectedAgentId.value,
        });
      }

      channelRunStatus = ev.activity.stage === 'run_status' ? (runStatusFromActivityEvent(ev)?.status ?? '') : '';
      if (channelRunStatus === SESSION_RUN_STATUS.RUNNING) {
        unsealTurnStream(sessionId);
      }
    }

    const isCurrent = deps.selectedSessionId.value === sessionId;

    const entityMatch = matchesSelectedEntityActivity(ev);
    const turnComplete = isTurnCompleteActivity(ev);

    // NOTE: Stream patching (text_delta/text_done/tool_call/tool_result) is
    // intentionally omitted — chat-rendering ActivityEvents are routed directly
    // to useActivityTimeline by useChatWorkspace.handleActivityEvent. The
    // inbound sync pipeline only handles system/control events.

    if (channelInbound && channelAgentId) {
      const focusTrigger = channelRunStatus === SESSION_RUN_STATUS.RUNNING || isSessionRevisionSyncActivity(ev);
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

    if (isCurrent && ev.activity.stage === 'run_status') {
      const rs = runStatusFromActivityEvent(ev);
      if (rs?.status === SESSION_RUN_STATUS.RUNNING) {
        unsealTurnStream(sessionId);
        if (deps.sessionStore.entityKind === 'team') {
          deps.ensureTeamStream(sessionId);
        } else {
          deps.ensureChatStream(sessionId);
        }
      }
    }

    if (!entityMatch && !isCurrent && !(channelInbound && turnComplete)) return;

    if (isCurrent && isSessionRevisionSyncActivity(ev)) {
      if (shouldSkipHydrateActivity(ev)) {
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

    if (shouldGlobalHubFinalizeTurnActivity(channelInbound, isCurrent, turnComplete)) {
      await finalizeTurnActivity(sessionId, envRev);
    }

    deps.onTurnComplete?.(sessionId);
  }

  /** ActivityEvent variant of finalizeTurn — seals the turn stream and hydrates. */
  async function finalizeTurnActivity(sessionId: string, envRev: number) {
    const localRev = deps.messageStore.sessionRevisionBySession[sessionId] ?? 0;
    flushInboundWriter(sessionId);
    sealedTurnBySession.set(sessionId, {
      revision: Math.max(envRev, localRev),
    });
    flushInboundWriter(sessionId);
    await hydrateCurrentSession(sessionId, true, true);
  }

  onMounted(() => {
    hubId = acquireGlobalWsConsumer({
      channels: ['chat'],
      logEnabled: false,
      // AF: Legacy Envelope path removed. onEnvelope is required by
      // GlobalWsConsumer but is now a no-op — all chat traffic arrives as
      // ActivityEvent via onActivityEvent.
      onEnvelope: () => {},
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

  return {
    /**
     * Activity-First: Handle an inbound ActivityEvent from the global WS hub.
     * Replaces the ActivityEvent → Envelope bridge conversion. Processes
     * system/control events (run_status, error, session.status_changed,
     * metrics_updated, spirit/team orchestration, channel inbound routing).
     * Chat-rendering events are routed to useActivityTimeline by
     * useChatWorkspace.handleActivityEvent, NOT here.
     */
    handleInboundActivityEvent,
  };
}
