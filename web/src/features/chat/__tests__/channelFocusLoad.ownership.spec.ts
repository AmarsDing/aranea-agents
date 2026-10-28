import { describe, expect, it, vi } from 'vitest';
import { hydrateSessionForChannelFocus } from '../channelFocusLoad';
describe('channel focus transport ownership', () => {
  it('does not reopen the old stream after a late history load', async () => {
    let resolve!: () => void;
    let current = true;
    const deps = {
      getMessages: () => [],
      loadMessages: vi.fn(
        () =>
          new Promise<void>((r) => {
            resolve = r;
          }),
      ),
      setMessages: vi.fn(),
      ensureChatStream: vi.fn(),
    };
    const pending = hydrateSessionForChannelFocus(deps, 'A', false, () => current);
    current = false;
    resolve();
    await pending;
    expect(deps.ensureChatStream).not.toHaveBeenCalled();
  });
});
