import { computed } from 'vue';
import { useActivityQueries } from './useActivityQueries';

export function useTaskWorkspaceActivity(sessionId: () => string | undefined) {
  const queries = useActivityQueries();
  const tasks = computed(() => (sessionId() ? queries.getSessionTasks(sessionId()!) : []));
  const incomplete = computed(() => tasks.value.some((task) => !queries.isTaskHydrated(task.ID)));
  const pendingSteps = computed(() => {
    const active = new Set(
      tasks.value.filter((task) => task.Status === 'running' || task.Status === 'pending').map((task) => task.ID),
    );
    return [...queries.steps().values()].filter(
      (step) =>
        step.SessionID === sessionId() &&
        active.has(step.TaskID) &&
        ((step.Kind === 'confirm' && ['pending', 'running', 'tool_blocked', 'awaiting_input'].includes(step.Status)) ||
          (step.Kind === 'clarify' && step.Status === 'awaiting_input')),
    );
  });
  return { tasks, incomplete, pendingSteps };
}
