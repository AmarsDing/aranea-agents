import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SessionView } from '../../../components/chat/types';
import { clearCachedMobileSessions, readCachedMobileSessions, writeCachedMobileSessions } from '../offlineCache';

function makeSession(id: string, overrides?: Partial<SessionView>): SessionView {
  return { id, title: `Session ${id}`, context_used_ratio: 0, at: '2026-08-08T01:00:00Z', ...overrides };
}

const KEY = 'aranea.mobile.sessionCache.v1.agent-1';

beforeEach(() => {
  localStorage.clear();
});

describe('offlineCache', () => {
  it('round-trips a session list for the same agent', () => {
    writeCachedMobileSessions('agent-1', [makeSession('s1'), makeSession('s2', { message_count: 3 })]);
    const cached = readCachedMobileSessions('agent-1');
    expect(cached.map((s) => s.id)).toEqual(['s1', 's2']);
    expect(cached[1]?.message_count).toBe(3);
  });

  it('isolates caches per agent', () => {
    writeCachedMobileSessions('agent-1', [makeSession('s1')]);
    writeCachedMobileSessions('agent-2', [makeSession('s9')]);
    expect(readCachedMobileSessions('agent-1').map((s) => s.id)).toEqual(['s1']);
    expect(readCachedMobileSessions('agent-2').map((s) => s.id)).toEqual(['s9']);
  });

  it('returns empty for missing key / blank agent / empty write', () => {
    expect(readCachedMobileSessions('agent-1')).toEqual([]);
    writeCachedMobileSessions('', [makeSession('s1')]);
    writeCachedMobileSessions('agent-1', []);
    expect(localStorage.getItem(KEY)).toBeNull();
    expect(readCachedMobileSessions('agent-1')).toEqual([]);
  });

  it('returns empty on corrupt JSON and on wrong envelope shape', () => {
    localStorage.setItem(KEY, '{not json');
    expect(readCachedMobileSessions('agent-1')).toEqual([]);
    localStorage.setItem(KEY, JSON.stringify({ v: 2, agentId: 'agent-1', sessions: [makeSession('s1')] }));
    expect(readCachedMobileSessions('agent-1')).toEqual([]);
    localStorage.setItem(KEY, JSON.stringify({ v: 1, agentId: 'other', sessions: [makeSession('s1')] }));
    expect(readCachedMobileSessions('agent-1')).toEqual([]);
  });

  it('drops malformed session entries but keeps valid ones', () => {
    localStorage.setItem(
      KEY,
      JSON.stringify({
        v: 1,
        agentId: 'agent-1',
        cachedAt: 1,
        sessions: [makeSession('s1'), { id: 42 }, null, { title: 'no id' }],
      }),
    );
    expect(readCachedMobileSessions('agent-1').map((s) => s.id)).toEqual(['s1']);
  });

  it('caps the cached list at 50 sessions', () => {
    const many = Array.from({ length: 60 }, (_, i) => makeSession(`s${i}`));
    writeCachedMobileSessions('agent-1', many);
    const cached = readCachedMobileSessions('agent-1');
    expect(cached).toHaveLength(50);
    expect(cached[0]?.id).toBe('s0');
    expect(cached[49]?.id).toBe('s49');
  });

  it('swallows quota errors instead of throwing', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('quota', 'QuotaExceededError');
    });
    expect(() => writeCachedMobileSessions('agent-1', [makeSession('s1')])).not.toThrow();
    spy.mockRestore();
  });

  it('clearCachedMobileSessions removes only that agent entry', () => {
    writeCachedMobileSessions('agent-1', [makeSession('s1')]);
    writeCachedMobileSessions('agent-2', [makeSession('s2')]);
    clearCachedMobileSessions('agent-1');
    expect(readCachedMobileSessions('agent-1')).toEqual([]);
    expect(readCachedMobileSessions('agent-2')).toHaveLength(1);
  });
});
