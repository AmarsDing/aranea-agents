// web/src/features/chat/composables/useChatEventRouter.ts
import type { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { V2WsEnvelope, EventKind } from '../v2Types';

type Store = ReturnType<typeof useChatActivityStore>;

/**
 * useChatEventRouter dispatches v2 WS events into Pinia store mutations.
 *
 * The router is a pure function of (store, envelope) → store mutation.
 * No inference, no dedup, no re-parenting — just direct Map updates.
 */
export function useChatEventRouter(store: Store) {
  function dispatch(env: V2WsEnvelope) {
    if (env.type !== 'v2_event') return;
    handleKind(env.kind, env.payload);
  }

  function handleKind(kind: EventKind, payload: unknown) {
    const p = payload as Record<string, unknown>;
    switch (kind) {
      // Task events
      case 'task.created':
      case 'task.updated':
      case 'task.completed':
      case 'task.failed':
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
          store.appendStepDelta(p.StepID as string, p.DeltaField as 'content' | 'reasoning', p.DeltaChunk as string);
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

      // Phase 3b-D Task 12: system-domain events are intercepted in
      // useChatWorkspace.handleV2Event (side-effect routing via
      // inboundActivityEventHandler) and never reach this router. The cases
      // are listed here for exhaustiveness and to prevent future regressions
      // if the intercept is removed without re-adding handling.
      case 'system.run_status':
      case 'system.heartbeat':
      case 'system.notice':
        break;

      // Phase 3b-D: activity.bridge wraps a v1 ActivityEvent and is intercepted
      // in useChatWorkspace.handleV2Event (routed to handleActivityEvent for
      // dedup + system-event routing + sender reset). Listed here for
      // exhaustiveness; never reaches this router.
      case 'activity.bridge':
        break;

      default:
        // Unknown event kind — silently ignore (forward compatibility)
        break;
    }
  }

  return { dispatch };
}
