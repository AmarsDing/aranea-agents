// web/src/features/chat/composables/useTaskTree.ts
import { computed } from 'vue';
import type { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task, Turn, Step, TeamStage, PlanBoard } from '../v2Types';

type Store = ReturnType<typeof useChatActivityStore>;

export interface TurnTree {
  turn: Turn;
  steps: Step[];
}

export interface TaskTree {
  task: Task;
  turnTrees: TurnTree[];
  teamStages: TeamStage[];
  planBoards: PlanBoard[];
}

/**
 * useTaskTree builds a TaskTree from the Pinia store by filtering
 * entities by task_id and sorting by seq. No inference, no re-parenting.
 */
export function useTaskTree(store: Store) {
  function buildTaskTree(taskId: string): TaskTree | null {
    const task = store.tasks.get(taskId);
    if (!task) return null;

    const turns = store.getTaskTurns(taskId);
    const turnTrees: TurnTree[] = turns.map((turn) => ({
      turn,
      steps: store.getTurnSteps(turn.ID),
    }));

    const teamStages = store.getTaskTeamStages(taskId);
    const planBoards = store.getTaskPlanBoards(taskId);

    return { task, turnTrees, teamStages, planBoards };
  }

  return { buildTaskTree };
}
