import { computed, reactive, ref } from 'vue';
import { clearChatDrafts } from '../chatDraftPersistence';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../../../stores/auth';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { createChatStream, createTeamStream } from '../useEnvelopeStream';
import type { WsSessionStream } from '../../../realtime/createWsSessionStream';
import type { WsUpstream } from '../../../realtime/ws-transport';
import type { V2WsEnvelope } from '../v2Types';
import { getChannelWsCursor } from '../channelWsCursor';

export type StreamManagerDeps = {
  runtimeStore: ReturnType<typeof useChatRuntimeStore>;
  /** v2 chat events: dispatched when a v2_event WS envelope arrives. */
  onV2Event?: (envelope: V2WsEnvelope) => void;
  refreshRunStatus: (sessionId?: string) => Promise<void>;
  /**
   * B-06: authoritative v2 snapshot hydration after WS reconnect.
   * Server no longer replays events; clients must re-fetch REST history.
   */
  onReconnectHydrate?: (sessionId: string) => Promise<void>;
  visibleSessionId?: () => string | undefined;
  /** businessQueue drop of user_message — clear matching optimistic pending card. */
  onUpstreamUserMessageDropped?: (sessionId: string, requestId: string) => void;
};

export function useChatStreamManager(deps: StreamManagerDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const router = useRouter();

  let chatStream: WsSessionStream | null = null;
  let chatStreamSessionId: string | null = null;
  let teamStream: WsSessionStream | null = null;
  let teamStreamSessionId: string | null = null;
  let chatEpoch = 0;
  let teamEpoch = 0;
  const activeSessionId = ref('');

  /** True while reconnect snapshot hydrate is in flight (B-06). */
  const recovery = reactive(new Map<string, 'syncing' | 'error'>());
  const recoveryEpochs = new Map<string, number>();
  const recoveryRequests = new Map<string, { epoch: number; promise: Promise<void> }>();
  const visibleSessionId = () => (deps.visibleSessionId ? (deps.visibleSessionId() ?? '') : activeSessionId.value);
  const wsReplaying = computed(() => recovery.get(visibleSessionId()) === 'syncing');
  const reconnectError = computed(() => recovery.get(visibleSessionId()) === 'error');
  /** Per-session: true after a disconnect so next onConnected triggers hydrate. */
  const needsHydrateAfterReconnect = new Map<string, boolean>();

  function invalidateRecovery(sessionId: string) {
    recoveryEpochs.set(sessionId, (recoveryEpochs.get(sessionId) ?? 0) + 1);
    recovery.delete(sessionId);
  }
  async function hydrateAfterReconnect(sessionId: string): Promise<void> {
    const epoch = recoveryEpochs.get(sessionId) ?? 0;
    const pending = recoveryRequests.get(sessionId);
    if (pending?.epoch === epoch) return pending.promise;
    recovery.set(sessionId, 'syncing');
    const promise = (async () => {
      try {
        await Promise.all([deps.refreshRunStatus(sessionId), deps.onReconnectHydrate?.(sessionId)]);
        if (epoch !== (recoveryEpochs.get(sessionId) ?? 0)) return;
        needsHydrateAfterReconnect.set(sessionId, false);
        recovery.delete(sessionId);
      } catch (e) {
        if (epoch !== (recoveryEpochs.get(sessionId) ?? 0)) return;
        needsHydrateAfterReconnect.set(sessionId, true);
        recovery.set(sessionId, 'error');
        console.warn('[chat] reconnect v2 hydrate failed', e);
      }
    })();
    recoveryRequests.set(sessionId, { epoch, promise });
    try {
      await promise;
    } finally {
      if (recoveryRequests.get(sessionId)?.promise === promise) recoveryRequests.delete(sessionId);
    }
  }

  /**
   * F1（2026-08-21 全链路审查）：业务消息因发送队列满被 transport 丢弃时
   * 的通知接线。此前全链路无人传 onDrop，user_message 会静默丢失——用户
   * 看到消息已发出但后端从未收到。现在至少明示用户重发，并在下次连接后
   * 触发 REST 水合以对齐时间线。
   */
  function handleUpstreamDrop(sessionId: string) {
    return (upstream: WsUpstream) => {
      if (upstream.type === 'user_message') {
        $q.notify({
          type: 'warning',
          message: t('chat.wsQueueFullDropped', '连接拥塞，消息未能送达，请重新发送'),
        });
        // Clear the optimistic pending card for this exact request so the user
        // can resend without a stuck "pending" ghost task.
        const requestId = String(upstream.request_id ?? '').trim();
        if (requestId && deps.onUpstreamUserMessageDropped) {
          deps.onUpstreamUserMessageDropped(sessionId, requestId);
        }
      }
      needsHydrateAfterReconnect.set(sessionId, true);
    };
  }

  function ensureChatStream(sessionId: string) {
    activeSessionId.value = sessionId;
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
    // B-02: Disconnect previous stream before creating a new one.
    chatEpoch++;
    if (chatStreamSessionId) invalidateRecovery(chatStreamSessionId);
    chatStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);
    needsHydrateAfterReconnect.delete(sessionId);

    chatStreamSessionId = sessionId;
    const epoch = chatEpoch;
    const current = () => epoch === chatEpoch && chatStreamSessionId === sessionId;
    chatStream = createChatStream(sessionId, {
      lastEventId: getChannelWsCursor(sessionId),
      // E2E-C2: transport 自动重连时实时回传最新游标（live 推进只进 cursor 表）。
      getLastEventId: () => getChannelWsCursor(sessionId),
      onConnected: () => {
        if (!current()) return;
        deps.runtimeStore.setWsConnected(sessionId, true);
        // B-06: after a disconnect, re-hydrate authoritative v2 snapshot.
        // Server also replays missed critical outbox frames via last_event_id;
        // REST hydrate remains the safety net for non-critical state.
        if (needsHydrateAfterReconnect.get(sessionId)) {
          void hydrateAfterReconnect(sessionId);
        } else
          void deps.refreshRunStatus(sessionId).catch(() => {
            if (current()) recovery.set(sessionId, 'error');
          });
      },
      onDisconnected: () => {
        if (!current()) return;
        invalidateRecovery(sessionId);
        deps.runtimeStore.setWsConnected(sessionId, false);
        needsHydrateAfterReconnect.set(sessionId, true);
      },
      onServerShutdown: () => {
        if (!current()) return;
        clearChatDrafts();
        // F4 (2026-10-19): timeout:0 的持久 toast 必须提供可用的 dismiss
        // 路径——旧 action handler 为空函数，toast 永远无法关闭。
        const dismiss = $q.notify({
          type: 'warning',
          message: t('chat.serverShutdown', '服务器已关闭，请重新登录'),
          timeout: 0,
          actions: [{ label: t('chat.relogin', '重新登录'), color: 'white', handler: () => dismiss() }],
        });
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: 'login' });
      },
      onV2Event: deps.onV2Event,
      onDrop: handleUpstreamDrop(sessionId),
    });

    chatStream.connect();
    chatStreamSessionId = sessionId;
    return chatStream;
  }

  function ensureTeamStream(sessionId: string) {
    activeSessionId.value = sessionId;
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
    // B-02: Disconnect previous stream before creating a new one.
    teamEpoch++;
    if (teamStreamSessionId) invalidateRecovery(teamStreamSessionId);
    teamStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);
    needsHydrateAfterReconnect.delete(sessionId);

    teamStreamSessionId = sessionId;
    const epoch = teamEpoch;
    const current = () => epoch === teamEpoch && teamStreamSessionId === sessionId;
    teamStream = createTeamStream(sessionId, {
      lastEventId: getChannelWsCursor(sessionId),
      // E2E-C2: transport 自动重连时实时回传最新游标（live 推进只进 cursor 表）。
      getLastEventId: () => getChannelWsCursor(sessionId),
      onServerShutdown: () => {
        if (!current()) return;
        clearChatDrafts();
        // F4: 持久 toast 的 action 必须真正可关闭（详见上方 chatStream 同注释）。
        const dismiss = $q.notify({
          type: 'warning',
          message: t('chat.serverShutdown', '服务器已关闭，请重新登录'),
          timeout: 0,
          actions: [{ label: t('heartbeat.relogin'), color: 'white', handler: () => dismiss() }],
        });
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: 'login' });
      },
      onConnected: () => {
        if (!current()) return;
        deps.runtimeStore.setWsConnected(sessionId, true);
        if (needsHydrateAfterReconnect.get(sessionId)) {
          void hydrateAfterReconnect(sessionId);
        } else
          void deps.refreshRunStatus(sessionId).catch(() => {
            if (current()) recovery.set(sessionId, 'error');
          });
      },
      onDisconnected: () => {
        if (!current()) return;
        invalidateRecovery(sessionId);
        deps.runtimeStore.setWsConnected(sessionId, false);
        needsHydrateAfterReconnect.set(sessionId, true);
      },
      onV2Event: deps.onV2Event,
      onDrop: handleUpstreamDrop(sessionId),
    });

    teamStream.connect();
    teamStreamSessionId = sessionId;
    return teamStream;
  }

  function sendChatViaWs(stream: WsSessionStream, upstream: WsUpstream): void {
    stream.connect();
    const transport = stream.transport.value;
    if (!transport) {
      throw new Error('WebSocket transport unavailable');
    }
    // Delegate to transport.send — it already handles non-OPEN state correctly
    // by enqueuing to businessQueue (never dropped) and flushing on ws.onopen.
    // This guarantees the backend WS subscription (setupEventSubscription runs
    // after handshake) is ready before the user_message is delivered, so the
    // subsequent v2_event frames flow back through the same WS.
    // Do NOT throw on !transport.connected — that would force HTTP fallback
    // and create a subscription race (HTTP delivers immediately but the WS
    // subscription isn't set up yet, so emitted events are missed → UI blank
    // until page refresh).
    transport.send(upstream);
  }

  function disconnectChatStream() {
    chatEpoch++;
    if (chatStreamSessionId) {
      invalidateRecovery(chatStreamSessionId);
      deps.runtimeStore.setWsConnected(chatStreamSessionId, false);
    }
    chatStream?.disconnect();
    chatStream = null;
    chatStreamSessionId = null;
  }

  function disconnectTeamStream() {
    teamEpoch++;
    if (teamStreamSessionId) {
      invalidateRecovery(teamStreamSessionId);
      deps.runtimeStore.setWsConnected(teamStreamSessionId, false);
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

  return {
    wsReplaying,
    reconnectError,
    retryReconnect: async () => {
      const sid = visibleSessionId();
      const stream = sid === chatStreamSessionId ? chatStream : sid === teamStreamSessionId ? teamStream : null;
      if (!stream) return;
      needsHydrateAfterReconnect.set(sid, true);
      if (stream.connected.value) await hydrateAfterReconnect(sid);
      else stream.connect();
    },
    ensureChatStream,
    ensureTeamStream,
    sendChatViaWs,
    disconnectChatStream,
    disconnectTeamStream,
    disconnectAll,
    cancelActiveStream,
  };
}
