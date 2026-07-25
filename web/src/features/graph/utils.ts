export function truncate(text: string, max = 120): string {
  return text.length > max ? `${text.slice(0, max)}…` : text;
}

/**
 * R2-A 卡片/详情面板状态徽章派生。
 * 列表页无逐图执行数据（避免 N+1），按内容推导：
 * draft（灰）= 无节点；ready（绿）= 已有节点。
 */
export type GraphCardStatus = 'draft' | 'ready';

export function deriveGraphStatus(graph: { nodes?: unknown[] | null }): GraphCardStatus {
  return (graph.nodes?.length ?? 0) === 0 ? 'draft' : 'ready';
}

export const GRAPH_CARD_STATUS_LABEL_KEYS: Record<GraphCardStatus, string> = {
  draft: 'graphs.cardStatusDraft',
  ready: 'graphs.cardStatusReady',
};

/**
 * R2-A.3 节点构成 chips：按计数降序（同计数按类型名升序，保证确定性），
 * 超出 max 类折叠为 +N（N = 剩余类型数）。
 */
export type GraphCompositionChip = { type: string; count: number };

export function buildCompositionChips(
  counts: Partial<Record<string, number | undefined>>,
  max = 4,
): { chips: GraphCompositionChip[]; overflow: number } {
  const entries = Object.entries(counts)
    .filter(([, count]) => (count ?? 0) > 0)
    .map(([type, count]) => ({ type, count: count ?? 0 }))
    .sort((a, b) => b.count - a.count || a.type.localeCompare(b.type));
  return {
    chips: entries.slice(0, max),
    overflow: Math.max(0, entries.length - max),
  };
}

export function formatTime(dateStr: string): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  return Number.isNaN(date.getTime()) ? dateStr : date.toLocaleString();
}

export function relativeTime(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return '刚刚';
  if (minutes < 60) return `${minutes}分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}小时前`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}天前`;
  const months = Math.floor(days / 30);
  return `${months}个月前`;
}

export function stepIcon(status: string): string {
  if (status === 'completed') return 'check_circle';
  if (status === 'error' || status === 'failed') return 'error';
  if (status === 'running') return 'sync';
  return 'radio_button_unchecked';
}

export function stepColor(status: string): string {
  if (status === 'completed') return 'positive';
  if (status === 'error' || status === 'failed') return 'negative';
  if (status === 'running') return 'blue';
  return 'grey';
}

export function execDuration(startedAt: string, finishedAt: string): string {
  if (!startedAt) return '';
  const start = new Date(startedAt).getTime();
  const end = finishedAt ? new Date(finishedAt).getTime() : Date.now();
  const ms = end - start;
  if (ms < 0) return '';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  const min = Math.floor(ms / 60000);
  const sec = Math.floor((ms % 60000) / 1000);
  return `${min}m${sec}s`;
}
