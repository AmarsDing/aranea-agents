// web/src/features/chat/composables/useActivityQueries.ts
//
// Layer-compliance wrapper: components under src/components/ must NOT import
// Pinia stores directly (enforced by scripts/check-frontend-layer.mjs). This
// composable provides the same read-only query surface as useChatActivityStore
// so that v2 rendering components can fetch child entities without violating
// the layer boundary.
//
// The store remains the single source of truth; this composable is a thin
// accessor that keeps the `useXxxStore()` call outside of `components/`.
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type {
  Task,
  Turn,
  Step,
  TeamStage,
  TeamRun,
  MemberSession,
  PlanBoard,
  PlanStep,
  GraphStage,
  GraphNode,
} from '../v2Types';

export function useActivityQueries() {
  const store = useChatActivityStore();

  return {
    // --- Session-scoped ---
    getSessionTasks(sessionId: string): Task[] {
      return store.getSessionTasks(sessionId);
    },
    hasSessionTasks(sessionId: string): boolean {
      return store.getSessionTasks(sessionId).length > 0;
    },

    // --- Task-scoped ---
    getTaskTurns(taskId: string): Turn[] {
      return store.getTaskTurns(taskId);
    },
    getTaskOrphanSteps(taskId: string): Step[] {
      return store.getTaskOrphanSteps(taskId);
    },
    getTaskPlanBoards(taskId: string): PlanBoard[] {
      return store.getTaskPlanBoards(taskId);
    },
    getTaskTeamStages(taskId: string): TeamStage[] {
      return store.getTaskTeamStages(taskId);
    },
    getTaskGraphStages(taskId: string): GraphStage[] {
      return store.getTaskGraphStages(taskId);
    },

    // --- Turn-scoped ---
    getTurnSteps(turnId: string): Step[] {
      return store.getTurnSteps(turnId);
    },
    getTurnTeamStages(turnId: string): TeamStage[] {
      return store.getTurnTeamStages(turnId);
    },

    // --- TeamStage-scoped ---
    getTeamStageTeamRuns(teamStageId: string): TeamRun[] {
      return store.getTeamStageTeamRuns(teamStageId);
    },

    // --- TeamRun-scoped ---
    getTeamRunMemberSessions(teamRunId: string): MemberSession[] {
      return store.getTeamRunMemberSessions(teamRunId);
    },

    // --- PlanBoard-scoped ---
    getPlanBoardSteps(planBoardId: string): PlanStep[] {
      return store.getPlanBoardSteps(planBoardId);
    },

    // --- GraphStage-scoped ---
    getGraphStageNodes(graphStageId: string): GraphNode[] {
      return store.getGraphStageNodes(graphStageId);
    },
    getGraphStageByPlanBoard(planBoardId: string): GraphStage | undefined {
      return store.getGraphStageByPlanBoard(planBoardId);
    },

    // --- MemberSession-scoped ---
    getMemberSessionSteps(memberSession: MemberSession): Step[] {
      return store.getMemberSessionSteps(memberSession);
    },
    getTaskOrphanMemberSessions(taskId: string): MemberSession[] {
      return store.getTaskOrphanMemberSessions(taskId);
    },

    // --- Lazy hydration (chat history lazy load) ---
    isTaskHydrated(taskId: string): boolean {
      return store.hydratedTaskIds.has(taskId);
    },
    taskHydrationState(taskId: string): 'loading' | 'error' | undefined {
      return store.taskHydration.get(taskId);
    },
    /** 门面转发 store action：components 禁止直访 store（layer 检查）。 */
    hydrateTask(taskId: string): Promise<void> {
      return store.hydrateTask(taskId);
    },

    // --- Direct map access (for components that iterate or .get by ID) ---
    /** Read-only view of the steps map. */
    steps(): ReadonlyMap<string, Step> {
      return store.steps;
    },
    /** Read-only view of the teamStages map. */
    teamStages(): ReadonlyMap<string, TeamStage> {
      return store.teamStages;
    },
    /** Read-only view of the teamRuns map. */
    teamRuns(): ReadonlyMap<string, TeamRun> {
      return store.teamRuns;
    },
    /** Read-only view of the planSteps map. */
    planSteps(): ReadonlyMap<string, PlanStep> {
      return store.planSteps;
    },
    /** Read-only view of the memberSessions map（弹框等场景按 ID 实时查询，避免快照过期）。 */
    memberSessions(): ReadonlyMap<string, MemberSession> {
      return store.memberSessions;
    },
  };
}
