import type { BatchOperationResult, BatchPreviewResult } from './types';

/** Rows matched at resolve time but not updated (state changed before SQL, or partial chunk skip). */
export function skippedAtExecute(result: BatchOperationResult): number {
  const accounted = result.processed + result.failed_ids.length;
  return Math.max(0, result.matched - accounted);
}

export function formatBatchNotifyMessage(
  action: 'archive' | 'delete',
  result: BatchOperationResult,
  requested?: number,
): string {
  const verb = action === 'archive' ? '归档' : '删除';
  let msg = `已${verb} ${result.processed} 个会话`;
  const extras: string[] = [];
  if (requested != null && requested > 0 && requested !== result.processed) {
    extras.push(`请求 ${requested} 项`);
  }
  if (result.matched > 0 && result.matched !== result.processed) {
    extras.push(`匹配 ${result.matched} 项`);
  }
  const executeSkipped = skippedAtExecute(result);
  if (executeSkipped > 0) {
    extras.push(`执行时跳过 ${executeSkipped}（状态已变化）`);
  }
  if (result.skipped_running > 0) {
    extras.push(`解析时跳过运行中 ${result.skipped_running}`);
  }
  if (result.skipped_not_found > 0) {
    extras.push(`未找到 ${result.skipped_not_found}`);
  }
  if (result.failed_ids.length > 0) {
    extras.push(`失败 ${result.failed_ids.length}`);
  }
  if (result.truncated) {
    extras.push('扫描达上限，请再次执行以处理剩余');
  }
  if (extras.length > 0) {
    msg += `（${extras.join('；')}）`;
  }
  return msg;
}

export function formatBatchPreviewHint(preview: BatchPreviewResult): string {
  const parts = [`将处理 ${preview.matched} 个会话`];
  if (preview.skipped_running > 0) {
    parts.push(`运行中将跳过 ${preview.skipped_running}`);
  }
  if (preview.skipped_not_found > 0) {
    parts.push(`未找到 ${preview.skipped_not_found}`);
  }
  if (preview.truncated) {
    parts.push('扫描达上限，结果可能不完整');
  }
  return parts.join('；');
}
