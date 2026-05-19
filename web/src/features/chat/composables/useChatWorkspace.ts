import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import {
  listChatOptions,
  stopGeneration,
  getPendingMessages,
  cancelPendingMessage,
  updatePendingMessage,
  getRunStatus,
  awaitUserReply,
  type RunStatusValue,
} from "../api";
import type { PendingMessage } from "../api";
import {
  archiveSession,
  createSession,
  getSession,
  listSessionChatMessages as listMessages,
  listTeamSessions,
  restoreSession,
  updateSessionTitle
} from "../../session/api";
import { deleteTeam, listTeams, updateTeam } from "../../teams/api";
import {
  listPlatformResources,
  listPlatformResourceTree,
  type PlatformResource,
  type PlatformResourceTreeNode
} from "../../platform/api";
import type {
  Agent,
  ChatAttachment,
  ChatEntityKind,
  DeleteKind,
  Message,
  Session,
  SessionView,
  TeamRow
} from "../../../components/chat/types";
import type { ChatOption } from "../types";
import {
  CHAT_MODE_OPTIONS,
  loadDialogModeFromStorage,
  loadModelFromStorage,
  saveDialogModeToStorage,
  saveModelToStorage
} from "../../../config/chatOptions";
import { useAppStore } from "../../../stores/app";
import { useAuthStore } from "../../../stores/auth";
import { useArtifactStore } from "../../../stores/artifact";
import type { ArtifactMeta } from "../../artifact/types";
import { useChatStream, useTeamStream } from "../useEnvelopeStream";
import type { Envelope } from "../envelope";
import { upsertToolMessage } from "../envelopeToolCall";
import { runStatusFromEnvelope } from "../envelopeRunStatus";
import { patchStreamingMessage } from "../streamContentPatch";
import type { TeamDefinition } from "../../teams/types";
import { getServerHeartbeatState } from "../../heartbeat/useServerHeartbeat";

function mockMessage(id: string, sessionID: string, role: string, content: string): Message {
  return {
    id,
    session_id: sessionID,
    parent_message_id: "",
    turn_index: 1,
    role,
    content_markdown: content,
    model_name: "mock",
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: "ok",
    attachments_count: 0,
    options_json: "",
    error_message: "",
    created_at: new Date().toISOString()
  };
}

export function useChatWorkspace() {
  const { t } = useI18n();
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const store = useAppStore();

  const LS_AG_ORDER = "chat:order:agents";
  const LS_TM_ORDER = "chat:order:teams";

  const isDark = computed(() => $q.dark.isActive);
  const leftOpen = ref(true);
  const rightOpen = ref(true);
  const search = ref("");
  const selectedEntityKind = ref<ChatEntityKind>("agent");
  const selectedTeamId = ref<string | null>(null);
  const teamSelectedSessionId = ref<string | null>(null);
  const defaultAgentId = ref<string | null>(null);
  const defaultTeamId = ref("team-default-1");
  const fileRef = ref<HTMLInputElement | null>(null);

  const displayAgents = ref<Agent[]>([]);
  const categoryTree = ref<PlatformResourceTreeNode[]>([]);
  const displayTeams = ref<TeamRow[]>([]);
  const teamSessions = ref<Record<string, Array<Session & { at: string }>>>({});
  const teamMessages = ref<Record<string, Message[]>>({});

  const inputText = ref("");
  const dialogMode = ref(loadDialogModeFromStorage("default"));
  const modelProvider = ref(loadModelFromStorage(""));
  const sending = ref(false);
  let sendingTimeout: ReturnType<typeof setTimeout> | null = null;
  const SENDING_TIMEOUT_MS = 120_000;

  function markSending() {
    sending.value = true;
    clearSendingTimeout();
    sendingTimeout = setTimeout(() => {
      if (sending.value) {
        sending.value = false;
        $q.notify({ type: "warning", message: "响应超时，请重试" });
      }
    }, SENDING_TIMEOUT_MS);
  }

  function markSendingDone() {
    sending.value = false;
    clearSendingTimeout();
  }

  function clearSendingTimeout() {
    if (sendingTimeout != null) {
      clearTimeout(sendingTimeout);
      sendingTimeout = null;
    }
  }
  const modeOpts = ref<Array<{ label: string; value: string }>>(
    CHAT_MODE_OPTIONS.map((o) => ({ label: o.label, value: o.value }))
  );
  const providerModels = ref<PlatformResource[]>([]);
  const provOpts = ref<Array<{ label: string; value: string; caption?: string }>>([]);
  const attachments = ref<ChatAttachment[]>([]);
  const pendingMessages = ref<PendingMessage[]>([]);
  const runStatus = ref<RunStatusValue>("idle");
  const isAwaitingUser = ref(false);
  const awaitingRunId = ref("");
  const wsReplaying = ref(false);
  const artifactStore = useArtifactStore();
  const sessionArtifacts = ref<ArtifactMeta[]>([]);
  const sessionArtifactsLoading = ref(false);
  let pendingPollTimer: ReturnType<typeof setInterval> | null = null;

  const settingsOpen = ref(false);
  const settingsMode = ref<ChatEntityKind | null>(null);
  const settingsId = ref<string | null>(null);
  const editName = ref("");
  const editKey = ref("");
  const editProvider = ref("");
  const editModel = ref("");
  const settingsSaving = ref(false);

  const deleteOpen = ref(false);
  const deleteKind = ref<DeleteKind>("agent");
  const deleteTargetId = ref<string | null>(null);
  const deleteNameInput = ref("");
  const deleteBlockBusy = ref(false);
  const deleteBlockDefault = ref(false);
  const deleting = ref(false);
  const traceOpen = ref(false);
  const traceSessionId = ref<string | null>(null);
  const traceSessionTitle = ref("");

  const settingsTitle = computed(() => {
    if (settingsMode.value === "agent") return t("chat.settingsTitleAgent");
    if (settingsMode.value === "team") return t("chat.settingsTitleTeam");
    return t("chat.settings");
  });
  const selectedProviderModel = computed(() =>
    providerModels.value.find((row) => getProviderModelValue(row) === modelProvider.value)
  );

  const displaySessions = computed((): SessionView[] => {
    if (selectedEntityKind.value === "team" && selectedTeamId.value) {
      return (teamSessions.value[selectedTeamId.value] ?? []).map((session) => ({
        id: session.id,
        title: session.title || t("chat.untitledSession"),
        context_used_ratio: session.context_used_ratio,
        at: session.at,
        timeline_at: session.last_message_at || session.updated_at || session.created_at,
        agent_id: session.agent_id,
        status: session.status
      }));
    }

    if (selectedEntityKind.value === "agent" && store.selectedAgent) {
      return store.sessions.map((session) => ({
        id: session.id,
        title: session.title || t("chat.untitledSession"),
        context_used_ratio: session.context_used_ratio,
        at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at),
        timeline_at: session.last_message_at || session.updated_at || session.created_at,
        status: session.status
      }));
    }

    return [];
  });

  const selectedSessionForUi = computed((): SessionView | null => {
    if (selectedEntityKind.value === "team" && teamSelectedSessionId.value) {
      return displaySessions.value.find((session) => session.id === teamSelectedSessionId.value) ?? null;
    }

    if (!store.selectedSession) return null;

    return (
      displaySessions.value.find((session) => session.id === store.selectedSession!.id) ?? {
        id: store.selectedSession.id,
        title: store.selectedSession.title || t("chat.untitledSession"),
        context_used_ratio: store.selectedSession.context_used_ratio,
        at: formatSessionTime(
          store.selectedSession.last_message_at ||
            store.selectedSession.updated_at ||
            store.selectedSession.created_at
        ),
        timeline_at:
          store.selectedSession.last_message_at ||
          store.selectedSession.updated_at ||
          store.selectedSession.created_at
      }
    );
  });

  async function loadSessionArtifacts(sessionId: string) {
    if (!sessionId) {
      sessionArtifacts.value = [];
      return;
    }
    sessionArtifactsLoading.value = true;
    try {
      const res = await artifactStore.loadArtifacts({ session_id: sessionId, limit: 20 });
      sessionArtifacts.value = res.items;
    } finally {
      sessionArtifactsLoading.value = false;
    }
  }

  watch(
    () => selectedSessionForUi.value?.id ?? "",
    (sid) => void loadSessionArtifacts(sid),
    { immediate: true }
  );

  function openSessionArtifact(id: string) {
    void router.push({ path: "/artifacts", query: { id } });
  }

  const displayMessages = computed((): Message[] => {
    if (selectedEntityKind.value === "team" && teamSelectedSessionId.value) {
      return teamMessages.value[teamSelectedSessionId.value] ?? [];
    }
    return store.messages;
  });

  const expectedDeleteName = computed(() => {
    if (deleteKind.value === "agent" && deleteTargetId.value) {
      return (
        store.agents.find((agent) => agent.id === deleteTargetId.value)?.display_name ??
        displayAgents.value.find((agent) => agent.id === deleteTargetId.value)?.display_name ??
        ""
      );
    }
    if (deleteKind.value === "team") {
      return displayTeams.value.find((team) => team.id === deleteTargetId.value)?.display_name ?? "";
    }
    if (deleteKind.value === "session") {
      return displaySessions.value.find((session) => session.id === deleteTargetId.value)?.title ?? "";
    }
    return "";
  });

  const deleteNameError = computed(
    () => deleteNameInput.value && deleteNameInput.value !== expectedDeleteName.value
  );

  const canConfirmDelete = computed(() => {
    if (deleteBlockBusy.value || deleteBlockDefault.value) return false;
    if (deleteKind.value === "all" || deleteKind.value === "session") return true;
    return deleteNameInput.value === expectedDeleteName.value;
  });

  const deleteTitleText = computed(() => {
    if (deleteKind.value === "agent") return t("chat.deleteTitleAgent");
    if (deleteKind.value === "team") return t("chat.deleteTitleTeam");
    if (deleteKind.value === "session") return t("chat.deleteTitleSession");
    return t("chat.deleteAllTitle");
  });

  function isAgentWorking(agent: Agent) {
    return /work|run|busy|ing/i.test(agent.status || "");
  }

  function onEndAgent() {
    if (defaultAgentId.value) {
      const current = displayAgents.value;
      if (current[0] && current[0].id !== defaultAgentId.value) {
        const fixed = current.find((agent) => agent.id === defaultAgentId.value);
        if (fixed) displayAgents.value = [fixed, ...current.filter((agent) => agent.id !== fixed.id)];
      }
    }
    try {
      localStorage.setItem(LS_AG_ORDER, JSON.stringify(displayAgents.value.map((agent) => agent.id)));
    } catch {
      // localStorage can be unavailable in restricted contexts.
    }
  }

  function onEndTeam() {
    const current = displayTeams.value;
    if (current[0] && current[0].id !== defaultTeamId.value) {
      const fixed = current.find((team) => team.id === defaultTeamId.value);
      if (fixed) displayTeams.value = [fixed, ...current.filter((team) => team.id !== fixed.id)];
    }
    try {
      localStorage.setItem(LS_TM_ORDER, JSON.stringify(displayTeams.value.map((team) => team.id)));
    } catch {
      // localStorage can be unavailable in restricted contexts.
    }
  }

  function loadAgentOrder(agents: Agent[], defaultId: string | null): Agent[] {
    if (agents.length === 0) return [];
    const defaultResolved =
      defaultId && agents.some((agent) => agent.id === defaultId) ? defaultId : agents[0]!.id;
    const ordered = applyStoredOrder(agents, LS_AG_ORDER);
    const fixed = ordered.find((agent) => agent.id === defaultResolved) ?? ordered[0]!;
    return [fixed, ...ordered.filter((agent) => agent.id !== fixed.id)];
  }

  function loadTeamOrder(teams: TeamRow[]): TeamRow[] {
    const ordered = applyStoredOrder(teams, LS_TM_ORDER);
    const fixed = ordered.find((team) => team.id === defaultTeamId.value) ?? ordered[0];
    return fixed ? [fixed, ...ordered.filter((team) => team.id !== fixed.id)] : ordered;
  }

  function applyStoredOrder<T extends { id: string }>(items: T[], key: string): T[] {
    const byId = new Map(items.map((item) => [item.id, item] as const));
    const ordered: T[] = [];

    try {
      const ids = JSON.parse(localStorage.getItem(key) || "[]") as string[];
      for (const id of ids) {
        const item = byId.get(id);
        if (item) ordered.push(item);
      }
    } catch {
      // Ignore malformed stored order and rebuild below.
    }

    for (const item of items) {
      if (!ordered.some((candidate) => candidate.id === item.id)) ordered.push(item);
    }

    return ordered;
  }

  async function selectAgent(agent: Agent) {
    selectedEntityKind.value = "agent";
    selectedTeamId.value = null;
    teamSelectedSessionId.value = null;
    store.selectedAgent = agent;
    await store.loadSessions();
    await nextTick();
    store.selectedSession = store.sessions[0] ?? null;
    store.messages = [];
    if (store.selectedSession) await store.loadMessages();
  }

  async function selectTeam(team: TeamRow) {
    selectedEntityKind.value = "team";
    selectedTeamId.value = team.id;
    store.selectedSession = null;
    store.messages = [];
    await loadTeamSessions(team.id);
    teamSelectedSessionId.value = teamSessions.value[team.id]?.[0]?.id ?? null;
    if (teamSelectedSessionId.value) {
      teamMessages.value[teamSelectedSessionId.value] = await listMessages(teamSelectedSessionId.value);
    }
  }

  async function onSelectSession(sessionId: string) {
    if (selectedEntityKind.value === "team") {
      teamSelectedSessionId.value = sessionId;
      teamMessages.value[sessionId] = await listMessages(sessionId);
      return;
    }

    const session = store.sessions.find((item) => item.id === sessionId) ?? null;
    store.selectedSession = session;
    store.messages = [];
    if (session) await store.loadMessages();
  }

  async function onRenameSession(payload: { id: string; title: string }) {
    const title = payload.title.trim();
    if (!title) return;
    try {
      if (selectedEntityKind.value === "team" && selectedTeamId.value) {
        const updated = await updateSessionTitle(payload.id, title);
        teamSessions.value[selectedTeamId.value] = (teamSessions.value[selectedTeamId.value] ?? []).map((session) =>
          session.id === payload.id
            ? { ...updated, at: formatSessionTime(updated.last_message_at || updated.updated_at || updated.created_at) }
            : session
        );
        return;
      }

      await store.renameSessionLocal(payload.id, title);
    } catch (err) {
      $q.notify({ type: "negative", message: "重命名失败，请重试" });
    }
  }

  function openSessionTrace(sessionId: string) {
    const session = displaySessions.value.find((item) => item.id === sessionId);
    traceSessionId.value = sessionId;
    traceSessionTitle.value = session?.title ?? t("chat.untitledSession");
    traceOpen.value = true;
  }

  async function onRestoreSession(sessionId: string) {
    try {
      await restoreSession(sessionId);
      if (selectedEntityKind.value === "team" && selectedTeamId.value) {
        const sessions = teamSessions.value[selectedTeamId.value] ?? [];
        teamSessions.value[selectedTeamId.value] = sessions.map((s) =>
          s.id === sessionId ? { ...s, status: "active" } : s
        ) as typeof sessions;
      }
    } catch (err) {
      console.error("Restore session failed", err);
    }
  }

  async function onArchiveSession(sessionId: string) {
    try {
      await archiveSession(sessionId);
      if (selectedEntityKind.value === "team" && selectedTeamId.value) {
        const sessions = teamSessions.value[selectedTeamId.value] ?? [];
        teamSessions.value[selectedTeamId.value] = sessions.map((s) =>
          s.id === sessionId ? { ...s, status: "archived" } : s
        ) as typeof sessions;
      }
    } catch (err) {
      console.error("Archive session failed", err);
    }
  }

  function onSessionDetail(sessionId: string) {
    router.push({ name: "session-detail", params: { sessionId } });
  }

  async function onNewSession(title?: string) {
    if (selectedEntityKind.value === "agent" && store.selectedAgent) {
      const selectedModel = selectedProviderModel.value;
      await store.addSession(title || t("chat.untitledSession"), {
        dialog_mode: dialogMode.value,
        default_provider: selectedModel?.provider || store.selectedAgent.provider,
        default_model: selectedModel?.model || store.selectedAgent.model
      });
      if (store.selectedSession) await store.loadMessages();
      return;
    }

    if (selectedEntityKind.value === "team" && selectedTeamId.value) {
      const selectedModel = selectedProviderModel.value;
      const created = await createSession({
        owner_type: "team",
        team_id: selectedTeamId.value,
        title: title || t("chat.untitledSession"),
        dialog_mode: dialogMode.value,
        default_provider: selectedModel?.provider || "",
        default_model: selectedModel?.model || ""
      });
      teamSessions.value[selectedTeamId.value] = [
        { ...created, at: formatSessionTime(created.last_message_at || created.updated_at || created.created_at) },
        ...(teamSessions.value[selectedTeamId.value] ?? [])
      ];
      teamSelectedSessionId.value = created.id;
      teamMessages.value[created.id] = [];
    }
  }

  let chatStream: ReturnType<typeof useChatStream> | null = null;
  let chatStreamSessionId: string | null = null;
  let teamStream: ReturnType<typeof useTeamStream> | null = null;
  let teamStreamSessionId: string | null = null;

  function ensureChatStream(sessionId: string) {
    if (chatStream && chatStream.transport.value && chatStreamSessionId === sessionId) {
      return chatStream;
    }
    chatStream?.disconnect();
    chatStream = useChatStream(sessionId, {
      onServerShutdown: () => {
        $q.notify({
          type: "warning",
          message: t("chat.serverShutdown", "服务器已关闭，请重新登录"),
          timeout: 0,
          actions: [{ label: t("chat.relogin", "重新登录"), color: "white", handler: () => {} }],
        });
        const auth = useAuthStore();
        auth.user = null;
        auth.sessionChecked = true;
        router.push({ name: "login" });
      },
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
    });
    chatStream.onType("text_delta", (env: Envelope) => {
      const sid = store.selectedSession?.id;
      if (!sid || (!env.content?.text && !env.content?.reasoning)) return;
      patchAgentMessages(sid, `ws-stream-${sid}`, env, false);
    });
    chatStream.onType("text_done", (env: Envelope) => {
      const sid = store.selectedSession?.id;
      if (!sid) return;
      patchAgentMessages(sid, `ws-stream-${sid}`, env, true);
    });
    chatStream.onType("tool_call", (env: Envelope) => {
      const sid = store.selectedSession?.id;
      if (!sid || !env.tool_call) return;
      store.messages = upsertToolMessage(store.messages, sid, env, "before");
    });
    chatStream.onType("tool_result", (env: Envelope) => {
      const sid = store.selectedSession?.id;
      if (!sid || !env.tool_call) return;
      store.messages = upsertToolMessage(store.messages, sid, env, "after");
    });
    chatStream.onType("run_status", (env: Envelope) => {
      applyRunStatusFromEnvelope(env);
    });
    chatStream.onType("runner_completion", async () => {
      markSendingDone();
      applyRunStatusFromEnvelope({
        id: "",
        type: "run_status",
        author: "",
        session_id: sessionId,
        timestamp: new Date().toISOString(),
        version: 0,
        metadata: { status: "completed", run_id: awaitingRunId.value },
      });
      const sid = store.selectedSession?.id;
      if (sid) {
        try {
          await store.loadMessages();
          await store.loadSessions();
        } catch { /* ignore */ }
      }
    });
    chatStream.onType("error", (env: Envelope) => {
      const msg = env.error?.message ?? "stream failed";
      $q.notify({ type: "negative", message: msg });
      markSendingDone();
    });
    chatStream.onType("intent_pass", (env: Envelope) => {
      store.lastIntentPass = env.metadata as any;
    });
    chatStream.connect();
    chatStreamSessionId = sessionId;
    return chatStream;
  }

  function ensureTeamStream(sessionId: string) {
    if (teamStream && teamStream.transport.value && teamStreamSessionId === sessionId) {
      return teamStream;
    }
    teamStream?.disconnect();
    teamStream = useTeamStream(sessionId, {
      onReplayState: (replaying) => {
        wsReplaying.value = replaying;
      },
    });
    teamStream.onType("text_delta", (env: Envelope) => {
      const sid = teamSelectedSessionId.value;
      if (!sid || (!env.content?.text && !env.content?.reasoning)) return;
      patchTeamMessages(sid, `ws-team-stream-${sid}`, env, false);
    });
    teamStream.onType("text_done", (env: Envelope) => {
      const sid = teamSelectedSessionId.value;
      if (!sid) return;
      patchTeamMessages(sid, `ws-team-stream-${sid}`, env, true);
    });
    teamStream.onType("tool_call", (env: Envelope) => {
      const sid = teamSelectedSessionId.value;
      if (!sid || !env.tool_call) return;
      teamMessages.value[sid] = upsertToolMessage(teamMessages.value[sid] ?? [], sid, env, "before");
    });
    teamStream.onType("tool_result", (env: Envelope) => {
      const sid = teamSelectedSessionId.value;
      if (!sid || !env.tool_call) return;
      teamMessages.value[sid] = upsertToolMessage(teamMessages.value[sid] ?? [], sid, env, "after");
    });
    teamStream.onType("run_status", (env: Envelope) => {
      applyRunStatusFromEnvelope(env);
    });
    teamStream.onType("member_message_start", (env: Envelope) => {
      if (env.author && teamSelectedSessionId.value) {
        const sid = teamSelectedSessionId.value;
        const msgId = `member-${env.author}`;
        const cur = teamMessages.value[sid] ?? [];
        if (!cur.some((m) => m.id === msgId)) {
          const meta = resolveTeamMemberMeta(env.author);
          teamMessages.value[sid] = [
            ...cur,
            {
              ...mockMessage(msgId, sid, "assistant", ""),
              status: "streaming",
              model_name: `team/${meta.role || "member"}`,
              options_json: JSON.stringify({ team_member: meta }),
            },
          ];
        }
      }
    });
    teamStream.onType("member_delta", (env: Envelope) => {
      if (env.author && teamSelectedSessionId.value) {
        const sid = teamSelectedSessionId.value;
        const msgId = `member-${env.author}`;
        teamMessages.value[sid] = patchStreamingMessage(teamMessages.value[sid] ?? [], msgId, {
          text: env.content?.text,
          reasoning: env.content?.reasoning,
        });
      }
    });
    teamStream.onType("member_message_done", (env: Envelope) => {
      if (env.author && teamSelectedSessionId.value) {
        const sid = teamSelectedSessionId.value;
        const msgId = `member-${env.author}`;
        teamMessages.value[sid] = patchStreamingMessage(teamMessages.value[sid] ?? [], msgId, {
          replaceText: env.content?.text,
          replaceReasoning: env.content?.reasoning,
          status: "ok",
        });
      }
    });
    teamStream.onType("runner_completion", async () => {
      markSendingDone();
      applyRunStatusFromEnvelope({
        id: "",
        type: "run_status",
        author: "",
        session_id: sessionId,
        timestamp: new Date().toISOString(),
        version: 0,
        metadata: { status: "completed", run_id: awaitingRunId.value },
      });
      if (teamSelectedSessionId.value) {
        try {
          teamMessages.value[teamSelectedSessionId.value] = await listMessages(teamSelectedSessionId.value);
        } catch { /* keep assembled rows */ }
        if (selectedTeamId.value) await loadTeamSessions(selectedTeamId.value);
      }
    });
    teamStream.onType("error", (env: Envelope) => {
      const msg = env.error?.message ?? "stream failed";
      $q.notify({ type: "negative", message: msg });
      markSendingDone();
    });
    teamStream.onType("intent_pass", (env: Envelope) => {
      store.lastIntentPass = env.metadata as any;
    });
    teamStream.connect();
    teamStreamSessionId = sessionId;
    return teamStream;
  }

  async function onSend() {
    const content = inputText.value.trim();
    if (!content || sending.value) return;
    if (isAwaitingUser.value) {
      await submitAwaitingReply();
      return;
    }

    if (selectedEntityKind.value === "agent") {
      markSending();
      try {
        if (!store.selectedSession) await onNewSession(makeSessionTitle(content));
        if (!store.selectedSession) {
          $q.notify({ type: "negative", message: "未创建会话或会话无效，请重试" });
          markSendingDone();
          return;
        }
        const sessionId = store.selectedSession.id;
        const selectedModel = selectedProviderModel.value;
        inputText.value = "";

        const pendingUserId = `pending-user-${Date.now()}`;
        store.messages = [
          ...store.messages,
          mockMessage(pendingUserId, sessionId, "user", content)
        ];

        const stream = ensureChatStream(sessionId);
        const transport = stream.transport.value;
        if (!transport || !transport.connected) {
          const heartbeat = getServerHeartbeatState();
          if (!heartbeat.isAlive.value) {
            $q.notify({ type: "negative", message: "后端服务不可用，请重新登录", timeout: 0 });
            const auth = useAuthStore();
            auth.user = null;
            auth.sessionChecked = true;
            router.push({ name: "login" });
          } else {
            $q.notify({ type: "negative", message: "WebSocket 未连接，正在重连..." });
            stream.connect();
          }
          store.messages = store.messages.filter((m) => !String(m.id).startsWith("pending-user-"));
          markSendingDone();
          return;
        }
        transport.send({
          direction: "client_to_server",
          channel: "chat",
          type: "user_message",
          payload: {
            session_id: sessionId,
            agent_key: store.selectedAgent?.agent_key,
            content,
            options: {
              dialog_mode: dialogMode.value,
              provider: selectedModel?.provider || store.selectedSession.provider || store.selectedAgent?.provider || "",
              model: selectedModel?.model || store.selectedSession.model || store.selectedAgent?.model || "",
              attachments: attachments.value.map((item) => ({ id: item.id }))
            }
          }
        });
      } catch (error) {
        if (!(error instanceof DOMException && error.name === "AbortError")) {
          $q.notify({
            type: "negative",
            message: error instanceof Error ? error.message : "发送失败，请稍后重试"
          });
          store.messages = store.messages.filter((m) => !String(m.id).startsWith("pending-user-"));
          if (store.selectedSession) {
            try {
              await store.loadMessages();
              await store.loadSessions();
            } catch { /* ignore */ }
          }
          markSendingDone();
        }
      } finally {
        attachments.value = [];
      }
      return;
    }

    if (selectedEntityKind.value === "team" && selectedTeamId.value) {
      markSending();
      let sessionIdForCatch = "";
      try {
        if (!teamSelectedSessionId.value) await onNewSession(makeSessionTitle(content));
        const sessionId = teamSelectedSessionId.value;
        if (!sessionId) {
          $q.notify({ type: "negative", message: "未创建会话或会话无效，请重试" });
          markSendingDone();
          return;
        }
        sessionIdForCatch = sessionId;
        const session = teamSessions.value[selectedTeamId.value]?.find((item) => item.id === sessionId);
        const selectedModel = selectedProviderModel.value;
        inputText.value = "";

        const pendingUserId = `pending-user-${Date.now()}`;
        teamMessages.value[sessionId] = [...(teamMessages.value[sessionId] ?? []), mockMessage(pendingUserId, sessionId, "user", content)];

        const stream = ensureTeamStream(sessionId);
        const transport = stream.transport.value;
        if (!transport || !transport.connected) {
          const heartbeat = getServerHeartbeatState();
          if (!heartbeat.isAlive.value) {
            $q.notify({ type: "negative", message: "后端服务不可用，请重新登录", timeout: 0 });
            const auth = useAuthStore();
            auth.user = null;
            auth.sessionChecked = true;
            router.push({ name: "login" });
          } else {
            $q.notify({ type: "negative", message: "WebSocket 未连接，正在重连..." });
            stream.connect();
          }
          const cur = teamMessages.value[sessionId] ?? [];
          teamMessages.value[sessionId] = cur.filter((item) => !String(item.id).startsWith("pending-user-"));
          markSendingDone();
          return;
        }
        transport.send({
          direction: "client_to_server",
          channel: "chat",
          type: "user_message",
          payload: {
            session_id: sessionId,
            team_id: selectedTeamId.value,
            content,
            options: {
              dialog_mode: dialogMode.value,
              provider: selectedModel?.provider || session?.provider || "",
              model: selectedModel?.model || session?.model || "",
              attachments: attachments.value.map((item) => ({ id: item.id }))
            }
          }
        });
      } catch (error) {
        if (!(error instanceof DOMException && error.name === "AbortError")) {
          $q.notify({ type: "negative", message: error instanceof Error ? error.message : "Team 发送失败" });
        }
        if (sessionIdForCatch) {
          const cur = teamMessages.value[sessionIdForCatch] ?? [];
          teamMessages.value[sessionIdForCatch] = cur.filter((item) => !String(item.id).startsWith("pending-user-"));
        }
      } finally {
        markSendingDone();
        attachments.value = [];
      }
    }
  }

  function onModeChange(value: string) {
    dialogMode.value = value;
    saveDialogModeToStorage(value);
  }

  function onProviderChange(value: string) {
    modelProvider.value = value;
    saveModelToStorage(value);
  }

  function stopStreaming() {
    chatStream?.cancel();
    teamStream?.cancel();
    const sid = selectedSessionForUi.value?.id;
    if (sid) {
      stopGeneration(sid);
    }
    markSendingDone();
  }

  async function onCancelPending(pendingId: string) {
    const sid = selectedSessionForUi.value?.id;
    if (!sid || !pendingId) return;
    const ok = await cancelPendingMessage(sid, pendingId);
    if (ok) {
      pendingMessages.value = pendingMessages.value.filter((pm) => pm.id !== pendingId);
    }
  }

  async function onUpdatePending(pendingId: string, content: string) {
    const sid = selectedSessionForUi.value?.id;
    if (!sid || !pendingId || !content.trim()) return;
    const ok = await updatePendingMessage(sid, pendingId, content.trim());
    if (ok) {
      pendingMessages.value = pendingMessages.value.map((pm) =>
        pm.id === pendingId ? { ...pm, content: content.trim() } : pm
      );
    }
  }

  async function refreshPendingMessages() {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) {
      pendingMessages.value = [];
      return;
    }
    pendingMessages.value = await getPendingMessages(sid);
  }

  function startPendingPoll() {
    stopPendingPoll();
    refreshPendingMessages();
    pendingPollTimer = setInterval(refreshPendingMessages, 3000);
  }

  function stopPendingPoll() {
    if (pendingPollTimer != null) {
      clearInterval(pendingPollTimer);
      pendingPollTimer = null;
    }
  }

  watch(sending, (val) => {
    if (val) {
      startPendingPoll();
    } else {
      setTimeout(() => {
        refreshPendingMessages();
        if (pendingMessages.value.length === 0) {
          stopPendingPoll();
        }
      }, 1000);
    }
  });

  onUnmounted(() => {
    stopPendingPoll();
    clearSendingTimeout();
    chatStream?.disconnect();
    teamStream?.disconnect();
  });

  function makeSessionTitle(content: string) {
    const plain = content
      .replace(/[#>*_`~\[\]()]/g, "")
      .replace(/\s+/g, " ")
      .trim();
    if (!plain) return t("chat.untitledSession");
    return plain.length > 22 ? `${plain.slice(0, 22)}…` : plain;
  }

  function formatSessionTime(iso: string) {
    if (!iso) return "—";
    try {
      return new Date(iso).toLocaleString();
    } catch {
      return iso;
    }
  }

  async function openSettings(kind: ChatEntityKind, id: string) {
    if (kind === "agent") {
      await router.push(`/agents/${id}/settings`);
      return;
    }
    if (kind === "team") {
      await router.push({ name: "team", query: { edit: id } });
      return;
    }

    settingsMode.value = kind;
    settingsId.value = id;

    const team = displayTeams.value.find((item) => item.id === id);
    if (team) editName.value = team.display_name;

    settingsOpen.value = true;
  }

  async function onSaveSettings() {
    settingsSaving.value = true;
    try {
      if (settingsMode.value === "agent" && settingsId.value) {
        const agent =
          store.agents.find((item) => item.id === settingsId.value) ??
          displayAgents.value.find((item) => item.id === settingsId.value);
        if (agent) {
          const updated = await store.patchAgent(agent.id, {
            ...agent,
            display_name: editName.value,
            provider: editProvider.value,
            model: editModel.value
          });
          if (updated) {
            displayAgents.value = displayAgents.value.map((item) =>
              item.id === updated.id ? { ...item, ...updated } : item
            );
          }
        }
      } else if (settingsMode.value === "team" && settingsId.value) {
        const team = displayTeams.value.find((item) => item.id === settingsId.value);
        if (team) {
          const updated = await updateTeam(team.id, {
            team_key: team.team_key,
            display_name: editName.value,
            status: team.status,
            definition_json: team.definition_json || "{}"
          });
          team.display_name = updated.display_name;
          team.definition_json = updated.definition_json;
        }
      }

      settingsOpen.value = false;
      $q.notify({ type: "positive", message: t("chat.save") });
    } finally {
      settingsSaving.value = false;
    }
  }

  function openDelete(kind: DeleteKind, id: string) {
    deleteKind.value = kind;
    deleteTargetId.value = id;
    deleteNameInput.value = "";
    deleteBlockBusy.value = false;
    deleteBlockDefault.value = false;

    if (kind === "agent" && id) {
      const agent = store.agents.find((item) => item.id === id) ?? displayAgents.value.find((item) => item.id === id);
      deleteBlockBusy.value = agent ? isAgentWorking(agent) : false;
    }

    if (kind === "team" && id) {
      const team = displayTeams.value.find((item) => item.id === id);
      if (team?.isDefault) {
        deleteBlockDefault.value = true;
        $q.notify({ type: "warning", message: t("chat.deleteBlockedDefault") });
        return;
      }
      deleteBlockBusy.value = team?.isWorking ?? false;
    }

    deleteOpen.value = true;
  }

  async function onConfirmDelete() {
    const id = deleteTargetId.value;
    if (deleteBlockBusy.value || deleteBlockDefault.value) return;

    if (deleteKind.value === "agent" && id) {
      deleting.value = true;
      try {
        localStorage.removeItem(LS_AG_ORDER);
        await store.removeAgentFromList(id);
        displayAgents.value = displayAgents.value.filter((agent) => agent.id !== id);
        defaultAgentId.value = store.agents[0]?.id ?? null;
        if (selectedEntityKind.value === "agent") {
          if (store.selectedAgent) {
            await store.loadSessions();
            store.selectedSession = store.sessions[0] ?? null;
            store.messages = [];
            if (store.selectedSession) await store.loadMessages();
          } else if (displayTeams.value[0]) {
            selectTeam(displayTeams.value[0]);
          }
        }
      } finally {
        deleting.value = false;
      }
    } else if (deleteKind.value === "team" && id) {
      await deleteTeam(id);
      localStorage.removeItem(LS_TM_ORDER);
      displayTeams.value = displayTeams.value.filter((team) => team.id !== id);
      if (selectedTeamId.value === id) selectedTeamId.value = null;
    } else if (deleteKind.value === "session" && id) {
      await deleteSession(id);
    } else if (deleteKind.value === "all") {
      await clearSessions();
    }

    deleteOpen.value = false;
    $q.notify({ type: "info", message: t("chat.deleteSuccess") });
  }

  async function deleteSession(id: string) {
    if (selectedEntityKind.value === "team" && selectedTeamId.value) {
      teamSessions.value[selectedTeamId.value] = (teamSessions.value[selectedTeamId.value] ?? []).filter(
        (session) => session.id !== id
      );
      delete teamMessages.value[id];
      if (teamSelectedSessionId.value === id) {
        teamSelectedSessionId.value = teamSessions.value[selectedTeamId.value]?.[0]?.id ?? null;
      }
      return;
    }

    await store.removeSessionLocal(id);
    if (store.selectedSession) await store.loadMessages();
  }

  async function clearSessions() {
    if (selectedEntityKind.value === "agent") {
      await store.clearAllSessions();
      return;
    }

    if (selectedEntityKind.value === "team" && selectedTeamId.value) {
      teamSessions.value[selectedTeamId.value] = [];
      teamSelectedSessionId.value = null;
      for (const id of Object.keys(teamMessages.value)) delete teamMessages.value[id];
    }
  }

  function pickFile() {
    fileRef.value?.click();
  }

  function onFileChange(event: Event) {
    const input = event.target as HTMLInputElement;
    if (!input.files?.length) return;

    Array.from(input.files).forEach((file, i) => {
      const record: ChatAttachment = { id: `f-${Date.now()}-${i}`, name: file.name, progress: 0 };
      attachments.value.push(record);
      const timer = setInterval(() => {
        const stillExists = attachments.value.some((item) => item.id === record.id);
        if (!stillExists) {
          clearInterval(timer);
          return;
        }
        record.progress = Math.min(1, record.progress + 0.2);
        if (record.progress >= 1) {
          clearInterval(timer);
          delete record.timer;
        }
      }, 200);
      record.timer = timer;
    });

    input.value = "";
  }

  function removeAttachment(id: string) {
    const target = attachments.value.find((item) => item.id === id);
    if (target?.timer) clearInterval(target.timer);
    attachments.value = attachments.value.filter((item) => item.id !== id);
  }

  function onVoiceClick() {
    $q.notify({ type: "info", message: t("chat.voicePlaceholder") });
  }

  async function loadCategoryTree() {
    categoryTree.value = await listPlatformResourceTree("agent-categories");
  }

  async function loadTeams() {
    try {
      const rows = await listTeams();
      displayTeams.value = rows.map((team) => ({
        id: team.id,
        team_key: team.team_key,
        display_name: team.display_name,
        status: team.status,
        isDefault: team.is_default,
        isWorking: /work|run|busy|ing/i.test(team.status || ""),
        definition_json: team.definition_json
      }));
    } catch {
      displayTeams.value = [];
    }
  }

  async function loadTeamSessions(teamID: string) {
    const rows = await listTeamSessions(teamID);
    teamSessions.value[teamID] = rows.map((session) => ({
      ...session,
      at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at)
    }));
  }

  function chatModelOptionsToPlatform(rows: ChatOption[]): PlatformResource[] {
    return rows
      .filter((item) => item.enabled !== false)
      .map((item, index) => {
        let provider = "";
        let model = "";
        try {
          const meta = JSON.parse(item.metadata_json || "{}") as { provider?: string; model?: string };
          provider = meta.provider ?? "";
          model = meta.model ?? "";
        } catch {
          /* ignore */
        }
        return {
          id: item.key || `chat-opt-${index}`,
          resource: "llm-provider-models" as const,
          key: item.key,
          name: item.label || item.key,
          description: "",
          status: "active",
          enabled: item.enabled,
          sort_order: item.sort_order,
          parent_id: "",
          level: "",
          agent_id: "",
          provider,
          model,
          config_json: "{}",
          metadata_json: item.metadata_json,
          created_at: "",
          updated_at: "",
          deleted_at: "",
        };
      });
  }

  function applyRunStatusFromEnvelope(env: Envelope) {
    const rs = runStatusFromEnvelope(env);
    if (!rs) return;
    runStatus.value = rs.status;
    isAwaitingUser.value = rs.status === "awaiting_user";
    awaitingRunId.value = rs.runId;
  }

  async function refreshRunStatus() {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) {
      runStatus.value = "idle";
      isAwaitingUser.value = false;
      awaitingRunId.value = "";
      return;
    }
    try {
      const rs = await getRunStatus(sid);
      runStatus.value = rs.status;
      isAwaitingUser.value = rs.status === "awaiting_user";
      awaitingRunId.value = rs.runId;
    } catch {
      /* ignore transient errors */
    }
  }

  function resolveTeamMemberMeta(agentKey: string) {
    const team = displayTeams.value.find((row) => row.id === selectedTeamId.value);
    let def: TeamDefinition | null = null;
    try {
      def = team?.definition_json ? (JSON.parse(team.definition_json) as TeamDefinition) : null;
    } catch {
      def = null;
    }
    const member = def?.members?.find((m) => m.agent_key === agentKey || m.name === agentKey);
    return {
      agent_key: agentKey,
      name: member?.name || agentKey,
      role: member?.role || "",
    };
  }

  function patchAgentMessages(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    const exists = store.messages.some((m) => m.id === streamId);
    if (!exists) {
      store.messages = [
        ...store.messages,
        { ...mockMessage(streamId, sessionId, "assistant", ""), status: "streaming" },
      ];
    }
    store.messages = patchStreamingMessage(store.messages, streamId, {
      text: isDone ? undefined : env.content?.text,
      reasoning: isDone ? undefined : env.content?.reasoning,
      replaceText: isDone ? env.content?.text : undefined,
      replaceReasoning: isDone ? env.content?.reasoning : undefined,
      status: isDone ? "ok" : "streaming",
    });
  }

  function patchTeamMessages(sessionId: string, streamId: string, env: Envelope, isDone: boolean) {
    const cur = teamMessages.value[sessionId] ?? [];
    const exists = cur.some((m) => m.id === streamId);
    if (!exists) {
      teamMessages.value[sessionId] = [...cur, { ...mockMessage(streamId, sessionId, "assistant", ""), status: "streaming" }];
    }
    teamMessages.value[sessionId] = patchStreamingMessage(teamMessages.value[sessionId] ?? [], streamId, {
      text: isDone ? undefined : env.content?.text,
      reasoning: isDone ? undefined : env.content?.reasoning,
      replaceText: isDone ? env.content?.text : undefined,
      replaceReasoning: isDone ? env.content?.reasoning : undefined,
      status: isDone ? "ok" : "streaming",
    });
  }

  async function submitAwaitingReply() {
    const sid = selectedSessionForUi.value?.id;
    const reply = inputText.value.trim();
    if (!sid || !reply || !isAwaitingUser.value) return;
    try {
      const ok = await awaitUserReply(sid, reply, awaitingRunId.value || undefined);
      if (ok) {
        inputText.value = "";
        isAwaitingUser.value = false;
        runStatus.value = "running";
        $q.notify({ type: "positive", message: t("chat.awaitReplySent", "已提交回复，继续执行") });
        void refreshRunStatus();
      }
    } catch (err) {
      $q.notify({
        type: "negative",
        message: err instanceof Error ? err.message : t("chat.awaitReplyFailed", "提交回复失败"),
      });
    }
  }

  async function loadChatOptions() {
    let modeRows: ChatOption[] = [];
    try {
      modeRows = await listChatOptions("dialog_mode");
    } catch {
      /* keep CHAT_MODE_OPTIONS fallback */
    }
    let modelRows: PlatformResource[] = [];
    try {
      modelRows = await listPlatformResources("llm-provider-models");
    } catch {
      /* keep empty; model selector falls back to labels only */
    }
    if (!modelRows.length) {
      try {
        const catalogModels = await listChatOptions("model");
        if (catalogModels.length) {
          modelRows = chatModelOptionsToPlatform(catalogModels);
        }
      } catch {
        /* ignore */
      }
    }
    if (modeRows.length) {
      modeOpts.value = modeRows.map((item) => ({ label: item.label, value: item.key }));
    }
    providerModels.value = modelRows.filter((item) => item.enabled !== false);
    if (providerModels.value.length) {
      provOpts.value = providerModels.value.map((item) => ({
        label: item.name || item.model,
        value: getProviderModelValue(item),
        caption: `${item.provider} / ${item.model}`
      }));
      ensureSelectedModel();
    }
  }

  function ensureSelectedModel() {
    if (!providerModels.value.length) return;
    const stored = providerModels.value.find((item) => getProviderModelValue(item) === modelProvider.value);
    if (stored) return;

    const agentModel = store.selectedAgent
      ? providerModels.value.find(
          (item) => item.provider === store.selectedAgent?.provider && item.model === store.selectedAgent?.model
        )
      : null;
    const nextModel = agentModel ?? providerModels.value[0];
    modelProvider.value = getProviderModelValue(nextModel);
    saveModelToStorage(modelProvider.value);
  }

  function getProviderModelValue(row: PlatformResource) {
    return row.key || `${row.provider}:${row.model}`;
  }

  watch(
    () => selectedSessionForUi.value?.id,
    (sid) => {
      if (sid) {
        void refreshRunStatus();
      } else {
        runStatus.value = "idle";
        isAwaitingUser.value = false;
        awaitingRunId.value = "";
      }
    },
    { immediate: true }
  );

  onMounted(async () => {
    await Promise.all([loadChatOptions(), loadCategoryTree(), loadTeams()]);
    await store.loadAgents();
    defaultAgentId.value = store.agents[0]?.id ?? null;
    displayAgents.value = loadAgentOrder(store.agents, defaultAgentId.value);
    displayTeams.value = loadTeamOrder([...displayTeams.value]);

    const routeTeamID = typeof route.query.team === "string" ? route.query.team : "";
    const routeTeam = routeTeamID ? displayTeams.value.find((team) => team.id === routeTeamID) : undefined;
    if (routeTeam) {
      await selectTeam(routeTeam);
    } else if (store.selectedAgent) {
      await store.loadSessions();
      store.selectedSession = store.sessions[0] ?? null;
      store.messages = [];
      if (store.selectedSession) await store.loadMessages();
    } else if (store.agents[0]) {
      await selectAgent(store.agents[0]);
    } else if (displayTeams.value[0]) {
      await selectTeam(displayTeams.value[0]!);
    }
  });

  return {
    t,
    isDark,
    leftOpen,
    rightOpen,
    search,
    selectedEntityKind,
    selectedTeamId,
    teamSelectedSessionId,
    defaultAgentId,
    defaultTeamId,
    fileRef,
    displayAgents,
    categoryTree,
    displayTeams,
    teamSessions,
    teamMessages,
    inputText,
    dialogMode,
    modelProvider,
    sending,
    modeOpts,
    provOpts,
    attachments,
    pendingMessages,
    isAwaitingUser,
    runStatus,
    wsReplaying,
    submitAwaitingReply,
    sessionArtifacts,
    sessionArtifactsLoading,
    openSessionArtifact,
    settingsOpen,
    settingsMode,
    settingsId,
    editName,
    editKey,
    editProvider,
    editModel,
    settingsSaving,
    deleteOpen,
    deleteKind,
    deleteTargetId,
    deleteNameInput,
    deleteBlockBusy,
    deleteBlockDefault,
    deleting,
    traceOpen,
    traceSessionId,
    traceSessionTitle,
    settingsTitle,
    selectedProviderModel,
    displaySessions,
    selectedSessionForUi,
    displayMessages,
    expectedDeleteName,
    deleteNameError,
    canConfirmDelete,
    deleteTitleText,
    store,
    onEndAgent,
    onEndTeam,
    selectAgent,
    selectTeam,
    onSelectSession,
    onRenameSession,
    openSessionTrace,
    onRestoreSession,
    onArchiveSession,
    onSessionDetail,
    onNewSession,
    onSend,
    onModeChange,
    onProviderChange,
    stopStreaming,
    openSettings,
    onSaveSettings,
    openDelete,
    onConfirmDelete,
    pickFile,
    onFileChange,
    removeAttachment,
    onCancelPending,
    onUpdatePending,
    onVoiceClick
  };
}
