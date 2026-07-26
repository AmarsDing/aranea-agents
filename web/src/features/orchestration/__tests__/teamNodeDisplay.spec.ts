import { describe, expect, it } from 'vitest';
import { teamTopologySummary } from '../teamNodeDisplay';
import type { CompileTeamGraphResult } from '../compileApi';
import type { TeamDefinition } from '../../teams/types';
import type { CompiledGraphNodeView } from '../../../services/kratos/team/v1/index';

const node = (id: string, role: string, agentDisplayName: string, agentName = ''): CompiledGraphNodeView =>
  ({
    id,
    type: 'agent',
    role,
    agentName,
    agentDisplayName,
    description: '',
    taskPrompt: '',
  }) as CompiledGraphNodeView;

const mkCompiled = (mode: string, nodes: CompiledGraphNodeView[]): CompileTeamGraphResult => ({
  template_id: 't',
  mode,
  entry_point: nodes[0]?.id ?? '',
  finish_point: nodes[nodes.length - 1]?.id ?? '',
  nodes,
  edges: [],
  conditional_edges: [],
  graph_json: '',
  issues: [],
  valid: true,
  definition_graph_json: '',
});

const mkDef = (mode = 'sequential'): TeamDefinition => ({
  version: 1,
  description: '',
  mode,
  max_concurrency: 2,
  timeout_seconds: 600,
  loop_max_iterations: 0,
  members: [],
});

describe('teamNodeDisplay.teamTopologySummary', () => {
  it('returns empty string when no agent nodes', () => {
    expect(teamTopologySummary(null, null)).toBe('');
    expect(teamTopologySummary(mkCompiled('sequential', []), mkDef())).toBe('');
  });

  it('sequential lists member display names in order', () => {
    const compiled = mkCompiled('sequential', [
      node('member-1', 'worker', '市场分析'),
      node('member-2', 'worker', '策略研究'),
      node('member-3', 'worker', '报告撰写'),
    ]);
    expect(teamTopologySummary(compiled, mkDef())).toBe('顺序执行：市场分析 → 策略研究 → 报告撰写');
  });

  it('parallel groups workers and marks the synthesizer', () => {
    const compiled = mkCompiled('parallel', [
      node('member-1', 'worker', '市场分析'),
      node('member-2', 'worker', '策略研究'),
      node('member-3', 'synthesizer', '汇总报告'),
    ]);
    expect(teamTopologySummary(compiled, mkDef('parallel'))).toBe('并行执行：市场分析、策略研究 → 汇总：汇总报告');
  });

  it('coordinator marks the coordinator then sequential workers', () => {
    const compiled = mkCompiled('coordinator', [
      node('member-1', 'coordinator', '主控'),
      node('member-2', 'worker', '市场分析'),
      node('member-3', 'worker', '策略研究'),
    ]);
    expect(teamTopologySummary(compiled, mkDef('coordinator'))).toBe('协调分派：主控 → 顺序执行：市场分析 → 策略研究');
  });

  it('critic_loop shows generator/critic loop with max iterations', () => {
    const compiled = mkCompiled('critic_loop', [
      node('member-1', 'generator', '撰稿'),
      node('member-2', 'critic', '评审员'),
    ]);
    const def: TeamDefinition = { ...mkDef('critic_loop'), critic_loop: { max_iterations: 5, score_threshold: 0.8 } };
    expect(teamTopologySummary(compiled, def)).toBe('生成评审循环：撰稿 ⇄ 评审员（最多 5 轮）');
  });

  it('critic_loop without iteration limit omits the suffix', () => {
    const compiled = mkCompiled('critic_loop', [
      node('member-1', 'generator', '撰稿'),
      node('member-2', 'critic', '评审员'),
    ]);
    expect(teamTopologySummary(compiled, mkDef('critic_loop'))).toBe('生成评审循环：撰稿 ⇄ 评审员');
  });

  it('adaptive shows free collaboration with member names', () => {
    const compiled = mkCompiled('adaptive', [
      node('member-1', 'worker', '市场分析'),
      node('member-2', 'worker', '策略研究'),
    ]);
    expect(teamTopologySummary(compiled, mkDef('adaptive'))).toBe(
      '自由协作：市场分析、策略研究（成员间可相互转交任务）',
    );
  });

  it('swarm shares the adaptive description', () => {
    const compiled = mkCompiled('swarm', [node('member-1', 'worker', '市场分析')]);
    expect(teamTopologySummary(compiled, mkDef('swarm'))).toBe('自由协作：市场分析（成员间可相互转交任务）');
  });

  it('falls back to agentName, then friendly member label instead of node id', () => {
    const compiled = mkCompiled('sequential', [node('member-1', 'worker', '', 'quant-agent')]);
    expect(teamTopologySummary(compiled, mkDef())).toBe('顺序执行：quant-agent');
    const bare = mkCompiled('sequential', [node('member-1', 'worker', '')]);
    expect(teamTopologySummary(bare, mkDef())).toBe('顺序执行：成员 1');
  });
});
