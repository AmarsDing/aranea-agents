import { describe, expect, it } from 'vitest';
import { validRolesForMode, roleOptionsForMode, validateTeamDefinition } from '../teamUtils';
import type { TeamDefinition } from '../../../features/teams/types';

function makeDef(overrides: Partial<TeamDefinition> = {}): TeamDefinition {
  return {
    version: 1,
    description: '',
    mode: 'sequential',
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    members: [
      { agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
    ],
    ...overrides,
  };
}

describe('validRolesForMode', () => {
  it('sequential only allows worker', () => {
    const roles = validRolesForMode('sequential');
    expect(roles?.has('worker')).toBe(true);
    expect(roles?.has('coordinator')).toBe(false);
    expect(roles?.has('synthesizer')).toBe(false);
  });

  it('parallel allows synthesizer and worker', () => {
    const roles = validRolesForMode('parallel');
    expect(roles?.has('synthesizer')).toBe(true);
    expect(roles?.has('worker')).toBe(true);
    expect(roles?.has('coordinator')).toBe(false);
  });

  it('coordinator allows coordinator, worker, synthesizer', () => {
    const roles = validRolesForMode('coordinator');
    expect(roles?.has('coordinator')).toBe(true);
    expect(roles?.has('worker')).toBe(true);
    expect(roles?.has('synthesizer')).toBe(true);
    expect(roles?.has('generator')).toBe(false);
  });

  it('critic_loop allows generator, critic, synthesizer', () => {
    const roles = validRolesForMode('critic_loop');
    expect(roles?.has('generator')).toBe(true);
    expect(roles?.has('critic')).toBe(true);
    expect(roles?.has('synthesizer')).toBe(true);
    expect(roles?.has('worker')).toBe(false);
  });

  it('swarm/adaptive returns null (any role)', () => {
    expect(validRolesForMode('swarm')).toBeNull();
    expect(validRolesForMode('adaptive')).toBeNull();
  });
});

describe('roleOptionsForMode', () => {
  it('sequential returns only worker', () => {
    const opts = roleOptionsForMode('sequential');
    expect(opts.map((o) => o.value)).toEqual(['worker']);
  });

  it('adaptive returns all 5 roles', () => {
    const opts = roleOptionsForMode('adaptive');
    expect(opts).toHaveLength(5);
  });
});

describe('validateTeamDefinition', () => {
  it('returns null for valid sequential definition', () => {
    expect(validateTeamDefinition(makeDef())).toBeNull();
  });

  it('returns error when no enabled members', () => {
    const def = makeDef({ members: [{ agent_id: 'a1', role: 'worker', name: 'W1', enabled: false, sort_order: 10 }] });
    expect(validateTeamDefinition(def)).toContain('至少启用一名成员');
  });

  it('returns error when enabled member has no agent_id', () => {
    const def = makeDef({
      members: [
        { agent_id: '', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
        { agent_id: 'a2', role: 'worker', name: 'W2', enabled: true, sort_order: 20 },
      ],
    });
    expect(validateTeamDefinition(def)).toContain('必须选择 Agent');
  });

  it('returns error for incompatible role-mode (coordinator in sequential)', () => {
    const def = makeDef({
      mode: 'sequential',
      members: [{ agent_id: 'a1', role: 'coordinator', name: 'C1', enabled: true, sort_order: 10 }],
    });
    expect(validateTeamDefinition(def)).toContain('不兼容');
  });

  it('returns error for parallel without synthesizer', () => {
    const def = makeDef({
      mode: 'parallel',
      members: [
        { agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
        { agent_id: 'a2', role: 'worker', name: 'W2', enabled: true, sort_order: 20 },
      ],
    });
    expect(validateTeamDefinition(def)).toContain('汇总 Agent');
  });

  it('passes for parallel with synthesizer member', () => {
    const def = makeDef({
      mode: 'parallel',
      members: [
        { agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
        { agent_id: 'a2', role: 'synthesizer', name: 'S1', enabled: true, sort_order: 20 },
      ],
    });
    expect(validateTeamDefinition(def)).toBeNull();
  });

  it('returns error for coordinator without synthesizer or coordinator', () => {
    const def = makeDef({
      mode: 'coordinator',
      members: [{ agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 }],
    });
    expect(validateTeamDefinition(def)).toContain('synthesizer 或 coordinator');
  });

  it('passes for coordinator with coordinator member', () => {
    const def = makeDef({
      mode: 'coordinator',
      members: [
        { agent_id: 'a1', role: 'coordinator', name: 'C1', enabled: true, sort_order: 10 },
        { agent_id: 'a2', role: 'worker', name: 'W1', enabled: true, sort_order: 20 },
      ],
    });
    expect(validateTeamDefinition(def)).toBeNull();
  });

  it('returns error for critic_loop without generator', () => {
    const def = makeDef({
      mode: 'critic_loop',
      members: [
        { agent_id: 'a1', role: 'critic', name: 'C1', enabled: true, sort_order: 10 },
        { agent_id: 'a2', role: 'synthesizer', name: 'S1', enabled: true, sort_order: 20 },
      ],
    });
    expect(validateTeamDefinition(def)).toContain('generator 和 critic');
  });

  it('passes for critic_loop with generator and critic', () => {
    const def = makeDef({
      mode: 'critic_loop',
      members: [
        { agent_id: 'a1', role: 'generator', name: 'G1', enabled: true, sort_order: 10 },
        { agent_id: 'a2', role: 'critic', name: 'C1', enabled: true, sort_order: 20 },
      ],
    });
    expect(validateTeamDefinition(def)).toBeNull();
  });

  it('adaptive mode accepts any role', () => {
    const def = makeDef({
      mode: 'adaptive',
      members: [{ agent_id: 'a1', role: 'coordinator', name: 'C1', enabled: true, sort_order: 10 }],
    });
    expect(validateTeamDefinition(def)).toBeNull();
  });
});
