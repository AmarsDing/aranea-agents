/**
 * 概览页共享时间格式化：绝对时间优先（诚实、无歧义），跨组件复用。
 * 空/非法输入统一回退 '—'。
 */
export function fmtDateTime(iso: string | null | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleString();
}

export function fmtClock(iso: string | null | undefined): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '—';
  return d.toLocaleTimeString();
}
