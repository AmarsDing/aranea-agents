import { onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer } from '../features/chat/globalWsHub';
import { runStatusFromV2Payload } from '../features/chat/activityRunStatus';
import { parseChannelSessionMeta } from '../features/chat/channelSessionMeta';
import type { Session } from '../features/session/types';
import {
  isChannelInboundSession,
  resolveInboundSession,
} from '../features/chat/channelInboundSession';
import { useInboundNotificationStore } from '../stores/inboundNotifications';
import { useAppStore } from '../stores/app';
import { useChatSessionStore } from '../stores/chat/sessionStore';
import { useChatConversationStore } from '../stores/chat/conversationStore';
import { refreshAgentSessionsForChannel } from '../features/chat/channelInboundSessionRefresh';
import type { RunStatusEventPayload, V2WsEnvelope } from '../features/chat/v2Types';
import { SESSION_RUN_STATUS } from '../features/chat/sessionRunStatus';

const TOAST_DEDUPE_MS = 4000;

const TERMINAL_KINDS = new Set([
  'task.completed',
  'task.failed',
  'turn.completed',
  'turn.failed',
]);

/**
 * App-wide channel inbound bell + toast. Mounted from MainLayout so notifications
 * work on every route (not only /chat).
 */
export function useGlobalInboundNotifications() {
  const notifyStore = useInboundNotificationStore();
  const appStore = useAppStore();
  const sessionStore = useChatSessionStore();
  const conversationStore = useChatConversationStore();
  const router = useRouter();
  const $q = useQuasar();
  const { t } = useI18n();
  let hubId: string | null = null;
  const toastDedupeBySession = new Map<string, number>();

  async function resolveSession(sessionId: string): Promise<Session | null> {
    return resolveInboundSession(sessionId, sessionStore);
  }

  async function isChannelInbound(sessionId: string, source: string): Promise<boolean> {
    return isChannelInboundSession(sessionId, source, sessionStore);
  }

  function isViewingSession(sessionId: string, agentId: string): boolean {
    const route = router.currentRoute.value;
    if (route.name !== 'chat') return false;
    const sid = sessionStore.currentSessionId();
    if (sid !== sessionId) return false;
    const selectedAgent = appStore.selectedAgent?.id?.trim() ?? '';
    return selectedAgent === agentId.trim();
  }

  function syncConversationInbox(sessionId: string, sess: Session | null, isViewing: boolean) {
    if (!sess) return;
    const title = sess.title?.trim() || parseChannelSessionMeta(sess.metadata_json)?.channel_key || 'Channel';
    const existing = conversationStore.sessionsById[sessionId];
    conversationStore.upsertSession({
      id: sessionId,
      title,
      target: existing?.target ?? {
        type: 'agent',
        id: sess.agent_id,
        source: 'channel',
      },
      unreadCount: existing?.unreadCount ?? 0,
    });
    if (!isViewing) {
      conversationStore.addInboxSession(sessionId);
    }
  }

  function pushNotification(sessionId: string, agentId: string, kind: 'running' | 'completed', title: string) {
    notifyStore.upsert({
      id: `channel:${sessionId}`,
      sessionId,
      agentId,
      title: title || t('chat.inboundNotify.title', '渠道通知'),
      preview:
        kind === 'running'
          ? t('chat.channelInboundRunning', '飞书/渠道有新消息进行中')
          : t('chat.channelInboundNotify', '飞书/渠道有新回复'),
      source: 'channel',
      kind,
      ts: Date.now(),
    });
  }

  async function handleInboundSession(sessionId: string, source: string, opts: {
    running?: boolean;
    completed?: boolean;
    toast?: boolean;
  }) {
    if (!(await isChannelInbound(sessionId, source))) return;

    const sess = await resolveSession(sessionId);
    const agentId = sess?.agent_id?.trim() ?? '';
    if (!agentId) return;

    void refreshAgentSessionsForChannel(sessionStore, agentId, {
      entityKind: sessionStore.entityKind,
      activeAgentId: appStore.selectedAgent?.id,
    });

    const title = sess?.title?.trim() || parseChannelSessionMeta(sess?.metadata_json)?.channel_key || 'Channel';
    syncConversationInbox(sessionId, sess, isViewingSession(sessionId, agentId));

    if (opts.running) {
      pushNotification(sessionId, agentId, 'running', title);
    }
    if (!opts.completed) return;

    pushNotification(sessionId, agentId, 'completed', title);

    const now = Date.now();
    const lastToast = toastDedupeBySession.get(sessionId) ?? 0;
    const dedupeOk = now - lastToast >= TOAST_DEDUPE_MS;

    if (!isViewingSession(sessionId, agentId) && opts.toast && dedupeOk) {
      toastDedupeBySession.set(sessionId, now);
      $q.notify({
        type: 'info',
        message: t('chat.channelInboundNotify', '飞书/渠道有新回复'),
        timeout: 5000,
        actions: [
          {
            label: t('chat.channelInboundOpen', '查看'),
            color: 'white',
            handler: () => {
              void router.push({ name: 'chat', query: { session: sessionId, agent: agentId } });
            },
          },
        ],
      });
    }
  }

  async function handleV2Event(envelope: V2WsEnvelope) {
    const sessionId = String(envelope.session_id ?? '').trim();
    if (!sessionId) return;

    if (envelope.kind === 'system.run_status') {
      const payload = envelope.payload as RunStatusEventPayload;
      const rs = runStatusFromV2Payload(payload);
      if (!rs) return;
      const source = String(payload.Meta?.source ?? '').trim();
      if (rs.status === 'running') {
        await handleInboundSession(sessionId, source, { running: true });
        return;
      }
      const terminal =
        rs.status === SESSION_RUN_STATUS.COMPLETED ||
        rs.status === SESSION_RUN_STATUS.FAILED ||
        rs.status === SESSION_RUN_STATUS.CANCELLED;
      if (!terminal) return;
      const toast =
        rs.status === SESSION_RUN_STATUS.FAILED ||
        rs.status === SESSION_RUN_STATUS.CANCELLED ||
        (rs.status === SESSION_RUN_STATUS.COMPLETED && source === 'channel');
      await handleInboundSession(sessionId, source, { completed: true, toast });
      return;
    }

    if (TERMINAL_KINDS.has(envelope.kind)) {
      await handleInboundSession(sessionId, 'channel', { completed: true, toast: true });
    }
  }

  onMounted(() => {
    hubId = acquireGlobalWsConsumer({
      channels: ['chat'],
      logEnabled: false,
      onV2Event: (envelope) => {
        void handleV2Event(envelope);
      },
    });
  });

  onUnmounted(() => {
    if (hubId) {
      releaseGlobalWsConsumer(hubId);
      hubId = null;
    }
  });
}
