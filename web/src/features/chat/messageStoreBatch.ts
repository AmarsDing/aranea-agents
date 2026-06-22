import type { Message } from './types';

export type MessageWriter = {
  /**
   * Patch an existing message by id. The patcher receives the current
   * (possibly already patched in this batch) message and should return the
   * replacement, or undefined to leave it unchanged.
   */
  update: (id: string, patch: (msg: Message) => Message | undefined) => void;
  /**
   * Replace the whole message array in one batch. Used by inbound sync where
   * helpers operate on the full array (insert + update combined).
   */
  batchPatch: (patch: (msgs: Message[]) => Message[]) => void;
  /** Append a new message in the next flush. */
  insert: (msg: Message) => void;
  flushSync: () => void;
  dispose: () => void;
};

/**
 * Coalesce rapid WS tool/text patches into one render per animation frame.
 *
 * Instead of applying every single delta with a full array copy, we accumulate
 * per-message patches and pending inserts, then clone the underlying array
 * exactly once per animation frame. This avoids O(n) findIndex scans on every
 * delta and prevents intermediate array allocations from piling up.
 */
export function createMessageBatchWriter(
  getMessages: () => Message[],
  setMessages: (rows: Message[]) => void,
): MessageWriter {
  let pendingBase: Message[] | null = null;
  const pendingPatches = new Map<string, Message>();
  const pendingInserts: Message[] = [];
  let rafId = 0;
  let disposed = false;

  function flushSync() {
    if (rafId) {
      cancelAnimationFrame(rafId);
      rafId = 0;
    }
    if (!pendingBase && pendingPatches.size === 0 && pendingInserts.length === 0) return;

    const base = pendingBase ?? getMessages();
    const next = [...base];
    const indexById = new Map<string, number>();
    for (let i = 0; i < next.length; i++) {
      indexById.set(next[i].id, i);
    }

    // If a message was inserted and then patched in the same batch, the
    // patched version should be pushed, not the original insert.
    for (let i = 0; i < pendingInserts.length; i++) {
      const replacement = pendingPatches.get(pendingInserts[i].id);
      if (replacement) {
        pendingInserts[i] = replacement;
        pendingPatches.delete(pendingInserts[i].id);
      }
    }

    // Apply remaining patches to existing messages.
    for (const [id, replacement] of pendingPatches) {
      const idx = indexById.get(id);
      if (idx !== undefined) {
        next[idx] = replacement;
      } else {
        next.push(replacement);
      }
    }

    // Append inserts that are not already in the base array.
    for (const ins of pendingInserts) {
      if (!indexById.has(ins.id)) {
        next.push(ins);
      }
    }

    setMessages(next);
    pendingBase = null;
    pendingPatches.clear();
    pendingInserts.length = 0;
  }

  function schedule() {
    if (rafId) return;
    rafId = requestAnimationFrame(() => {
      rafId = 0;
      if (disposed) return;
      flushSync();
    });
  }

  function update(id: string, patch: (msg: Message) => Message | undefined) {
    if (disposed) return;
    if (!pendingBase) pendingBase = getMessages();

    // Use the latest pending value so consecutive patches to the same id
    // compose on top of each other instead of overwriting on the base row.
    let msg: Message | undefined = pendingPatches.get(id);
    if (!msg) msg = pendingInserts.find((m) => m.id === id);
    if (!msg) msg = pendingBase.find((m) => m.id === id);
    if (!msg) return;

    const replacement = patch(msg);
    if (!replacement) return;
    pendingPatches.set(id, replacement);
    schedule();
  }

  function batchPatch(patch: (msgs: Message[]) => Message[]) {
    if (disposed) return;
    const base = pendingBase ?? getMessages();
    pendingBase = patch([...base]);
    pendingPatches.clear();
    pendingInserts.length = 0;
    schedule();
  }

  function insert(msg: Message) {
    if (disposed) return;
    pendingInserts.push(msg);
    schedule();
  }

  function dispose() {
    disposed = true;
    if (rafId) {
      cancelAnimationFrame(rafId);
      rafId = 0;
    }
    pendingBase = null;
    pendingPatches.clear();
    pendingInserts.length = 0;
  }

  return { update, batchPatch, insert, flushSync, dispose };
}
