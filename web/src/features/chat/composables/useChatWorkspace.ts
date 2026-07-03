import { computed, onMounted, onUnmounted, reactive, ref, toRef, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useRoute } from 'vue-router';
import type { SessionView, TeamRow } from '../../../components/chat/types';
import type { Agent } from '../../agents/types';
import type { CompressStatus } from '../../session/types';
import type { SpiritPanelMode } from '../../spirit/types';
import { useAppStore } from '../../../stores/app';
import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { useChatConversationStore } from '../../../stores/chat/conversationStore';
import { useSpiritTeamStore } from '../../../stores/spirit';
import { cancelRunningToolMessages } from '../activityToolCall';
import { runStatusFromActivityEvent } from '../activityRunStatus';
import { confirmActivity } from '../api';
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
import { favoriteSessionIDs, toggleFavoriteSession } from '../../../stores/sessionSync';
import { agentNeedsSettingsHydration, hydrateAgentSettings } from '../agentPlannerSettings';
import { parseChannelSessionMeta } from '../channelSessionMeta';
import { useKnowledgeStore } from '../../../stores/knowledge';
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
import { useChatEventRouter } from './useChatEventRouter';
import type {
  V2WsEnvelope,
  SystemNoticeEventPayload,
  RunStatusEventPayload,
  ActivityBridgeEventPayload,
} from '../v2Types';
import { useSessionTree } from './useSessionTree';
import { useSystemEventNotification } from './useSystemEventNotification';
import type { ActivityEvent as AFActivityEvent } from '../../../realtime/activityEvent';

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
  findMemberSessionId: (spiritSessionId: string, agentKey: string) => string | null;
}): string | null {
  const { mode, spiritSessionId, activeTeamSessionId, activeMemberAgentKey, findMemberSessionId } = args;
  if (mode === 'spirit') return spiritSessionId;
  if (mode === 'team') return activeTeamSessionId;
  if (mode === 'member') {
    if (!spiritSessionId || !activeMemberAgentKey) return null;
    return findMemberSessionId(spiritSessionId, activeMemberAgentKey);
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

  // Phase 2 v2: Activity store + event router for the new v2 event pipeline.
  const activityStore = useChatActivityStore();
  const eventRouter = useChatEventRouter(activityStore);

  // v2 WS event handler — dispatched by the stream manager's onV2Event callback.
  const handleV2Event = (envelope: V2WsEnvelope) => {
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
      // Phase 3b-D: backend PublishRunStatus/PublishRunStatusFull migrated to v2
      // (commit d5a52ea7e). Route to applyRunStatusFromActivityEvent (follow-up
      // queue, runStatus ref, tool-message cancellation) and inboundActivityEventHandler
      // (session refresh, channel focus, stream unseal).
      handleV2RunStatus(envelope.payload as RunStatusEventPayload);
      return;
    }
    if (envelope.kind === 'system.heartbeat') {
      // Acknowledged but no side-effect routing needed yet.
      // system.heartbeat carries progress metadata for a future heartbeat display.
      return;
    }
    if (envelope.kind === 'activity.bridge') {
      // Phase 3b-D: bridge wraps a v1 ActivityEvent. Route through the
      // existing v1 AF handler to reuse dedup + system-event routing
      // (useSystemEventNotification) + contextual loading + sender reset.
      // The wrapped Event retains all snake_case JSON tags from
      // biz.ActivityEvent, so it maps directly to AFActivityEvent.
      handleActivityEvent((envelope.payload as ActivityBridgeEventPayload).Event);
      return;
    }
    eventRouter.dispatch(envelope);
  };

  // Phase B-2: per-spirit-session recursive tree cache for SessionTreeSidebar.
  const sessionTree = useSessionTree();

  // AF-GAP-02: Bounded dedup set for ActivityEvent IDs. Both the real-time
  // WS stream path (ctx.onActivityEvent) and the inbound-sync path
  // (useChatInboundSync → deps.onActivityEvent) may forward the same
  // ActivityEvent for the current session. Without dedup, handleActivityEvent
  // would process the same event twice, duplicating content in the timeline.
  // The set is bounded (LRU eviction) to prevent unbounded memory growth.
  const ACTIVITY_DEDUP_LIMIT = 512;
  const activityDedupIds = new Set<string>();
  const activityDedupRing: string[] = [];

  // Forward reference to handleInboundActivityEvent from useChatInboundSync.
  // Set after useChatInboundSync returns (below). Used by
  // useSystemEventNotification to route system/control ActivityEvents
  // (run_status, error, team_*, graph_*, spirit_*) directly to the
  // ActivityEvent-based inbound pipeline — no Envelope conversion.
  let inboundActivityEventHandler: ((ev: AFActivityEvent) => void | Promise<void>) | null = null;

  // Phase 3 / ADR-03 D6: useSystemEventNotification handles Domain=system
  // ActivityEvents by routing them to the inbound-sync pipeline. Event
  // classification prefers `ev.domain`; falls back to kind/event heuristics
  // for older backends that don't set the domain field. Chat-rendering
  // events (task streaming/created/completed, thinking, action, reply,
  // confirm) bypass this handler and go straight to the Activity timeline.
  const systemEventNotification = useSystemEventNotification({
    applyRunStatusFromActivityEvent: (ev) => applyRunStatusFromActivityEvent(ev),
    getInboundActivityEventHandler: () => inboundActivityEventHandler,
  });

  // Activity-First (AF): Handler for the new business-semantic ActivityEvent
  // format (event + full Activity snapshot). This replaces the legacy
  // activity_start/delta/done/child_start envelopes for chat events.
  // Dedup is by activity.id + event type for non-streaming events (which
  // may arrive via both the real-time stream and the inbound-sync path).
  // Streaming events are NOT deduped — each carries a unique delta chunk.
  const handleActivityEvent = (ev: AFActivityEvent) => {
    if (ev.event !== 'streaming') {
      const dedupKey = `${ev.activity.id}:${ev.event}`;
      if (activityDedupIds.has(dedupKey)) return;
      activityDedupIds.add(dedupKey);
      activityDedupRing.push(dedupKey);
      if (activityDedupRing.length > ACTIVITY_DEDUP_LIMIT) {
        const evicted = activityDedupRing.shift();
        if (evicted) activityDedupIds.delete(evicted);
      }
    }

    // Phase 3 / ADR-03 D6: route system/control ActivityEvents through
    // useSystemEventNotification (run_status, error, team_*, graph_*,
    // spirit_*, …). Returns true if the event was handled as a system
    // event; false means it's a chat-rendering event (handled below).
    if (systemEventNotification.handleSystemEvent(ev)) {
      return;
    }

    // Chat-rendering events (task streaming/created/completed, thinking,
    // action, reply, confirm) are now handled by the v2 event pipeline
    // (handleV2Event → eventRouter → activityStore). The v1 ActivityEvent
    // path here only retains dedup + system-event routing + sender reset.
    // OBS-02: Trigger contextual loading message for tool events (kind=action).
    if (ev.activity.kind === 'action') {
      contextualLoading.onSpiritActivityEvent(ev);
    }
    // Bugfix P1#4 (robust fallback): when the backend completes a turn but
    // the terminal run_status event is missing/late/coalesced, the watch on
    // runStatus alone cannot reset sending. Treat task completed/failed/
    // cancelled as a definitive turn-end signal and reset sending here.
    // Without this, the composer stays stuck on "停止生成" after the reply
    // is fully rendered.
    if (
      ev.activity.kind === 'task' &&
      (ev.event === 'completed' || ev.event === 'failed' || ev.event === 'cancelled') &&
      sender.sending.value
    ) {
      sender.markSendingDone();
    }
  };

  const runStatusCtrl = useChatRunStatus({ applyAwaitRunStatus });
  const { runStatus, runMeta, applyFromActivityEvent, onSessionSwitch, refreshRunStatus, forceSetRunStatus } =
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
    // Activity-First (AF): route new business-semantic ActivityEvent messages
    // from the WS transport to useActivityTimeline.
    onActivityEvent: handleActivityEvent,
    onV2Event: handleV2Event,
    refreshRunStatus: refreshRunStatusForUi,
  });

  const contextualLoading = useContextualLoadingMessage(streamManager.wsReplaying);
  const statusPulse = useStatusPulse(streamManager.wsReplaying);

  // D1: Load spirit teams when a Spirit session is selected
  watch(
    () => ({
      sessionId: sessionStore.selectedSession?.id,
      agentKey: appStore.selectedAgent?.agent_key,
    }),
    ({ sessionId, agentKey }) => {
      if (!sessionId) return;
      if (agentKey === '__spirit__') {
        spiritStore.loadSpiritTeams(sessionId);
      } else {
        if (spiritStore.teams.length > 0) {
          spiritStore.reset();
        }
      }
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

  const inboundSync = useChatInboundSync({
    appStore,
    sessionStore,
    messageStore,
    spiritStore,
    selectedAgentId,
    selectedSessionId,
    wsReplaying: streamManager.wsReplaying,
    // AF: Spirit/team orchestration events now consume ActivityEvent directly.
    onSpiritActivityEvent: contextualLoading.onSpiritActivityEvent,
    // AF: Route new ActivityEvent messages from the global hub to the same
    // useActivityTimeline instance. Dedup is handled inside handleActivityEvent.
    onActivityEvent: handleActivityEvent,
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
      // AF architecture: Activity data is the source of truth for the timeline.
      // Turn completion must NOT clear it — the AF path uses Activity records
      // (grouped by turnId) to render each turn's thinking/action/reply in
      // correct temporal order. Clearing would force degradation to the message
      // inference path, which cannot reconstruct multi-round ReAct timelines.
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

  // AF: Capture handleInboundActivityEvent from useChatInboundSync so
  // handleActivityEvent can route system/control ActivityEvents directly
  // to the ActivityEvent-based inbound pipeline (no Envelope conversion).
  inboundActivityEventHandler = inboundSync.handleInboundActivityEvent;

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
    refreshPendingMessages: () => pendingMsgRef.fn?.() ?? Promise.resolve(),
  });

  const followUp = useFollowUpQueue(sessionIdForPending, sender.sending, (message) =>
    $q.notify({ type: 'negative', message }),
  );
  const { pendingMessages, refreshPendingMessages, onCancelPending, onInterruptPending, onUpdatePending } = followUp;
  pendingMsgRef.fn = refreshPendingMessages;

  watch(sender.sending, (val) => followUp.watchSending(val));

  /**
   * Activity-First: Apply run status from an ActivityEvent (stage=run_status).
   * Consumes ActivityEvent directly, updating the follow-up queue, runStatus
   * ref, and cancelling tool messages on terminal statuses. Called from
   * handleActivityEvent for run_status events that arrive via the
   * ActivityEvent path.
   */
  function applyRunStatusFromActivityEvent(ev: AFActivityEvent) {
    followUp.onRunStatusActivityEvent(ev);
    applyFromActivityEvent(ev);
    const rs = runStatusFromActivityEvent(ev);
    if (rs?.status === 'cancelled' || rs?.status === 'failed') {
      const sid = selectedSessionForUi.value?.id;
      if (sid) {
        messageStore.setMessages(sid, cancelRunningToolMessages(messageStore.getMessages(sid)));
      }
    }
  }

  /**
   * Phase 3b-D Task 12: Route v2 system.notice events to side-effects.
   *
   * Backend migrated system-domain notices (orchestration_started, metrics_updated,
   * knowledge_ingest, etc.) from v1 ActivityEventBus to v2 EventBus. These arrive
   * as v2_event envelopes but need the same side-effects as v1 (spirit store
   * updates, session refresh). We adapt the v2 payload to a minimal v1
   * ActivityEvent and route through inboundActivityEventHandler, which dispatches
   * to spiritStore and sessionStore based on kind/stage.
   *
   * v1 system events (run_status, session_status_changed, graph_stage) still
   * arrive via the v1 activity_event path and are handled by
   * useSystemEventNotification — those are NOT touched here.
   */
  function handleV2SystemNotice(payload: SystemNoticeEventPayload) {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    const afEv = v2NoticeToAFEvent(payload, sid);
    inboundActivityEventHandler?.(afEv);
  }

  /**
   * Build a minimal v1 ActivityEvent from a v2 SystemNoticeEventPayload so it
   * can be consumed by the existing inbound-sync pipeline (which checks
   * `activity.kind` and `activity.stage` to dispatch side-effects).
   *
   * kind='session' is required for orchestration notices so isSpiritActivityEvent
   * matches them (it checks kind==='session' for orchestration_* stages).
   * kind='notice' is used for all other notice types (metrics_updated, etc.).
   */
  function v2NoticeToAFEvent(payload: SystemNoticeEventPayload, sessionId: string): AFActivityEvent {
    const orchestrationStages = new Set([
      'orchestration_started',
      'orchestration_checkpoint',
      'orchestration_interrupted',
      'orchestration_completed',
      'orchestration_failed',
      'synthesis_completed',
      'plan_created',
      'allocation_created',
    ]);
    const isOrchestration = orchestrationStages.has(payload.NoticeType);
    return {
      event: 'created',
      activity: {
        id: `v2-sys-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        kind: isOrchestration ? 'session' : 'notice',
        status: 'running',
        session_id: sessionId,
        turn_id: '',
        parent_activity_id: '',
        timestamp: new Date().toISOString(),
        duration_ms: 0,
        seq: 0,
        prompt_tokens: 0,
        completion_tokens: 0,
        content: payload.Message ?? '',
        reasoning: '',
        tool_name: '',
        tool_category: 'other',
        tool_call_id: '',
        tool_arguments: '',
        tool_result: '',
        tool_duration_ms: 0,
        tool_error_code: '',
        stage: payload.NoticeType,
        child_board_id: '',
        spirit_session_id: sessionId,
        team_id: '',
        dag_node_id: '',
        depends_on: [],
        agent_key: '',
        agent_name: '',
        collapsed: false,
        label: '',
        meta: { ...(payload.Meta ?? {}), notice_type: payload.NoticeType, message: payload.Message },
      },
    };
  }

  /**
   * Phase 3b-D: Route v2 system.run_status events to side-effects.
   *
   * Backend PublishRunStatus/PublishRunStatusFull (commit d5a52ea7e) emit
   * biz.NewRunStatusEvent on the v2 EventBus. These arrive as v2_event
   * envelopes but need the same side-effects as v1 stage=run_status events:
   *   - applyRunStatusFromActivityEvent: follow-up queue, runStatus ref,
   *     tool-message cancellation on terminal statuses.
   *   - inboundActivityEventHandler: session refresh, channel focus, stream
   *     unseal on RUNNING status.
   *
   * v1 run_status events still arrive via the v1 activity_event path and are
   * handled by useSystemEventNotification — those continue to work.
   */
  function handleV2RunStatus(payload: RunStatusEventPayload) {
    const sid = selectedSessionForUi.value?.id;
    if (!sid) return;
    const afEv = v2RunStatusToAFEvent(payload, sid);
    applyRunStatusFromActivityEvent(afEv);
    inboundActivityEventHandler?.(afEv);
  }

  /**
   * Build a minimal v1 ActivityEvent from a v2 RunStatusEventPayload so it
   * can be consumed by runStatusFromActivityEvent (which requires
   * `activity.stage === 'run_status'` and reads status/run_id/error_message/
   * await_kind/await_tool_key/await_tool_call_id from `activity.meta`).
   *
   * Backend PublishRunStatusFull populates Meta with all await fields plus
   * run_id/status/error_message/session_run_id/turn_id/notice_type. Top-level
   * RunID/Status fields are merged as fallback in case Meta is partial.
   */
  function v2RunStatusToAFEvent(payload: RunStatusEventPayload, sessionId: string): AFActivityEvent {
    const meta = {
      ...(payload.Meta ?? {}),
      run_id: payload.RunID,
      status: payload.Status,
    };
    return {
      event: 'created',
      activity: {
        id: `v2-rs-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
        kind: 'session',
        status: 'running',
        session_id: sessionId,
        turn_id: '',
        parent_activity_id: '',
        timestamp: new Date().toISOString(),
        duration_ms: 0,
        seq: 0,
        prompt_tokens: 0,
        completion_tokens: 0,
        content: '',
        reasoning: '',
        tool_name: '',
        tool_category: 'other',
        tool_call_id: '',
        tool_arguments: '',
        tool_result: '',
        tool_duration_ms: 0,
        tool_error_code: '',
        stage: 'run_status',
        child_board_id: '',
        spirit_session_id: sessionId,
        team_id: '',
        dag_node_id: '',
        depends_on: [],
        agent_key: '',
        agent_name: '',
        collapsed: false,
        label: '',
        meta,
      },
    };
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
    if (!sid) return;
    await sessionStore.fetchCompressStatus(sid);
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

  watch(compressStatus, (status) => {
    if (status === 'normal') {
      if (!compressNormalSince) {
        compressNormalSince = Date.now();
      }
      if (Date.now() - compressNormalSince >= COMPRESS_NORMAL_COOLDOWN_MS) {
        stopCompressPolling();
      }
    } else {
      compressNormalSince = null;
      if (!compressPollTimer) {
        startCompressPolling();
      }
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

      // Phase 2 v2: Activity data is hydrated via the v2 WS replay path
      // (useChatStreamManager → onV2Event → eventRouter → activityStore).
      // No explicit REST pre-load is needed; messages load below.
      await messageStore.loadMessages(replace ? { sessionId, replace: true } : { sessionId });
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
      const memberSessionId = sessionTree.findMemberSessionId(spiritSessionId, agentKey);
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

    void Promise.all([
      entityNav.loadTaxonomyTree(),
      entityNav.loadTeams(),
      (async () => {
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
      })(),
    ]).then(() => {
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
