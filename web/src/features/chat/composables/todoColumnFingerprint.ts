/**
 * TD-TK-4: TodoColumn fingerprint + cross-instance state for kanban pulse
 * animation.
 *
 * Problem:
 *   The previous `TodoColumn` only watched `props.items.length`, which has
 *   two failure modes in production:
 *     1. Status changes within an unchanged-length list (pending →
 *        in_progress) didn't trigger the pulse.
 *     2. When the kanban lives inside a virtual-scrolled list, columns
 *        can be unmounted and re-mounted. The watch handler runs only
 *        on data changes while the component is mounted, so a status
 *        flip that happens during the unmount window is lost.
 *   3. The `setTimeout` pulse reset could fire after `onBeforeUnmount`,
 *      warning and visually flashing on the next mount.
 *
 * Solution:
 *   - Define a stable `TodoColumnFingerprint` derived from
 *     `id:status:content` triples. Two `TodoItem[]` arrays share a
 *     fingerprint iff their items are identical as observed by the
 *     kanban animation.
 *   - Module-level `lastFingerprintByColumn` cache: even if a column
 *     unmounts, the most-recent fingerprint is preserved. On re-mount,
 *     `TodoColumn.vue` compares the current fingerprint against the
 *     stored one and replays the pulse when they differ.
 *   - Helpers are pure: no Vue refs, no timers. The component is
 *     responsible for cleanup.
 */

import type { TodoItem } from '../agentTreeTypes';

/**
 * Stable, change-detecting serialization of a list of TodoItems.
 * Order-preserving so the same set in a different order is a different
 * fingerprint (e.g., an item moving from `in_progress` column back to
 * `pending` should be a real change).
 *
 * Format: `${id}:${status}|${id}:${status}|…`
 * (Content is intentionally excluded to keep the string short —
 *  content edits are user-visible only on the item card itself.)
 */
export type TodoColumnFingerprint = string;

export function computeTodoFingerprint(items: readonly TodoItem[]): TodoColumnFingerprint {
  if (!items || items.length === 0) return '';
  const parts: string[] = [];
  for (const item of items) {
    parts.push(`${item.id}:${item.status}`);
  }
  return parts.join('|');
}

// ── Cross-instance last-seen cache ────────────────────────────────────
//
// Keyed by an opaque column id passed in by the parent (e.g. "pending",
// "in_progress", "completed"). When `TodoColumn.vue` is reused for the
// same logical column across virtual-scroll recycling, the cache survives
// the unmount/remount and the new instance can replay any missed pulse.

const lastFingerprintByColumn = new Map<string, TodoColumnFingerprint>();

/**
 * Return the fingerprint that was last observed for `columnKey`, or
 * `undefined` if none has been recorded yet.
 */
export function readLastFingerprint(columnKey: string): TodoColumnFingerprint | undefined {
  return lastFingerprintByColumn.get(columnKey);
}

/**
 * Record the current fingerprint for `columnKey` so the next mount
 * of the same column can detect a transition that happened while it
 * was unmounted.
 */
export function writeLastFingerprint(columnKey: string, fp: TodoColumnFingerprint): void {
  if (fp === '') {
    // Empty lists are not interesting — clear the slot to avoid
    // a false-positive diff when a transient empty list is observed.
    lastFingerprintByColumn.delete(columnKey);
    return;
  }
  lastFingerprintByColumn.set(columnKey, fp);
}

/**
 * Test-only helper to reset the module-level cache. Not exported from
 * the package barrel.
 */
export function __resetTodoFingerprintCache(): void {
  lastFingerprintByColumn.clear();
}
