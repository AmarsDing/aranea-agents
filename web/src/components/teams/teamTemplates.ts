/**
 * Team 模板工厂：根据模板 key 生成 TeamDefinition。
 * 与 `components/teams/*.vue` 共址，见 aranea-frontend-guide SKILL §3.3 路径硬性约定。
 */
import type { Agent } from '../../features/agents/types';
import type { TeamDefinition } from '../../features/teams/types';
import { buildGraphFromDefinition } from '../../features/teams/graphUtils';
import type { TeamTemplateKey } from './teamConstants';

export function defaultA2AConfig() {
  return {
    enabled: true,
    envelope_version: 'a2a.v1',
    message_format: 'markdown_json',
    include_trace: true,
    max_payload_chars: 6000,
  };
}

export function defaultDefinition(): TeamDefinition {
  const definition: TeamDefinition = {
    version: 1,
    description: '',
    runtime_engine: 'graph',
    team_graph_runtime: true,
    mode: 'sequential',
    max_concurrency: 2,
    timeout_seconds: 600,
    loop_max_iterations: 0,
    intent_anchor_agent_id: '',
    a2a: defaultA2AConfig(),
    members: [],
    critic_loop: { max_iterations: 3, score_threshold: 0.8 },
  };
  return withGraph(definition);
}

export function definitionFromTemplate(template: TeamTemplateKey, agents: Agent[]): TeamDefinition {
  const base = defaultDefinition();
  if (template === 'parallel_experts') {
    const synthId = agents.length ? agents[agents.length - 1].id : '';
    return withGraph({
      ...base,
      description: '并行专家组：靠前槽位的专家并行产出；团队 Agent 列表中的最后一位负责汇总（与并行槽位区分）。',
      mode: 'parallel',
      max_concurrency: Math.max(2, Math.min(3, agents.length || 2)),
      synthesizer_agent_id: synthId,
      members: [
        templateMember(agents, 0, 'worker', '专家 A', 10),
        templateMember(agents, 1, 'worker', '专家 B', 20),
        templateMember(agents, 2, 'worker', '专家 C', 30),
      ].filter((member) => member.agent_id),
    });
  }
  if (template === 'critic_loop') {
    return withGraph({
      ...base,
      description: '生成评审：generator 先产出初稿，critic 评审后按需修订。',
      mode: 'critic_loop',
      max_concurrency: 1,
      critic_loop: { max_iterations: 2, score_threshold: 0.8 },
      members: [
        templateMember(agents, 0, 'generator', '生成者', 10),
        templateMember(agents, 1, 'critic', '评审者', 20),
      ].filter((member) => member.agent_id),
    });
  }
  if (template === 'coordinator') {
    return withGraph({
      ...base,
      description: '主控分派：coordinator 先拆解任务，worker 按计划完成分工。',
      mode: 'coordinator',
      max_concurrency: 2,
      loop_max_iterations: 1,
      members: [
        templateMember(agents, 0, 'coordinator', '主控', 10),
        templateMember(agents, 1, 'worker', '执行者 A', 20),
        templateMember(agents, 2, 'worker', '执行者 B', 30),
      ].filter((member) => member.agent_id),
    });
  }
  return withGraph({
    ...base,
    description: '顺序协作：多个 Agent 按顺序接力，上一个成员输出作为下一个成员输入。',
    mode: 'sequential',
    max_concurrency: 1,
    members: [
      templateMember(agents, 0, 'worker', '第一棒', 10),
      templateMember(agents, 1, 'worker', '第二棒', 20),
    ].filter((member) => member.agent_id),
  });
}

function templateMember(agents: Agent[], index: number, role: string, fallbackName: string, sortOrder: number) {
  const agent = pickAgent(agents, index);
  return {
    agent_id: agent?.id ?? '',
    role,
    name: agent?.display_name || fallbackName,
    enabled: true,
    sort_order: sortOrder,
  };
}

function pickAgent(agents: Agent[], index: number) {
  if (agents.length === 0) return undefined;
  return agents[Math.min(index, agents.length - 1)];
}

export function withGraph(definition: TeamDefinition): TeamDefinition {
  return {
    ...definition,
    graph: definition.graph?.nodes?.length ? definition.graph : buildGraphFromDefinition(definition),
  };
}
