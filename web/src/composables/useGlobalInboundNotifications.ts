import { onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { acquireGlobalWsConsumer, releaseGlobalWsConsumer } from "../features/chat/globalWsHub";
import type { Envelope } from "../features/chat/envelope";
import { runStatusFromEnvelope } from "../features/chat/envelopeRunStatus";
import { parseChannelSessionMeta, isChannelSession } from "../features/chat/channelSessionMeta";
import { getSession } from "../features/session/api";
import type { Session } from "../features/session/types";
import { useInboundNotificationStore } from "../stores/inboundNotifications";
import { useAppStore } from "../stores/app";
import { useChatStore } from "../stores/chat";
import { refreshAgentSessionsForChannel } from "../features/chat/channelInboundSessionRefresh";

const SESSION_CACHE_MAX = 64;
const sessionCache = new Map<string, Session>();

function cacheSession(sessionId: string, row: Session) {
  sessionCache.delete(sessionId);
  sessionCache.set(sessionId, row);
  while (sessionCache.size > SESSION_CACHE_MAX) {
    const oldest = sessionCache.keys().next().value;
    if (!oldest) break;
    sessionCache.delete(oldest);
  }
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

/** Toast only on runner_completion or terminal run_status (failed/cancelled) to avoid duplicate toasts. */
function shouldToastOnTurnComplete(env: Envelope): boolean {
  if (env.type === "runner_completion") return true;
  if (env.type === "run_status") {
    const status = runStatusFromEnvelope(env)?.status;
    return status === "failed" || status === "cancelled";
  }
  return false;
}

/**
 * App-wide channel inbound bell + toast. Mounted from MainLayout so notifications
 * work on every route (not only /chat).
 */
export function useGlobalInboundNotifications() {
  const notifyStore = useInboundNotificationStore();
  const appStore = useAppStore();
  const chatStore = useChatStore();
  const router = useRouter();
  const $q = useQuasar();
  const { t } = useI18n();
  let hubId: string | null = null;

  async function resolveSession(sessionId: string): Promise<Session | null> {
    const hit = chatStore.findSessionById(sessionId);
    if (hit) return hit as Session;
    const cached = sessionCache.get(sessionId);
    if (cached) return cached;
    try {
      const row = await getSession(sessionId);
      cacheSession(sessionId, row);
      return row;
    } catch {
      return null;
    }
  }

  async function isChannelInbound(sessionId: string, source: string): Promise<boolean> {
    if (source === "channel") return true;
    const sess = await resolveSession(sessionId);
    if (!sess) return false;
    return parseChannelSessionMeta(sess.metadata_json) !== null || isChannelSession(sess.metadata_json, sess.title);
  }

  function isViewingSession(sessionId: string): boolean {
    const route = router.currentRoute.value;
    if (route.name !== "chat") return false;
    const q = route.query.session;
    const querySid = typeof q === "string" ? q.trim() : Array.isArray(q) ? String(q[0] ?? "").trim() : "";
    if (querySid === sessionId) return true;
    return chatStore.currentSessionId() === sessionId;
  }

  function pushNotification(
    sessionId: string,
    agentId: string,
    kind: "running" | "completed",
    title: string
  ) {
    notifyStore.upsert({
      id: `channel:${sessionId}`,
      sessionId,
      agentId,
      title: title || t("chat.inboundNotify.title", "渠道通知"),
      preview:
        kind === "running"
          ? t("chat.channelInboundRunning", "飞书/渠道有新消息进行中")
          : t("chat.channelInboundNotify", "飞书/渠道有新回复"),
      source: "channel",
      kind,
      ts: Date.now(),
    });
  }

  async function handleEnvelope(env: Envelope) {
    if (env.channel !== "chat") return;
    const sessionId = (env.session_id ?? "").trim();
    if (!sessionId) return;

    const source = envelopeSource(env);
    if (!(await isChannelInbound(sessionId, source))) return;

    const sess = await resolveSession(sessionId);
    const agentId = sess?.agent_id?.trim() ?? "";
    if (!agentId) return;

    void refreshAgentSessionsForChannel(chatStore, agentId, {
      entityKind: chatStore.entityKind,
      activeAgentId: appStore.selectedAgent?.id,
    });

    const title = sess?.title?.trim() || parseChannelSessionMeta(sess?.metadata_json)?.channel_key || "Channel";

    if (env.type === "run_status") {
      const rs = runStatusFromEnvelope(env);
      if (rs?.status === "running") {
        pushNotification(sessionId, agentId, "running", title);
      }
    }

    if (!isTurnCompleteEnvelope(env)) return;

    pushNotification(sessionId, agentId, "completed", title);

    if (!isViewingSession(sessionId) && shouldToastOnTurnComplete(env)) {
      $q.notify({
        type: "info",
        message: t("chat.channelInboundNotify", "飞书/渠道有新回复"),
        timeout: 5000,
        actions: [
          {
            label: t("chat.channelInboundOpen", "查看"),
            color: "white",
            handler: () => {
              void router.push({ name: "chat", query: { session: sessionId, agent: agentId } });
            },
          },
        ],
      });
    }
  }

  onMounted(() => {
    hubId = acquireGlobalWsConsumer({
      channels: ["chat"],
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
