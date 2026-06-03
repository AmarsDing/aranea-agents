import { computed, shallowRef, type Ref } from 'vue';
import type { GraphExecution, GraphStepSnapshot } from '../types';
import { useGraphExecutionStream } from './useGraphExecutionStream';

export function useGraphRunStream(graphId: Ref<string>, execId: Ref<string>, execution: Ref<GraphExecution | null>) {
  const streamRef = shallowRef<ReturnType<typeof useGraphExecutionStream> | null>(null);

  const execNodeStates = computed(() => streamRef.value?.execNodeStates.value ?? new Map());
  const executionSummary = computed(() => streamRef.value?.executionSummary.value ?? null);
  const streamConnected = computed(() => streamRef.value?.streamConnected.value ?? false);
  const interrupt = computed(() => streamRef.value?.interrupt.value ?? null);
  const taskList = computed(() => streamRef.value?.taskList.value ?? []);
  const liveStatus = computed(() => streamRef.value?.liveStatus.value ?? execution.value?.status ?? 'loading');
  const liveSteps = computed(() => streamRef.value?.liveSteps.value ?? execution.value?.steps ?? []);

  function connectStream(steps: GraphStepSnapshot[] = execution.value?.steps ?? []) {
    streamRef.value?.disconnect();
    const sessionId = execution.value?.sessionId?.trim();
    if (!sessionId || !graphId.value || !execId.value) {
      streamRef.value = null;
      return;
    }
    streamRef.value = useGraphExecutionStream(sessionId, graphId.value, execId.value, steps);
  }

  function disconnectStream() {
    streamRef.value?.disconnect();
    streamRef.value = null;
  }

  function seedTasks(items: Parameters<NonNullable<ReturnType<typeof useGraphExecutionStream>>['seedTasks']>[0]) {
    streamRef.value?.seedTasks(items);
  }

  function upsertTask(task: Parameters<NonNullable<ReturnType<typeof useGraphExecutionStream>>['upsertTask']>[0]) {
    streamRef.value?.upsertTask(task);
  }

  function clearInterrupt() {
    streamRef.value?.clearInterrupt();
  }

  return {
    execNodeStates,
    executionSummary,
    streamConnected,
    interrupt,
    taskList,
    liveStatus,
    liveSteps,
    connectStream,
    disconnectStream,
    seedTasks,
    upsertTask,
    clearInterrupt,
  };
}
