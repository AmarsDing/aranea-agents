/**
 * Shared utilities for Agent Tree Timeline components.
 */

/** Format milliseconds into a human-readable duration string. */
export function formatDuration(ms: number | null | undefined): string {
  if (ms == null || ms <= 0) return '';
  if (ms < 1000) return `${ms}ms`;
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const rem = sec % 60;
  return `${m}m ${rem}s`;
}

/**
 * Extract the first sentence from text and truncate for collapsed thinking summary.
 * Matches the first sentence boundary (。.!?！？\n) and caps at maxLength chars.
 */
export function truncateThinkingSummary(text: string, maxLength = 60): string {
  if (!text) return '';
  const match = text.match(/^(.+?)([。.!?！？\n])/);
  if (match) {
    const first = match[1] + match[2];
    return first.length > maxLength ? first.slice(0, maxLength) + '…' : first;
  }
  return text.length > maxLength ? text.slice(0, maxLength) + '…' : text;
}
