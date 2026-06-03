import type { Task, TaskStatus } from '../types';

const WS_STATUS_TO_TASK: Record<string, TaskStatus> = {
  pending: 'TASK_PENDING',
  claimed: 'TASK_CLAIMED',
  complete: 'TASK_COMPLETE',
  blocked: 'TASK_BLOCKED',
  review_required: 'TASK_REVIEW_REQUIRED',
  failed: 'TASK_FAILED',
  timed_out: 'TASK_TIMED_OUT',
  cancelled: 'TASK_CANCELLED',
  crashed: 'TASK_CRASHED',
  pending_assignment: 'TASK_PENDING_ASSIGNMENT',
};

export function wsTaskStatusToTaskStatus(raw: string): TaskStatus | null {
  const key = raw.trim().toLowerCase();
  if (key.startsWith('task_')) {
    return key.toUpperCase() as TaskStatus;
  }
  return WS_STATUS_TO_TASK[key] ?? null;
}

export function seedTaskMap(items: Task[]): Map<string, Task> {
  const map = new Map<string, Task>();
  for (const task of items) {
    if (task.taskId) map.set(task.taskId, { ...task });
  }
  return map;
}

export function applyTaskStatusMetadata(task: Task, metadata: Record<string, unknown>): Task {
  const next = { ...task };
  const wsStatus = String(metadata.task_status ?? '');
  const mapped = wsTaskStatusToTaskStatus(wsStatus);
  if (mapped) next.status = mapped;
  if (metadata.node_id) next.nodeId = String(metadata.node_id);
  if (metadata.assignee) next.assignee = String(metadata.assignee);
  if (metadata.summary) next.summary = String(metadata.summary);
  if (metadata.execution_id) next.executionId = String(metadata.execution_id);
  return next;
}

export function tasksToList(map: Map<string, Task>): Task[] {
  return [...map.values()].sort((a, b) => a.nodeId.localeCompare(b.nodeId));
}
