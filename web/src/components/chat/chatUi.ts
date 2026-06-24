import type { EntityItem } from './ChatEntityGroup.vue';

/**
 * 工作中状态白名单（精确匹配）。
 * 修复 P0 #5：原 /work|run|busy|ing/i 的 `ing` 子串会误匹配 pending/interesting/swing 等。
 * isEntityInactive 保持原正则不变（其子串 inactive/disabled/stop/pause 无误匹配风险）。
 */
const WORKING_STATUSES = new Set(['working', 'running', 'busy']);

/** EntityItem 是否处于"工作中"状态 */
export function isEntityWorking(item: EntityItem) {
  return WORKING_STATUSES.has((item.status || '').trim().toLowerCase());
}

/** EntityItem 是否处于"停用"状态 */
export function isEntityInactive(item: EntityItem) {
  return /inactive|disabled|stop|pause/i.test(item.status || '');
}

/** EntityItem 状态图标 */
export function entityStatusIconFor(item: EntityItem) {
  return isEntityWorking(item) ? 'bolt' : 'task_alt';
}

/** EntityItem 状态颜色 */
export function entityStatusColorFor(item: EntityItem) {
  if (isEntityWorking(item)) return 'negative';
  if (isEntityInactive(item)) return 'grey';
  return 'positive';
}

/** EntityItem 状态标签 */
export function entityStatusLabelFor(item: EntityItem) {
  if (isEntityWorking(item)) return '工作中';
  if (isEntityInactive(item)) return '已停用';
  return '空闲';
}

/** BackgroundJob 状态颜色 */
export function backgroundJobStatusColor(status: string) {
  switch (status) {
    case 'running':
    case 'accepted':
    case 'interactive':
    case 'durable':
      return 'info';
    case 'completed':
      return 'positive';
    case 'failed':
    case 'timeout':
      return 'negative';
    case 'async_queued':
    case 'queued':
      return 'purple';
    case 'cancelled':
      return 'warning';
    default:
      return 'grey';
  }
}
