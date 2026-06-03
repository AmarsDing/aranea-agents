import { describe, expect, it, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mergeSessionMessages } from '../mergeSessionMessages';
import { createPlaceholderMessage } from '../streamHandlers';
import type { Message } from '../types';

describe('chat send → delta → completion → merge flow', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('merges streaming ws row with server persistence after completion', () => {
    const sessionId = 'sess-1';
    const streamId = `ws-stream-${sessionId}`;
    const pendingId = 'pending-user-abc';

    let local: Message[] = [
      createPlaceholderMessage(pendingId, sessionId, 'user', 'hi'),
      { ...createPlaceholderMessage(streamId, sessionId, 'assistant', 'partial'), status: 'streaming' },
    ];

    local = mergeSessionMessages([], local);
    expect(local.some((m) => m.id === streamId)).toBe(true);

    local = local.map((m) => (m.id === streamId ? { ...m, content_markdown: 'Hello world', status: 'ok' } : m));

    const server: Message[] = [
      {
        ...createPlaceholderMessage('db-user-1', sessionId, 'user', 'hi'),
        id: 'db-user-1',
        status: 'ok',
      },
      {
        ...createPlaceholderMessage('db-asst-1', sessionId, 'assistant', 'Hello world'),
        id: 'db-asst-1',
        status: 'ok',
      },
    ];

    const merged = mergeSessionMessages(
      server,
      local.filter((m) => !m.id.startsWith('pending-user-')),
    );
    expect(merged.some((m) => m.id === 'ws-stream-sess-1')).toBe(true);

    const afterPersist = mergeSessionMessages(server, []);
    expect(afterPersist.map((m) => m.id)).toEqual(['db-user-1', 'db-asst-1']);
    expect(afterPersist.find((m) => m.id === 'db-asst-1')?.content_markdown).toBe('Hello world');
  });
});
