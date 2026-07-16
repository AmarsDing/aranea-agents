import { computed, ref } from 'vue';
import { useGraphStream } from './useGraphStream';
import type { GraphStepSnapshot, Task } from '../types';
import { buildExecNodeStatesFromGraphNodes, seedGraphNodeStatesFromSteps } from './graphExecutionProjection';
import { stepEventFromEnvelopeMetadata, upsertStepFromStreamEvent } from './stepStreamProjection';
import { applyTaskStatusMetadata, seedTaskMap, tasksToList } from '../tasks/taskStreamProjection';

function emptyTask(taskId: string, executionId: string, nodeId: string): Task {
  return {
    taskId,
    nodeId,
    executionId,
    assignee: '',
    status: 'TASK_PENDING',
    context: '',
    input: '',
    output: '',
    summary: '',
    metadata: '',
    requiredRole: '',
    assignmentMode: '',
    createdAt: '',
    claimedAt: '',
    completedAt: '',
  };
}

export function useGraphExecutionStream(
  sessionId: string,
  graphId: string,
  executionId: string,
  initialSteps: GraphStepSnapshot[] = [],
) {
  const stream = useGraphStream(sessionId, graphId, executionId);
  const tasks = ref(new Map<string, Task>());
  const liveSteps = ref<GraphStepSnapshot[]>([...initialSteps]);

  if (initialSteps.length > 0) {
    const seeded = seedGraphNodeStatesFromSteps(initialSteps);
    for (const [id, node] of seeded.entries()) {
      stream.execution.value.nodes.set(id, { ...stream.execution.value.nodes.get(id), ...node });
    }
  }

  function applyStepEvent(meta: Record<string, unknown>, status: string) {
    if (String(meta?.execution_id ?? executionId) !== executionId) return;
    const event = stepEventFromEnvelopeMetadata(meta, status);
    if (!event) return;
    liveSteps.value = upsertStepFromStreamEvent(liveSteps.value, event);
  }

  // Graph lifecycle arrives as v2 system.notice (NoticeType = stage).
  stream.onGraphNotice((stage, meta) => {
    switch (stage) {
      case 'node_start':
        applyStepEvent(meta, 'running');
        break;
      case 'node_end':
        applyStepEvent(meta, 'completed');
        break;
      case 'node_error':
        applyStepEvent(meta, 'failed');
        break;
      case 'task_status': {
        if (String(meta?.execution_id ?? '') !== executionId) return;
        const taskId = String(meta?.task_id ?? '');
        if (!taskId) return;
        const nodeId = String(meta?.node_id ?? '');
        const next = new Map(tasks.value);
        const existing = next.get(taskId) ?? emptyTask(taskId, executionId, nodeId);
        next.set(taskId, applyTaskStatusMetadata(existing, meta));
        tasks.value = next;
        break;
      }
      default:
        break;
    }
  });

  const execNodeStates = computed(() => buildExecNodeStatesFromGraphNodes(stream.execution.value.nodes));
  const liveStatus = computed(() => stream.execution.value.status);
  const taskList = computed(() => tasksToList(tasks.value));

  function seedTasks(items: Task[]) {
    tasks.value = seedTaskMap(items);
  }

  function upsertTask(task: Task) {
    if (!task.taskId) return;
    const next = new Map(tasks.value);
    next.set(task.taskId, task);
    tasks.value = next;
  }

  return {
    execNodeStates,
    liveStatus,
    liveSteps,
    executionSummary: stream.executionSummary,
    interrupt: stream.interrupt,
    streamConnected: stream.connected,
    tasks,
    taskList,
    seedTasks,
    upsertTask,
    clearInterrupt: stream.clearInterrupt,
    disconnect: stream.disconnect,
  };
}
