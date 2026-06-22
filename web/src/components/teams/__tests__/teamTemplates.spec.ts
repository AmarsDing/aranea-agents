import { describe, expect, it } from 'vitest';
import { defaultDefinition, definitionFromTemplate, defaultA2AConfig, withGraph } from '../teamTemplates';
import type { Agent } from '../../../features/agents/types';

const mockAgents: Agent[] = [
  { id: 'a1', display_name: 'Agent 1' } as Agent,
  { id: 'a2', display_name: 'Agent 2' } as Agent,
  { id: 'a3', display_name: 'Agent 3' } as Agent,
];

describe('teamTemplates.defaultDefinition', () => {
  it('returns a valid default definition', () => {
    const def = defaultDefinition();
    expect(def.version).toBe(1);
    expect(def.mode).toBe('sequential');
    expect(def.runtime_engine).toBe('graph');
    expect(def.team_graph_runtime).toBe(true);
    expect(def.members).toEqual([]);
    expect(def.a2a).toBeDefined();
    expect(def.graph).toBeDefined();
    expect(def.graph?.nodes?.length).toBeGreaterThan(0);
  });
});

describe('teamTemplates.defaultA2AConfig', () => {
  it('returns enabled a2a config', () => {
    const config = defaultA2AConfig();
    expect(config.enabled).toBe(true);
    expect(config.envelope_version).toBe('a2a.v1');
    expect(config.message_format).toBe('markdown_json');
  });
});

describe('teamTemplates.definitionFromTemplate', () => {
  it('sequential template creates sequential mode', () => {
    const def = definitionFromTemplate('sequential', mockAgents);
    expect(def.mode).toBe('sequential');
    expect(def.members.length).toBeGreaterThanOrEqual(2);
    expect(def.members.every((m) => m.agent_id)).toBe(true);
    expect(def.graph?.nodes?.length).toBeGreaterThan(0);
  });

  it('parallel_experts template creates parallel mode with synthesizer', () => {
    const def = definitionFromTemplate('parallel_experts', mockAgents);
    expect(def.mode).toBe('parallel');
    expect(def.synthesizer_agent_id).toBeTruthy();
    expect(def.members.length).toBeGreaterThanOrEqual(2);
  });

  it('critic_loop template creates critic_loop mode', () => {
    const def = definitionFromTemplate('critic_loop', mockAgents);
    expect(def.mode).toBe('critic_loop');
    expect(def.critic_loop).toBeDefined();
    expect(def.critic_loop!.max_iterations).toBeGreaterThan(0);
    const roles = def.members.map((m) => m.role);
    expect(roles).toContain('generator');
    expect(roles).toContain('critic');
  });

  it('coordinator template creates coordinator mode', () => {
    const def = definitionFromTemplate('coordinator', mockAgents);
    expect(def.mode).toBe('coordinator');
    const roles = def.members.map((m) => m.role);
    expect(roles).toContain('coordinator');
    expect(roles).toContain('worker');
  });

  it('handles empty agents gracefully', () => {
    const def = definitionFromTemplate('sequential', []);
    expect(def.mode).toBe('sequential');
    expect(def.members).toEqual([]);
  });
});

describe('teamTemplates.withGraph', () => {
  it('preserves existing graph if it has nodes', () => {
    const def = defaultDefinition();
    const originalGraph = def.graph;
    const result = withGraph(def);
    expect(result.graph).toBe(originalGraph);
  });

  it('builds graph if missing', () => {
    const def = { ...defaultDefinition(), graph: { nodes: [], edges: [] } };
    const result = withGraph(def);
    expect(result.graph?.nodes?.length).toBeGreaterThan(0);
  });
});
