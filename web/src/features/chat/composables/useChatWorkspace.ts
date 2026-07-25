import { computed, onMounted, onUnmounted, reactive, ref, toRef, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRoute } from 'vue-router';
import type { SessionView, TeamRow } from '../../../components/chat/types';
import type { Agent } from '../../agents/types';
import type { CompressStatus } from '../../session/types';
import type { SpiritPanelMode } from '../../spirit/types';
import type { SubmitClarificationPayload } from '../types';
import { useAppStore } from '../../../stores/app';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { useChatConversationStore } from '../../../stores/chat/conversationStore';
import { useSpiritTeamStore } from '../../../stores/spirit';
import { cancelRunningToolMessages } from '../activityToolCall';
import { runStatusFromActivityEvent } from '../activityRunStatus';
import { confirmActivity, confirmActivityGrant, submitClarification } from '../api';
import { useChatRunStatus } from './useChatRunStatus';
import { useChatStreamManager } from './useChatStreamManager';
import { useChatInboundSync } from './useChatInboundSync';
import { useChatSender } from './useChatSender';
import { useFollowUpQueue } from './useFollowUpQueue';
import { useAwaitReply } from './useAwaitReply';
import { formatUserActionMessage, type A2UIUserActionPayload } from '../a2uiUserAction';
import { clearChatMarkdownCache } from '../chatMessageMarkdown';
import { getProviderModelValue, sessionToView } from './chatWorkspaceUtils';
import { useChatSidebarOrder } from './useChatSidebarOrder';
import { useChatAttachments } from './useChatAttachments';
import { fileAcceptForModel, modelSupportsFileInput, type ChatModelDescriptor } from '../modelCapabilities';
import { useChatProviderOptions } from './useChatProviderOptions';
import { useChatDeleteFlow } from './useChatDeleteFlow';
import { createChatFocusCoordinator } from '../chatFocusCoordinator';
import { useChatEntityNav } from './useChatEntityNav';
import { useChatTraceDialog, useChatSessionArtifacts } from './useChatTraceAndArtifacts';
import { useChatSettingsDialog } from './useChatSettingsDialog';
import { useChatDialogs } from './useChatDialogs';
import { useChatComposerActions } from './useChatComposerActions';
import { favoriteSessionIDs, toggleFavoriteSession, emitSessionMutation } from '../../../stores/sessionSync';
import { agentNeedsSettingsHydration, hydrateAgentSettings } from '../agentPlannerSettings';
import { parseChannelSessionMeta } from '../channelSessionMeta';

import { useArtifactStore } from '../../../stores/artifact';
import type { ComposerUsageSnapshot } from '../composerUsageMetrics';
import { useContextBreakdown } from './useContextBreakdown';
import { mapPreviewToReport, type PromptPreviewReport } from '../contextBreakdown';
import { useAgentDetailStore } from '../../../stores/agents/detail';
import type { AgentPromptPreview } from '../../agents/types';
import { useReasoningSidebar } from './useReasoningSidebar';
import { useContextualLoadingMessage } from './useContextualLoadingMessage';
import { useStatusPulse } from './useStatusPulse';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useLlmRetryStore } from '../../../stores/chat/llmRetryStore';
import { useChatEventRouter } from './useChatEventRouter';
import type { V2WsEnvelope, SystemNoticeEventPayload, RunStatusEventPayload } from '../v2Types';
import { noteChannelWsEnvelope } from '../channelWsCursor';
import { useSessionTree } from './useSessionTree';
import { SESSION_RUN_STATUS } from '../sessionRunStatus';
import { runStatusFromV2Payload } from '../activityRunStatus';

/**
 * Phase B-3: Resolve which session ID should be active given the current
 * spirit panel mode. Returns null if the session cannot be resolved yet
 * (e.g. member mode but tree not loaded). Pure function — testable in
 * isolation without mounting useChatWorkspace.
 */
export function resolvePanelSessionId(args: {
  mode: SpiritPanelMode;
  spiritSessionId: string | null;
  activeTeamSessionId: string | null;
  activeMemberAgentKey: string | null;
  findMemberSessionId: (spiritSessionId: string, agentKey: string, teamSessionId?: string | null) => string | null;
}): string | null {
  const { mode, spiritSessionId, activeTeamSessionId, activeMemberAgentKey, findMemberSessionId } = args;
  if (mode === 'spirit') return spiritSessionId;
  if (mode === 'team') return activeTeamSessionId;
  if (mode === 'member') {
    if (!spiritSessionId || !activeMemberAgentKey) return null;
    return findMemberSessionId(spiritSessionId, activeMemberAgentKey, activeTeamSessionId);
  }
  return null;
}

export function useChatWorkspace() {
  const { t } = useI18n();
  const $q = useQuasar();
  const route = useRoute();
  const appStore = useAppStore();
  const sessionStore = useChatSessionStore();
  const messageStore = useChatMessageStore();
  const spiritStore = useSpiritTeamStore();
  const runtimeStore = useChatRuntimeStore();
  const conversationStore = useChatConversationStore();
  const agentDetailStore = useAgentDetailStore();

  const isDark = computed(() => $q.dark.isActive);
  const leftOpen = ref(true);
  const rightOpen = ref(true);
  const search = ref('');
  const defaultAgentId = ref<string | null>(null);
  const defaultTeamId = ref('team-default-1');

  const displayAgents = ref<Agent[]>([]);
  const displayTeams = ref<TeamRow[]>([]);
  const inputText = ref('');
  const sessionDrafts = reactive(new Map<string, string>());

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

  // Phase 2 v2: Activity store + event router for the new v2 event pipeline.
  const activityStore = useChatActivityStore();
  const eventRouter = useChatEventRouter(activityStore);
  const llmRetryStore = useLlmRetryStore();

  // v2 WS event handler — dispatched by the stream manager's onV2Event callback.
  const handleV2Event = (envelope: V2WsEnvelope) => {
    // B-06: advance WS cursor from durable outbox event_id when present.
    const cursorEventId = String(envelope.event_id ?? '').trim();
    const cursorSessionId = String(envelope.session_id ?? '').trim();
    if (cursorEventId && cursorSessionId) {
      noteChannelWsEnvelope(cursorSessionId, cursorEventId);
    }
    // Phase 3b-D Task 12: intercept v2 system-domain events for side-effect
    // routing. Entity events (task/turn/step/team_stage/etc.) go to the v2
    // event router → activityV2Store. System events need side-effect routing
    // (metrics refresh, spirit store updates, run-status application) that the
    // store cannot provide.
    if (envelope.kind === 'system.notice') {
      handleV2SystemNotice(envelope.payload as SystemNoticeEventPayload);
      return;
    }
    if (envelope.kind === 'system.run_status') {
      handleV2RunStatus(envelope.payload as RunStatusEventPayload);
      return;
    }
    if (envelope.kind === 'system.heartbeat') {
      // Acknowledged but no side-effect routing needed yet.
      // system.heartbeat carries progress metadata for a future heartbeat display.
      return;
    }
    // Route team/member v2 events to spirit store for left sidebar updates.
    // The v2 event router (below) also receives these for activityV2Store
    // rendering — both paths run in parallel.
    routeTeamEventToSpiritStore(envelope);
    eventRouter.dispatch(envelope);

    // LLM retry banner lifecycle: any sign of stream progress (tokens flowing
    // again, a new turn starting, or a terminal step/turn/task event) means
    // the retry loop ended — clear the transient "reconnecting" state.
    switch (envelope.kind) {
      case 'step.streaming':
      case 'step.completed':
      case 'step.failed':
      case 'turn.started':
      case 'turn.completed':
      case 'turn.failed':
      case 'task.completed':
      case 'task.failed': {
        const sid = String(envelope.session_id ?? '').trim() || selectedSessionForUi.value?.id;
        if (sid) llmRetryStore.clear(sid);
        break;
      }
    }

    // Robust fallback: when the backend completes a turn but the terminal
    // run_status is missing/late, task.completed/failed still resets sending
    // so the composer does not stay stuck on "停止生成".
    if ((envelope.kind === 'task.completed' || envelope.kind === 'task.failed') && sender.sending.value) {
      sender.markSendingDone();
    }
  };

  /**
   * Route v2 team_stage and member_session events to the spirit store so the
   * left sidebar agent list updates in real-time. Without this, the spirit
   * store only refreshes on session switch (loadSpiritTeams API), leaving the
   * agent list empty after team assembly and member statuses stale.
   */
  function routeTeamEventToSpiritStore(envelope: V2WsEnvelope) {
    const p = envelope.payload as unknown as Record<string, unknown>;
    switch (envelope.kind) {
      case 'team_stage.created':
      case 'team_stage.updated':
      case 'team_stage.completed':
      case 'team_stage.failed':
        if (p.TeamStage) spiritStore.upsertTeamFromV2TeamStage(p.TeamStage as never);
        break;
      case 'member_session.created':
      case 'member_session.updated':
        if (p.MemberSession) spiritStore.updateMemberFromV2MemberSession(p.MemberSession as never);
        break;
      default:
        break;
    }
  }

  // Phase B-2: per-spirit-session recursive tree cache for SessionTreeSidebar.
  const sessionTree = useSessionTree();

  const runStatusCtrl = useChatRunStatus({ applyAwaitRunStatus });
  const { runStatus, runMeta, applyFromV2RunStatus, onSessionSwitch, refreshRunStatus, forceSetRunStatus } =
    runStatusCtrl;

  const providerOpts = useChatProviderOptions(appStore);
  const {
    dialogMode,
    modelProvider,
    modeOpts,
    provOpts,
    providerModels,
    loadChatOptions,
    onModeChange,
    onProviderChange,
  } = providerOpts;

  const selectedProviderModel = computed(() =>
    providerModels.value.find((row) => getProviderModelValue(row) === modelProvider.value),
  );

  const fileSupported = computed(() => {
    const m = selectedProviderModel.value;
    if (!m) return true;
    return modelSupportsFileInput({
      provider: m.provider,
      model: m.model,
      capabilities: m.capabilities as ChatModelDescriptor['capabilities'],
    });
  });

  const fileAccept = computed(() => {
    const m = selectedProviderModel.value;
    if (!m) return '';
    return fileAcceptForModel({
      provider: m.provider,
      model: m.model,
      capabilities: m.capabilities as ChatModelDescriptor['capabilities'],
    });
  });

  const activePlannerKind = computed(() => (appStore.selectedAgent?.settings?.planner_kind ?? '').trim().toLowerCase());

  const displaySessions = computed((): SessionView[] => {
    if (sessionStore.entityKind === 'team' && sessionStore.selectedTeamId) {
      return (sessionStore.teamSessions[sessionStore.selectedTeamId] ?? []).map((session) => sessionToView(session, t));
    }
    if (sessionStore.entityKind === 'agent' && appStore.selectedAgent) {
      return sessionStore.sessions.map((session) => sessionToView(session, t));
    }
    return [];
  });

  const selectedSessionForUi = computed((): SessionView | null => {
    if (sessionStore.entityKind === 'team' && sessionStore.teamSelectedSessionId) {
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

  const promptPreviewData = ref<AgentPromptPreview | null>(null);

  const promptPreviewRef = computed<PromptPreviewReport | null>(() =>
    promptPreviewData.value ? mapPreviewToReport(promptPreviewData.value) : null,
  );

  async function loadPromptPreviewForAgent(agentId: string) {
    try {
      promptPreviewData.value = await agentDetailStore.fetchPromptPreview(agentId);
    } catch {
      promptPreviewData.value = null;
    }
  }

  const contextBreakdown = useContextBreakdown({
    usageSnapshot: composerUsageSnapshot,
    promptPreview: promptPreviewRef,
    toolCallCount: computed(() => selectedSessionForUi.value?.tool_call_count ?? 0),
    messageCount: computed(() => selectedSessionForUi.value?.message_count ?? 0),
  });

  const displayMessages = computed(() => {
    const sid = sessionStore.currentSessionId();
    return sid ? messageStore.getMessages(sid) : [];
  });

  const sessionIdForPending = computed(() => selectedSessionForUi.value?.id);
  const sessionIdForArtifacts = computed(() => selectedSessionForUi.value?.id);
  const selectedSessionId = computed(() => selectedSessionForUi.value?.id);

  const reasoningSidebar = useReasoningSidebar({
    messages: displayMessages,
    sessionId: selectedSessionId,
  });
  const jobsRefreshNonce = ref(0);
  const inboundHydrateError = ref('');
  const sessionLoading = ref(false);
  const coreReady = ref(false);
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

  const streamManager = useChatStreamManager({
    runtimeStore,
    onV2Event: handleV2Event,
    refreshRunStatus: refreshRunStatusForUi,
    // B-06: after WS reconnect, re-fetch authoritative v2 entity snapshot.
    // Server also replays missed critical outbox frames via last_event_id;
    // REST hydrate remains the safety net for non-critical / full state.
    onReconnectHydrate: async (sessionId) => {
      await activityStore.fetchSessionHistory(sessionId);
    },
  });

  const contextualLoading = useContextualLoadingMessage(streamManager.wsReplaying);
  const statusPulse = useStatusPulse(streamManager.wsReplaying);

  // D1: Load spirit teams whenever a session is selected.
  // Teams are bound to the spirit session (session.id), not to the currently
  // selected agent in the UI. Previously this only loaded teams when
  // agentKey === '__spirit__', which caused the sidebar to clear when the
  // user switched to a team/member view — even though those teams still
  // belong to the same spirit session. The store's loadSpiritTeams handles
  // session switching (clears old teams) and empty results (no-op), so it's
  // safe to call unconditionally.
  watch(
    () => sessionStore.selectedSession?.id,
    (sessionId) => {
      if (!sessionId) return;
      spiritStore.loadSpiritTeams(sessionId);
    },
    { immediate: true },
  );

  // D1: Reload teams after WS reconnect
  watch(streamManager.wsReplaying, (replaying, wasReplaying) => {
    if (wasReplaying && !replaying && spiritStore.currentSpiritSessionId) {
      spiritStore.reloadTeams();
    }
  });

  const selectedAgentId = computed(() => appStore.selectedAgent?.id);

  watch(
    () => ({
      entityKind: sessionStore.entityKind,
      agent: appStore.selectedAgent,
      teamId: sessionStore.selectedTeamId,
      team: displayTeams.value.find((team) => team.id === sessionStore.selectedTeamId),
    }),
    ({ entityKind, agent, teamId, team }) => {
      if (entityKind === 'team' && teamId) {
        conversationStore.setCurrentTarget({
          type: 'team',
          id: teamId,
          name: team?.display_name,
          source: 'web',
        });
        return;
      }
      if (agent?.id) {
        conversationStore.setCurrentTarget({
          type: 'agent',
          id: agent.id,
          key: agent.agent_key,
          name: agent.display_name,
          source: 'web',
        });
        return;
      }
      conversationStore.setCurrentTarget(null);
    },
    { immediate: true },
  );

  watch(
    selectedAgentId,
    async (id) => {
      if (id) {
        void loadPromptPreviewForAgent(id);
        if (sessionStore.entityKind === 'agent') {
          const agent = appStore.selectedAgent;
          if (agent) {
            if (agentNeedsSettingsHydration(agent)) {
              const hydrated = await hydrateAgentSettings(agent);
              appStore.selectedAgent = hydrated;
              appStore.upsertAgent(hydrated);
            }
            await sessionStore.loadAgentSessions(id);
            if (!sessionStore.selectedSession) {
              sessionStore.selectedSession = sessionStore.sessions[0] ?? null;
            }
          }
        }
      } else {
        promptPreviewData.value = null;
      }
    },
    { immediate: true },
  );

  async function refreshRunStatusForUi() {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) {
      isAwaitingUser.value = false;
      awaitingRunId.value = '';
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
    notifyError: (message) => $q.notify({ type: 'negative', message }),
  });

  async function onMessageFeedback(payload: { messageId: string; rating: 'positive' | 'negative' }) {
    const sid = sessionStore.currentSessionId();
    if (!sid) return;
    try {
      await runtimeStore.submitFeedback({
        session_id: sid,
        message_id: payload.messageId,
        rating: payload.rating,
      });
      $q.notify({ type: 'positive', message: t('chat.feedbackThanks', '感谢反馈') });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('chat.feedbackFailed', '反馈提交失败'),
      });
    }
  }

  function makeSessionTitle(content: string) {
    const plain = content
      .replace(/[#>*_`~[\]()]/g, '')
      .replace(/\s+/g, ' ')
      .trim();
    if (!plain) return t('chat.untitledSession');
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

  // Channel focus / session list refresh for inbound; chat timeline is v2-only.
  useChatInboundSync({
    appStore,
    sessionStore,
    messageStore,
    spiritStore,
    selectedAgentId,
    selectedSessionId,
    wsReplaying: streamManager.wsReplaying,
    onSpiritActivityEvent: contextualLoading.onSpiritActivityEvent,
    isChatRoute: () => route.name === 'chat',
    shouldAutoFocusChannel: () => {
      // Default OFF: channel inbound messages no longer auto-focus the session
      // to avoid interrupting the user's current workflow. Users can opt in
      // via localStorage channel_auto_focus=true.
      if (typeof localStorage !== 'undefined' && localStorage.getItem('channel_auto_focus') === 'true') {
        return !inputText.value.trim();
      }
      return false;
    },
    focusChannelSession: (sessionId, agentId, options) => entityNav.focusAgentSessionView(sessionId, agentId, options),
    onTurnComplete: () => {
      jobsRefreshNonce.value += 1;
      inboundHydrateError.value = '';
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

  const pendingMsgRef = { fn: undefined as (() => Promise<void>) | undefined };

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
    ensureChatStream: streamManager.ensureChatStream,
    ensureTeamStream: streamManager.ensureTeamStream,
    sendChatViaWs: streamManager.sendChatViaWs,
    onNewSession: (title?: string) => entityNav.onNewSession(title),
    makeSessionTitle,
    refreshRunStatus: refreshRunStatusForUi,
    setRunStatus: forceSetRunStatus,
    submitAwaitingReply: awaitSubmit.submitAwaitingReply,
    submitToolConfirm: awaitSubmit.submitToolConfirm,
    refreshPendingMessages: () => pendingMsgRef.fn?.() ?? Promise.resolve(),
  });

  const followUp = useFollowUpQueue(sessionIdForPending, sender.sending, (message) =>
    $q.notify({ type: 'negative', message }),
  );
  const { pendingMessages, refreshPendingMessages, onCancelPending, onInterruptPending, onUpdatePending } = followUp;
  pendingMsgRef.fn = refreshPendingMessages;

  watch(sender.sending, (val) => followUp.watchSending(val));

  /**
   * Apply run status from a v2 system.run_status payload (follow-up queue,
   * runStatus ref, tool-message cancellation on terminal statuses).
   */
  function applyV2RunStatusSideEffects(payload: RunStatusEventPayload) {
    followUp.onRunStatusV2(payload);
    applyFromV2RunStatus(payload);
    const rs = runStatusFromV2Payload(payload);
    if (rs?.status === 'cancelled' || rs?.status === 'failed') {
      const sid = selectedSessionForUi.value?.id;
      if (sid) {
        messageStore.setMessages(sid, cancelRunningToolMessages(messageStore.getMessages(sid)));
      }
    }
  }

  /**
   * Route v2 system.notice events to side-effects without AF conversion.
   */
  function handleV2SystemNotice(payload: SystemNoticeEventPayload) {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    const noticeType = String(payload.NoticeType ?? '').trim();
    if (!noticeType) return;
    const meta: Record<string, unknown> = {
      ...(payload.Meta ?? {}),
      notice_type: noticeType,
      message: payload.Message,
    };

    if (noticeType === 'metrics_updated') {
      sessionStore.fetchAndReconcileSession(sid);
    }
    if (noticeType === 'session_status_changed') {
      const status = typeof meta.status === 'string' ? meta.status : '';
      const statusReason = typeof meta.status_reason === 'string' ? meta.status_reason : '';
      const statusChangedAt = typeof meta.status_changed_at === 'string' ? meta.status_changed_at : '';
      if (status) {
        emitSessionMutation({ type: 'status_changed', id: sid, status, statusReason, statusChangedAt });
      }
    }
    // Transient reconnect signal from the provider retry transport — surfaced
    // as a dedicated banner (not an activity node) and cleared on stream resume.
    if (noticeType === 'llm_retry') {
      llmRetryStore.noteRetry(sid, meta);
    }
    spiritStore.handleSystemNotice(noticeType, meta);
    contextualLoading.onSpiritNoticeType(noticeType);
  }

  /**
   * Route v2 system.run_status events to side-effects without AF conversion.
   */
  function handleV2RunStatus(payload: RunStatusEventPayload) {
    // Patch MemberSession card when pause/resume publishes chat_session_id
    // (MemberSessionUpdatedEvent is the primary path; this covers race/late WS).
    const chatSessionId = String(payload.Meta?.chat_session_id ?? '').trim();
    const status = String(payload.Status ?? payload.Meta?.status ?? '').trim();
    if (chatSessionId && (status === 'paused' || status === 'running')) {
      for (const ms of activityStore.memberSessions.values()) {
        if (ms.SessionID === chatSessionId && ms.Status !== status) {
          activityStore.upsertMemberSession({ ...ms, Status: status as typeof ms.Status });
          break;
        }
      }
    }
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    applyV2RunStatusSideEffects(payload);
    // Terminal run statuses end any in-flight retry loop — clear the banner.
    const runStatus = String(payload.Status ?? payload.Meta?.status ?? '').trim();
    if (
      runStatus === SESSION_RUN_STATUS.COMPLETED ||
      runStatus === SESSION_RUN_STATUS.FAILED ||
      runStatus === SESSION_RUN_STATUS.CANCELLED ||
      runStatus === SESSION_RUN_STATUS.IDLE
    ) {
      llmRetryStore.clear(sid);
    }
    const rs = runStatusFromV2Payload(payload);
    if (rs?.status === SESSION_RUN_STATUS.RUNNING) {
      if (sessionStore.entityKind === 'team') {
        streamManager.ensureTeamStream(sid);
      } else {
        streamManager.ensureChatStream(sid);
      }
    }
  }

  const { loadAgentOrder, loadTeamOrder, onEndAgent, onEndTeam, onGroupReorder } = useChatSidebarOrder(
    displayAgents,
    displayTeams,
    defaultAgentId,
    defaultTeamId,
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
  const { settingsOpen, settingsMode, settingsId, editName, onSaveSettings: saveSettingsDialog } = settingsDialog;

  const traceAndArtifacts = useChatTraceDialog(toRef(sessionStore, 'entityKind'), displaySessions);
  const {
    traceOpen,
    traceSessionId,
    traceSessionTitle,
    traceInitialTab,
    traceStreamDeps,
    openSessionTrace,
    timeline: traceTimeline,
    timelineLoading: traceTimelineLoading,
    timelineError: traceTimelineError,
    reloadTimeline: reloadTraceTimeline,
  } = traceAndArtifacts;

  const {
    sessionArtifacts,
    sessionArtifactsLoading,
    openSessionArtifact,
    onArtifactDeleted: removeArtifactFromList,
  } = useChatSessionArtifacts(sessionIdForArtifacts);

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

  const composerActions = useChatComposerActions({
    sessionStore,
    messageStore,
    runtimeStore,
    streamManager,
    sender,
    runStatus,
    selectedSessionId: computed(() => selectedSessionForUi.value?.id),
    notify: (opts) => $q.notify(opts),
    t,
    sessionDrafts,
  });

  const isRunnerActive = computed(
    () => runStatus.value === 'running' || runStatus.value === 'pending' || sender.sending.value,
  );

  // Refresh background jobs when run transitions to idle (tasks may complete after turn ends)
  watch(runStatus, (newVal, oldVal) => {
    if (newVal === 'idle' && oldVal !== 'idle') {
      jobsRefreshNonce.value += 1;
    }
    // Bugfix P1#4: WS send success path doesn't call markSendingDone() —
    // sending.value stays true after the run ends. Reset sending whenever
    // runStatus reaches a terminal state (idle/completed/failed/cancelled).
    // awaiting_user is NOT terminal — the user still needs to reply.
    if (
      sender.sending.value &&
      (newVal === 'idle' || newVal === 'completed' || newVal === 'failed' || newVal === 'cancelled')
    ) {
      sender.markSendingDone();
    }
  });

  // ── Long-running stall detection ──
  // When run is in 'running' status but no events arrive for 5 minutes,
  // prompt the user with a "seems stuck, stop?" notification.
  const STALL_NOTIFY_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes
  let stallNotifyTimer: ReturnType<typeof setTimeout> | null = null;
  let stallNotified = false;

  function clearStallNotifyTimer() {
    if (stallNotifyTimer != null) {
      clearTimeout(stallNotifyTimer);
      stallNotifyTimer = null;
    }
  }

  function resetStallNotifyTimer() {
    clearStallNotifyTimer();
    stallNotified = false;
    if (runStatus.value === 'running') {
      stallNotifyTimer = setTimeout(() => {
        stallNotified = true;
        $q.notify({
          type: 'warning',
          message: t('chat.runLongStallWarning', '似乎没有进展，是否停止？'),
          actions: [
            {
              label: t('chat.stop', '停止'),
              color: 'negative',
              handler: () => stopStreaming(),
            },
            {
              label: t('chat.wait', '继续等待'),
              color: 'grey',
              handler: () => {},
            },
          ],
          timeout: 15_000,
        });
      }, STALL_NOTIFY_TIMEOUT_MS);
    }
  }

  // Start/stop stall timer based on runStatus
  watch(runStatus, (newVal) => {
    if (newVal === 'running') {
      resetStallNotifyTimer();
    } else {
      clearStallNotifyTimer();
      stallNotified = false;
    }
  });

  // Reset stall timer whenever we receive a run activity event
  const origTouchRunActivity = sender.touchRunActivity.bind(sender);
  sender.touchRunActivity = () => {
    origTouchRunActivity();
    if (stallNotified) {
      resetStallNotifyTimer();
    } else if (runStatus.value === 'running' && stallNotifyTimer != null) {
      resetStallNotifyTimer();
    }
  };

  const sessionRevision = computed(() => {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return null;
    return messageStore.sessionRevisionBySession[sid] ?? 0;
  });

  async function submitA2UIUserAction(payload: A2UIUserActionPayload) {
    if (sessionStore.entityKind !== 'agent') {
      $q.notify({ type: 'warning', message: 'A2UI 交互仅支持 Agent 会话' });
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
      const res = await runtimeStore.enqueue(sid, content.trim());
      if (res.accepted) {
        // Clear input only after successful enqueue
        inputText.value = '';
        $q.notify({
          type: 'positive',
          message: res.queued
            ? t('chat.enqueueQueued', 'Message queued for after the current run')
            : t('chat.enqueueAccepted', 'Message will be injected at the next tool boundary'),
        });
        await refreshPendingMessages();
        return;
      }
      $q.notify({ type: 'warning', message: t('chat.enqueueRejected', 'Could not enqueue message') });
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('chat.enqueueRejected', 'Could not enqueue message'),
      });
    }
  }

  async function openSettings(kind: Parameters<typeof entityNav.openSettings>[0], id: string) {
    if (kind === 'agent' || kind === 'team') {
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
    $q.notify({ type: 'info', message: t('chat.voicePlaceholder') });
  }

  function onPasteUnsupported() {
    $q.notify({ type: 'warning', message: t('chat.clipboardFileUnsupported', '当前模型不支持此类型的文件粘贴') });
  }

  async function onCompactSession(sessionId: string) {
    try {
      const result = await sessionStore.compactSessionAction(sessionId);
      if (result.compacted) {
        const before = Math.round((result.estimated_tokens_before / 1000) * 10) / 10;
        const after = Math.round((result.estimated_tokens_after / 1000) * 10) / 10;
        $q.notify({
          type: 'positive',
          message: t('chat.contextManuallyCompressed', `上下文已压缩 (${before}k → ${after}k tokens)`),
          timeout: 4000,
        });
      } else {
        $q.notify({
          type: 'info',
          message: t('chat.contextNoCompactionNeeded', '当前上下文无需压缩'),
          timeout: 3000,
        });
      }
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e);
      $q.notify({ type: 'negative', message: t('chat.contextCompactFailed', '压缩失败') + `: ${msg}`, timeout: 5000 });
    }
  }

  // N-14: Handle confirm-activity event from ConfirmBlock → API call.
  // Encapsulated here (rather than in ChatPage.vue) to comply with FD2:
  // Page must not import API directly.
  async function onConfirmActivity(activityId: string, approved: boolean) {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    try {
      const ok = await confirmActivity(sid, activityId, approved);
      if (!ok) {
        $q.notify({
          type: 'warning',
          message: approved ? t('chat.confirmActivity.approveRejected') : t('chat.confirmActivity.denyRejected'),
        });
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.confirmActivity.failed') });
    }
  }

  async function onConfirmActivityGrant(payload: { sessionId: string; activityId: string; reply: string }) {
    try {
      const ok = await confirmActivityGrant(payload);
      if (!ok) {
        $q.notify({
          type: 'warning',
          message: t('chat.confirmActivity.approveRejected'),
        });
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.confirmActivity.failed') });
    }
  }

  // Clarification Gate (B.10.18): Handle submit-clarification event from
  // ClarifyBlock → API call. The backend flips the step to completed and
  // resumes the turn; the WS step.updated event drives the card's summary view.
  async function onSubmitClarification(payload: SubmitClarificationPayload) {
    try {
      const ok = await submitClarification(payload);
      if (!ok) {
        $q.notify({
          type: 'warning',
          message: t('chat.clarify.submitRejected'),
        });
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.clarify.submitFailed') });
    }
  }

  /**
   * L3: Resume an interrupted task (server-restart recovery).
   *
   * Sends a `resume_task` WS upstream on the task's chat stream. The backend
   * CAS-claims the task (interrupted → running) and reruns it with the
   * persisted execution trace; the resulting `task.updated` event drives the
   * UI back to the running state — no optimistic local update needed.
   * Failures surface as a ws_error notice from the backend.
   */
  function resumeTask(task: { ID: string; SessionID: string }) {
    const sid = task.SessionID || selectedSessionForUi.value?.id;
    if (!sid || !task.ID) return;
    try {
      const stream = streamManager.ensureChatStream(sid);
      streamManager.sendChatViaWs(stream, {
        direction: 'client_to_server',
        channel: 'chat',
        type: 'resume_task',
        payload: { task_id: task.ID },
      });
      $q.notify({ type: 'info', message: t('chat.v2.resumeTaskSent'), timeout: 1500 });
    } catch (err) {
      console.warn('[chat] resume_task send failed', err);
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('chat.sendFailed') });
    }
  }

  /**
   * P3-4: ErrorBlock inline action handlers.
   *
   * ErrorBlock emits typed actions based on the resolved `errorCode`. The
   * handlers below wire those actions to existing composer/session methods
   * or surface a guidance notification when no direct action is available.
   *
   * The `event` payload carries the original `ErrorEvent` (message + errorCode)
   * so handlers can log diagnostics or correlate with the failed turn.
   */

  /** Retry: find the latest failed pending-user message and re-send it. */
  async function onErrorRetry() {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    const failed = pendingMessages.value.find((m) => m.id.startsWith('pending-user-') && m.status === 'failed');
    if (failed) {
      await sender.retryFailedMessage(failed.id);
    } else {
      $q.notify({ type: 'info', message: t('chat.errorBlock.retryNoTarget') });
    }
  }

  /** Switch model: prompt the user to switch model via the header provider selector. */
  function onErrorSwitchModel() {
    $q.notify({ type: 'info', message: t('chat.errorBlock.switchModelHint', '请在顶部切换到其他模型后重试') });
  }

  /** Rephrase: focus the composer so the user can edit and re-send. */
  function onErrorRephrase() {
    $q.notify({ type: 'info', message: t('chat.errorBlock.rephraseHint') });
  }

  /** Check config: open the agent settings dialog. */
  function onErrorCheckConfig() {
    const agent = appStore.selectedAgent;
    if (agent) {
      void openSettings('agent', agent.id);
    } else {
      $q.notify({ type: 'warning', message: t('chat.errorBlock.checkConfigHint') });
    }
  }

  /** Remove attachment: notify the user to remove the offending attachment. */
  function onErrorRemoveAttachment() {
    $q.notify({ type: 'info', message: t('chat.errorBlock.removeAttachmentHint') });
  }

  // --- Compress status polling ---
  const compressStatus = computed<CompressStatus>(() => sessionStore.compressStatus);
  let compressPollTimer: ReturnType<typeof setInterval> | null = null;
  let compressNormalSince: number | null = null;
  const COMPRESS_POLL_INTERVAL_MS = 5_000;
  const COMPRESS_NORMAL_COOLDOWN_MS = 10_000;

  async function pollCompressStatus() {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) {
      // No session selected (e.g. navigated away from chat): stop the timer
      // instead of spinning a no-op interval forever.
      stopCompressPolling();
      return;
    }
    await sessionStore.fetchCompressStatus(sid);
    // The stop-condition must be evaluated here, after every poll, rather
    // than in a watch on compressStatus: a watch only fires on value change,
    // so a steady 'normal' never re-triggers it and the cooldown check below
    // would never run again — polling continued forever (5s interval kept the
    // network busy, which starved browser-tool readiness on the chat page).
    if (sessionStore.compressStatus === 'normal') {
      if (!compressNormalSince) {
        compressNormalSince = Date.now();
      }
      if (Date.now() - compressNormalSince >= COMPRESS_NORMAL_COOLDOWN_MS) {
        stopCompressPolling();
      }
    } else {
      compressNormalSince = null;
    }
  }

  function startCompressPolling() {
    stopCompressPolling();
    void pollCompressStatus();
    compressPollTimer = setInterval(() => {
      void pollCompressStatus();
    }, COMPRESS_POLL_INTERVAL_MS);
  }

  function stopCompressPolling() {
    if (compressPollTimer) {
      clearInterval(compressPollTimer);
      compressPollTimer = null;
    }
    compressNormalSince = null;
  }

  // Restart polling if the status ever flips to non-normal while the timer
  // is stopped (defensive: today only pollCompressStatus updates the status,
  // but a future WS push path would need this to resume polling).
  watch(compressStatus, (status) => {
    if (status !== 'normal' && !compressPollTimer) {
      startCompressPolling();
    }
  });

  let visibleRefreshTimer: ReturnType<typeof setTimeout> | null = null;

  async function bindSessionView(sessionId: string, replace = true, streamKind?: 'chat' | 'team') {
    if (streamKind === 'team' || (!streamKind && sessionStore.entityKind === 'team')) {
      streamManager.ensureTeamStream(sessionId);
    } else {
      streamManager.ensureChatStream(sessionId);
    }
    sessionLoading.value = true;
    try {
      if (replace) clearChatMarkdownCache();

      // B-06: hydrate Activity v2 via REST authoritative snapshot.
      // WS last_event_id drives critical outbox replay on connect; reconnect
      // also calls fetchSessionHistory via onReconnectHydrate.
      const v2Promise = activityStore.fetchSessionHistory(sessionId).catch((e) => {
        console.warn('[chat] v2 history fetch failed', e);
      });
      await messageStore.loadMessages(replace ? { sessionId, replace: true } : { sessionId });
      await v2Promise;
    } catch (err) {
      $q.notify({
        type: 'negative',
        message: err instanceof Error ? err.message : t('chat.loadMessagesFailed'),
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
        awaitingRunId.value = '';
        clearAwaitMeta();
        inputText.value = '';
        sender.clearFailedPendingForSession(prevSid);
        return;
      }
      inputText.value = sessionDrafts.get(sid) || '';
      onSessionSwitch(sid);
      sessionStore.resetCompressStatus();
      stopCompressPolling();
      startCompressPolling();
      if (sid !== prevSid) {
        sender.clearFailedPendingForSession(prevSid);
        // Bugfix P1#5: previous run's sending state must not leak into the
        // newly-selected session. onSessionSwitch only resets runStatus via
        // HTTP hydrate (delayed); sending.value is left untouched, leaving
        // the composer stuck in "stop" mode for the new session.
        sender.markSendingDone();
        void bindSessionView(sid, true);
        // Phase B-2: preload session tree for sidebar.
        void sessionTree.loadTreeFor(sid).catch(() => {
          /* swallow; sidebar shows error state */
        });
      }
    },
    { immediate: true },
  );

  // Phase B-3: Resolve spirit panel-mode → target session id and bind it.
  // - spirit mode: target = selectedSessionForUi.id
  // - team mode:   target = activeTeam.teamSessionId
  // - member mode: resolve via sessionTree.findMemberSessionId
  watch(
    () => spiritStore.activePanelMode,
    async (mode) => {
      const spiritSessionId = selectedSessionForUi.value?.id ?? null;
      const teamSessionId = spiritStore.activeTeam?.teamSessionId ?? null;
      const activeMemberAgentKey = (() => {
        const memberId = spiritStore.activeMemberId;
        if (!memberId) return null;
        return spiritStore.activeTeam?.members.find((m) => m.agentId === memberId)?.agentKey ?? null;
      })();
      // For member mode, ensure the tree is loaded before resolving.
      if (mode === 'member' && spiritSessionId && activeMemberAgentKey) {
        await sessionTree.loadTreeFor(spiritSessionId);
      }
      const targetSessionId = resolvePanelSessionId({
        mode,
        spiritSessionId,
        activeTeamSessionId: teamSessionId,
        activeMemberAgentKey,
        findMemberSessionId: sessionTree.findMemberSessionId,
      });
      if (targetSessionId) {
        const streamKind: 'chat' | 'team' = mode === 'team' ? 'team' : 'chat';
        void bindSessionView(targetSessionId, true, streamKind);
      }
    },
  );

  // Phase B-3: When user switches to a different member while already in
  // member mode (activePanelMode stays 'member' but activeMemberId changes).
  watch(
    () => spiritStore.activeMemberId,
    async (memberId) => {
      if (spiritStore.activePanelMode !== 'member' || !memberId) return;
      const spiritSessionId = selectedSessionForUi.value?.id ?? null;
      const agentKey = spiritStore.activeTeam?.members.find((m) => m.agentId === memberId)?.agentKey ?? null;
      if (!spiritSessionId || !agentKey) return;
      await sessionTree.loadTreeFor(spiritSessionId);
      const teamSessionId = spiritStore.activeTeam?.teamSessionId ?? null;
      const memberSessionId = sessionTree.findMemberSessionId(spiritSessionId, agentKey, teamSessionId);
      if (memberSessionId) {
        void bindSessionView(memberSessionId, true, 'chat');
      }
    },
  );

  function onPageVisible() {
    if (document.visibilityState !== 'visible') return;
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    const meta = parseChannelSessionMeta(
      selectedSessionForUi.value?.metadata_json ?? sessionStore.selectedSession?.metadata_json,
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
    clearStallNotifyTimer();
    stopCompressPolling();
    document.removeEventListener('visibilitychange', onPageVisible);
    streamManager.disconnectAll();
  });

  onMounted(async () => {
    document.addEventListener('visibilitychange', onPageVisible);

    await Promise.all([loadChatOptions(), appStore.loadAgents()]);

    const defaultAgent = appStore.agents.find((a) => a.agent_key === '__spirit__') || appStore.agents[0];
    defaultAgentId.value = defaultAgent?.id ?? null;
    displayAgents.value = loadAgentOrder(appStore.agents, defaultAgentId.value);
    coreReady.value = true;

    // Explicitly select the default agent to ensure full initialization
    // (load sessions, set URL, hydrate session, etc.). The watch on
    // selectedAgentId only loads sessions but skips URL sync and hydration.
    const routeSession = typeof route.query.session === 'string' ? route.query.session.trim() : '';
    const routeAgent = typeof route.query.agent === 'string' ? route.query.agent.trim() : '';
    if (routeSession && routeAgent) {
      void entityNav.focusSessionById(routeSession, routeAgent);
    } else if (appStore.selectedAgent) {
      void entityNav.selectAgent(appStore.selectedAgent, { sessionId: routeSession || undefined });
    }

    void Promise.all([entityNav.loadTaxonomyTree(), entityNav.loadTeams()]).then(() => {
      displayTeams.value = loadTeamOrder([...displayTeams.value], defaultTeamId.value);
      const routeTeamID = typeof route.query.team === 'string' ? route.query.team : '';
      const routeTeam = routeTeamID ? displayTeams.value.find((team) => team.id === routeTeamID) : undefined;
      if (routeTeam) {
        void entityNav.selectTeam(routeTeam);
      } else if (!appStore.agents[0] && displayTeams.value[0]) {
        void entityNav.selectTeam(displayTeams.value[0]!);
      }
    });
  });

  watch(
    () => ({ session: route.query.session, agent: route.query.agent }),
    ({ session: sid }) => {
      if (focusCoordinator.isRouteSessionWatchSuppressed()) return;
      if (typeof sid !== 'string' || !sid.trim()) return;
      const routeAgent = typeof route.query.agent === 'string' ? route.query.agent.trim() : '';
      void entityNav.focusSessionById(sid.trim(), routeAgent || undefined);
    },
  );

  return {
    coreReady,
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
      taxonomyTree: entityNav.taxonomyTree,
      activePlannerKind,
      onEndAgent,
      onEndTeam,
      onGroupReorder,
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
      contextBreakdown,
      displayMessages,
      sessionRevision,
      wsConnected: computed(() => {
        const sid = sessionStore.currentSessionId();
        return sid ? runtimeStore.isWsConnected(sid) : false;
      }),
      wsReplaying: streamManager.wsReplaying,
      spiritLoadingMessage: contextualLoading.loadingMessage,
      spiritPulseStates: statusPulse.pulseStates,
      spiritOnTeamStatusChanged: statusPulse.onTeamStatusChanged,
      jobsRefreshNonce,
      inboundHydrateError,
      sessionLoading,
      focusTurnId,
      focusSessionTurn,
      clearFocusTurn,
      reasoningSidebarOpen: reasoningSidebar.open,
      reasoningSidebarActive: reasoningSidebar.activeReasoning,
      toggleReasoningSidebar: reasoningSidebar.toggle,
      pinReasoningMessage: reasoningSidebar.pinMessage,
      unpinReasoning: reasoningSidebar.unpin,
      sessionArtifacts,
      sessionArtifactsLoading,
      fileSupported,
      fileAccept,
      openSessionArtifact,
      onArtifactDeleted,
      downloadArtifact: async (meta: import('../../artifact/types').ArtifactMeta) => {
        try {
          const artifactStore = useArtifactStore();
          const signed = await artifactStore.signDownload(meta.id, meta.version);
          window.open(artifactStore.artifactDownloadHref(signed.url), '_blank', 'noopener,noreferrer');
        } catch {
          $q.notify({ type: 'negative', message: t('chat.attachmentDownloadFailed') });
        }
      },
      onSelectSession: entityNav.onSelectSession,
      onRenameSession: entityNav.onRenameSession,
      onTogglePinSession: entityNav.onTogglePinSession,
      favoriteIds: computed(() => favoriteSessionIDs.value),
      onToggleFavorite: (id: string) => {
        toggleFavoriteSession(id);
      },
      onRestoreSession: entityNav.onRestoreSession,
      onArchiveSession: entityNav.onArchiveSession,
      onSessionDetail: entityNav.onSessionDetail,
      onNewSession: entityNav.onNewSession,
      openSessionTrace,
      openSessionEvents,
      compactSessionAction: sessionStore.compactSessionAction,
      onCompactSession,
      onConfirmActivity,
      onConfirmActivityGrant,
      onSubmitClarification,
      resumeTask,
      compressStatus,
      activityStore,
      v2Tasks: computed(() => activityStore.getSessionTasks(selectedSessionForUi.value?.id ?? '')),
      sessionTree,
    }),
    composer: reactive({
      inputText,
      dialogMode,
      modelProvider,
      modeOpts,
      provOpts,
      attachments,
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
      onSend: composerActions.onSend,
      submitA2UIUserAction,
      onModeChange,
      onProviderChange,
      stopStreaming,
      pickFile,
      onFileChange,
      uploadFile,
      removeAttachment,
      onCancelPending,
      onInterruptPending,
      onUpdatePending,
      onVoiceClick,
      onMessageFeedback,
      retryFailedMessage: sender.retryFailedMessage,
      dismissFailedMessage: composerActions.dismissFailedMessage,
      regenerateMessage: composerActions.regenerateMessage,
      regenerateV2Task: composerActions.regenerateV2Task,
      cancelBackgroundJob: composerActions.cancelBackgroundJob,
      onPasteUnsupported,
    }),
    dialogs: useChatDialogs({
      deleteFlow,
      settingsDialog,
      traceOpen,
      traceSessionId,
      traceSessionTitle,
      traceInitialTab,
      traceStreamDeps,
      timeline: traceTimeline,
      timelineLoading: traceTimelineLoading,
      timelineError: traceTimelineError,
      reloadTimeline: reloadTraceTimeline,
      selectedProviderModel,
      fileSupported,
      onSaveSettings,
    }),
    errorBlock: reactive({
      onErrorRetry,
      onErrorSwitchModel,
      onErrorRephrase,
      onErrorCheckConfig,
      onErrorRemoveAttachment,
    }),
  };
}
