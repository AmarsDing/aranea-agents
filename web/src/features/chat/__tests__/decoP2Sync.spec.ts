import { describe, expect, it, vi } from 'vitest';

describe('inbound hydrate error callback (DECO-14)', () => {
  it('invokes onHydrateError when loadMessages rejects', async () => {
    const onHydrateError = vi.fn();
    const sessionId = 'sess-hydrate';
    const chatStore = {
      sessionRevisionBySession: { [sessionId]: 2 } as Record<string, number>,
      getMessages: () => [],
      setMessages: vi.fn(),
      loadMessages: vi.fn().mockRejectedValue(new Error('network down')),
      entityKind: 'agent' as const,
      sessions: [],
      selectedSession: null,
    };
    const ensureChatStream = vi.fn(() => ({
      patchMessages: vi.fn(),
    }));

    async function hydrateCurrentSession(sid: string) {
      const localRev = chatStore.sessionRevisionBySession[sid] ?? 0;
      try {
        await chatStore.loadMessages({ sessionId: sid, afterRevision: localRev });
      } catch (err) {
        const message = err instanceof Error ? err.message : 'hydrate failed';
        onHydrateError(sid, message);
      }
    }

    await hydrateCurrentSession(sessionId);
    expect(onHydrateError).toHaveBeenCalledWith(sessionId, 'network down');
    expect(ensureChatStream).not.toHaveBeenCalled();
  });
});
