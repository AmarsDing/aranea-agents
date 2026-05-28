import { computed, onMounted, onUnmounted, reactive, ref, shallowRef, toRef, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { useRoute } from "vue-router";
import { enqueueMessage, submitMessageFeedback } from "../api";
import type { SessionView, TeamRow } from "../../../components/chat/types";
import type { Agent } from "../../agents/types";
import { useAppStore } from "../../../stores/app";
import { useChatSessionStore } from "../../../stores/chat/sessionStore";
import { useChatMessageStore } from "../../../stores/chat/messageStore";
import { useChatRuntimeStore } from "../../../stores/chat/runtimeStore";
import { useChatConversationStore } from "../../../stores/chat/conversationStore";
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
import { formatSessionTime, getProviderModelValue, sessionToView } from "./chatWorkspaceUtils";
import { useChatSidebarOrder } from "./useChatSidebarOrder";
import { useChatAttachments } from "./useChatAttachments";
import { fileAcceptForModel, modelSupportsFileInput, type ChatModelDescriptor } from "../modelCapabilities";
import { useChatProviderOptions } from "./useChatProviderOptions";
import { useChatDeleteFlow } from "./useChatDeleteFlow";
import { createChatFocusCoordinator } from "../chatFocusCoordinator";
import { useChatEntityNav } from "./useChatEntityNav";
import { useChatTraceDialog, useChatSessionArtifacts } from "./useChatTraceAndArtifacts";
import { useChatSettingsDialog } from "./useChatSettingsDialog";
import { hydrateAgentSettings } from "../agentPlannerSettings";
import { parseChannelSessionMeta } from "../channelSessionMeta";
import { useKnowledgeStore } from "../../../stores/knowledge";
import type { ComposerUsageSnapshot } from "../composerUsageMetrics";

export function useChatWorkspace() {
  const { t } = useI18n();
  const $q = useQuasar();
  const route = useRoute();
  const appStore = useAppStore();
  const sessionStore = useChatSessionStore();
  const messageStore = useChatMessageStore();
  const runtimeStore = useChatRuntimeStore();
  const conversationStore = useChatConversationStore();

  const isDark = computed(() => $q.dark.isActive);
  const leftOpen = ref(true);
  const rightOpen = ref(true);
  const search = ref("");
  const defaultAgentId = ref<string | null>(null);
  const defaultTeamId = ref("team-default-1");

  const displayAgents = ref<Agent[]>([]);
  const displayTeams = ref<TeamRow[]>([]);
  const inputText = ref("");
  const sessionDrafts = reactive(new Map<string, string>());
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
  const { runStatus, runMeta, applyFromEnvelope, onSessionSwitch, refreshRunStatus, forceSetRunStatus } = runStatusCtrl;

  const providerOpts = useChatProviderOptions(appStore);
  const { dialogMode, modelProvider, modeOpts, provOpts, providerModels, loadChatOptions, onModeChange, onProviderChange } =
    providerOpts;

  const selectedProviderModel = computed(() =>
    providerModels.value.find((row) => getProviderModelValue(row) === modelProvider.value)
  );

  const fileSupported = computed(() => {
    const m = selectedProviderModel.value;
    if (!m) return true;
    return modelSupportsFileInput({
      provider: m.provider,
      model: m.model,
      capabilities: m.capabilities as ChatModelDescriptor["capabilities"],
    });
  });

  const fileAccept = computed(() => {
    const m = selectedProviderModel.value;
    if (!m) return "";
    return fileAcceptForModel({
      provider: m.provider,
      model: m.model,
      capabilities: m.capabilities as ChatModelDescriptor["capabilities"],
    });
  });

  const activePlannerKind = computed(() =>
    (appStore.selectedAgent?.settings?.planner_kind ?? "").trim().toLowerCase()
  );

  const displaySessions = computed((): SessionView[] => {
    if (sessionStore.entityKind === "team" && sessionStore.selectedTeamId) {
      return (sessionStore.teamSessions[sessionStore.selectedTeamId] ?? []).map((session) =>
        sessionToView(session, t)
      );
    }
    if (sessionStore.entityKind === "agent" && appStore.selectedAgent) {
      return sessionStore.sessions.map((session) => sessionToView(session, t));
    }
    return [];
  });

  const selectedSessionForUi = computed((): SessionView | null => {
    if (sessionStore.entityKind === "team" && sessionStore.teamSelectedSessionId) {
      return displaySessions.value.find((session) => session.id === sessionStore.teamSelectedSessionId) ?? null;
    }
    if (!sessionStore.selectedSession) return null;
    return (
      displaySessions.value.find((session) => session.id === sessionStore.selectedSession!.id) ??
      sessionToView(sessionStore.selectedSession, t)
    );
  });

  const composerUsageSnapshot = computed((): ComposerUsageSnapshot | null => {
    const s = selectedSessionForUi.value;
    if (!s) return null;
    return {
      contextRatio: s.context_used_ratio ?? 0,
      contextStatus: s.context_status,
      contextUsedTokens: s.context_used_tokens,
      contextWindow: s.last_context_window_tokens,
      inputTokens: s.input_tokens ?? 0,
      outputTokens: s.output_tokens ?? 0,
      totalTokens: s.total_tokens ?? 0,
      totalCostMicroUsd: s.total_cost_micro_usd ?? 0,
    };
  });

  const displayMessages = computed(() => messageStore.messages);

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
  const sessionLoading = ref(false);
  const focusTurnId = ref<string | undefined>(undefined);

  function focusSessionTurn(turnId: string) {
    const id = turnId.trim();
    if (!id) return;
    focusTurnId.value = id;
  }

  function clearFocusTurn() {
    focusTurnId.value = undefined;
  }

  const { fileRef, attachments, pickFile, onFileChange, uploadFile, removeAttachment } =
    useChatAttachments(sessionIdForArtifacts);

  let applyRunStatusFromEnvelope!: (env: Envelope) => void;

  const streamManager = useChatStreamManager({
    sessionStore,
    messageStore,
    runtimeStore,
    displayTeams,
    resolveAgentId: () => appStore.selectedAgent?.id,
    markSendingDone: () => sender.markSendingDone(),
    onRunStatus: (env) => applyRunStatusFromEnvelope(env),
  });

  const selectedAgentId = computed(() => appStore.selectedAgent?.id);
  const selectedSessionId = computed(() => selectedSessionForUi.value?.id);

  watch(
    () => ({
      entityKind: sessionStore.entityKind,
      agent: appStore.selectedAgent,
      teamId: sessionStore.selectedTeamId,
      team: displayTeams.value.find((team) => team.id === sessionStore.selectedTeamId),
    }),
    ({ entityKind, agent, teamId, team }) => {
      if (entityKind === "team" && teamId) {
        conversationStore.setCurrentTarget({
          type: "team",
          id: teamId,
          name: team?.display_name,
          source: "web",
        });
        return;
      }
      if (agent?.id) {
        conversationStore.setCurrentTarget({
          type: "agent",
          id: agent.id,
          key: agent.agent_key,
          name: agent.display_name,
          source: "web",
        });
        return;
      }
      conversationStore.setCurrentTarget(null);
    },
    { immediate: true }
  );

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
    resolveSessionId: () => sessionStore.currentSessionId() ?? undefined,
    inputText,
    awaitingRunId,
    awaitKind,
    refreshRunStatus: refreshRunStatusForUi,
    notifyError: (message) => $q.notify({ type: "negative", message }),
  });

  async function onMessageFeedback(payload: { messageId: string; rating: "positive" | "negative" }) {
    const sid = sessionStore.currentSessionId();
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

  const focusCoordinator = createChatFocusCoordinator();

  const entityNav = useChatEntityNav({
    appStore,
    sessionStore,
    messageStore,
    streamManager,
    focusCoordinator,
    displayAgents,
    displayTeams,
    dialogMode,
    selectedProviderModel,
    makeSessionTitle,
    t,
  });

  useChatInboundSync({
    appStore,
    sessionStore,
    messageStore,
    selectedAgentId,
    selectedSessionId,
    wsReplaying: streamManager.wsReplaying,
    isChatRoute: () => route.name === "chat",
    shouldAutoFocusChannel: () => {
      // Default OFF: channel inbound messages no longer auto-focus the session
      // to avoid interrupting the user's current workflow. Users can opt in
      // via localStorage channel_auto_focus=true.
      if (typeof localStorage !== "undefined" && localStorage.getItem("channel_auto_focus") === "true") {
        return !inputText.value.trim();
      }
      return false;
    },
    focusChannelSession: (sessionId, agentId, options) =>
      entityNav.focusAgentSessionView(sessionId, agentId, options),
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
    loadTeamSessions: (teamId: string) => entityNav.loadTeamSessions(teamId),
  });

  let refreshPendingMessagesFn: (() => Promise<void>) | undefined;

  const sender = useChatSender({
    appStore,
    sessionStore,
    messageStore,
    inputText,
    dialogMode,
    attachments,
    isAwaitingUser,
    awaitingRunId,
    awaitKind,
    runStatus,
    selectedProviderModel,
    selectedKnowledgeBases,
    ensureChatStream: streamManager.ensureChatStream,
    ensureTeamStream: streamManager.ensureTeamStream,
    sendChatViaWs: streamManager.sendChatViaWs,
    onNewSession: (title?: string) => entityNav.onNewSession(title),
    makeSessionTitle,
    refreshRunStatus: refreshRunStatusForUi,
    setRunStatus: forceSetRunStatus,
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
        messageStore.setMessages(sid, cancelRunningToolMessages(messageStore.getMessages(sid)));
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
    sessionStore,
    messageStore,
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

  const traceAndArtifacts = useChatTraceDialog(toRef(sessionStore, "entityKind"), displaySessions, streamManager);
  const {
    traceOpen,
    traceSessionId,
    traceSessionTitle,
    traceInitialTab,
    traceStreamDeps,
    openSessionTrace,
  } = traceAndArtifacts;

  const { sessionArtifacts, sessionArtifactsLoading, openSessionArtifact, onArtifactDeleted: removeArtifactFromList } =
    useChatSessionArtifacts(sessionIdForArtifacts);

  function onArtifactDeleted(id: string) {
    removeArtifactFromList(id);
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    const msgs = messageStore.getMessages(sid);
    const updated = msgs.map((m) => {
      if (m.attachments) {
        const filtered = m.attachments.filter((a) => a.id !== id);
        if (filtered.length === m.attachments.length) return m;
        return { ...m, attachments: filtered.length > 0 ? filtered : undefined };
      }
      if (!m.options_json) return m;
      try {
        const opts = JSON.parse(m.options_json);
        if (!Array.isArray(opts.attachments)) return m;
        const filtered = opts.attachments.filter((a: Record<string, unknown>) => a.id !== id);
        if (filtered.length === opts.attachments.length) return m;
        opts.attachments = filtered;
        return { ...m, options_json: JSON.stringify(opts) };
      } catch {
        return m;
      }
    });
    messageStore.setMessages(sid, updated);
  }

  const isRunnerActive = computed(
    () => runStatus.value === "running" || runStatus.value === "pending" || sender.sending.value
  );

  // Refresh background jobs when run transitions to idle (tasks may complete after turn ends)
  watch(runStatus, (newVal, oldVal) => {
    if (newVal === "idle" && oldVal !== "idle") {
      jobsRefreshNonce.value += 1;
    }
  });

  const sessionRevision = computed(() => {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return null;
    return messageStore.sessionRevisionBySession[sid] ?? 0;
  });

  async function submitA2UIUserAction(payload: A2UIUserActionPayload) {
    if (sessionStore.entityKind !== "agent") {
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
    if (sessionStore.entityKind === "team") {
      streamManager.ensureTeamStream(sessionId);
    } else {
      streamManager.ensureChatStream(sessionId);
    }
    sessionLoading.value = true;
    try {
      if (replace) clearChatMarkdownCache();
      await messageStore.loadMessages(replace ? { sessionId, replace: true } : { sessionId });
    } catch (err) {
      $q.notify({
        type: "negative",
        message: err instanceof Error ? err.message : "加载消息失败",
      });
    } finally {
      sessionLoading.value = false;
    }
  }

  watch(
    () => selectedSessionForUi.value?.id,
    (sid, prevSid) => {
      if (prevSid) {
        sessionDrafts.set(prevSid, inputText.value);
      }
      if (!sid) {
        onSessionSwitch(undefined);
        isAwaitingUser.value = false;
        awaitingRunId.value = "";
        clearAwaitMeta();
        inputText.value = "";
        return;
      }
      inputText.value = sessionDrafts.get(sid) || "";
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
      selectedSessionForUi.value?.metadata_json ?? sessionStore.selectedSession?.metadata_json
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
      await sessionStore.loadAgentSessions(hydrated.id);
      sessionStore.selectedSession = sessionStore.sessions[0] ?? null;
      if (sessionStore.selectedSession) {
        messageStore.clearSessionMessages(sessionStore.selectedSession.id);
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
      if (focusCoordinator.isRouteSessionWatchSuppressed()) return;
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
      selectedEntityKind: computed(() => sessionStore.entityKind),
      selectedTeamId: computed(() => sessionStore.selectedTeamId),
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
      inboxSessions: conversationStore.inboxSessions,
      selectedSessionForUi,
      composerUsageSnapshot,
      displayMessages,
      reactToolLinkIndex,
      sessionRevision,
      wsConnected: computed(() => runtimeStore.wsConnected),
      wsReplaying: streamManager.wsReplaying,
      jobsRefreshNonce,
      inboundHydrateError,
      sessionLoading,
      focusTurnId,
      focusSessionTurn,
      clearFocusTurn,
      sessionArtifacts,
      sessionArtifactsLoading,
      fileSupported,
      fileAccept,
      openSessionArtifact,
      onArtifactDeleted,
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
      onSend: async () => {
        const sid = selectedSessionForUi.value?.id;
        await sender.onSend();
        if (sid) sessionDrafts.delete(sid);
      },
      submitA2UIUserAction,
      onModeChange,
      onProviderChange,
      stopStreaming,
      pickFile,
      onFileChange,
      uploadFile,
      removeAttachment,
      onCancelPending,
      onUpdatePending,
      onVoiceClick,
      onMessageFeedback,
      retryFailedMessage: sender.retryFailedMessage,
      dismissFailedMessage: (messageId: string) => {
        const sid = sessionStore.currentSessionId();
        if (!sid) return;
        messageStore.setMessages(
          sid,
          messageStore.getMessages(sid).filter((m) => m.id !== messageId)
        );
      },
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
      fileSupported,
    }),
  };
}
