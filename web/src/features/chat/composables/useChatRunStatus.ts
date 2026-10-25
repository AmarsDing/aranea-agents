import { ref, type Ref } from 'vue';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import type { RunStatus, RunStatusValue } from '../types';
import type { ActivityEvent } from '../../../realtime/activityEvent';
import type { RunStatusEventPayload } from '../v2Types';
import {
  runStatusFromActivityEvent,
  messageQueuedFromActivityEvent,
  runStatusFromV2Payload,
  messageQueuedFromV2Payload,
  type RunStatusFromWs,
} from '../activityRunStatus';

const HYDRATE_DELAY_MS = 400;

// D9-19：终态集合与 run_status 接受谓词。freshness 追踪器（lastRunId /
// lastStatus / lastServerMs）由 useChatRunStatus 闭包持有，统一裁决 WS 帧与
// REST 对账响应：
//   R1 终态粘性 —— runId 相同且已接受 terminal 时拒绝非终态回落（idle 除外）；
//     接受 idle 后进入复位语义：同一 run 的任何迟到回放帧（含 terminal）不再复活旧状态。
//     E2E-B3 例外：paused 生命周期挂起帧可穿透粘性——PauseSession 的取消链按
//     cancelled→paused 顺序同 runId 连发两帧，拒绝 paused 会让前端恒停 cancelled。
//   R2 runId 绑定 —— 跨 runId 仅在上个 run 已 terminal/idle/未知时接受；
//     E2E-B3：paused（生命周期挂起）亦视为「上个 run 已结算」，否则暂停后新
//     消息的首帧被保守丢弃、前端滞留 paused。
//     non-terminal + 异 runId 保守丢弃，由 orphan 对账 / 下次切会话 hydrate 兜回。
//   R3 REST 单调 —— 同 runId 且双方带服务端时间时拒绝更旧快照。
//   runId 为空的 legacy 帧按当前 runId 处理且不回写 lastRunId。
const TERMINAL_RUN_STATUSES = new Set<string>(['completed', 'failed', 'cancelled']);
// E2E-B3：paused 是生命周期挂起帧，不是终态，但必须穿透 R1 终态粘性、且在
// R2 跨 runId 裁决中等价于「上个 run 已结算」。
const LIFECYCLE_HOLD_STATUSES = new Set<string>(['paused']);

function shouldAcceptRunStatus(
  last: { runId: string; status: string; serverMs: number },
  cand: { status: string; runId: string; serverMs: number },
): boolean {
  const lastTerminal = TERMINAL_RUN_STATUSES.has(last.status);
  const candTerminal = TERMINAL_RUN_STATUSES.has(cand.status);
  const lastHold = LIFECYCLE_HOLD_STATUSES.has(last.status);
  const candHold = LIFECYCLE_HOLD_STATUSES.has(cand.status);
  const effectiveRunId = cand.runId || last.runId; // legacy 空 runId 帧按当前 runId 处理
  const sameRun = effectiveRunId !== '' && last.runId !== '' && effectiveRunId === last.runId;

  if (sameRun) {
    // R1 终态粘性（E2E-B3：+ candHold 挂起帧穿透——cancelled→paused 同 run 连发）
    if (lastTerminal) return candTerminal || cand.status === 'idle' || candHold;
    if (last.status === 'idle') return cand.status === 'idle'; // 复位语义：同 run 不再复活
    // 同 run 非终态上：REST 服务端 updatedAt 单调
    if (
      !candTerminal &&
      Number.isFinite(last.serverMs) &&
      Number.isFinite(cand.serverMs) &&
      cand.serverMs < last.serverMs
    ) {
      return false; // R3 旧快照
    }
    return true;
  }

  if (cand.runId !== '' && last.runId !== '' && cand.runId !== last.runId) {
    // R2 跨 runId：上个 run 已 terminal/idle/paused 时接受（lastRunId 为空 = 上个 run 未知 → 接受）
    return lastTerminal || lastHold || last.status === 'idle';
  }
  return true;
}

type UseChatRunStatusDeps = {
  applyAwaitRunStatus: (rs: RunStatus) => void;
};

/**
 * WS-primary run status: envelopes update state immediately; HTTP hydrates only
 * when WS has not yet reported for the current session (e.g. after session switch).
 */
export function useChatRunStatus(deps: UseChatRunStatusDeps) {
  const runStatus: Ref<RunStatusValue> = ref('idle');
  const runMeta: Ref<RunStatus | null> = ref(null);

  let wsAuthoritative = false;
  let hydrateTimer: ReturnType<typeof setTimeout> | null = null;
  let hydrateSessionId = '';

  // D9-19 新鲜度追踪器（见模块级 TERMINAL_RUN_STATUSES / shouldAcceptRunStatus）。
  // lastRunId='' / lastStatus='' 表示未接受过任何帧（上个 run 未知）。
  let lastRunId = '';
  let lastStatus = '';
  let lastServerMs = Number.NaN;

  function clearHydrateTimer() {
    if (hydrateTimer) {
      clearTimeout(hydrateTimer);
      hydrateTimer = null;
    }
  }

  function lastTracker() {
    return { runId: lastRunId, status: lastStatus, serverMs: lastServerMs };
  }

  /** 接受一帧后更新追踪器。legacy 空 runId 不回写 lastRunId；WS 帧无服务端时间则保留既有值。 */
  function noteAcceptedRunStatus(candRunId: string, status: RunStatusValue, serverMs: number) {
    if (candRunId) lastRunId = candRunId;
    lastStatus = status;
    if (Number.isFinite(serverMs)) lastServerMs = serverMs;
  }

  function resetRunStatus() {
    clearHydrateTimer();
    wsAuthoritative = false;
    hydrateSessionId = '';
    // 清空全部追踪器（复位至初始未知态）
    lastRunId = '';
    lastStatus = '';
    lastServerMs = Number.NaN;
    runStatus.value = 'idle';
    runMeta.value = null;
  }

  function applyRunStatusFields(rs: RunStatusFromWs) {
    // WS 入口统一过新鲜度谓词：WS 帧不带服务端时间（updatedAt 恒为空串）。
    if (!shouldAcceptRunStatus(lastTracker(), { status: rs.status, runId: rs.runId, serverMs: Number.NaN })) return;
    wsAuthoritative = true;
    clearHydrateTimer();
    runStatus.value = rs.status;
    const meta: RunStatus = {
      status: rs.status,
      runId: rs.runId,
      errorMessage: rs.errorMessage,
      awaitKind: rs.awaitKind,
      awaitToolKey: rs.awaitToolKey,
      awaitToolCallId: rs.awaitToolCallId,
      updatedAt: '',
    };
    runMeta.value = meta;
    noteAcceptedRunStatus(rs.runId, rs.status, Number.NaN);
    deps.applyAwaitRunStatus(meta);
  }

  /** Apply run status from a v2 system.run_status payload. */
  function applyFromV2RunStatus(payload: RunStatusEventPayload) {
    if (messageQueuedFromV2Payload(payload)) return;
    const rs = runStatusFromV2Payload(payload);
    if (!rs) return;
    applyRunStatusFields(rs);
  }

  /** Legacy: apply run status from an ActivityEvent (stage=run_status). */
  function applyFromActivityEvent(ev: ActivityEvent) {
    if (messageQueuedFromActivityEvent(ev)) return;
    const rs = runStatusFromActivityEvent(ev);
    if (!rs) return;
    applyRunStatusFields(rs);
  }

  function scheduleHttpHydrate(sessionId: string) {
    hydrateSessionId = sessionId;
    wsAuthoritative = false;
    clearHydrateTimer();
    hydrateTimer = setTimeout(() => {
      void hydrateFromHttpIfNeeded(sessionId);
    }, HYDRATE_DELAY_MS);
  }

  async function hydrateFromHttpIfNeeded(sessionId: string) {
    if (!sessionId || sessionId !== hydrateSessionId || wsAuthoritative) return;
    try {
      const runtime = useChatRuntimeStore();
      const rs = await runtime.fetchRunStatus(sessionId);
      if (sessionId !== hydrateSessionId || wsAuthoritative) return;
      // REST 入口统一过新鲜度谓词（带服务端 updatedAt，供 R3 单调裁决）。
      const serverMs = Date.parse(rs.updatedAt || '');
      if (!shouldAcceptRunStatus(lastTracker(), { status: rs.status, runId: rs.runId, serverMs })) return;
      runStatus.value = rs.status;
      runMeta.value = rs;
      noteAcceptedRunStatus(rs.runId, rs.status, serverMs);
      deps.applyAwaitRunStatus(rs);
    } catch {
      /* ignore transient hydrate failures */
    }
  }

  /** Fallback HTTP refresh; skipped when WS already authoritative for this session. */
  async function refreshRunStatus(sessionId?: string) {
    if (!sessionId) {
      resetRunStatus();
      return;
    }
    await hydrateFromHttpIfNeeded(sessionId);
  }

  /**
   * Unconditional REST reconcile (bypasses the WS-authoritative guard).
   * Used by the sending watchdog when run activity has been silent for a long
   * stretch and the terminal WS frame may have been lost. Errors propagate so
   * the caller can distinguish "backend confirmed terminal" from "network
   * failure — try again later" (never auto-reset on network failure).
   *
   * Returns the freshly applied backend status; null means NO backend-confirmed
   * state was applied (no session, or the response was dropped by the staleness
   * guard). Callers must treat null as "unconfirmed" and never fall back to the
   * local runStatus value — it may be only the initial 'idle' default (REG-2).
   */
  async function refreshRunStatusForced(sessionId?: string): Promise<string | null> {
    if (!sessionId) {
      resetRunStatus();
      return null;
    }
    const runtime = useChatRuntimeStore();
    const rs = await runtime.fetchRunStatus(sessionId);
    // 会话过期守卫（同 hydrateFromHttpIfNeeded 手法）：对账请求在途时用户可能已
    // 切换会话——旧会话的迟到响应必须整个丢弃，不得写 runStatus/runMeta 或 apply；
    // 否则旧状态会覆盖当前会话面板，watchdog 读到过期终态还会误判并复位 sending。
    if (sessionId !== hydrateSessionId) return null;
    // 新鲜度守卫（D9-19）：同 run 内终端 WS 帧恒赢迟到 REST running；REST 同 runId
    // 旧快照不得回退。被丢弃时维持 REG-2 null 语义（未获后端新确认）。
    const serverMs = Date.parse(rs.updatedAt || '');
    if (!shouldAcceptRunStatus(lastTracker(), { status: rs.status, runId: rs.runId, serverMs })) return null;
    runStatus.value = rs.status;
    runMeta.value = rs;
    noteAcceptedRunStatus(rs.runId, rs.status, serverMs);
    deps.applyAwaitRunStatus(rs);
    return rs.status;
  }

  function onSessionSwitch(sessionId: string | undefined) {
    if (!sessionId) {
      resetRunStatus();
      return;
    }
    // Don't reset to "idle" immediately — keep previous value until HTTP hydrate
    // completes to avoid a brief incorrect "idle" flash during session switch.
    // The hydrate will set the correct status within HYDRATE_DELAY_MS.
    scheduleHttpHydrate(sessionId);
  }

  /** Force-set run status (used when backend rejects enqueue due to stale state). */
  function forceSetRunStatus(status: RunStatusValue) {
    wsAuthoritative = true;
    clearHydrateTimer();
    runStatus.value = status;
    // 重置追踪器：lastStatus 置为强制值；lastRunId 保留至「被复位」的 run，
    // 使该 run 的迟到回放帧经同 run 复位语义丢弃，不得复活旧状态（D9-19 ⑧）。
    lastRunId = runMeta.value?.runId || lastRunId;
    lastStatus = status;
    lastServerMs = Number.NaN;
    if (status === 'idle') {
      runMeta.value = null;
    }
  }

  return {
    runStatus,
    runMeta,
    applyFromV2RunStatus,
    applyFromActivityEvent,
    onSessionSwitch,
    refreshRunStatus,
    refreshRunStatusForced,
    forceSetRunStatus,
  };
}
