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
    members: [{ agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 }],
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
    expect(validateTeamDefinition(def)).toContain('「汇总」或「协调」');
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
    expect(validateTeamDefinition(def)).toContain('「生成」和「评审」');
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

  // ADR-08 A4：definition 携带 embedded graph 时，拓扑以 graph 为唯一真相源，
  // 跳过 role-mode 耦合校验（角色兼容 / parallel 汇总 / coordinator / critic_loop 角色要求）。
  describe('with embedded graph (custom path)', () => {
    const customGraph = {
      version: 1,
      layout: 'custom',
      nodes: [
        { id: 'start', type: 'start', label: 'Start', x: 0, y: 0 },
        { id: 'n1', type: 'agent', label: 'N1', agent_id: 'a1', x: 160, y: 0 },
        { id: 'end', type: 'end', label: 'End', x: 320, y: 0 },
      ],
      edges: [
        { id: 'start-n1', source: 'start', target: 'n1' },
        { id: 'n1-end', source: 'n1', target: 'end' },
      ],
    };

    it('skips role-mode compatibility check', () => {
      const def = makeDef({
        mode: 'sequential',
        graph: customGraph,
        members: [{ agent_id: 'a1', role: 'coordinator', name: 'C1', enabled: true, sort_order: 10 }],
      });
      expect(validateTeamDefinition(def)).toBeNull();
    });

    it('skips parallel synthesizer requirement', () => {
      const def = makeDef({
        mode: 'parallel',
        graph: customGraph,
        members: [
          { agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
          { agent_id: 'a2', role: 'worker', name: 'W2', enabled: true, sort_order: 20 },
        ],
      });
      expect(validateTeamDefinition(def)).toBeNull();
    });

    it('skips critic_loop generator/critic requirement', () => {
      const def = makeDef({
        mode: 'critic_loop',
        graph: customGraph,
        members: [{ agent_id: 'a1', role: 'worker', name: 'W1', enabled: true, sort_order: 10 }],
      });
      expect(validateTeamDefinition(def)).toBeNull();
    });

    it('still enforces agent_id and enabled-member checks', () => {
      const missingAgent = makeDef({
        mode: 'sequential',
        graph: customGraph,
        members: [
          { agent_id: '', role: 'worker', name: 'W1', enabled: true, sort_order: 10 },
          { agent_id: 'a2', role: 'worker', name: 'W2', enabled: true, sort_order: 20 },
        ],
      });
      expect(validateTeamDefinition(missingAgent)).toContain('必须选择 Agent');
      const allDisabled = makeDef({
        mode: 'sequential',
        graph: customGraph,
        members: [{ agent_id: 'a1', role: 'worker', name: 'W1', enabled: false, sort_order: 10 }],
      });
      expect(validateTeamDefinition(allDisabled)).toContain('至少启用一名成员');
    });
  });
});
