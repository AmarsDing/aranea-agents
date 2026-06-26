import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useAuthStore } from '../../../stores/auth';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { createChatStream, createTeamStream, type UseEnvelopeStreamReturn } from '../useEnvelopeStream';
import type { Envelope, EnvelopeType, WsUpstream } from '../envelope';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import { getChannelWsCursor } from '../channelWsCursor';

export type StreamManagerDeps = {
  runtimeStore: ReturnType<typeof useChatRuntimeStore>;
  /**
   * Activity-First (AF): called when an activity_event WS message arrives.
   * Replaces the legacy onActivityEnvelope for chat events. The handler
   * receives a business-semantic ActivityEvent with a full Activity snapshot.
   */
  onActivityEvent?: (ev: ActivityEvent) => void;
  refreshRunStatus: (sessionId?: string) => Promise<void>;
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
    // B-02: Disconnect previous stream before creating a new one.
    chatStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);

    chatStream = createChatStream(sessionId, {
      lastEventId: getChannelWsCursor(sessionId),
      onConnected: () => {
        deps.runtimeStore.setWsConnected(sessionId, true);
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
      onActivityEvent: deps.onActivityEvent,
    });

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
    // B-02: Disconnect previous stream before creating a new one.
    teamStream?.disconnect();
    deps.runtimeStore.setWsConnected(sessionId, false);

    teamStream = createTeamStream(sessionId, {
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
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
        void deps.refreshRunStatus(sessionId);
      },
      onActivityEvent: deps.onActivityEvent,
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
    chatStream?.disconnect();
    chatStream = null;
    chatStreamSessionId = null;
  }

  function disconnectTeamStream() {
    if (teamStreamSessionId) {
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

  /**
   * Subscribe to raw envelope types on the session stream. Used by the event
   * inspector (GET /v1/events API), which still returns envelopes. This is
   * the ONE exception to the ActivityEvent-only rule — the inspector API has
   * not been migrated to ActivityEvent yet.
   */
  function subscribeSessionStream(
    sessionId: string,
    ownerKind: 'agent' | 'team',
    types: EnvelopeType[],
    handler: (env: Envelope) => void,
  ): () => void {
    const stream = ownerKind === 'team' ? ensureTeamStream(sessionId) : ensureChatStream(sessionId);
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
  };
}
