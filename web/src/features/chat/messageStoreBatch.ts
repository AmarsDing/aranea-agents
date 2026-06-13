import type { Message } from './types';

type MessageWriter = {
  update: (updater: (current: Message[]) => Message[]) => void;
  flushSync: () => void;
  dispose: () => void;
};

/** Coalesce rapid WS tool/text patches into one render per animation frame. */
export function createMessageBatchWriter(
  getMessages: () => Message[],
  setMessages: (rows: Message[]) => void,
): MessageWriter {
  let pending: Message[] | null = null;
  let rafId = 0;
  let disposed = false;

  function flushSync() {
    if (rafId) {
      cancelAnimationFrame(rafId);
      rafId = 0;
    }
    if (pending) {
      setMessages(pending);
      pending = null;
    }
  }

  function schedule() {
    if (rafId) return;
    rafId = requestAnimationFrame(() => {
      rafId = 0;
      if (pending && !disposed) {
        setMessages(pending);
        pending = null;
      }
    });
  }

  function update(updater: (current: Message[]) => Message[]) {
    if (disposed) return;
    const base = pending ?? getMessages();
    pending = updater(base);
    schedule();
  }

  function dispose() {
    disposed = true;
    if (rafId) {
      cancelAnimationFrame(rafId);
      rafId = 0;
    }
    pending = null;
  }

  return { update, flushSync, dispose };
}
