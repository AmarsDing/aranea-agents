import { describe, expect, it } from 'vitest';
import { clearChannelWsCursor, getChannelWsCursor, noteChannelWsEnvelope } from '../channelWsCursor';

describe('channelWsCursor', () => {
  const sid = 'sess-1';

  it('tracks and clears last envelope id', () => {
    clearChannelWsCursor(sid);
    expect(getChannelWsCursor(sid)).toBeUndefined();
    noteChannelWsEnvelope(sid, 'env-99');
    expect(getChannelWsCursor(sid)).toBe('env-99');
    clearChannelWsCursor(sid);
    expect(getChannelWsCursor(sid)).toBeUndefined();
  });

  it('hydrates cursor from sessionStorage when memory is cold (DECO-R-P3-02)', () => {
    clearChannelWsCursor(sid);
    sessionStorage.setItem('aranea:channel-ws-cursor:sess-1', 'env-cold');
    expect(getChannelWsCursor(sid)).toBe('env-cold');
    clearChannelWsCursor(sid);
  });
});
