import { describe, it, expect, vi, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { useSpiritTeamStore } from '../index';
import type { SpiritTeam, SpiritTeamStatus } from '../../../features/spirit/types';

vi.mock('../../../features/spirit/api', () => ({
  listSpiritTeams: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  cancelSpiritTeam: vi.fn(),
  resumeSpiritTeam: vi.fn(),
  archiveSpiritTeam: vi.fn(),
  retrySpiritTeam: vi.fn(),
  pauseSpiritTeam: vi.fn(),
  unpauseSpiritTeam: vi.fn(),
  injectSpiritTeam: vi.fn(),
  pauseAgentSession: vi.fn(),
  injectAgentSession: vi.fn(),
  cancelAgentSession: vi.fn(),
  resumeAgentSession: vi.fn(),
  retryAgentSession: vi.fn(),
}));

vi.mock('../../../i18n', () => ({
  i18n: { global: { t: (k: string) => k } },
}));

function mkTeam(id: string, status: SpiritTeamStatus): SpiritTeam {
  return {
    id,
    teamName: `team-${id}`,
    taskSummary: '',
    status,
    mode: 'coordinator',
    memberAvatars: [],
    completedSteps: 0,
    totalSteps: 1,
    progressPct: 0,
    durationMs: 0,
    spiritSessionId: 'spirit-1',
    teamSessionId: `ts-${id}`,
    members: [],
    sharedAgentIds: [],
    createdAt: Date.now(),
  };
}

describe('useSpiritTeamStore.updateTeamStatus 终态优先级（partial_failure）', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  function setup(status: SpiritTeamStatus) {
    const store = useSpiritTeamStore();
    store.addTeam(mkTeam('t1', status));
    return store;
  }

  it.each([
    ['running', 'partial_failure', true],
    ['running', 'completed', true],
    // partial_failure 是 completed 的精化收敛态：允许 completed → partial_failure
    ['completed', 'partial_failure', true],
    // partial_failure 不得被低优先级终态/非终态覆盖
    ['partial_failure', 'completed', false],
    ['partial_failure', 'failed', false],
    ['partial_failure', 'cancelled', false],
    ['partial_failure', 'running', false],
    // 既有终态保护语义不回归
    ['completed', 'failed', false],
    ['completed', 'cancelled', false],
    ['completed', 'running', false],
    ['failed', 'cancelled', false],
    ['cancelled', 'failed', true],
  ] as Array<[SpiritTeamStatus, SpiritTeamStatus, boolean]>)(
    '%s → %s applied=%s',
    (from, to, applied) => {
      const store = setup(from);
      store.updateTeamStatus('t1', to);
      const team = store.teams.find((t) => t.id === 't1');
      expect(team?.status).toBe(applied ? to : from);
    },
  );

  it('不存在的 team 静默忽略', () => {
    const store = useSpiritTeamStore();
    expect(() => store.updateTeamStatus('missing', 'partial_failure')).not.toThrow();
  });
});

describe('useSpiritTeamStore 派生集合（partial_failure 口径）', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
  });

  it('completedTeams 含 partial_failure（与后端 checkAllTeamsCompleted 同口径）', () => {
    const store = useSpiritTeamStore();
    store.addTeam(mkTeam('t1', 'completed'));
    store.addTeam(mkTeam('t2', 'partial_failure'));
    store.addTeam(mkTeam('t3', 'failed'));
    store.addTeam(mkTeam('t4', 'running'));

    const ids = store.completedTeams.map((t) => t.id);
    expect(ids).toContain('t1');
    expect(ids).toContain('t2');
    expect(ids).not.toContain('t3');
    expect(ids).not.toContain('t4');
  });

  it('activeTeams 排除全部终态（含 partial_failure）', () => {
    const store = useSpiritTeamStore();
    store.addTeam(mkTeam('t1', 'running'));
    store.addTeam(mkTeam('t2', 'pending'));
    store.addTeam(mkTeam('t3', 'partial_failure'));
    store.addTeam(mkTeam('t4', 'completed'));
    store.addTeam(mkTeam('t5', 'failed'));
    store.addTeam(mkTeam('t6', 'cancelled'));
    store.addTeam(mkTeam('t7', 'archived'));

    const ids = store.activeTeams.map((t) => t.id);
    expect(ids).toEqual(expect.arrayContaining(['t1', 't2']));
    expect(ids).toHaveLength(2);
  });
});
