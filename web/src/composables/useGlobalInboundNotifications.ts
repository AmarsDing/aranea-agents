import { onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer } from '../features/chat/globalWsHub';
import type { Envelope } from '../features/chat/envelope';
import { runStatusFromEnvelope } from '../features/chat/envelopeRunStatus';
import { parseChannelSessionMeta } from '../features/chat/channelSessionMeta';
import type { Session } from '../features/session/types';
import {
  isChannelInboundSession,
  resolveInboundSession,
  shouldChannelInboundCompleteToast,
} from '../features/chat/channelInboundSession';
import { useInboundNotificationStore } from '../stores/inboundNotifications';
import { useAppStore } from '../stores/app';
import { useChatSessionStore } from '../stores/chat/sessionStore';
import { refreshAgentSessionsForChannel } from '../features/chat/channelInboundSessionRefresh';

const TOAST_DEDUPE_MS = 4000;

function envelopeSource(env: Envelope): string {
  const direct = (env.source ?? '').trim();
  if (direct) return direct;
  const md = env.metadata as Record<string, unknown> | undefined;
  return typeof md?.source === 'string' ? md.source.trim() : '';
}

function isTurnCompleteEnvelope(env: Envelope): boolean {
  if (env.type === 'runner_completion') return true;
  if (env.type !== 'run_status') return false;
  const status = runStatusFromEnvelope(env)?.status;
  return status === 'completed' || status === 'failed' || status === 'cancelled';
}

/**
 * App-wide channel inbound bell + toast. Mounted from MainLayout so notifications
 * work on every route (not only /chat).
 */
export function useGlobalInboundNotifications() {
  const notifyStore = useInboundNotificationStore();
  const appStore = useAppStore();
  const sessionStore = useChatSessionStore();
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

  async function handleEnvelope(env: Envelope) {
    if (env.channel !== 'chat') return;
    const sessionId = (env.session_id ?? '').trim();
    if (!sessionId) return;

    const source = envelopeSource(env);
    if (!(await isChannelInbound(sessionId, source))) return;

    const sess = await resolveSession(sessionId);
    const agentId = sess?.agent_id?.trim() ?? '';
    if (!agentId) return;

    void refreshAgentSessionsForChannel(sessionStore, agentId, {
      entityKind: sessionStore.entityKind,
      activeAgentId: appStore.selectedAgent?.id,
    });

    const title = sess?.title?.trim() || parseChannelSessionMeta(sess?.metadata_json)?.channel_key || 'Channel';

    if (env.type === 'run_status') {
      const rs = runStatusFromEnvelope(env);
      if (rs?.status === 'running') {
        pushNotification(sessionId, agentId, 'running', title);
      }
    }

    if (!isTurnCompleteEnvelope(env)) return;

    pushNotification(sessionId, agentId, 'completed', title);

    const wantsToast = shouldChannelInboundCompleteToast(env);
    const now = Date.now();
    const lastToast = toastDedupeBySession.get(sessionId) ?? 0;
    const dedupeOk = now - lastToast >= TOAST_DEDUPE_MS;

    if (!isViewingSession(sessionId, agentId) && wantsToast && dedupeOk) {
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

  onMounted(() => {
    hubId = acquireGlobalWsConsumer({
      channels: ['chat'],
      logEnabled: false,
      onEnvelope: (env) => {
        void handleEnvelope(env);
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
