/**
 * Shared utilities for Agent Tree Timeline components.
 */

/** Format milliseconds into a human-readable duration string. */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const rem = sec % 60;
  return `${m}m ${rem}s`;
}
