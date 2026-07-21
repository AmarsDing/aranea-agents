import type { TaskStatus } from '../types';

export type GraphTaskKanbanColumnDef = {
  key: string;
  labelKey: string;
  statuses: TaskStatus[];
};

export const GRAPH_TASK_KANBAN_COLUMNS: GraphTaskKanbanColumnDef[] = [
  { key: 'pending', labelKey: 'graphs.kanbanColumnPending', statuses: ['TASK_PENDING', 'TASK_PENDING_ASSIGNMENT'] },
  { key: 'active', labelKey: 'graphs.kanbanColumnActive', statuses: ['TASK_CLAIMED'] },
  { key: 'review', labelKey: 'graphs.kanbanColumnReview', statuses: ['TASK_REVIEW_REQUIRED'] },
  { key: 'done', labelKey: 'graphs.kanbanColumnDone', statuses: ['TASK_COMPLETE'] },
  {
    key: 'issue',
    labelKey: 'graphs.kanbanColumnIssue',
    statuses: ['TASK_BLOCKED', 'TASK_FAILED', 'TASK_TIMED_OUT', 'TASK_CANCELLED', 'TASK_CRASHED'],
  },
];

export const GRAPH_TASK_KANBAN_EMPTY_HINT = {
  titleKey: 'graphs.kanbanEmptyTitle',
  detailKey: 'graphs.kanbanEmptyDetail',
};

export type KanbanAdminAction = 'unblock' | 'approve';

/** Map drag target column + source status to an admin RPC action. */
export function kanbanAdminActionForDrop(targetColumnKey: string, taskStatus: TaskStatus): KanbanAdminAction | null {
  if (targetColumnKey === 'pending' && taskStatus === 'TASK_BLOCKED') {
    return 'unblock';
  }
  if (targetColumnKey === 'done' && taskStatus === 'TASK_REVIEW_REQUIRED') {
    return 'approve';
  }
  return null;
}
