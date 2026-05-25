import type { TaskStatus } from "../types";

export type GraphTaskKanbanColumnDef = {
  key: string;
  label: string;
  statuses: TaskStatus[];
};

export const GRAPH_TASK_KANBAN_COLUMNS: GraphTaskKanbanColumnDef[] = [
  { key: "pending", label: "待处理", statuses: ["TASK_PENDING", "TASK_PENDING_ASSIGNMENT"] },
  { key: "active", label: "执行中", statuses: ["TASK_CLAIMED"] },
  { key: "review", label: "待审核", statuses: ["TASK_REVIEW_REQUIRED"] },
  { key: "done", label: "已完成", statuses: ["TASK_COMPLETE"] },
  {
    key: "issue",
    label: "异常",
    statuses: ["TASK_BLOCKED", "TASK_FAILED", "TASK_TIMED_OUT", "TASK_CANCELLED", "TASK_CRASHED"],
  },
];

export const GRAPH_TASK_KANBAN_EMPTY_HINT = {
  title: "Graph 执行中尚未产生任务卡片。",
  detail: "含 Agent 节点的 Graph 在节点激活时会自动创建任务；Worker Agent 可通过 kanban_* 工具更新状态。",
};

export type KanbanAdminAction = "unblock" | "approve";

/** Map drag target column + source status to an admin RPC action. */
export function kanbanAdminActionForDrop(
  targetColumnKey: string,
  taskStatus: TaskStatus,
): KanbanAdminAction | null {
  if (targetColumnKey === "pending" && taskStatus === "TASK_BLOCKED") {
    return "unblock";
  }
  if (targetColumnKey === "done" && taskStatus === "TASK_REVIEW_REQUIRED") {
    return "approve";
  }
  return null;
}
