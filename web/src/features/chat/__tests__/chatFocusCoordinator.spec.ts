import { describe, expect, it, vi } from 'vitest';
import { createChatFocusCoordinator } from '../chatFocusCoordinator';
import { hydrateSessionForChannelFocus } from '../channelFocusLoad';
import type { Message } from '../types';

describe('createChatFocusCoordinator', () => {
  it('suppresses route watch while navigation is in flight', async () => {
    const coord = createChatFocusCoordinator();
    expect(coord.isRouteSessionWatchSuppressed()).toBe(false);
    await coord.withRouteWatchSuppressed(async () => {
      expect(coord.isRouteSessionWatchSuppressed()).toBe(true);
    });
    expect(coord.isRouteSessionWatchSuppressed()).toBe(false);
  });

  it('dedupes concurrent focus and merges skipMessageReload with OR', async () => {
    const coord = createChatFocusCoordinator();
    const resolveSkipLog: boolean[] = [];
    let runs = 0;

    const task = () =>
      coord.runFocusOnce('sess-1:agent-1', { skipMessageReload: false }, async (resolveSkip) => {
        runs += 1;
        await new Promise((r) => setTimeout(r, 10));
        resolveSkipLog.push(resolveSkip());
      });

    const live = coord.runFocusOnce('sess-1:agent-1', { skipMessageReload: true }, async (resolveSkip) => {
      runs += 1;
      await new Promise((r) => setTimeout(r, 0));
      resolveSkipLog.push(resolveSkip());
    });

    await Promise.all([task(), live]);
    expect(runs).toBe(1);
    expect(resolveSkipLog.some((v) => v)).toBe(true);
  });

  it('logs focus failures', async () => {
    const coord = createChatFocusCoordinator();
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    await expect(
      coord.runFocusOnce('sess-x:', undefined, async () => {
        throw new Error('boom');
      }),
    ).rejects.toThrow('boom');
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });
});

describe('hydrateSessionForChannelFocus', () => {
  const baseMsg: Message = {
    id: 'u1',
    session_id: 'sess-1',
    parent_message_id: '',
    turn_id: '',
    turn_number: 1,
    seq_in_turn: 0,
    role: 'user',
    content_markdown: 'hi',
    model_name: '',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: 'ok',
    attachments_count: 0,
    options_json: '',
    error_message: '',
    created_at: '2026-05-23T00:00:00Z',
  };

  it('skips reload when skipMessageReload and user row exists locally', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    const ensureChatStream = vi.fn();
    const rows: Message[] = [{ ...baseMsg }];
    await hydrateSessionForChannelFocus(
      {
        getMessages: () => rows,
        loadMessages,
        setMessages: (_sid, next) => {
          rows.splice(0, rows.length, ...next);
        },
        ensureChatStream,
      },
      'sess-1',
      true,
    );
    expect(loadMessages).not.toHaveBeenCalled();
    expect(ensureChatStream).toHaveBeenCalledWith('sess-1');
  });

  it('loads user turn when skipMessageReload but no local user content', async () => {
    const loadMessages = vi.fn().mockResolvedValue(undefined);
    await hydrateSessionForChannelFocus(
      {
        getMessages: () => [],
        loadMessages,
        setMessages: () => {},
        ensureChatStream: () => {},
      },
      'sess-1',
      true,
    );
    expect(loadMessages).toHaveBeenCalledWith({ sessionId: 'sess-1', replace: true });
  });
});
