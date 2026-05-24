import { computed, onMounted, onUnmounted, reactive, ref, shallowRef, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRoute } from "vue-router";
import { enqueueMessage, submitMessageFeedback } from "../api";
import type { SessionView, TeamRow } from "../../../components/chat/types";
import type { Agent } from "../../agents/types";
import { useAppStore } from "../../../stores/app";
import { useChatStore } from "../../../stores/chat";
import { cancelRunningToolMessages } from "../envelopeToolCall";
import { runStatusFromEnvelope } from "../envelopeRunStatus";
import type { Envelope } from "../envelope";
import { useChatRunStatus } from "./useChatRunStatus";
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
import { messageListStructureFingerprint } from "../messageListFingerprint";
import { clearChatMarkdownCache } from "../chatMessageMarkdown";
import { formatSessionTime, getProviderModelValue } from "./chatWorkspaceUtils";
import { useChatSidebarOrder } from "./useChatSidebarOrder";
import { useChatAttachments } from "./useChatAttachments";
import { useChatProviderOptions } from "./useChatProviderOptions";
import { useChatDeleteFlow } from "./useChatDeleteFlow";
import { useChatEntityNav } from "./useChatEntityNav";
import { useChatTraceDialog, useChatSessionArtifacts } from "./useChatTraceAndArtifacts";
import { useChatSettingsDialog } from "./useChatSettingsDialog";
import { hydrateAgentSettings } from "../agentPlannerSettings";
import { parseChannelSessionMeta } from "../channelSessionMeta";
import { useKnowledgeStore } from "../../../stores/knowledge";

export function useChatWorkspace() {
  const { t } = useI18n();
  const $q = useQuasar();
  const route = useRoute();
  const appStore = useAppStore();
  const chatStore = useChatStore();

  const isDark = computed(() => $q.dark.isActive);
  const leftOpen = ref(true);
  const rightOpen = ref(true);
  const search = ref("");
  const defaultAgentId = ref<string | null>(null);
  const defaultTeamId = ref("team-default-1");

  const displayAgents = ref<Agent[]>([]);
  const displayTeams = ref<TeamRow[]>([]);
  const inputText = ref("");
  const selectedKnowledgeBases = ref<string[]>([]);
  const knowledgeBaseOptions = ref<Array<{ label: string; value: string }>>([]);

  const awaitReply = useAwaitReply();
  const {
    isAwaitingUser,
    awaitingRunId,
    awaitKind,
    awaitToolKey,
    applyRunStatus: applyAwaitRunStatus,
    clearAwaitMeta,
    createSubmitHandlers,
  } = awaitReply;

  const runStatusCtrl = useChatRunStatus({ applyAwaitRunStatus });
  const { runStatus, runMeta, applyFromEnvelope, onSessionSwitch, refreshRunStatus } = runStatusCtrl;

  const providerOpts = useChatProviderOptions(appStore);
  const { dialogMode, modelProvider, modeOpts, provOpts, providerModels, loadChatOptions, onModeChange, onProviderChange } =
    providerOpts;

  const selectedProviderModel = computed(() =>
    providerModels.value.find((row) => getProviderModelValue(row) === modelProvider.value)
  );

  const activePlannerKind = computed(() =>
    (appStore.selectedAgent?.settings?.planner_kind ?? "").trim().toLowerCase()
  );

  const displaySessions = computed((): SessionView[] => {
    if (chatStore.entityKind === "team" && chatStore.selectedTeamId) {
      return (chatStore.teamSessions[chatStore.selectedTeamId] ?? []).map((session) => ({
        id: session.id,
        title: session.title || t("chat.untitledSession"),
        context_used_ratio: session.context_used_ratio,
        total_tokens: session.total_tokens,
        at: session.at,
        timeline_at: session.last_message_at || session.updated_at || session.created_at,
        agent_id: session.agent_id,
        status: session.status,
        pinned_at: session.pinned_at,
        metadata_json: session.metadata_json,
      }));
    }
    if (chatStore.entityKind === "agent" && appStore.selectedAgent) {
      return chatStore.sessions.map((session) => ({
        id: session.id,
        title: session.title || t("chat.untitledSession"),
        context_used_ratio: session.context_used_ratio,
        total_tokens: session.total_tokens,
        at: formatSessionTime(session.last_message_at || session.updated_at || session.created_at),
        timeline_at: session.last_message_at || session.updated_at || session.created_at,
        status: session.status,
        pinned_at: session.pinned_at,
        metadata_json: session.metadata_json,
      }));
    }
    return [];
  });

  const selectedSessionForUi = computed((): SessionView | null => {
    if (chatStore.entityKind === "team" && chatStore.teamSelectedSessionId) {
      return displaySessions.value.find((session) => session.id === chatStore.teamSelectedSessionId) ?? null;
    }
    if (!chatStore.selectedSession) return null;
    return (
      displaySessions.value.find((session) => session.id === chatStore.selectedSession!.id) ?? {
        id: chatStore.selectedSession.id,
        title: chatStore.selectedSession.title || t("chat.untitledSession"),
        context_used_ratio: chatStore.selectedSession.context_used_ratio,
        total_tokens: chatStore.selectedSession.total_tokens,
        at: formatSessionTime(
          chatStore.selectedSession.last_message_at ||
            chatStore.selectedSession.updated_at ||
            chatStore.selectedSession.created_at
        ),
        timeline_at:
          chatStore.selectedSession.last_message_at ||
          chatStore.selectedSession.updated_at ||
          chatStore.selectedSession.created_at,
        pinned_at: chatStore.selectedSession.pinned_at,
      }
    );
  });

  const displayMessages = computed(() => chatStore.messages);

  const reactToolLinkIndex = shallowRef(buildReactToolLinkIndex([]));
  watch(
    () => messageListStructureFingerprint(displayMessages.value),
    () => {
      reactToolLinkIndex.value = buildReactToolLinkIndex(displayMessages.value);
    },
    { immediate: true }
  );

  const sessionIdForPending = computed(() => selectedSessionForUi.value?.id);
  const sessionIdForArtifacts = computed(() => selectedSessionForUi.value?.id);
  const jobsRefreshNonce = ref(0);
  const inboundHydrateError = ref("");
  const focusTurnId = ref<string | undefined>(undefined);

  function focusSessionTurn(turnId: string) {
    const id = turnId.trim();
    if (!id) return;
    focusTurnId.value = id;
  }

  function clearFocusTurn() {
    focusTurnId.value = undefined;
  }

  const { fileRef, attachments, pickFile, onFileChange, removeAttachment } =
    useChatAttachments(sessionIdForArtifacts);

  let applyRunStatusFromEnvelope!: (env: Envelope) => void;

  const streamManager = useChatStreamManager({
    chatStore,
    displayTeams,
    resolveAgentId: () => appStore.selectedAgent?.id,
    markSendingDone: () => sender.markSendingDone(),
    onRunStatus: (env) => applyRunStatusFromEnvelope(env),
  });

  const selectedAgentId = computed(() => appStore.selectedAgent?.id);
  const selectedSessionId = computed(() => selectedSessionForUi.value?.id);

  async function refreshRunStatusForUi() {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) {
      isAwaitingUser.value = false;
      awaitingRunId.value = "";
      clearAwaitMeta();
    }
    await refreshRunStatus(sid);
  }

  const awaitSubmit = createSubmitHandlers({
    resolveSessionId: () => chatStore.currentSessionId() ?? undefined,
    inputText,
    awaitingRunId,
    awaitKind,
    refreshRunStatus: refreshRunStatusForUi,
    notifyError: (message) => $q.notify({ type: "negative", message }),
  });

  async function onMessageFeedback(payload: { messageId: string; rating: "positive" | "negative" }) {
    const sid = chatStore.currentSessionId();
    if (!sid) return;
    try {
      await submitMessageFeedback({
        session_id: sid,
        message_id: payload.messageId,
        rating: payload.rating,
      });
      $q.notify({ type: "positive", message: t("chat.feedbackThanks", "感谢反馈") });
    } catch (err) {
      $q.notify({
        type: "negative",
        message: err instanceof Error ? err.message : t("chat.feedbackFailed", "反馈提交失败"),
      });
    }
  }

  function makeSessionTitle(content: string) {
    const plain = content
      .replace(/[#>*_`~\[\]()]/g, "")
      .replace(/\s+/g, " ")
      .trim();
    if (!plain) return t("chat.untitledSession");
    return plain.length > 22 ? `${plain.slice(0, 22)}…` : plain;
  }

  const entityNav = useChatEntityNav({
    appStore,
    chatStore,
    streamManager,
    displayAgents,
    displayTeams,
    dialogMode,
    selectedProviderModel,
    makeSessionTitle,
    t,
  });

  useChatInboundSync({
    appStore,
    chatStore,
    selectedAgentId,
    selectedSessionId,
    wsReplaying: streamManager.wsReplaying,
    isChatRoute: () => route.name === "chat",
    shouldAutoFocusChannel: () => {
      if (typeof localStorage !== "undefined" && localStorage.getItem("channel_auto_focus") === "false") {
        return false;
      }
      return !inputText.value.trim();
    },
    focusChannelSession: (sessionId, agentId) =>
      entityNav.focusAgentSessionView(sessionId, agentId),
    onTurnComplete: () => {
      jobsRefreshNonce.value += 1;
    },
    onHydrateError: (sessionId, message) => {
      if (selectedSessionId.value === sessionId) {
        inboundHydrateError.value = message;
      }
    },
    ensureChatStream: streamManager.ensureChatStream,
    ensureTeamStream: streamManager.ensureTeamStream,
    patchAgentMessages: streamManager.patchAgentMessages,
    patchTeamMessages: streamManager.patchTeamMessages,
    loadTeamSessions: (teamId: string) => entityNav.loadTeamSessions(teamId),
  });

  let refreshPendingMessagesFn: (() => Promise<void>) | undefined;

  const sender = useChatSender({
    appStore,
    chatStore,
    inputText,
    dialogMode,
    attachments,
    isAwaitingUser,
    awaitingRunId,
    runStatus,
    selectedProviderModel,
    selectedKnowledgeBases,
    ensureChatStream: streamManager.ensureChatStream,
    ensureTeamStream: streamManager.ensureTeamStream,
    sendChatViaWs: streamManager.sendChatViaWs,
    onNewSession: (title?: string) => entityNav.onNewSession(title),
    makeSessionTitle,
    refreshRunStatus: refreshRunStatusForUi,
    submitAwaitingReply: awaitSubmit.submitAwaitingReply,
    submitToolConfirm: awaitSubmit.submitToolConfirm,
    refreshPendingMessages: () => refreshPendingMessagesFn?.() ?? Promise.resolve(),
  });

  const followUp = useFollowUpQueue(sessionIdForPending, sender.sending, (message) =>
    $q.notify({ type: "negative", message })
  );
  const { pendingMessages, refreshPendingMessages, onCancelPending, onUpdatePending } = followUp;
  refreshPendingMessagesFn = refreshPendingMessages;

  watch(sender.sending, (val) => followUp.watchSending(val));

  applyRunStatusFromEnvelope = (env: Envelope) => {
    followUp.onRunStatusEnvelope(env);
    applyFromEnvelope(env);
    const rs = runStatusFromEnvelope(env);
    if (rs?.status === "cancelled") {
      const sid = selectedSessionForUi.value?.id;
      if (sid) {
        chatStore.setMessages(sid, cancelRunningToolMessages(chatStore.getMessages(sid)));
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
    appStore,
    chatStore,
    displayAgents,
    displayTeams,
    displaySessions,
    defaultAgentId,
    selectTeam: entityNav.selectTeam,
  });

  const settingsDialog = useChatSettingsDialog(appStore, displayAgents, displayTeams);
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

  const traceAndArtifacts = useChatTraceDialog(chatStore.entityKind, displaySessions, streamManager);
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

  const sessionRevision = computed(() => {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return null;
    return chatStore.sessionRevisionBySession[sid] ?? 0;
  });

  async function submitA2UIUserAction(payload: A2UIUserActionPayload) {
    if (chatStore.entityKind !== "agent") {
      $q.notify({ type: "warning", message: "A2UI 交互仅支持 Agent 会话" });
      return;
    }
    if (!appStore.selectedAgent?.id) return;
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
    try {
      const res = await enqueueMessage(sid, content.trim());
      if (res.accepted) {
        $q.notify({
          type: "positive",
          message: res.queued
            ? t("chat.enqueueQueued", "Message queued for after the current run")
            : t("chat.enqueueAccepted", "Message will be injected at the next tool boundary"),
        });
        await refreshPendingMessages();
        return;
      }
      $q.notify({ type: "warning", message: t("chat.enqueueRejected", "Could not enqueue message") });
    } catch (err) {
      $q.notify({
        type: "negative",
        message: err instanceof Error ? err.message : t("chat.enqueueRejected", "Could not enqueue message"),
      });
    }
  }

  async function openSettings(kind: Parameters<typeof entityNav.openSettings>[0], id: string) {
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

  let visibleRefreshTimer: ReturnType<typeof setTimeout> | null = null;

  async function bindSessionView(sessionId: string, replace = true) {
    if (chatStore.entityKind === "team") {
      streamManager.ensureTeamStream(sessionId);
    } else {
      streamManager.ensureChatStream(sessionId);
    }
    try {
      if (replace) clearChatMarkdownCache();
      await chatStore.loadMessages(replace ? { sessionId, replace: true } : { sessionId });
    } catch (err) {
      $q.notify({
        type: "negative",
        message: err instanceof Error ? err.message : "加载消息失败",
      });
    }
  }

  watch(
    () => selectedSessionForUi.value?.id,
    (sid, prevSid) => {
      if (!sid) {
        onSessionSwitch(undefined);
        isAwaitingUser.value = false;
        awaitingRunId.value = "";
        clearAwaitMeta();
        return;
      }
      onSessionSwitch(sid);
      if (sid !== prevSid) {
        void bindSessionView(sid, true);
      }
    },
    { immediate: true }
  );

  function onPageVisible() {
    if (document.visibilityState !== "visible") return;
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    const meta = parseChannelSessionMeta(
      selectedSessionForUi.value?.metadata_json ?? chatStore.selectedSession?.metadata_json
    );
    if (!meta) return;
    if (visibleRefreshTimer) clearTimeout(visibleRefreshTimer);
    visibleRefreshTimer = setTimeout(() => {
      visibleRefreshTimer = null;
      void bindSessionView(sid, false);
    }, 600);
  }

  onUnmounted(() => {
    if (visibleRefreshTimer) clearTimeout(visibleRefreshTimer);
    document.removeEventListener("visibilitychange", onPageVisible);
    sender.clearSendingTimeout();
    streamManager.disconnectAll();
  });

  onMounted(async () => {
    document.addEventListener("visibilitychange", onPageVisible);
    await Promise.all([loadChatOptions(), entityNav.loadCategoryTree(), entityNav.loadTeams()]);
    try {
      const knowledgeStore = useKnowledgeStore();
      const cols = await knowledgeStore.loadCollections({ limit: 50 });
      knowledgeBaseOptions.value = cols.items.map((c) => ({
        label: c.name || c.id,
        value: c.id,
      }));
    } catch {
      knowledgeBaseOptions.value = [];
    }
    await appStore.loadAgents();
    defaultAgentId.value = appStore.agents[0]?.id ?? null;
    displayAgents.value = loadAgentOrder(appStore.agents, defaultAgentId.value);
    displayTeams.value = loadTeamOrder([...displayTeams.value], defaultTeamId.value);

    const routeTeamID = typeof route.query.team === "string" ? route.query.team : "";
    const routeTeam = routeTeamID ? displayTeams.value.find((team) => team.id === routeTeamID) : undefined;
    if (routeTeam) {
      await entityNav.selectTeam(routeTeam);
    } else if (appStore.selectedAgent) {
      const hydrated = await hydrateAgentSettings(appStore.selectedAgent);
      appStore.selectedAgent = hydrated;
      appStore.upsertAgent(hydrated);
      await chatStore.loadAgentSessions(hydrated.id);
      chatStore.selectedSession = chatStore.sessions[0] ?? null;
      if (chatStore.selectedSession) {
        chatStore.clearSessionMessages(chatStore.selectedSession.id);
      }
    } else if (appStore.agents[0]) {
      await entityNav.selectAgent(appStore.agents[0]);
    } else if (displayTeams.value[0]) {
      await entityNav.selectTeam(displayTeams.value[0]!);
    }
    const routeSession = typeof route.query.session === "string" ? route.query.session.trim() : "";
    if (routeSession) {
      const routeAgent = typeof route.query.agent === "string" ? route.query.agent.trim() : "";
      await entityNav.focusSessionById(routeSession, routeAgent || undefined);
    }
  });

  watch(
    () => route.query.session,
    (sid) => {
      if (typeof sid !== "string" || !sid.trim()) return;
      const routeAgent = typeof route.query.agent === "string" ? route.query.agent.trim() : "";
      void entityNav.focusSessionById(sid.trim(), routeAgent || undefined);
    }
  );

  return {
    fileRef,
    layout: reactive({
      t,
      isDark,
      leftOpen,
      rightOpen,
      search,
    }),
    entity: reactive({
      store: appStore,
      selectedEntityKind: computed(() => chatStore.entityKind),
      selectedTeamId: computed(() => chatStore.selectedTeamId),
      displayAgents,
      displayTeams,
      categoryTree: entityNav.categoryTree,
      activePlannerKind,
      onEndAgent,
      onEndTeam,
      selectAgent: entityNav.selectAgent,
      selectTeam: entityNav.selectTeam,
      openSettings,
      openDelete: deleteFlow.openDelete,
    }),
    session: reactive({
      displaySessions,
      selectedSessionForUi,
      displayMessages,
      reactToolLinkIndex,
      sessionRevision,
      wsConnected: computed(() => chatStore.wsConnected),
      wsReplaying: streamManager.wsReplaying,
      jobsRefreshNonce,
      inboundHydrateError,
      focusTurnId,
      focusSessionTurn,
      clearFocusTurn,
      sessionArtifacts,
      sessionArtifactsLoading,
      openSessionArtifact,
      onSelectSession: entityNav.onSelectSession,
      onRenameSession: entityNav.onRenameSession,
      onTogglePinSession: entityNav.onTogglePinSession,
      onRestoreSession: entityNav.onRestoreSession,
      onArchiveSession: entityNav.onArchiveSession,
      onSessionDetail: entityNav.onSessionDetail,
      onNewSession: entityNav.onNewSession,
      openSessionTrace,
      openSessionEvents,
    }),
    composer: reactive({
      inputText,
      dialogMode,
      modelProvider,
      modeOpts,
      provOpts,
      attachments,
      selectedKnowledgeBases,
      knowledgeBaseOptions,
      sending: sender.sending,
      inputDisabled: sender.inputDisabled,
      pendingMessages,
      isAwaitingUser,
      runStatus,
      runMeta,
      isRunnerActive,
      onEnqueueWhileRunning,
      awaitKind,
      awaitToolKey,
      submitAwaitingReply: sender.submitAwaitingReply,
      submitToolConfirm: sender.submitToolConfirm,
      onSend: sender.onSend,
      submitA2UIUserAction,
      onModeChange,
      onProviderChange,
      stopStreaming,
      pickFile,
      onFileChange,
      removeAttachment,
      onCancelPending,
      onUpdatePending,
      onVoiceClick,
      onMessageFeedback,
    }),
    dialogs: reactive({
      settingsOpen,
      settingsMode,
      settingsId,
      editName,
      editKey,
      editProvider,
      editModel,
      settingsSaving,
      settingsTitle,
      onSaveSettings,
      deleteOpen: deleteFlow.deleteOpen,
      deleteKind: deleteFlow.deleteKind,
      deleteTargetId: deleteFlow.deleteTargetId,
      deleteNameInput: deleteFlow.deleteNameInput,
      deleteBlockBusy: deleteFlow.deleteBlockBusy,
      deleteBlockDefault: deleteFlow.deleteBlockDefault,
      deleting: deleteFlow.deleting,
      expectedDeleteName: deleteFlow.expectedDeleteName,
      deleteNameError: deleteFlow.deleteNameError,
      canConfirmDelete: deleteFlow.canConfirmDelete,
      deleteTitleText: deleteFlow.deleteTitleText,
      onConfirmDelete: deleteFlow.onConfirmDelete,
      traceOpen,
      traceSessionId,
      traceSessionTitle,
      traceInitialTab,
      traceStreamDeps,
      selectedProviderModel,
    }),
  };
}
