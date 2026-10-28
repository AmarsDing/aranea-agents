import { computed, ref, watch, type Ref } from 'vue';
import { useActivityQueries } from './useActivityQueries';
import { isSystemInternalNotice } from '../noticeFilter';
import { isPlanAndExecuteTool } from '../executionCardHelpers';
import { useUiConfigStore } from '../../../stores/uiConfig';
import type { TaskWorkspaceProcessView } from '../types';

interface ProcessOptions {
  enabled: () => boolean;
  sessionId: () => string | undefined;
  taskId: Ref<string | null>;
  turnId: Ref<string | null>;
}

export function useTaskWorkspaceProcess(options: ProcessOptions) {
  const queries = useActivityQueries();
  const ui = useUiConfigStore();
  const tasks = computed(() => (options.sessionId() ? queries.getSessionTasks(options.sessionId()!) : []));
  const task = computed(() => {
    if (options.taskId.value) return tasks.value.find((row) => row.ID === options.taskId.value) ?? null;
    return (
      tasks.value.filter((row) => row.Status === 'pending' || row.Status === 'running').at(-1) ??
      tasks.value.at(-1) ??
      null
    );
  });
  const turns = computed(() =>
    task.value
      ? queries
          .getTaskTurns(task.value.ID)
          .filter(
            (row) =>
              row.TaskID === task.value!.ID &&
              (row.SessionID === options.sessionId() || row.SpiritSessionID === options.sessionId()),
          )
      : [],
  );
  const turn = computed(() =>
    options.turnId.value
      ? (turns.value.find((row) => row.ID === options.turnId.value) ?? null)
      : (turns.value.at(-1) ?? null),
  );
  watch(
    () => [options.taskId.value, options.turnId.value, turns.value.at(-1)?.ID] as const,
    ([taskId, turnId, latest]) => {
      if (taskId && !turnId && latest) options.turnId.value = latest;
    },
    { immediate: true },
  );
  const steps = computed(() => {
    if (!task.value) return [];
    const rows = turn.value
      ? queries
          .getTurnSteps(turn.value.ID)
          .filter(
            (row) =>
              row.TaskID === task.value!.ID && row.TurnID === turn.value!.ID && row.SessionID === turn.value!.SessionID,
          )
      : options.turnId.value
        ? []
        : queries
            .getTaskOrphanSteps(task.value.ID)
            .filter((row) => row.TaskID === task.value!.ID && row.SessionID === options.sessionId());
    return rows.filter((row) => {
      if (isSystemInternalNotice(row.Kind, row.NoticeType)) return false;
      if (row.Kind === 'reply' && row.Status !== 'running' && !row.Content?.trim()) return false;
      return row.Kind !== 'action' || ui.showToolCalls || isPlanAndExecuteTool(row.ToolName);
    });
  });
  const state = ref<TaskWorkspaceProcessView['state']>('idle');
  const retryNonce = ref(0);
  let generation = 0;
  // Explicit load transitions: selection/retry -> loading -> ready/error;
  // inactive or empty scope -> idle. Cached hydration can enter ready directly.
  watch(
    () =>
      [
        options.enabled(),
        options.sessionId(),
        task.value?.ID,
        retryNonce.value,
        task.value ? queries.isTaskHydrated(task.value.ID) : false,
      ] as const,
    async ([enabled, sid, taskId], _previous, onCleanup) => {
      const own = ++generation;
      onCleanup(() => generation++);
      const current = () =>
        own === generation && options.enabled() && options.sessionId() === sid && task.value?.ID === taskId;
      state.value = 'idle';
      if (!enabled || !sid || !taskId) return;
      if (queries.isTaskHydrated(taskId)) {
        state.value = 'ready';
        return;
      }
      state.value = 'loading';
      try {
        await queries.hydrateTask(taskId);
        if (current()) state.value = queries.isTaskHydrated(taskId) ? 'ready' : 'error';
      } catch {
        if (current()) state.value = 'error';
      }
    },
    { immediate: true },
  );
  const view = computed<TaskWorkspaceProcessView>(() => ({
    task: task.value,
    turn: turn.value,
    turns: turns.value,
    steps: steps.value,
    state: state.value,
    manual: options.taskId.value !== null || options.turnId.value !== null,
    missingSelection: (!!options.taskId.value && !task.value) || (!!options.turnId.value && !turn.value),
    canLocateTurn: !!turn.value && turn.value.SessionID === options.sessionId() && !turn.value.TeamStageID,
    member: turn.value
      ? ([...queries.memberSessions().values()].find(
          (row) =>
            row.TaskID === task.value?.ID &&
            row.SessionID === turn.value!.SessionID &&
            row.SpiritSessionID === options.sessionId(),
        ) ?? null)
      : null,
  }));
  function selectTask(taskId: string) {
    if (!tasks.value.some((row) => row.ID === taskId)) return;
    options.taskId.value = taskId;
    options.turnId.value = null;
  }
  function selectTurn(turnId: string) {
    if (!task.value || !turns.value.some((row) => row.ID === turnId)) return;
    // Pin the task too: a new task must not invalidate an explicitly chosen turn.
    options.taskId.value = task.value.ID;
    options.turnId.value = turnId;
  }
  function followCurrent() {
    options.taskId.value = null;
    options.turnId.value = null;
  }
  function selectLocation(taskId: string, stepId?: string) {
    if (!tasks.value.some((row) => row.ID === taskId)) return;
    selectTask(taskId);
    const step = stepId ? queries.steps().get(stepId) : null;
    if (
      step?.TaskID === taskId &&
      step.SessionID === options.sessionId() &&
      turns.value.some((row) => row.ID === step.TurnID)
    )
      options.turnId.value = step.TurnID;
  }
  return { view, selectTask, selectTurn, selectLocation, followCurrent, retry: () => retryNonce.value++ };
}
