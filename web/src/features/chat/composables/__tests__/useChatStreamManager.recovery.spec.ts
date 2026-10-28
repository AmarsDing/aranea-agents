import { beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, ref } from 'vue';
import { flushPromises } from '@vue/test-utils';
import { useChatStreamManager, type StreamManagerDeps } from '../useChatStreamManager';

const fixtures = vi.hoisted(() => ({
  streams: [] as {
    sid: string;
    callbacks: Record<string, () => void>;
    connected: { value: boolean };
    connect: ReturnType<typeof vi.fn>;
  }[],
}));
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock('quasar', () => ({ useQuasar: () => ({ notify: vi.fn() }) }));
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock('../../../../stores/auth', () => ({ useAuthStore: () => ({ user: null, sessionChecked: true }) }));
vi.mock('../../useEnvelopeStream', () => {
  const create = (sid: string, callbacks: Record<string, () => void>) => {
    const stream = {
      sid,
      callbacks,
      connected: { value: false },
      connect: vi.fn(),
      disconnect: vi.fn(),
      cancel: vi.fn(),
      transport: { value: { send: vi.fn() } },
    };
    fixtures.streams.push(stream);
    return stream;
  };
  return { createChatStream: create, createTeamStream: create };
});
function deferred() {
  let resolve!: () => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<void>((r, j) => {
    resolve = r;
    reject = j;
  });
  return { promise, resolve, reject };
}
function setup(hydrate: (sid: string) => Promise<void>) {
  const selected = ref('A');
  const runtime = { setWsConnected: vi.fn() };
  const scope = effectScope();
  const api = scope.run(() =>
    useChatStreamManager({
      runtimeStore: runtime,
      refreshRunStatus: vi.fn().mockResolvedValue(undefined),
      onReconnectHydrate: hydrate,
      visibleSessionId: () => selected.value,
    } as unknown as StreamManagerDeps),
  )!;
  return {
    selected,
    runtime,
    api,
    stop: () => {
      api.disconnectAll();
      scope.stop();
    },
  };
}
function recover(stream: (typeof fixtures.streams)[number]) {
  stream.callbacks.onDisconnected();
  stream.connected.value = true;
  stream.callbacks.onConnected();
}
beforeEach(() => {
  fixtures.streams.length = 0;
});

describe('reconnect reconciliation ownership', () => {
  it('keeps failure visible and allows explicit retry without duplicate concurrent hydration', async () => {
    const next = deferred();
    const hydrate = vi.fn().mockRejectedValueOnce(new Error('offline')).mockReturnValue(next.promise);
    const c = setup(hydrate);
    c.api.ensureChatStream('A');
    recover(fixtures.streams[0]!);
    await flushPromises();
    expect(c.api.reconnectError.value).toBe(true);
    const retry = c.api.retryReconnect();
    const retryAgain = c.api.retryReconnect();
    expect(c.api.wsReplaying.value).toBe(true);
    expect(hydrate).toHaveBeenCalledTimes(2);
    next.resolve();
    await Promise.all([retry, retryAgain]);
    expect(c.api.reconnectError.value).toBe(false);
    expect(c.api.wsReplaying.value).toBe(false);
    c.stop();
  });
  it('A completion cannot end B reconciliation', async () => {
    const a = deferred(),
      b = deferred();
    const c = setup((sid) => (sid === 'A' ? a.promise : b.promise));
    c.api.ensureChatStream('A');
    recover(fixtures.streams[0]!);
    c.selected.value = 'B';
    c.api.ensureChatStream('B');
    recover(fixtures.streams[1]!);
    a.resolve();
    await flushPromises();
    expect(c.api.wsReplaying.value).toBe(true);
    b.resolve();
    await flushPromises();
    expect(c.api.wsReplaying.value).toBe(false);
    c.stop();
  });
  it('ignores callbacks from a replaced stream even when the same session is reopened', () => {
    const c = setup(async () => undefined);
    c.api.ensureChatStream('A');
    const old = fixtures.streams[0]!;
    c.api.ensureChatStream('B');
    c.api.ensureChatStream('A');
    c.runtime.setWsConnected.mockClear();
    old.callbacks.onConnected();
    old.callbacks.onDisconnected();
    expect(c.runtime.setWsConnected).not.toHaveBeenCalled();
    c.stop();
  });
  it('a previous failed request cannot overwrite a newer reconnect state', async () => {
    const old = deferred(),
      next = deferred();
    const hydrate = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(next.promise);
    const c = setup(hydrate);
    c.api.ensureTeamStream('A');
    const stream = fixtures.streams[0]!;
    recover(stream);
    recover(stream);
    old.reject(new Error('old failure'));
    await flushPromises();
    expect(c.api.reconnectError.value).toBe(false);
    expect(c.api.wsReplaying.value).toBe(true);
    next.resolve();
    await flushPromises();
    expect(c.api.wsReplaying.value).toBe(false);
    c.stop();
  });
});
