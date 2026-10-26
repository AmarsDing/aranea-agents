// web/src/features/chat/composables/useChatEventRouter.ts
import type { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { V2WsEnvelope, EventKind } from '../v2Types';

type Store = ReturnType<typeof useChatActivityStore>;

// D9-22：未知 kind / 非 v2_event 帧的纯可观测性提示。逐键 60s 节流防异常帧洪泛
// 刷屏；键集合有界（kind 枚举 + type）。Map 为模块级单例随页面生命周期存在。
const WS_FRAME_WARN_INTERVAL_MS = 60_000;
const lastFrameWarnAt = new Map<string, number>();

function warnDroppedFrameThrottled(key: string, message: string, detail: unknown) {
  const now = Date.now();
  const last = lastFrameWarnAt.get(key) ?? -Infinity;
  if (now - last < WS_FRAME_WARN_INTERVAL_MS) return;
  lastFrameWarnAt.set(key, now);
  console.warn(message, detail);
}

/**
 * useChatEventRouter dispatches v2 WS events into Pinia store mutations.
 *
 * The router is a pure function of (store, envelope) → store mutation.
 * Streaming delta dedup lives in activityV2Store.appendStepDelta (P3: DeltaSeq-based).
 * Reconnect reconciliation uses fetchSessionHistory (no WS replay).
 */
export function useChatEventRouter(store: Store) {
  function dispatch(env: V2WsEnvelope) {
    if (env.type !== 'v2_event') {
      // 非 v2_event 帧：忽略但打点（前向兼容不变，行为语义不动）。
      warnDroppedFrameThrottled(
        `type:${env.type}:${String((env as { kind?: unknown }).kind ?? '')}`,
        '[chat] ws frame dropped: non-v2_event',
        {
          type: env.type,
          kind: (env as { kind?: unknown }).kind,
        },
      );
      return;
    }
    handleKind(env.kind, env.payload);
  }

  function handleKind(kind: EventKind, payload: unknown) {
    const p = payload as Record<string, unknown>;
    switch (kind) {
      // Task events
      case 'task.created':
        if (p.Task) {
          store.upsertTask(p.Task as never);
          // 会话进行中新建的 task 默认展开（活跃任务永远自动水合，设计 P5/§5）。
          store.hydratedTaskIds.add((p.Task as { ID: string }).ID);
        }
        break;
      // 2026-10-20: 取消终态由专用 kind 承载（后端 projector 不再复用
      // task.completed 表达取消），与终态事件同路径 upsert。
      case 'task.updated':
      case 'task.completed':
      case 'task.failed':
      case 'task.cancelled':
        if (p.Task) store.upsertTask(p.Task as never);
        break;

      // Turn events
      case 'turn.started':
      case 'turn.completed':
      case 'turn.failed':
        if (p.Turn) store.upsertTurn(p.Turn as never);
        break;

      // Step events
      case 'step.created':
        if (p.Step) store.upsertStep(p.Step as never);
        break;
      case 'step.streaming':
        if (p.StepID && p.DeltaField && p.DeltaChunk) {
          // P2-06: pass DeltaField as string — the store handles known fields
          // (content, reasoning) and silently ignores unknown ones.
          // P3: pass DeltaSeq — the store dedups redelivered deltas by
          // sequence; undefined seq (legacy producer) always applies.
          // W-1: pass GenerationID — the store resets accumulated text on
          // generation change (liveness reconnect replays from scratch).
          store.appendStepDelta(
            p.StepID as string,
            p.DeltaField as string,
            p.DeltaChunk as string,
            p.DeltaSeq as number | undefined,
            p.GenerationID as string | undefined,
          );
        }
        break;
      case 'step.updated':
      case 'step.completed':
      case 'step.failed':
        if (p.Step) store.upsertStep(p.Step as never);
        break;

      // TeamStage events
      case 'team_stage.created':
      case 'team_stage.updated':
      case 'team_stage.completed':
      case 'team_stage.failed':
        if (p.TeamStage) store.upsertTeamStage(p.TeamStage as never);
        break;

      // TeamRun events
      case 'team_run.started':
      case 'team_run.completed':
      case 'team_run.failed':
      case 'team_run.cancelled':
        if (p.TeamRun) store.upsertTeamRun(p.TeamRun as never);
        break;

      // MemberSession events
      case 'member_session.created':
      case 'member_session.updated':
        if (p.MemberSession) store.upsertMemberSession(p.MemberSession as never);
        break;

      // PlanBoard events
      case 'plan_board.created':
      case 'plan_board.updated':
        if (p.PlanBoard) store.upsertPlanBoard(p.PlanBoard as never);
        break;

      // PlanStep events
      case 'plan_step.started':
      case 'plan_step.completed':
      case 'plan_step.failed':
      case 'plan_step.updated':
        if (p.PlanStep) store.upsertPlanStep(p.PlanStep as never);
        break;
      case 'plan_step.skipped':
        if (p.PlanStep) store.upsertPlanStep(p.PlanStep as never);
        break;

      case 'graph_stage.created':
      case 'graph_stage.updated':
      case 'graph_stage.completed':
      case 'graph_stage.failed':
      case 'graph_stage.interrupted':
        if (p.GraphStage) store.upsertGraphStage(p.GraphStage as never);
        break;

      // GraphNode 事件
      case 'graph_node.updated':
        if (p.GraphNode) store.upsertGraphNode(p.GraphNode as never);
        break;

      // Phase 3b-D Task 12: system-domain events are intercepted in
      // useChatWorkspace.handleV2Event (side-effect routing via
      // inboundActivityEventHandler) and never reach this router. The cases
      // are listed here for exhaustiveness and to prevent future regressions
      // if the intercept is removed without re-adding handling.
      // Design 69 Phase 3: skill.catalog is likewise intercepted upstream
      // (runtimeStore.setSkillCatalog) — no entity mutation here.
      case 'system.run_status':
      case 'system.heartbeat':
      case 'system.notice':
      case 'skill.catalog':
        break;

      default:
        // Unknown event kind — silently ignore (forward compatibility), 仅节流打点。
        warnDroppedFrameThrottled(`kind:${String(kind)}`, '[chat] unknown v2 event kind', kind);
        break;
    }
  }

  return { dispatch };
}
