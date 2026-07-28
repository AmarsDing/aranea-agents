/**
 * mobileTasksView — 移动版任务页视图状态判定（纯函数，便于单测）。
 *
 * 移动任务页复用桌面 TaskList（activityV2Store 数据），仅在外层决定
 * 渲染哪种视图：未选会话 / 会话无任务 / 任务列表。
 */
export type MobileTasksViewState = 'no-session' | 'empty' | 'list';

export function resolveMobileTasksView(sessionId: string | null | undefined, taskCount: number): MobileTasksViewState {
  if (!sessionId?.trim()) return 'no-session';
  return taskCount > 0 ? 'list' : 'empty';
}
