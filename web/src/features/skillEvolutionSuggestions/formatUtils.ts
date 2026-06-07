/** Format a timestamp string for display. */
export function formatDate(ts: unknown): string {
  if (!ts || typeof ts !== 'string') return '—';
  try {
    return new Date(ts).toLocaleString('zh-CN', { hour12: false });
  } catch {
    return ts;
  }
}
