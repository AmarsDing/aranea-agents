import { ref, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useChatStreamingSnapshots } from '../../../stores/chatStreamingSnapshots';
import { useAuthStore } from '../../../stores/auth';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { createChatStream, createTeamStream, type UseEnvelopeStreamReturn } from '../useEnvelopeStream';
import type { Envelope, EnvelopeType, WsUpstream } from '../envelope';
import { bindStreamHandlers, patchStreamingEnvelope } from '../streamHandlers';
import { getChannelWsCursor } from '../channelWsCursor';
import { reloadSessionAfterCompletion } from '../sessionCompletionReload';
import type { TeamRow } from '../../../components/chat/types';
import type { TeamDefinition } from '../../teams/types';

export type StreamManagerDeps = {
  sessionStore: ReturnType<typeof useChatSessionStore>;
  messageStore: ReturnType<typeof useChatMessageStore>;
  runtimeStore: ReturnType<typeof useChatRuntimeStore>;
  displayTeams: Ref<TeamRow[]>;
  resolveAgentId: () => string | undefined;
  markSendingDone: () => void;
  clearSendingTimeout: () => void;
  onRunAccepted: () => void;
  onRunStatus: (env: Envelope) => void;
  touchRunActivity: () => void;
  onFirstByteArrived: () => void;
  refreshRunStatus: (sessionId?: string) => Promise<void>;
  onCompressNotice?: (sessionId: string, prevRatio: number, newRatio: number) => void;
  // T1.6: Real-time AF integration. Route activity envelopes from the stream
  // directly to useActivityTimeline so the UI updates in real-time without
  // waiting for the inbound-sync polling path.
  onActivityEnvelope?: (env: Envelope) => void;
};

export function useChatStreamManager(deps: StreamManagerDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const router = useRouter();
  const streamingSnapshots = useChatStreamingSnapshots();

  let chatStream: UseEnvelopeStreamReturn | null = null;
  let chatStreamSessionId: string | null = null;
  let chatStreamCleanup: (() => void) | null = null;
  let teamStream: UseEnvelopeStreamReturn | null = null;
  let teamStreamSessionId: string | null = null;
  let teamStreamCleanup: (() => void) | null = null;

  const wsReplaying = ref(false);
  /**
   * P3-5: Run-stale indicator. Set to `true` when no `run_heartbeat` arrives
   * within `WS_RUN_STALE_TIMEOUT_MS` (30s). Reset to `false` on heartbeat,
   * reconnect, or explicit `recover()` call.
   *
   * The stale timer is started by `transport.resetStaleTimer()` (called on
   * run start via `onRunAccepted`). When the timer fires, `onStale` is
   * invoked and `isStale` flips to `true`, surfacing a "Recover" button in
   * the chat UI.
   */
  const isStale = ref(false);
  let lastErrorNotifyMessage = '';
  let lastErrorNotifyAt = 0;

  /**
   * Ordered execution_progress envelopes for the active chat / team stream.
   * Implemented as a bounded ring buffer (S2 + S11):
   *   - O(1) `pushProgress` append (the spread pattern was O(n) per insert)
   *   - capped at MAX_PROGRESS_ENVELOPES to keep long sessions bounded
   *   - cleared on stream re-create and on every new run-accepted
   *
   * The exposed ref shares the underlying array — Vue 3's reactive array
   * tracking handles `push()` / `splice()` correctly, so we don't need to
   * clone the buffer on every mutation. The TS type is `readonly Envelope[]`
   * so consumers cannot accidentally mutate it.
   *
   * @see docs/reports/2026-06-10-proposal-execution-progress-inline.md
   */
  const MAX_PROGRESS_ENVELOPES = 200;
  const progressBuffer: Envelope[] = [];
  const executionProgress = ref<readonly Envelope[]>(progressBuffer);

  function pushProgress(env: Envelope): void {
    progressBuffer.push(env);
    if (progressBuffer.length > MAX_PROGRESS_ENVELOPES) {
      // Drop oldest envelopes to keep the buffer bounded. Amortized O(1)
      // per push — `splice(0, n)` only runs when we overflow.
      const overflow = progressBuffer.length - MAX_PROGRESS_ENVELOPES;
      progressBuffer.splice(0, overflow);
    }
    // Vue 3's reactive ref tracks array mutations on `.push()`/`.splice()`
    // automatically. We do NOT re-assign `.value` to avoid the spread cost.
  }

  function clearProgress(): void {
    progressBuffer.length = 0;
  }

  function notifyError(message: string) {
    const now = Date.now();
    if (message === lastErrorNotifyMessage && now - lastErrorNotifyAt < 5000) {
      return;
    }
    lastErrorNotifyMessage = message;
    lastErrorNotifyAt = now;
    $q.notify({ type: 'negative', message, group: 'chat-stream-error' });
  }

  function notifyOrchestration(message: string) {
    $q.notify({ type: 'info', message, timeout: 4000, group: false });
  }

  async function reloadSessionMessagesAfterCompletion(sessionId: string) {
    try {
      await reloadSessionAfterCompletion({
        sessionStore: deps.sessionStore,
        messageStore: deps.messageStore,
        streamingSnapshots,
        sessionId,
        resolveAgentId: deps.resolveAgentId,
      });
    } catch (err) {
      notifyError(err instanceof Error ? err.message : t('chat.loadMessagesFailed', '加载消息失败'));
    }
  }

  function resolveTeamMemberMeta(agentKey: string) {
    const team = deps.displayTeams.value.find((row) => row.id === deps.sessionStore.selectedTeamId);
    const def: TeamDefinition | null = (() => {
      try {
        return team?.definition_json ? (JSON.parse(team.definition_json) as TeamDefinition) : null;
      } catch {
        return null;
      }
    })();
    const member = def?.members?.find((m) => m.agent_id === agentKey || m.name === agentKey);
    return {
      agent_key: agentKey,
      name: member?.name || agentKey,
      role: member?.role || '',
    };
  }

  function sessionContextHandlers(sessionId: string) {
    return {
      onSessionContextPatch: (sid: string, patch: Parameters<typeof deps.sessionStore.patchSessionMetricsLocal>[1]) => {
        deps.sessionStore.patchSessionMetricsLocal(sid, patch);
      },
      onCompressNotice: (sid: string, prevRatio: number, newRatio: number) => {
        deps.onCompressNotice?.(sid, prevRatio, newRatio);
      },
      getSessionMetrics: (sid: string) => {
        const row = deps.sessionStore.findSessionById(sid);
        if (!row) return undefined;
        return {
          total_tokens: row.total_tokens,
          max_context_used_ratio: row.max_context_used_ratio,
          input_tokens: row.input_tokens,
          output_tokens: row.output_tokens,
        };
      },
    };
  }

  function ensureChatStream(sessionId: string) {
    if (chatStream && chatStreamSessionId === sessionId) {
      // Only sync wsConnected→true when the stream reports connected;
      // never downgrade to false here — onDisconnected is the authoritative
      // source for disconnection and avoids stale ref reads after onError.
      if (chatStream.connected.value) {
        deps.runtimeStore.setWsConnected(sessionId, true);
      }
      if (!chatStream.connected.value) {
        chatStream.connect();
      }
      return chatStream;
    }
    // B-02: Clean up previous stream's timeout & batch writer before creating
    // a new one, preventing the stale timeout from firing on the new session.
    chatStreamCleanup?.();
    chatStreamCleanup = null;
    chatStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);
    // New stream: reset progress accumulator so we don't leak envelopes from
    // a prior turn into the current one.
    clearProgress();

    chatStream = createChatStream(sessionId, {
      lastEventId: getChannelWsCursor(sessionId),
      onConnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, true);
        // P3-5: a fresh connection implies the run is no longer stale.
        isStale.value = false;
        void deps.refreshRunStatus(sessionId);
      },
      onDisconnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, false);
      },
      onServerShutdown: () => {
        $q.notify({
          type: 'warning',
          message: t('chat.serverShutdown', '服务器已关闭，请重新登录'),
          timeout: 0,
          actions: [{ label: t('chat.relogin', '重新登录'), color: 'white', handler: () => {} }],
        });
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: 'login' });
      },
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
      onReconnectFailed: () => {
        $q.notify({
          type: 'negative',
          message: t('chat.reconnectFailed', '连接已断开，请刷新页面重试'),
          timeout: 0,
          actions: [{ label: t('chat.refresh', '刷新页面'), color: 'white', handler: () => window.location.reload() }],
        });
      },
      // P3-5: heartbeat clears stale; stale timer fires onStale.
      onHeartbeat: () => {
        isStale.value = false;
      },
      onStale: () => {
        isStale.value = true;
      },
    });

    chatStreamCleanup = bindStreamHandlers(
      chatStream,
      {
        sessionId,
        resolveActiveSessionId: () => deps.sessionStore.selectedSession?.id ?? null,
        getMessages: (sid) => deps.messageStore.getMessages(sid),
        setMessages: (sid, rows) => deps.messageStore.setMessages(sid, rows),
        markSendingDone: deps.markSendingDone,
        clearSendingTimeout: deps.clearSendingTimeout,
        // Reset the progress accumulator when a new run is accepted so a new
        // turn starts with a clean timeline (avoids leaking the previous
        // turn's orchestration steps into the next one).
        // P3-5: start the run-stale timer so that if no `run_heartbeat`
        // arrives within WS_RUN_STALE_TIMEOUT_MS (30s), `onStale` fires and
        // surfaces the "Recover" button. The timer is auto-reset by the
        // transport on every subsequent heartbeat.
        onRunAccepted: () => {
          clearProgress();
          chatStream?.transport.value?.resetStaleTimer();
          deps.onRunAccepted();
        },
        onRunStatus: deps.onRunStatus,
        onErrorNotify: notifyError,
        onOrchestrationNotice: notifyOrchestration,
        onReloadAfterCompletion: reloadSessionMessagesAfterCompletion,
        ...sessionContextHandlers(sessionId),
        onRunActivity: deps.touchRunActivity,
        onFirstByteArrived: deps.onFirstByteArrived,
        onExecutionProgress: pushProgress,
        // T1.6: Real-time AF — route activity envelopes directly to the
        // timeline handler for immediate UI updates.
        onActivityEnvelope: deps.onActivityEnvelope,
      },
      { batched: true },
    );

    chatStream.connect();
    chatStreamSessionId = sessionId;
    return chatStream;
  }

  function ensureTeamStream(sessionId: string) {
    if (teamStream && teamStreamSessionId === sessionId) {
      // Only sync wsConnected→true; never downgrade — onDisconnected is authoritative.
      if (teamStream.connected.value) {
        deps.runtimeStore.setWsConnected(sessionId, true);
      }
      // B-01: Reconnect if the transport exists but is disconnected.
      if (!teamStream.connected.value) {
        teamStream.connect();
      }
      return teamStream;
    }
    // B-02: Clean up previous stream's timeout & batch writer before creating
    // a new one, preventing the stale timeout from firing on the new session.
    teamStreamCleanup?.();
    teamStreamCleanup = null;
    teamStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);
    // New team stream: also reset progress accumulator so we don't leak
    // envelopes from a prior turn (or a different session kind) into the
    // current one.
    clearProgress();

    teamStream = createTeamStream(sessionId, {
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
      onReconnectFailed: () => {
        $q.notify({
          type: 'negative',
          message: t('chat.reconnectFailed', '连接已断开，请刷新页面重试'),
          timeout: 0,
          actions: [{ label: t('chat.refresh', '刷新页面'), color: 'white', handler: () => window.location.reload() }],
        });
      },
      onServerShutdown: () => {
        $q.notify({
          type: 'warning',
          message: t('chat.serverShutdown', '服务器已关闭，请重新登录'),
          timeout: 0,
          actions: [{ label: t('chat.relogin', '重新登录'), color: 'white', handler: () => {} }],
        });
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: 'login' });
      },
      onConnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, true);
        // P3-5: a fresh connection implies the run is no longer stale.
        isStale.value = false;
        void deps.refreshRunStatus(sessionId);
      },
      // P3-5: heartbeat clears stale; stale timer fires onStale. Mirrors
      // the chat stream binding so team sessions get the same stale
      // detection as agent sessions.
      onHeartbeat: () => {
        isStale.value = false;
      },
      onStale: () => {
        isStale.value = true;
      },
    });

    teamStreamCleanup = bindStreamHandlers(teamStream, {
      sessionId,
      resolveActiveSessionId: () => deps.sessionStore.teamSelectedSessionId,
      getMessages: (sid) => deps.messageStore.getMessages(sid),
      setMessages: (sid, rows) => deps.messageStore.setMessages(sid, rows),
      markSendingDone: deps.markSendingDone,
      clearSendingTimeout: deps.clearSendingTimeout,
      // Mirror chat stream: a new team run means a new turn — drop the
      // progress accumulator to avoid cross-turn envelope leakage.
      // P3-5: start the run-stale timer for team sessions too.
      onRunAccepted: () => {
        clearProgress();
        teamStream?.transport.value?.resetStaleTimer();
        deps.onRunAccepted();
      },
      onRunStatus: deps.onRunStatus,
      onErrorNotify: notifyError,
      onOrchestrationNotice: notifyOrchestration,
      onReloadAfterCompletion: reloadSessionMessagesAfterCompletion,
      ...sessionContextHandlers(sessionId),
      resolveMemberMeta: resolveTeamMemberMeta,
      onRunActivity: deps.touchRunActivity,
      onFirstByteArrived: deps.onFirstByteArrived,
      // Spirit / team sessions must also accumulate execution_progress so the
      // timeline shows inline orchestration / team / tool step cards
      // during the multi-agent fan-out. Mirrors the chat stream binding.
      onExecutionProgress: pushProgress,
      // T1.6: Real-time AF — route activity envelopes directly to the
      // timeline handler for immediate UI updates.
      onActivityEnvelope: deps.onActivityEnvelope,
    });

    teamStream.connect();
    teamStreamSessionId = sessionId;
    return teamStream;
  }

  function sendChatViaWs(stream: UseEnvelopeStreamReturn, upstream: WsUpstream): void {
    stream.connect();
    const transport = stream.transport.value;
    if (!transport) {
      throw new Error('WebSocket transport unavailable');
    }
    transport.send(upstream);
  }

  function disconnectChatStream() {
    if (chatStreamSessionId) {
      deps.runtimeStore.setWsConnected(chatStreamSessionId, false);
    }
    chatStreamCleanup?.();
    chatStreamCleanup = null;
    chatStream?.disconnect();
    chatStream = null;
    chatStreamSessionId = null;
  }

  function disconnectTeamStream() {
    if (teamStreamSessionId) {
      deps.runtimeStore.setWsConnected(teamStreamSessionId, false);
    }
    teamStreamCleanup?.();
    teamStreamCleanup = null;
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
    deps.messageStore.setMessages(
      sessionId,
      patchStreamingEnvelope(deps.messageStore.getMessages(sessionId), sessionId, streamId, env, isDone),
    );
  }

  function subscribeSessionStream(
    sessionId: string,
    ownerKind: 'agent' | 'team',
    types: EnvelopeType[],
    handler: (env: Envelope) => void,
  ): () => void {
    const stream = ownerKind === 'team' ? ensureTeamStream(sessionId) : ensureChatStream(sessionId);
    return stream.onType(types, handler);
  }

  /**
   * P3-5: Recover from a stale WS run. Tears down and recreates the active
   * stream(s) so the new transport gets a fresh stale timer, and clears
   * `isStale` immediately so the UI hides the "Recover" button.
   *
   * We capture the session ids before teardown because `disconnectXxxStream`
   * nulls them. If a stream is not active (null session id), it is skipped —
   * recovering only the streams that were actually in use.
   */
  function recover(): void {
    const chatSid = chatStreamSessionId;
    if (chatSid) {
      disconnectChatStream();
      ensureChatStream(chatSid);
    }
    const teamSid = teamStreamSessionId;
    if (teamSid) {
      disconnectTeamStream();
      ensureTeamStream(teamSid);
    }
    isStale.value = false;
  }

  return {
    wsReplaying,
    executionProgress,
    isStale,
    ensureChatStream,
    ensureTeamStream,
    subscribeSessionStream,
    sendChatViaWs,
    disconnectChatStream,
    disconnectTeamStream,
    disconnectAll,
    cancelActiveStream,
    patchAgentMessages,
    recover,
  };
}
