import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import {
  listChatOptions,
  getPendingMessages,
  cancelPendingMessage,
  updatePendingMessage,
  getRunStatus,
  enqueueUserMessage,
  type RunStatus,
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
  updateSessionTitle,
} from "../../session/api";
import { deleteTeam, listTeams, updateTeam } from "../../teams/api";
import {
  listPlatformResources,
  listPlatformResourceTree,
  type PlatformResource,
  type PlatformResourceTreeNode,
} from "../../platform/api";
import type {
  Agent,
  ChatAttachment,
  ChatEntityKind,
  DeleteKind,
  Message,
  Session,
  SessionView,
  TeamRow,
} from "../../../components/chat/types";
import type { ChatOption } from "../types";
import {
  CHAT_MODE_OPTIONS,
  loadDialogModeFromStorage,
  loadModelFromStorage,
  saveDialogModeToStorage,
  saveModelToStorage,
} from "../../../config/chatOptions";
import { useAppStore } from "../../../stores/app";
import { uploadArtifact } from "../../artifact/api";
import { useArtifactStore } from "../../../stores/artifact";
import type { ArtifactMeta } from "../../artifact/types";
import { cancelRunningToolMessages } from "../envelopeToolCall";
import { runStatusFromEnvelope, messageQueuedFromEnvelope } from "../envelopeRunStatus";
import type { Envelope } from "../envelope";
import { useChatStreamManager } from "./useChatStreamManager";
import { useChatSender } from "./useChatSender";
import { hydrateAgentSettings } from "../agentPlannerSettings";
import {
  formatUserActionMessage,
  type A2UIUserActionPayload,
} from "../a2uiUserAction";
import { buildReactToolLinkIndex } from "../reactToolLinkIndex";

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
  const attachments = ref<ChatAttachment[]>([]);
  const pendingMessages = ref<PendingMessage[]>([]);
  const runStatus = ref<RunStatusValue>("idle");
  const runMeta = ref<RunStatus | null>(null);
  const isAwaitingUser = ref(false);
  const awaitingRunId = ref("");
  const awaitKind = ref("");
  const awaitToolKey = ref("");
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
  const traceInitialTab = ref<"trace" | "events">("trace");
  const traceSessionOwnerKind = ref<"agent" | "team">("agent");

  const modeOpts = ref<Array<{ label: string; value: string }>>(
    CHAT_MODE_OPTIONS.map((o) => ({ label: o.label, value: o.value }))
  );
  const providerModels = ref<PlatformResource[]>([]);
  const provOpts = ref<Array<{ label: string; value: string; caption?: string }>>([]);
  const artifactStore = useArtifactStore();
  const sessionArtifacts = ref<ArtifactMeta[]>([]);
  const sessionArtifactsLoading = ref(false);

  const selectedProviderModel = computed(() =>
    providerModels.value.find((row) => getProviderModelValue(row) === modelProvider.value)
  );

  /** Agent-level planner_kind for Chat message presentation (react steps / a2ui preview). */
  const activePlannerKind = computed(() =>
    (store.selectedAgent?.settings?.planner_kind ?? "").trim().toLowerCase()
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
        status: session.status,
      }));
    }
    if (selectedEntityKind.value === "agent" && store.selectedAgent) {
      return store.sessions.map((session) => ({
        id: session.id,
        title: session.title || t("chat.untitledSession"),
        context_used_ratio: session.context_used_ratio,
        at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at),
        timeline_at: session.last_message_at || session.updated_at || session.created_at,
        status: session.status,
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
          store.selectedSession.created_at,
      }
    );
  });

  const displayMessages = computed((): Message[] => {
    if (selectedEntityKind.value === "team" && teamSelectedSessionId.value) {
      return teamMessages.value[teamSelectedSessionId.value] ?? [];
    }
    return store.messages;
  });

  const reactToolLinkIndex = computed(() => buildReactToolLinkIndex(displayMessages.value));

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

  const settingsTitle = computed(() => {
    if (settingsMode.value === "agent") return t("chat.settingsTitleAgent");
    if (settingsMode.value === "team") return t("chat.settingsTitleTeam");
    return t("chat.settings");
  });

  function applyAwaitMeta(rs: { awaitKind?: string; awaitToolKey?: string }) {
    awaitKind.value = rs.awaitKind ?? "";
    awaitToolKey.value = rs.awaitToolKey ?? "";
  }

  function clearAwaitMeta() {
    awaitKind.value = "";
    awaitToolKey.value = "";
  }

  function applyRunStatusFromEnvelope(env: Envelope) {
    if (messageQueuedFromEnvelope(env)) {
      void refreshPendingMessages();
      return;
    }
    const rs = runStatusFromEnvelope(env);
    if (!rs) return;
    runStatus.value = rs.status;
    isAwaitingUser.value = rs.status === "awaiting_user";
    awaitingRunId.value = rs.runId;
    if (rs.status === "awaiting_user") {
      applyAwaitMeta(rs);
    } else {
      clearAwaitMeta();
    }
    if (rs.status === "cancelled") {
      store.messages = cancelRunningToolMessages(store.messages);
      const sid = selectedSessionForUi.value?.id;
      if (sid && teamMessages.value[sid]) {
        teamMessages.value[sid] = cancelRunningToolMessages(teamMessages.value[sid]);
      }
    }
  }

  const streamManager = useChatStreamManager({
    store,
    teamMessages,
    teamSelectedSessionId,
    selectedTeamId,
    displayTeams,
    markSendingDone: () => sender.markSendingDone(),
    onRunStatus: applyRunStatusFromEnvelope,
  });

  const sender = useChatSender({
    store,
    selectedEntityKind,
    selectedTeamId,
    teamSelectedSessionId,
    teamMessages,
    inputText,
    dialogMode,
    attachments,
    isAwaitingUser,
    awaitingRunId,
    runStatus,
    selectedProviderModel,
    ensureChatStream: streamManager.ensureChatStream,
    ensureTeamStream: streamManager.ensureTeamStream,
    sendChatViaWs: streamManager.sendChatViaWs,
    onNewSession: (title?: string) => onNewSession(title),
    makeSessionTitle: (content: string) => makeSessionTitle(content),
    refreshRunStatus: () => refreshRunStatus(),
    loadTeamSessions: (teamId: string) => loadTeamSessions(teamId),
    teamSessions,
  });

  async function submitA2UIUserAction(payload: A2UIUserActionPayload) {
    if (selectedEntityKind.value !== "agent") {
      $q.notify({ type: "warning", message: "A2UI 交互仅支持 Agent 会话" });
      return;
    }
    if (!store.selectedAgent?.id) return;
    await sender.sendAgentUserContent(formatUserActionMessage(payload));
  }

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
    } catch { /* ignore */ }
  }

  function onEndTeam() {
    const current = displayTeams.value;
    if (current[0] && current[0].id !== defaultTeamId.value) {
      const fixed = current.find((team) => team.id === defaultTeamId.value);
      if (fixed) displayTeams.value = [fixed, ...current.filter((team) => team.id !== fixed.id)];
    }
    try {
      localStorage.setItem(LS_TM_ORDER, JSON.stringify(displayTeams.value.map((team) => team.id)));
    } catch { /* ignore */ }
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
    } catch { /* ignore */ }
    for (const item of items) {
      if (!ordered.some((candidate) => candidate.id === item.id)) ordered.push(item);
    }
    return ordered;
  }

  function sessionOwnerIsTeam(session: Session) {
    return session.owner_type === "team" || Boolean(session.team_id?.trim());
  }

  async function resolveSessionById(sessionId: string): Promise<Session | null> {
    const fromAgentList = store.sessions.find((item) => item.id === sessionId);
    if (fromAgentList) return fromAgentList;
    if (selectedTeamId.value) {
      const fromCurrentTeam = teamSessions.value[selectedTeamId.value]?.find((item) => item.id === sessionId);
      if (fromCurrentTeam) return fromCurrentTeam;
    }
    for (const sessions of Object.values(teamSessions.value)) {
      const hit = sessions.find((item) => item.id === sessionId);
      if (hit) return hit;
    }
    try {
      return await getSession(sessionId);
    } catch {
      return null;
    }
  }

  async function selectAgent(agent: Agent, options?: { sessionId?: string }) {
    if (selectedEntityKind.value === "team") {
      streamManager.disconnectTeamStream();
      for (const key of Object.keys(teamMessages.value)) {
        delete teamMessages.value[key];
      }
    }
    selectedEntityKind.value = "agent";
    selectedTeamId.value = null;
    teamSelectedSessionId.value = null;
    const resolved = await hydrateAgentSettings(agent);
    store.selectedAgent = resolved;
    store.upsertAgent(resolved);
    await store.loadSessions();
    await nextTick();
    const preferredId = options?.sessionId?.trim();
    store.selectedSession = preferredId
      ? store.sessions.find((item) => item.id === preferredId) ?? store.sessions[0] ?? null
      : store.sessions[0] ?? null;
    store.messages = [];
    if (store.selectedSession) await store.loadMessages();
  }

  async function selectTeam(team: TeamRow, options?: { sessionId?: string }) {
    if (selectedTeamId.value && selectedTeamId.value !== team.id) {
      streamManager.disconnectTeamStream();
      for (const key of Object.keys(teamMessages.value)) {
        delete teamMessages.value[key];
      }
    }
    selectedEntityKind.value = "team";
    selectedTeamId.value = team.id;
    store.selectedSession = null;
    store.messages = [];
    await loadTeamSessions(team.id);
    const preferredId = options?.sessionId?.trim();
    const teamList = teamSessions.value[team.id] ?? [];
    teamSelectedSessionId.value = preferredId
      ? teamList.find((item) => item.id === preferredId)?.id ?? preferredId
      : teamList[0]?.id ?? null;
    if (teamSelectedSessionId.value) {
      teamMessages.value[teamSelectedSessionId.value] = await listMessages(teamSelectedSessionId.value);
    }
  }

  async function onSelectSession(sessionId: string) {
    const resolved = await resolveSessionById(sessionId);
    if (!resolved) return;

    if (sessionOwnerIsTeam(resolved)) {
      const teamId = resolved.team_id?.trim();
      if (!teamId) return;
      const team = displayTeams.value.find((item) => item.id === teamId);
      if (!team) {
        $q.notify({ type: "warning", message: "找不到该会话所属的 Team" });
        return;
      }
      if (selectedEntityKind.value !== "team" || selectedTeamId.value !== teamId) {
        await selectTeam(team, { sessionId });
        streamManager.ensureTeamStream(sessionId);
        return;
      }
      teamSelectedSessionId.value = sessionId;
      teamMessages.value[sessionId] = await listMessages(sessionId);
      streamManager.ensureTeamStream(sessionId);
      return;
    }

    const agentId = resolved.agent_id?.trim();
    if (!agentId) return;
    const agent =
      store.agents.find((item) => item.id === agentId) ?? displayAgents.value.find((item) => item.id === agentId);
    if (!agent) {
      $q.notify({ type: "warning", message: "找不到该会话所属的 Agent" });
      return;
    }
    if (selectedEntityKind.value !== "agent" || store.selectedAgent?.id !== agentId) {
      await selectAgent(agent, { sessionId });
      streamManager.ensureChatStream(sessionId);
      return;
    }

    const session = store.sessions.find((item) => item.id === sessionId) ?? resolved;
    store.selectedSession = session;
    store.messages = [];
    if (session) {
      await store.loadMessages();
      streamManager.ensureChatStream(session.id);
    }
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
    } catch {
      $q.notify({ type: "negative", message: "重命名失败，请重试" });
    }
  }

  function openSessionTrace(sessionId: string, tab: "trace" | "events" = "trace") {
    const session = displaySessions.value.find((item) => item.id === sessionId);
    traceSessionId.value = sessionId;
    traceSessionTitle.value = session?.title ?? t("chat.untitledSession");
    traceInitialTab.value = tab;
    traceSessionOwnerKind.value = selectedEntityKind.value === "team" ? "team" : "agent";
    traceOpen.value = true;
  }

  const traceStreamDeps = computed(() => ({
    ownerKind: traceSessionOwnerKind.value,
    subscribe: streamManager.subscribeSessionStream,
  }));

  function openSessionEvents() {
    const sessionId = selectedSessionForUi.value?.id;
    if (!sessionId) return;
    openSessionTrace(sessionId, "events");
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
        default_model: selectedModel?.model || store.selectedAgent.model,
      });
      if (store.selectedSession) {
        await store.loadMessages();
        streamManager.ensureChatStream(store.selectedSession.id);
      }
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
        default_model: selectedModel?.model || "",
      });
      teamSessions.value[selectedTeamId.value] = [
        { ...created, at: formatSessionTime(created.last_message_at || created.updated_at || created.created_at) },
        ...(teamSessions.value[selectedTeamId.value] ?? []),
      ];
      teamSelectedSessionId.value = created.id;
      teamMessages.value[created.id] = [];
      streamManager.ensureTeamStream(created.id);
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
    streamManager.cancelActiveStream();
    const sid = selectedSessionForUi.value?.id;
    sender.stopStreaming(sid);
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

  watch(sender.sending, (val) => {
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
    sender.clearSendingTimeout();
    streamManager.disconnectAll();
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
            model: editModel.value,
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
            definition_json: team.definition_json || "{}",
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

  async function readFileAsBase64(file: File): Promise<string> {
    const buf = await file.arrayBuffer();
    const bytes = new Uint8Array(buf);
    const chunks: string[] = [];
    const chunkSize = 8192;
    for (let i = 0; i < bytes.length; i += chunkSize) {
      const slice = bytes.subarray(i, i + chunkSize);
      chunks.push(String.fromCharCode(...slice));
    }
    return btoa(chunks.join(""));
  }

  async function onFileChange(event: Event) {
    const input = event.target as HTMLInputElement;
    if (!input.files?.length) return;

    const sessionId = selectedSessionForUi.value?.id ?? store.selectedSession?.id ?? "";
    if (!sessionId) {
      $q.notify({ type: "warning", message: "请先创建或选择会话再上传附件" });
      input.value = "";
      return;
    }

    for (const file of Array.from(input.files)) {
      const tempId = `pending-${Date.now()}-${file.name}`;
      const record: ChatAttachment = { id: tempId, name: file.name, progress: 0.1 };
      attachments.value.push(record);
      try {
        const meta = await uploadArtifact({
          session_id: sessionId,
          name: file.name,
          mime_type: file.type || "application/octet-stream",
          data_base64: await readFileAsBase64(file),
        });
        record.id = meta.id;
        record.progress = 1;
      } catch (e) {
        attachments.value = attachments.value.filter((item) => item.id !== tempId);
        $q.notify({
          type: "negative",
          message: e instanceof Error ? e.message : "附件上传失败",
        });
      }
    }
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
        definition_json: team.definition_json,
      }));
    } catch {
      displayTeams.value = [];
    }
  }

  async function loadTeamSessions(teamID: string) {
    const rows = await listTeamSessions(teamID);
    teamSessions.value[teamID] = rows.map((session) => ({
      ...session,
      at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at),
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
        } catch { /* ignore */ }
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

  async function refreshRunStatus() {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) {
      runStatus.value = "idle";
      runMeta.value = null;
      isAwaitingUser.value = false;
      awaitingRunId.value = "";
      clearAwaitMeta();
      return;
    }
    try {
      const rs = await getRunStatus(sid);
      runStatus.value = rs.status;
      runMeta.value = rs;
      isAwaitingUser.value = rs.status === "awaiting_user";
      awaitingRunId.value = rs.runId;
      if (rs.status === "awaiting_user") {
        applyAwaitMeta(rs);
      } else {
        clearAwaitMeta();
      }
    } catch { /* ignore */ }
  }

  const isRunnerActive = computed(
    () => runStatus.value === "running" || runStatus.value === "pending" || sender.sending.value
  );

  async function onEnqueueWhileRunning(content: string) {
    const sid = selectedSessionForUi.value?.id;
    if (!sid || !content.trim()) return;
    const res = await enqueueUserMessage(sid, content.trim());
    if (res.accepted) {
      $q.notify({
        type: "positive",
        message: res.queued
          ? t("chat.enqueueQueued", "Message queued for after the current run")
          : t("chat.enqueueAccepted", "Message will be injected at the next tool boundary"),
      });
      await refreshPendingMessages();
      await refreshRunStatus();
      return;
    }
    $q.notify({ type: "warning", message: t("chat.enqueueRejected", "Could not enqueue message") });
  }

  async function loadChatOptions() {
    let modeRows: ChatOption[] = [];
    try {
      modeRows = await listChatOptions("dialog_mode");
    } catch { /* keep fallback */ }
    let modelRows: PlatformResource[] = [];
    try {
      modelRows = await listPlatformResources("llm-provider-models");
    } catch { /* keep empty */ }
    if (!modelRows.length) {
      try {
        const catalogModels = await listChatOptions("model");
        if (catalogModels.length) {
          modelRows = chatModelOptionsToPlatform(catalogModels);
        }
      } catch { /* ignore */ }
    }
    if (modeRows.length) {
      modeOpts.value = modeRows.map((item) => ({ label: item.label, value: item.key }));
    }
    providerModels.value = modelRows.filter((item) => item.enabled !== false);
    if (providerModels.value.length) {
      provOpts.value = providerModels.value.map((item) => ({
        label: item.name || item.model,
        value: getProviderModelValue(item),
        caption: `${item.provider} / ${item.model}`,
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
      const hydrated = await hydrateAgentSettings(store.selectedAgent);
      store.selectedAgent = hydrated;
      store.upsertAgent(hydrated);
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
    activePlannerKind,
    sending: sender.sending,
    inputDisabled: sender.inputDisabled,
    modeOpts,
    provOpts,
    attachments,
    pendingMessages,
    isAwaitingUser,
    runStatus,
    runMeta,
    isRunnerActive,
    onEnqueueWhileRunning,
    wsReplaying: streamManager.wsReplaying,
    awaitKind,
    awaitToolKey,
    submitAwaitingReply: sender.submitAwaitingReply,
    submitToolConfirm: sender.submitToolConfirm,
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
    traceInitialTab,
    traceStreamDeps,
    settingsTitle,
    selectedProviderModel,
    displaySessions,
    selectedSessionForUi,
    displayMessages,
    reactToolLinkIndex,
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
    openSessionEvents,
    onRestoreSession,
    onArchiveSession,
    onSessionDetail,
    onNewSession,
    onSend: sender.onSend,
    submitA2UIUserAction,
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
    onVoiceClick,
  };
}
