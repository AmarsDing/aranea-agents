import { ref } from "vue";
import { useQuasar } from "quasar";
import type { Task, TaskComment, TaskEvent, TaskLog, TaskRun } from "./types";
import { useGraphStore } from "../../stores/graph";
import { listTaskEvents } from "./api";

export function useGraphRunTasks(
  executionId: () => string,
  seedTasks: (items: Task[]) => void,
  upsertTask: (task: Task) => void,
) {
  const $q = useQuasar();
  const graphStore = useGraphStore();

  const tasksLoading = ref(false);
  const selectedTaskId = ref<string | null>(null);
  const taskDrawerOpen = ref(false);
  const activeTask = ref<Task | null>(null);
  const taskComments = ref<TaskComment[]>([]);
  const taskLogs = ref<TaskLog[]>([]);
  const taskRuns = ref<TaskRun[]>([]);
  const taskEvents = ref<TaskEvent[]>([]);
  const taskDetailLoading = ref(false);
  const taskActionLoading = ref(false);

  async function loadTasks(execId?: string) {
    const id = execId ?? executionId();
    if (!id) return;
    tasksLoading.value = true;
    try {
      const result = await graphStore.fetchTasks(id);
      seedTasks(result.items ?? []);
    } catch {
      $q.notify({ type: "negative", message: "加载任务失败" });
    } finally {
      tasksLoading.value = false;
    }
  }

  async function openTaskDetail(taskId: string, onFocusNode?: (nodeId: string) => void) {
    selectedTaskId.value = taskId;
    taskDrawerOpen.value = true;
    taskDetailLoading.value = true;
    try {
      activeTask.value = await graphStore.fetchTask(taskId);
      if (activeTask.value?.nodeId) {
        onFocusNode?.(activeTask.value.nodeId);
      }
      const execId = executionId();
      const [comments, logs, runs, events] = await Promise.all([
        graphStore.fetchTaskComments(taskId),
        graphStore.fetchTaskLogs(taskId),
        graphStore.fetchTaskRuns(taskId),
        execId ? listTaskEvents(execId, taskId) : Promise.resolve([]),
      ]);
      taskComments.value = comments;
      taskLogs.value = logs;
      taskRuns.value = runs;
      taskEvents.value = events;
    } catch {
      $q.notify({ type: "negative", message: "加载任务详情失败" });
    } finally {
      taskDetailLoading.value = false;
    }
  }

  function focusTaskForNode(tasks: Task[], nodeId: string | null) {
    if (!nodeId) {
      selectedTaskId.value = null;
      return;
    }
    const match = tasks.find((task) => task.nodeId === nodeId);
    selectedTaskId.value = match?.taskId ?? null;
  }

  async function onClaimTask(payload: { taskId: string; agentKey: string }) {
    taskActionLoading.value = true;
    try {
      const task = await graphStore.claimTaskByAgent(payload.taskId, payload.agentKey);
      upsertTask(task);
      activeTask.value = task;
      $q.notify({ type: "positive", message: "任务已认领" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "认领失败" });
    } finally {
      taskActionLoading.value = false;
    }
  }

  async function onSubmitTask(payload: { taskId: string; output: string; summary: string }) {
    taskActionLoading.value = true;
    try {
      const task = await graphStore.submitTask(payload.taskId, payload.output, payload.summary);
      upsertTask(task);
      activeTask.value = task;
      $q.notify({ type: "positive", message: "任务结果已提交" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "提交失败" });
    } finally {
      taskActionLoading.value = false;
    }
  }

  async function onReportBlocked(payload: { taskId: string; reason: string }) {
    taskActionLoading.value = true;
    try {
      const task = await graphStore.reportTaskBlocked(payload.taskId, payload.reason);
      upsertTask(task);
      activeTask.value = task;
      $q.notify({ type: "warning", message: "已上报阻塞" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "上报失败" });
    } finally {
      taskActionLoading.value = false;
    }
  }

  async function onUnblockTask(payload: { taskId: string; comment: string }) {
    taskActionLoading.value = true;
    try {
      const task = await graphStore.unblockTaskByOperator(payload.taskId, payload.comment);
      upsertTask(task);
      activeTask.value = task;
      $q.notify({ type: "positive", message: "任务已解除阻塞" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "解除阻塞失败" });
    } finally {
      taskActionLoading.value = false;
    }
  }

  async function onReviewTask(payload: { taskId: string; reviewerAgent: string; approved: boolean; comment: string }) {
    taskActionLoading.value = true;
    try {
      const task = await graphStore.reviewTask(payload.taskId, payload.reviewerAgent, payload.approved, payload.comment);
      upsertTask(task);
      activeTask.value = task;
      $q.notify({ type: "positive", message: payload.approved ? "审核已通过" : "审核已拒绝" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "审核失败" });
    } finally {
      taskActionLoading.value = false;
    }
  }

  async function onAddTaskComment(payload: { taskId: string; author: string; content: string }) {
    try {
      const comment = await graphStore.postTaskComment(payload.taskId, payload.author, payload.content);
      taskComments.value = [...taskComments.value, comment];
      $q.notify({ type: "positive", message: "评论已添加" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "评论失败" });
    }
  }

  async function onKanbanAdminAction(payload: { taskId: string; action: "unblock" | "approve" }) {
    if (payload.action === "unblock") {
      await onUnblockTask({ taskId: payload.taskId, comment: "manual unblock from kanban" });
      return;
    }
    await onReviewTask({
      taskId: payload.taskId,
      reviewerAgent: "graph-operator",
      approved: true,
      comment: "manual approve from kanban",
    });
  }

  return {
    tasksLoading,
    selectedTaskId,
    taskDrawerOpen,
    activeTask,
    taskComments,
    taskLogs,
    taskRuns,
    taskEvents,
    taskDetailLoading,
    taskActionLoading,
    loadTasks,
    openTaskDetail,
    focusTaskForNode,
    onClaimTask,
    onSubmitTask,
    onReportBlocked,
    onUnblockTask,
    onReviewTask,
    onAddTaskComment,
    onKanbanAdminAction,
  };
}
