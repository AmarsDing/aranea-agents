import { useChatSessionStore } from '../../../stores/chat/sessionStore';
import { useChatMessageStore } from '../../../stores/chat/messageStore';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { useSpiritTeamStore } from '../../../stores/spirit';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useLlmRetryStore } from '../../../stores/chat/llmRetryStore';
import { useChatEventRouter } from './useChatEventRouter';
import { cancelRunningToolMessages } from '../activityToolCall';
import { emitSessionMutation } from '../../../stores/sessionMutationBus';
import { noteChannelWsEnvelope } from '../channelWsCursor';
import { SESSION_RUN_STATUS } from '../sessionRunStatus';
import { runStatusFromV2Payload } from '../activityRunStatus';
import { sessionContextPatchFromContextUsageMeta } from '../sessionContextPatch';
import type {
  V2WsEnvelope,
  SystemNoticeEventPayload,
  RunStatusEventPayload,
  HeartbeatEventPayload,
  SkillCatalogEventPayload,
} from '../v2Types';

/**
 * v2 WS 事件处理器：从 useChatWorkspace 抽出的 system/entity 事件分发逻辑。
 * 负责游标推进、system.notice / system.run_status 副作用路由、spirit store
 * 团队事件同步、LLM 重试横幅生命周期、sending 兜底复位。
 */
export function useChatV2EventHandlers(deps: {
  /** 当前选中会话 id（getter 延迟求值，事件到达时才读取） */
  selectedSessionId: () => string | undefined;
  sender: {
    sending: { value: boolean };
    markSendingDone: () => void;
    onRunAccepted: () => void;
    onFirstByteArrived: () => void;
    touchRunActivity: (eventSessionId?: string) => void;
  };
  followUp: { onRunStatusV2: (payload: RunStatusEventPayload) => void };
  applyFromV2RunStatus: (payload: RunStatusEventPayload) => void;
  contextualLoading: {
    onSpiritNoticeType: (noticeType: string) => void;
    onRunHeartbeat: (payload: HeartbeatEventPayload) => void;
    clearMessage: () => void;
  };
  streamManager: { ensureTeamStream: (sid: string) => void; ensureChatStream: (sid: string) => void };
}) {
  const sessionStore = useChatSessionStore();
  const messageStore = useChatMessageStore();
  const runtimeStore = useChatRuntimeStore();
  const spiritStore = useSpiritTeamStore();
  const activityStore = useChatActivityStore();
  const llmRetryStore = useLlmRetryStore();
  const eventRouter = useChatEventRouter(activityStore);

  // v2 WS event handler — dispatched by the stream manager's onV2Event callback.
  function handleV2Event(envelope: V2WsEnvelope) {
    // B-06: advance WS cursor from durable outbox event_id when present.
    const cursorEventId = String(envelope.event_id ?? '').trim();
    const cursorSessionId = String(envelope.session_id ?? '').trim();
    if (cursorEventId && cursorSessionId) {
      noteChannelWsEnvelope(cursorSessionId, cursorEventId);
    }
    // 发送态对账 watchdog（useChatSender.reconcileRunState）的活动基线：
    // run 相关 WS 帧（run_status / turn·step·task / heartbeat / notice）都说明
    // 数据仍在流动，刷新 lastRunEventAt 顺延 90s 对账；唯一例外是连接建立时的
    // skill.catalog 目录推送（与 run 进度无关，不算活动）。
    // REG-3：带上传事件的 session_id——基线只顺延当前发送会话，其他会话/团队流
    // 的活动不得掩盖本会话的终态丢失（见 useChatSender.touchRunActivity）。
    if (envelope.kind !== 'skill.catalog') {
      deps.sender.touchRunActivity(envelope.session_id ? String(envelope.session_id) : undefined);
    }
    // Stall / first-byte watchdogs: run_status=running arms the first-byte
    // notice (useChatSender.onRunAccepted); the first streaming token clears it.
    if (envelope.kind === 'step.streaming') {
      deps.sender.onFirstByteArrived();
    }
    // Phase 3b-D Task 12: intercept v2 system-domain events for side-effect
    // routing. Entity events (task/turn/step/team_stage/etc.) go to the v2
    // event router → activityV2Store. System events need side-effect routing
    // (metrics refresh, spirit store updates, run-status application) that the
    // store cannot provide.
    if (envelope.kind === 'system.notice') {
      handleV2SystemNotice(envelope.payload as SystemNoticeEventPayload, envelope.session_id);
      return;
    }
    if (envelope.kind === 'system.run_status') {
      handleV2RunStatus(envelope.payload as RunStatusEventPayload);
      return;
    }
    if (envelope.kind === 'system.heartbeat') {
      deps.contextualLoading.onRunHeartbeat(envelope.payload as HeartbeatEventPayload);
      return;
    }
    // Design 69 Phase 3: skill.catalog carries the agent-visible skill list
    // (pushed once per WS connection setup). Ephemeral UI state — store it
    // per session in the runtime store for the composer skill picker.
    if (envelope.kind === 'skill.catalog') {
      const sid = String(envelope.session_id ?? '').trim();
      const skills = (envelope.payload as SkillCatalogEventPayload)?.skills;
      if (sid && Array.isArray(skills)) {
        runtimeStore.setSkillCatalog(sid, skills);
      }
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
      case 'turn.started': {
        const sid = String(envelope.session_id ?? '').trim() || deps.selectedSessionId();
        if (sid) llmRetryStore.clear(sid);
        break;
      }
      case 'step.streaming':
      case 'step.completed':
      case 'turn.completed':
      case 'task.completed':
      case 'step.failed':
      case 'turn.failed':
      case 'task.failed':
      case 'task.cancelled': {
        const sid = String(envelope.session_id ?? '').trim() || deps.selectedSessionId();
        if (sid) llmRetryStore.clearTransient(sid);
        break;
      }
    }

    // Robust fallback: when the backend completes a turn but the terminal
    // run_status is missing/late, task.completed/failed/cancelled still resets
    // sending so the composer does not stay stuck on "停止生成".
    if (
      (envelope.kind === 'task.completed' || envelope.kind === 'task.failed' || envelope.kind === 'task.cancelled') &&
      deps.sender.sending.value
    ) {
      deps.sender.markSendingDone();
    }
  }

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

  /**
   * Apply run status from a v2 system.run_status payload (follow-up queue,
   * runStatus ref, tool-message cancellation on terminal statuses).
   */
  function applyV2RunStatusSideEffects(payload: RunStatusEventPayload) {
    deps.followUp.onRunStatusV2(payload);
    deps.applyFromV2RunStatus(payload);
    const rs = runStatusFromV2Payload(payload);
    if (rs?.status === 'cancelled' || rs?.status === 'failed') {
      const sid = deps.selectedSessionId();
      if (sid) {
        messageStore.setMessages(sid, cancelRunningToolMessages(messageStore.getMessages(sid)));
      }
    }
  }

  /**
   * Route v2 system.notice events to side-effects without AF conversion.
   */
  function handleV2SystemNotice(payload: SystemNoticeEventPayload, sessionId?: string) {
    const sid = String(sessionId ?? '').trim() || deps.selectedSessionId();
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
    // context_usage carries per-turn token occupancy plus the prompt-assembly
    // breakdown (meta.context_budget) — patch the session so the SpiritStatusBar
    // popup renders the composition without a REST round-trip.
    if (noticeType === 'context_usage') {
      const patch = sessionContextPatchFromContextUsageMeta(payload.Meta ?? undefined);
      if (patch) {
        sessionStore.patchSessionMetricsLocal(sid, patch);
      }
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
    if (noticeType === 'llm_billing') {
      llmRetryStore.noteAlert(sid, 'billing', meta);
    }
    if (noticeType === 'llm_auth') {
      llmRetryStore.noteAlert(sid, 'auth', meta);
    }
    if (noticeType === 'llm_stall') {
      llmRetryStore.noteAlert(sid, 'stall', meta);
    }
    // contract_mismatch（2026-10-20 P1-1）：dag_run 启动时 plan_executor 对
    // 交付物契约的 advisory 校验结果，非阻断；由 PlanBoardCard 渲染黄色横幅。
    // ephemeral notice——不入库、不 replay（front-end 仅按 plan_board_id 暂存）。
    if (noticeType === 'contract_mismatch') {
      activityStore.noteContractMismatch(sid, meta);
    }
    spiritStore.handleSystemNotice(noticeType, meta);
    deps.contextualLoading.onSpiritNoticeType(noticeType);
  }

  /**
   * Route v2 system.run_status events to side-effects without AF conversion.
   */
  function handleV2RunStatus(payload: RunStatusEventPayload) {
    // Patch MemberSession card when pause/resume publishes chat_session_id
    // (MemberSessionUpdatedEvent is the primary path; this covers race/late WS).
    // D9-20：走专用状态补丁（终态汇点、免伪造 Version）——旧 upsertMemberSession
    // 同 Version 会被乐观并发守卫拒收，迟到 paused/running 补丁被死代码吞掉。
    const chatSessionId = String(payload.Meta?.chat_session_id ?? '').trim();
    const status = String(payload.Status ?? payload.Meta?.status ?? '').trim();
    if (chatSessionId && (status === 'paused' || status === 'running')) {
      activityStore.patchMemberSessionStatusByChatSession(chatSessionId, status);
    }
    const sid = deps.selectedSessionId();
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
      llmRetryStore.clearTransient(sid);
      // 2026-08-06: pre-orchestration phases (routing/…/starting) set the
      // loading line on every turn; direct-answer turns have no
      // orchestration.completed event, so terminal run status is the only
      // reliable clearing point for them.
      deps.contextualLoading.clearMessage();
    }
    const rs = runStatusFromV2Payload(payload);
    if (rs?.status === SESSION_RUN_STATUS.RUNNING) {
      deps.sender.onRunAccepted();
      if (sessionStore.entityKind === 'team') {
        deps.streamManager.ensureTeamStream(sid);
      } else {
        deps.streamManager.ensureChatStream(sid);
      }
    }
  }

  return { handleV2Event };
}
