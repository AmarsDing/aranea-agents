import { afterEach, describe, expect, it, vi } from 'vitest';
import { effectScope, ref } from 'vue';
import { useFollowUpQueue } from '../useFollowUpQueue';
import type { PendingMessage } from '../../types';

const runtime = vi.hoisted(() => ({
  fetchPendingMessages: vi.fn(),
  updatePending: vi.fn(),
  cancelPending: vi.fn(),
  interruptAndSend: vi.fn(),
}));
vi.mock('../../../../stores/chat/runtimeStore', () => ({ useChatRuntimeStore: () => runtime }));
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((r, j) => {
    resolve = r;
    reject = j;
  });
  return { promise, resolve, reject };
}
function row(id = 'p', content = 'follow up'): PendingMessage {
  return { id, content, status: 'queued', created_at: '2026-10-26T00:00:00Z' };
}
const scopes: ReturnType<typeof effectScope>[] = [];
function setup() {
  const scope = effectScope();
  scopes.push(scope);
  const sid = ref<string | undefined>('A');
  const notify = vi.fn();
  const api = scope.run(() => useFollowUpQueue(sid, ref(false), notify))!;
  return { api, sid, notify, scope };
}
afterEach(() => {
  scopes.splice(0).forEach((scope) => scope.stop());
  vi.useRealTimers();
  vi.resetAllMocks();
});

describe('follow-up queue request ownership', () => {
  it('ignores an old session response and failure after switching', async () => {
    const old = deferred<PendingMessage[]>();
    const stale = deferred<PendingMessage[]>();
    runtime.fetchPendingMessages
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(stale.promise)
      .mockResolvedValueOnce([row('B')]);
    const c = setup();
    const a = c.api.refreshPendingMessages();
    const a2 = c.api.refreshPendingMessages();
    c.sid.value = 'B';
    await c.api.refreshPendingMessages();
    old.resolve([row('A')]);
    stale.reject(new Error('old failure'));
    await Promise.all([a, a2]);
    expect(c.api.pendingMessages.value.map((item) => item.id)).toEqual(['B']);
    expect(c.notify).not.toHaveBeenCalled();
  });
  it('keeps only the newest same-session fetch and preserves rows on retry failure', async () => {
    const older = deferred<PendingMessage[]>();
    runtime.fetchPendingMessages
      .mockReturnValueOnce(older.promise)
      .mockResolvedValueOnce([row('new')])
      .mockRejectedValueOnce(new Error('offline'));
    const c = setup();
    const first = c.api.refreshPendingMessages();
    await c.api.refreshPendingMessages();
    older.resolve([row('old')]);
    await first;
    expect(c.api.pendingMessages.value[0]?.id).toBe('new');
    await expect(c.api.refreshPendingMessages({ throwOnError: true })).rejects.toThrow('offline');
    expect(c.api.pendingMessages.value[0]?.id).toBe('new');
  });
  it('does not apply an old edit acknowledgement to a reused pending ID', async () => {
    const edit = deferred<boolean>();
    runtime.updatePending.mockReturnValueOnce(edit.promise);
    const c = setup();
    c.api.pendingMessages.value = [row()];
    const update = c.api.onUpdatePending('p', 'edited in A');
    c.sid.value = 'B';
    c.api.pendingMessages.value = [row('p', 'belongs to B')];
    edit.resolve(true);
    await update;
    expect(runtime.updatePending).toHaveBeenCalledWith('A', 'p', 'edited in A');
    expect(c.api.pendingMessages.value[0]?.content).toBe('belongs to B');
  });
  it('cancels completion polls when the session changes or the scope is disposed', () => {
    vi.useFakeTimers();
    const c = setup();
    c.api.watchSending(false);
    c.sid.value = 'B';
    vi.advanceTimersByTime(1001);
    expect(runtime.fetchPendingMessages).not.toHaveBeenCalled();
    c.api.watchSending(true);
    c.scope.stop();
    vi.advanceTimersByTime(501);
    expect(runtime.fetchPendingMessages).not.toHaveBeenCalled();
  });
});
