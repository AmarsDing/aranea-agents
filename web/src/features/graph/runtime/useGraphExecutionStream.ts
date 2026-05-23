import { computed, ref } from "vue";
import type { Envelope } from "../../chat/envelope";
import { useGraphStream } from "../../chat/useEnvelopeStream";
import type { GraphStepSnapshot, Task } from "../types";
import {
  buildExecNodeStatesFromGraphNodes,
  seedGraphNodeStatesFromSteps,
} from "./graphExecutionProjection";
import {
  stepEventFromEnvelopeMetadata,
  upsertStepFromStreamEvent,
} from "./stepStreamProjection";
import {
  applyTaskStatusMetadata,
  seedTaskMap,
  tasksToList,
} from "../tasks/taskStreamProjection";

function emptyTask(taskId: string, executionId: string, nodeId: string): Task {
  return {
    taskId,
    nodeId,
    executionId,
    assignee: "",
    status: "TASK_PENDING",
    context: "",
    input: "",
    output: "",
    summary: "",
    metadata: "",
    requiredRole: "",
    assignmentMode: "",
    createdAt: "",
    claimedAt: "",
    completedAt: "",
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

  function applyStepEvent(env: Envelope, status: string) {
    if (String(env.metadata?.execution_id ?? executionId) !== executionId) return;
    const event = stepEventFromEnvelopeMetadata(env.metadata as Record<string, unknown>, status);
    if (!event) return;
    liveSteps.value = upsertStepFromStreamEvent(liveSteps.value, event);
  }

  stream.onType("graph_node_start", (env) => {
    applyStepEvent(env, "running");
  });
  stream.onType("graph_node_end", (env) => {
    applyStepEvent(env, "completed");
  });
  stream.onType("graph_node_error", (env) => {
    applyStepEvent(env, "failed");
  });

  stream.onType("graph_task_status", (env: Envelope) => {
    if (String(env.metadata?.execution_id ?? "") !== executionId) return;
    const taskId = String(env.metadata?.task_id ?? "");
    if (!taskId) return;
    const nodeId = String(env.metadata?.node_id ?? "");
    const next = new Map(tasks.value);
    const existing = next.get(taskId) ?? emptyTask(taskId, executionId, nodeId);
    next.set(taskId, applyTaskStatusMetadata(existing, env.metadata ?? {}));
    tasks.value = next;
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
