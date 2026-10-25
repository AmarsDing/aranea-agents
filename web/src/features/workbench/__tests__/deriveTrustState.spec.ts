import { describe, expect, it } from 'vitest';
import { deriveTrustState } from '../useWorkbenchPage';

const base = { wsConnected: true, staleMs: 0, lanesDegraded: false, backlogCount: 0, replayMode: false };

describe('deriveTrustState', () => {
  it('replay 优先于一切（P0 恒 false，行为锁定防回归）', () => {
    expect(deriveTrustState({ ...base, replayMode: true, wsConnected: false, backlogCount: 9 })).toBe('replay');
  });

  it('offline 优先于 stale/backlog/degraded', () => {
    expect(deriveTrustState({ ...base, wsConnected: false, staleMs: 10 ** 9, backlogCount: 5 })).toBe('offline');
  });

  it('stale 由 staleMs 超窗裁决（窗口可覆盖）', () => {
    expect(deriveTrustState({ ...base, staleMs: 5 * 60_000 + 1 })).toBe('stale');
    expect(deriveTrustState({ ...base, staleMs: 1000, wsStaleMs: 500 })).toBe('stale');
    expect(deriveTrustState({ ...base, staleMs: 5 * 60_000 })).toBe('live');
  });

  it('backlog（告警堆单）优先于 degraded', () => {
    expect(deriveTrustState({ ...base, backlogCount: 1, lanesDegraded: true })).toBe('backlog');
  });

  it('degraded 与 live 依次兜底', () => {
    expect(deriveTrustState({ ...base, lanesDegraded: true })).toBe('degraded');
    expect(deriveTrustState(base)).toBe('live');
  });
});
