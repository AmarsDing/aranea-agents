import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRoute } from "vue-router";
import { enqueueUserMessage, getRunStatus, type RunStatus, type RunStatusValue } from "../api";
import type {
  Agent,
  ChatEntityKind,
  Message,
  SessionView,
  TeamRow,
} from "../../../components/chat/types";
import { useAppStore } from "../../../stores/app";
import { cancelRunningToolMessages } from "../envelopeToolCall";
import { runStatusFromEnvelope, messageQueuedFromEnvelope } from "../envelopeRunStatus";
import type { Envelope } from "../envelope";
import { useChatStreamManager } from "./useChatStreamManager";
import { useChatInboundSync } from "./useChatInboundSync";
import { useChatSender } from "./useChatSender";
import { useFollowUpQueue } from "./useFollowUpQueue";
import { useAwaitReply } from "./useAwaitReply";
import {
  formatUserActionMessage,
  type A2UIUserActionPayload,
} from "../a2uiUserAction";
import { buildReactToolLinkIndex } from "../reactToolLinkIndex";
import { formatSessionTime, getProviderModelValue } from "./chatWorkspaceUtils";
import { useChatSidebarOrder } from "./useChatSidebarOrder";
import { useChatAttachments } from "./useChatAttachments";
import { useChatProviderOptions } from "./useChatProviderOptions";
import { useChatDeleteFlow } from "./useChatDeleteFlow";
import { useChatEntityNav } from "./useChatEntityNav";
import { useChatTraceDialog, useChatSessionArtifacts } from "./useChatTraceAndArtifacts";
import { useChatSettingsDialog } from "./useChatSettingsDialog";
import { hydrateAgentSettings } from "../agentPlannerSettings";

export function useChatWorkspace() {
  const { t } = useI18n();
  const $q = useQuasar();
  const route = useRoute();
  const store = useAppStore();

  const isDark = computed(() => $q.dark.isActive);
  const leftOpen = ref(true);
  const rightOpen = ref(true);
  const search = ref("");
  const selectedEntityKind = ref<ChatEntityKind>("agent");
  const selectedTeamId = ref<string | null>(null);
  const teamSelectedSessionId = ref<string | null>(null);
  const defaultAgentId = ref<string | null>(null);
  const defaultTeamId = ref("team-default-1");

  const displayAgents = ref<Agent[]>([]);
  const displayTeams = ref<TeamRow[]>([]);
  const teamSessions = ref<Record<string, Array<import("../../../components/chat/types").Session & { at: string }>>>({});
  const teamMessages = ref<Record<string, Message[]>>({});

  const inputText = ref("");
  const runStatus = ref<RunStatusValue>("idle");
  const runMeta = ref<RunStatus | null>(null);

  const awaitReply = useAwaitReply();
  const {
    isAwaitingUser,
    awaitingRunId,
    awaitKind,
    awaitToolKey,
    applyRunStatus: applyAwaitRunStatus,
    clearAwaitMeta,
  } = awaitReply;

  const providerOpts = useChatProviderOptions(store);
  const { dialogMode, modelProvider, modeOpts, provOpts, providerModels, loadChatOptions, onModeChange, onProviderChange } =
    providerOpts;

  const selectedProviderModel = computed(() =>
    providerModels.value.find((row) => getProviderModelValue(row) === modelProvider.value)
  );

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
        metadata_json: session.metadata_json,
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
        metadata_json: session.metadata_json,
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

  const sessionIdForPending = computed(() => selectedSessionForUi.value?.id);
  const sessionIdForArtifacts = computed(() => selectedSessionForUi.value?.id);

  const { fileRef, attachments, pickFile, onFileChange, removeAttachment } =
    useChatAttachments(sessionIdForArtifacts);

  let applyRunStatusFromEnvelope!: (env: Envelope) => void;

  const streamManager = useChatStreamManager({
    store,
    teamMessages,
    teamSelectedSessionId,
    selectedTeamId,
    displayTeams,
    markSendingDone: () => sender.markSendingDone(),
    onRunStatus: (env) => applyRunStatusFromEnvelope(env),
  });

  const selectedAgentId = computed(() => store.selectedAgent?.id);
  const selectedSessionId = computed(() => selectedSessionForUi.value?.id);

  function makeSessionTitle(content: string) {
    const plain = content
      .replace(/[#>*_`~\[\]()]/g, "")
      .replace(/\s+/g, " ")
      .trim();
    if (!plain) return t("chat.untitledSession");
    return plain.length > 22 ? `${plain.slice(0, 22)}…` : plain;
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
      applyAwaitRunStatus(rs);
    } catch {
      /* ignore */
    }
  }

  const entityNav = useChatEntityNav({
    store,
    streamManager,
    selectedEntityKind,
    selectedTeamId,
    teamSelectedSessionId,
    displayAgents,
    displayTeams,
    teamSessions,
    teamMessages,
    dialogMode,
    selectedProviderModel,
    makeSessionTitle,
    t,
  });

  useChatInboundSync({
    selectedEntityKind,
    selectedAgentId,
    selectedTeamId,
    selectedSessionId,
    ensureChatStream: streamManager.ensureChatStream,
    ensureTeamStream: streamManager.ensureTeamStream,
    patchAgentMessages: streamManager.patchAgentMessages,
    patchTeamMessages: streamManager.patchTeamMessages,
    loadTeamSessions: (teamId: string) => entityNav.loadTeamSessions(teamId),
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
    onNewSession: (title?: string) => entityNav.onNewSession(title),
    makeSessionTitle,
    refreshRunStatus: () => refreshRunStatus(),
    loadTeamSessions: (teamId: string) => entityNav.loadTeamSessions(teamId),
    teamSessions,
  });

  const followUp = useFollowUpQueue(sessionIdForPending, sender.sending);
  const { pendingMessages, refreshPendingMessages, onCancelPending, onUpdatePending } = followUp;

  watch(sender.sending, (val) => followUp.watchSending(val));

  applyRunStatusFromEnvelope = (env: Envelope) => {
    followUp.onRunStatusEnvelope(env);
    if (messageQueuedFromEnvelope(env)) return;
    const rs = runStatusFromEnvelope(env);
    if (!rs) return;
    runStatus.value = rs.status;
    applyAwaitRunStatus({
      status: rs.status,
      runId: rs.runId,
      awaitKind: rs.awaitKind,
      awaitToolKey: rs.awaitToolKey,
    });
    if (rs.status === "cancelled") {
      store.messages = cancelRunningToolMessages(store.messages);
      const sid = selectedSessionForUi.value?.id;
      if (sid && teamMessages.value[sid]) {
        teamMessages.value[sid] = cancelRunningToolMessages(teamMessages.value[sid]);
      }
    }
  };

  const { loadAgentOrder, loadTeamOrder, onEndAgent, onEndTeam } = useChatSidebarOrder(
    displayAgents,
    displayTeams,
    defaultAgentId,
    defaultTeamId
  );

  const deleteFlow = useChatDeleteFlow({
    store,
    displayAgents,
    displayTeams,
    displaySessions,
    selectedEntityKind,
    selectedTeamId,
    teamSessions,
    teamSelectedSessionId,
    teamMessages,
    defaultAgentId,
    selectTeam: entityNav.selectTeam,
  });

  const settingsDialog = useChatSettingsDialog(store, displayAgents, displayTeams);
  const {
    settingsOpen,
    settingsMode,
    settingsId,
    editName,
    editKey,
    editProvider,
    editModel,
    settingsSaving,
    settingsTitle,
    onSaveSettings: saveSettingsDialog,
  } = settingsDialog;

  const traceAndArtifacts = useChatTraceDialog(selectedEntityKind, displaySessions, streamManager);
  const {
    traceOpen,
    traceSessionId,
    traceSessionTitle,
    traceInitialTab,
    traceStreamDeps,
    openSessionTrace,
  } = traceAndArtifacts;

  const { sessionArtifacts, sessionArtifactsLoading, openSessionArtifact } =
    useChatSessionArtifacts(sessionIdForArtifacts);

  const isRunnerActive = computed(
    () => runStatus.value === "running" || runStatus.value === "pending" || sender.sending.value
  );

  async function submitA2UIUserAction(payload: A2UIUserActionPayload) {
    if (selectedEntityKind.value !== "agent") {
      $q.notify({ type: "warning", message: "A2UI 交互仅支持 Agent 会话" });
      return;
    }
    if (!store.selectedAgent?.id) return;
    await sender.sendAgentUserContent(formatUserActionMessage(payload));
  }

  function stopStreaming() {
    streamManager.cancelActiveStream();
    const sid = selectedSessionForUi.value?.id;
    sender.stopStreaming(sid);
  }

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

  async function openSettings(kind: ChatEntityKind, id: string) {
    if (kind === "agent" || kind === "team") {
      await entityNav.openSettings(kind, id);
      return;
    }
    settingsMode.value = kind;
    settingsId.value = id;
    const team = displayTeams.value.find((item) => item.id === id);
    if (team) editName.value = team.display_name;
    settingsOpen.value = true;
  }

  async function onSaveSettings() {
    await saveSettingsDialog(entityNav.updateTeam);
  }

  function openSessionEvents() {
    traceAndArtifacts.openSessionEvents(selectedSessionForUi.value?.id);
  }

  function onVoiceClick() {
    $q.notify({ type: "info", message: t("chat.voicePlaceholder") });
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

  onUnmounted(() => {
    sender.clearSendingTimeout();
    streamManager.disconnectAll();
  });

  onMounted(async () => {
    await Promise.all([loadChatOptions(), entityNav.loadCategoryTree(), entityNav.loadTeams()]);
    await store.loadAgents();
    defaultAgentId.value = store.agents[0]?.id ?? null;
    displayAgents.value = loadAgentOrder(store.agents, defaultAgentId.value);
    displayTeams.value = loadTeamOrder([...displayTeams.value], defaultTeamId.value);

    const routeTeamID = typeof route.query.team === "string" ? route.query.team : "";
    const routeTeam = routeTeamID ? displayTeams.value.find((team) => team.id === routeTeamID) : undefined;
    if (routeTeam) {
      await entityNav.selectTeam(routeTeam);
    } else if (store.selectedAgent) {
      const hydrated = await hydrateAgentSettings(store.selectedAgent);
      store.selectedAgent = hydrated;
      store.upsertAgent(hydrated);
      await store.loadSessions();
      store.selectedSession = store.sessions[0] ?? null;
      store.messages = [];
      if (store.selectedSession) await store.loadMessages();
    } else if (store.agents[0]) {
      await entityNav.selectAgent(store.agents[0]);
    } else if (displayTeams.value[0]) {
      await entityNav.selectTeam(displayTeams.value[0]!);
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
    categoryTree: entityNav.categoryTree,
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
    deleteOpen: deleteFlow.deleteOpen,
    deleteKind: deleteFlow.deleteKind,
    deleteTargetId: deleteFlow.deleteTargetId,
    deleteNameInput: deleteFlow.deleteNameInput,
    deleteBlockBusy: deleteFlow.deleteBlockBusy,
    deleteBlockDefault: deleteFlow.deleteBlockDefault,
    deleting: deleteFlow.deleting,
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
    expectedDeleteName: deleteFlow.expectedDeleteName,
    deleteNameError: deleteFlow.deleteNameError,
    canConfirmDelete: deleteFlow.canConfirmDelete,
    deleteTitleText: deleteFlow.deleteTitleText,
    store,
    onEndAgent,
    onEndTeam,
    selectAgent: entityNav.selectAgent,
    selectTeam: entityNav.selectTeam,
    onSelectSession: entityNav.onSelectSession,
    onRenameSession: entityNav.onRenameSession,
    openSessionTrace,
    openSessionEvents,
    onRestoreSession: entityNav.onRestoreSession,
    onArchiveSession: entityNav.onArchiveSession,
    onSessionDetail: entityNav.onSessionDetail,
    onNewSession: entityNav.onNewSession,
    onSend: sender.onSend,
    submitA2UIUserAction,
    onModeChange,
    onProviderChange,
    stopStreaming,
    openSettings,
    onSaveSettings,
    openDelete: deleteFlow.openDelete,
    onConfirmDelete: deleteFlow.onConfirmDelete,
    pickFile,
    onFileChange,
    removeAttachment,
    onCancelPending,
    onUpdatePending,
    onVoiceClick,
  };
}
