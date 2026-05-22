import { onMounted, onUnmounted, type Ref } from "vue";
import { useQuasar } from "quasar";
import { useI18n } from "vue-i18n";
import {
  acquireGlobalWsConsumer,
  releaseGlobalWsConsumer,
} from "../globalWsHub";
import type { Envelope } from "../envelope";
import type { UseEnvelopeStreamReturn } from "../useEnvelopeStream";
import { useAppStore } from "../../../stores/app";
import { parseChannelSessionMeta } from "../channelSessionMeta";
import type { Session } from "../../session/api";

export type ChatInboundSyncDeps = {
  selectedEntityKind: Ref<"agent" | "team">;
  selectedAgentId: Ref<string | undefined>;
  selectedTeamId: Ref<string | undefined>;
  selectedSessionId: Ref<string | undefined>;
  ensureChatStream: (sessionId: string) => UseEnvelopeStreamReturn;
  ensureTeamStream: (sessionId: string) => UseEnvelopeStreamReturn;
  patchAgentMessages: (sessionId: string, streamId: string, env: Envelope, isDone: boolean) => void;
  patchTeamMessages: (sessionId: string, streamId: string, env: Envelope, isDone: boolean) => void;
  loadTeamSessions?: (teamId: string) => Promise<void>;
};

/**
 * Subscribes to global WS (`session_id=*`) so Channel/Cron inbound turns update the Chat UI
 * when the matching agent session is open or newly created.
 */
export function useChatInboundSync(deps: ChatInboundSyncDeps) {
  const store = useAppStore();
  const $q = useQuasar();
  const { t } = useI18n();
  let hubId: string | null = null;

  function agentIdFromEnvelope(env: Envelope): string {
    const md = env.metadata as Record<string, unknown> | undefined;
    const fromMeta = typeof md?.agent_id === "string" ? md.agent_id.trim() : "";
    if (fromMeta) return fromMeta;
    const sid = (env.session_id ?? "").trim();
    if (!sid) return "";
    const sess =
      store.sessions.find((s) => s.id === sid) ??
      (store.selectedSession?.id === sid ? store.selectedSession : null);
    return sess?.agent_id?.trim() ?? "";
  }

  function teamIdFromEnvelope(env: Envelope): string {
    if (env.team_id?.trim()) return env.team_id.trim();
    const sid = (env.session_id ?? "").trim();
    if (!sid) return "";
    const sess = store.sessions.find((s) => s.id === sid);
    return sess?.team_id?.trim() ?? "";
  }

  function matchesSelectedEntity(env: Envelope): boolean {
    if (deps.selectedEntityKind.value === "team") {
      const tid = deps.selectedTeamId.value?.trim();
      return !!tid && teamIdFromEnvelope(env) === tid;
    }
    const aid = deps.selectedAgentId.value?.trim();
    return !!aid && agentIdFromEnvelope(env) === aid;
  }

  function findSessionById(sessionId: string): Session | undefined {
    return store.sessions.find((s) => s.id === sessionId);
  }

  async function refreshSessionsAndMaybeNotify(sessionId: string) {
    const prevIds = new Set(store.sessions.map((s) => s.id));
    if (deps.selectedEntityKind.value === "agent") {
      await store.loadSessions();
    } else if (deps.selectedEntityKind.value === "team") {
      const tid = deps.selectedTeamId.value?.trim();
      if (tid && deps.loadTeamSessions) {
        await deps.loadTeamSessions(tid);
      }
    }
    const isNew = !prevIds.has(sessionId);
    const isCurrent = deps.selectedSessionId.value === sessionId;
    if (isNew && !isCurrent) {
      const sess = findSessionById(sessionId);
      const chMeta = sess ? parseChannelSessionMeta(sess.metadata_json) : null;
      if (chMeta) {
        $q.notify({
          type: "info",
          message: t("chat.channelInboundNotify", "飞书有新回复"),
          caption: sess?.title ?? chMeta.channel_key,
          timeout: 8000,
          actions: [
            {
              label: t("chat.channelInboundOpen", "查看"),
              color: "white",
              handler: () => {
                void store.loadSessions().then(() => {
                  const hit = store.sessions.find((s) => s.id === sessionId);
                  if (hit) {
                    store.selectedSession = hit;
                    void store.loadMessages();
                    deps.ensureChatStream(sessionId);
                  }
                });
              },
            },
          ],
        });
      }
    }
  }

  async function handleInboundEnvelope(env: Envelope) {
    const sessionId = (env.session_id ?? "").trim();
    if (!sessionId || !matchesSelectedEntity(env)) return;

    const isCurrent = deps.selectedSessionId.value === sessionId;
    const streamId =
      deps.selectedEntityKind.value === "team"
        ? `ws-team-stream-${sessionId}`
        : `ws-stream-${sessionId}`;

    if (env.type === "text_delta" && isCurrent) {
      if (deps.selectedEntityKind.value === "team") {
        deps.ensureTeamStream(sessionId);
        deps.patchTeamMessages(sessionId, streamId, env, false);
      } else {
        deps.ensureChatStream(sessionId);
        deps.patchAgentMessages(sessionId, streamId, env, false);
      }
      return;
    }

    if (env.type === "text_done" && isCurrent) {
      if (deps.selectedEntityKind.value === "team") {
        deps.patchTeamMessages(sessionId, streamId, env, true);
      } else {
        deps.patchAgentMessages(sessionId, streamId, env, true);
      }
      return;
    }

    if (env.type === "runner_completion") {
      await refreshSessionsAndMaybeNotify(sessionId);
      if (isCurrent) {
        if (deps.selectedEntityKind.value === "team") {
          deps.ensureTeamStream(sessionId);
        } else {
          deps.ensureChatStream(sessionId);
        }
        try {
          await store.loadMessages();
        } catch {
          /* ignore */
        }
      }
    }
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
    if (hubId) {
      releaseGlobalWsConsumer(hubId);
      hubId = null;
    }
  });
}
