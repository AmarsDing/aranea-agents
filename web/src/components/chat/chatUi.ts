import type { EntityItem } from './ChatEntityGroup.vue';

/** EntityItem 是否处于"工作中"状态 */
export function isEntityWorking(item: EntityItem) {
  return /work|run|busy|ing/i.test(item.status || '');
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
    case 'escalating':
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
