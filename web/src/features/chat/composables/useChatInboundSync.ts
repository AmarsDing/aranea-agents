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
import { runStatusFromEnvelope } from "../envelopeRunStatus";
import { upsertToolMessage } from "../envelopeToolCall";
import { createMessageBatchWriter } from "../messageStoreBatch";
import type { Session } from "../../session/types";

export type ChatInboundSyncDeps = {
  selectedEntityKind: Ref<"agent" | "team">;
  selectedAgentId: Ref<string | undefined>;
  selectedTeamId: Ref<string | undefined>;
  selectedSessionId: Ref<string | undefined>;
  wsReplaying?: Ref<boolean>;
  onTurnComplete?: (sessionId: string) => void;
  ensureChatStream: (sessionId: string) => UseEnvelopeStreamReturn;
  ensureTeamStream: (sessionId: string) => UseEnvelopeStreamReturn;
  patchAgentMessages: (sessionId: string, streamId: string, env: Envelope, isDone: boolean) => void;
  patchTeamMessages: (sessionId: string, streamId: string, env: Envelope, isDone: boolean) => void;
  loadTeamSessions?: (teamId: string) => Promise<void>;
};

const HYDRATE_DEBOUNCE_MS = 200;

function envelopeSessionRevision(env: Envelope): number {
  if (typeof env.session_revision === "number" && env.session_revision > 0) {
    return env.session_revision;
  }
  const md = env.metadata as Record<string, unknown> | undefined;
  const fromMeta = md?.session_revision;
  if (typeof fromMeta === "number" && fromMeta > 0) return fromMeta;
  return 0;
}

function isTurnCompleteEnvelope(env: Envelope): boolean {
  if (env.type === "runner_completion") return true;
  if (env.type !== "run_status") return false;
  const status = runStatusFromEnvelope(env)?.status;
  return status === "completed" || status === "failed" || status === "cancelled";
}

/**
 * Subscribes to global WS (`session_id=*`) so Channel/Cron inbound turns update the Chat UI
 * when the matching agent session is open or newly created.
 */
export function useChatInboundSync(deps: ChatInboundSyncDeps) {
  const store = useAppStore();
  const $q = useQuasar();
  const { t } = useI18n();
  let hubId: string | null = null;
  let hydrateTimer: ReturnType<typeof setTimeout> | null = null;
  let pendingHydrateSessionId = "";

  const inboundMessageWriter = createMessageBatchWriter(
    () => store.messages,
    (rows) => {
      store.messages = rows;
    }
  );

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

  async function refreshSessionsAndMaybeNotify(sessionId: string, source = "") {
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
    if (!isCurrent) {
      const sess = findSessionById(sessionId);
      const chMeta = sess ? parseChannelSessionMeta(sess.metadata_json) : null;
      if (chMeta && (isNew || source === "channel")) {
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
                    void store.loadMessages({ replace: true });
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

  function scheduleHydrate(sessionId: string) {
    if (deps.wsReplaying?.value) {
      return;
    }
    pendingHydrateSessionId = sessionId;
    if (hydrateTimer) clearTimeout(hydrateTimer);
    hydrateTimer = setTimeout(() => {
      hydrateTimer = null;
      void hydrateCurrentSession(pendingHydrateSessionId);
    }, HYDRATE_DEBOUNCE_MS);
  }

  async function hydrateCurrentSession(sessionId: string) {
    if (deps.selectedEntityKind.value === "team") {
      deps.ensureTeamStream(sessionId);
      return;
    }
    deps.ensureChatStream(sessionId);
    inboundMessageWriter.flushSync();
    const localRev = store.sessionRevisionBySession[sessionId] ?? 0;
    try {
      if (localRev > 0) {
        await store.loadMessages({ afterRevision: localRev });
      } else {
        await store.loadMessages();
      }
    } catch {
      /* ignore */
    }
  }

  async function handleInboundEnvelope(env: Envelope) {
    const sessionId = (env.session_id ?? "").trim();
    if (!sessionId) return;

    const envRev = envelopeSessionRevision(env);
    if (envRev > 0) {
      const prev = store.sessionRevisionBySession[sessionId] ?? 0;
      if (envRev > prev) {
        store.sessionRevisionBySession[sessionId] = envRev;
      }
    }

    const isCurrent = deps.selectedSessionId.value === sessionId;
    const entityMatch = matchesSelectedEntity(env);

    const inboundSource =
      (env.source ?? "").trim() ||
      (typeof (env.metadata as Record<string, unknown> | undefined)?.source === "string"
        ? String((env.metadata as Record<string, unknown>).source).trim()
        : "");

    if (isCurrent && env.type === "run_status") {
      const rs = runStatusFromEnvelope(env);
      if (rs?.status === "running") {
        if (deps.selectedEntityKind.value === "team") {
          deps.ensureTeamStream(sessionId);
        } else {
          deps.ensureChatStream(sessionId);
          if (inboundSource === "channel") {
            void store.loadMessages();
          }
        }
      }
    }

    if (!isCurrent && !entityMatch) return;

    if (isCurrent && deps.selectedEntityKind.value === "agent") {
      deps.ensureChatStream(sessionId);
      if (env.type === "text_delta" && (env.content?.text || env.content?.reasoning)) {
        deps.patchAgentMessages(sessionId, `ws-stream-${sessionId}`, env, false);
        return;
      }
      if (env.type === "text_done") {
        deps.patchAgentMessages(sessionId, `ws-stream-${sessionId}`, env, true);
        return;
      }
      if (env.type === "tool_call" && env.tool_call) {
        inboundMessageWriter.update((cur) => upsertToolMessage(cur, sessionId, env, "before"));
        return;
      }
      if (env.type === "tool_result" && env.tool_call) {
        inboundMessageWriter.update((cur) => upsertToolMessage(cur, sessionId, env, "after"));
        return;
      }
    }

    if (!isTurnCompleteEnvelope(env)) return;

    if (entityMatch || isCurrent) {
      await refreshSessionsAndMaybeNotify(sessionId, inboundSource);
    }

    if (isCurrent) {
      scheduleHydrate(sessionId);
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
