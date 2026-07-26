import type { NodeDef } from '../graph/types';
import type { CompileTeamGraphResult, CompiledGraphNodeView } from './compileApi';
import type { TeamDefinition } from '../teams/types';

export type TeamNodeDisplay = {
  nodeId: string;
  displayName: string;
  agentKey: string;
  role: string;
  roleLabel: string;
  responsibility: string;
  /** Design-time hint for upstream input (from mode / topology). */
  inputHint: string;
  /** Design-time hint for downstream output. */
  outputHint: string;
};

const ROLE_INPUT_HINTS: Record<string, string> = {
  coordinator: '接收用户任务，分解并分派给成员',
  worker: '接收协调者或上游节点的任务上下文',
  synthesizer: '汇聚各成员输出，生成最终答复',
  critic: '评审上游产出，给出通过/修改意见',
};

const ROLE_OUTPUT_HINTS: Record<string, string> = {
  coordinator: '任务计划、成员分派指令',
  worker: '阶段性成果、工具调用结果',
  synthesizer: '团队最终交付物',
  critic: '评审结论、修改建议',
};

const ROLE_LABELS: Record<string, string> = {
  coordinator: '协调',
  worker: '执行',
  synthesizer: '汇总',
  critic: '评审',
  generator: '生成',
};

function roleLabel(role: string): string {
  const key = role.trim().toLowerCase();
  return ROLE_LABELS[key] ?? (role || '成员');
}

/** 节点角色的中文展示标签（画布节点卡片等用户视角场景使用；未知角色原样透传）。 */
export function teamNodeRoleLabel(role: string): string {
  return roleLabel(role);
}

function findMemberForNode(nodeId: string, agentKey: string, definition: TeamDefinition | null) {
  const members = definition?.members ?? [];
  const graphNode = definition?.graph?.nodes?.find((n) => n.id === nodeId);
  if (graphNode?.agent_id) {
    const byId = members.find((m) => m.agent_id === graphNode.agent_id);
    if (byId) return byId;
  }
  if (agentKey) {
    const byKey = members.find((m) => m.agent_id === agentKey || m.name === agentKey);
    if (byKey) return byKey;
  }
  const sortMatch = /^member-(\d+)$/.exec(nodeId);
  if (sortMatch) {
    const order = Number(sortMatch[1]);
    return members.find((m) => m.sort_order === order) ?? members[order - 1];
  }
  return undefined;
}

export function resolveTeamNodeDisplay(
  node: NodeDef,
  compiled: CompileTeamGraphResult | null,
  definition: TeamDefinition | null,
): TeamNodeDisplay {
  const compiledNode = compiled?.nodes.find((n) => n.id === node.id);
  const member = findMemberForNode(node.id, node.agentName, definition);
  const role = (compiledNode?.role || node.requiredRole || member?.role || 'worker').trim().toLowerCase();
  const agentKey = compiledNode?.agentName || node.agentName || member?.agent_id || '';
  const displayName =
    compiledNode?.agentDisplayName?.trim() ||
    compiledNode?.description?.trim() ||
    member?.name?.trim() ||
    node.description?.trim() ||
    agentKey ||
    node.id;
  const responsibility =
    compiledNode?.taskPrompt?.trim() ||
    node.instruction?.trim() ||
    member?.task_prompt?.trim() ||
    member?.name?.trim() ||
    ROLE_INPUT_HINTS[role] ||
    '执行编排分配的子任务';

  return {
    nodeId: node.id,
    displayName,
    agentKey,
    role,
    roleLabel: roleLabel(role),
    responsibility,
    inputHint: ROLE_INPUT_HINTS[role] ?? '接收上游 state / 协调者输入',
    outputHint: ROLE_OUTPUT_HINTS[role] ?? '写入 state，传递给下游节点',
  };
}

export function graphNodeDisplayLabel(node: NodeDef): string {
  if (node.type === 'agent') {
    return node.description?.trim() || node.agentName?.trim() || node.id;
  }
  return node.agentName?.trim() || node.description?.trim() || node.id;
}

function nodeDisplayName(n: CompiledGraphNodeView, definition: TeamDefinition | null): string {
  const display = n.agentDisplayName?.trim() || n.description?.trim();
  if (display) return display;
  const member = findMemberForNode(n.id ?? '', n.agentName ?? '', definition);
  if (member?.name?.trim()) return member.name.trim();
  if (n.agentName?.trim()) return n.agentName.trim();
  const order = /^member-(\d+)$/.exec(n.id ?? '');
  if (order) return `成员 ${Number(order[1])}`;
  return n.id || '成员';
}

function nodeRole(n: CompiledGraphNodeView): string {
  return String(n.role ?? '')
    .trim()
    .toLowerCase();
}

function compiledAgentNodes(compiled: CompileTeamGraphResult | null): CompiledGraphNodeView[] {
  return (compiled?.nodes ?? []).filter((n) => (n.type ?? 'agent') === 'agent');
}

/** 成员展示行：只含用户可读的中文名与中文角色（技术编码不进 UI）。 */
export type TeamMemberDisplayRow = {
  key: string;
  name: string;
  roleLabel: string;
};

export function teamMemberDisplayRows(
  compiled: CompileTeamGraphResult | null,
  definition: TeamDefinition | null,
): TeamMemberDisplayRow[] {
  return compiledAgentNodes(compiled).map((n) => ({
    key: n.id ?? n.agentName ?? '',
    name: nodeDisplayName(n, definition),
    roleLabel: roleLabel(nodeRole(n) || 'worker'),
  }));
}

/**
 * 拓扑的用户语言摘要（编排信息面板 / 编辑器编译预览共用）。
 * 只使用成员显示名与中文角色标签；node_id / agent_key 等技术编码不出现在结果中。
 */
export function teamTopologySummary(
  compiled: CompileTeamGraphResult | null,
  definition: TeamDefinition | null,
): string {
  const agentNodes = compiledAgentNodes(compiled);
  if (agentNodes.length === 0) return '';

  const nameOf = (n: CompiledGraphNodeView): string => nodeDisplayName(n, definition);
  const roleOf = nodeRole;

  const mode = String(compiled?.mode || definition?.mode || 'sequential')
    .trim()
    .toLowerCase();
  const names = agentNodes.map(nameOf);

  if (mode === 'parallel') {
    const workers = agentNodes.filter((n) => roleOf(n) !== 'synthesizer').map(nameOf);
    const synth = agentNodes.find((n) => roleOf(n) === 'synthesizer');
    const base = `并行执行：${workers.join('、')}`;
    return synth ? `${base} → 汇总：${nameOf(synth)}` : base;
  }
  if (mode === 'coordinator') {
    const [first, ...rest] = agentNodes;
    if (first && roleOf(first) === 'coordinator') {
      return rest.length
        ? `协调分派：${nameOf(first)} → 顺序执行：${rest.map(nameOf).join(' → ')}`
        : `协调分派：${nameOf(first)}`;
    }
    return `顺序执行：${names.join(' → ')}`;
  }
  if (mode === 'critic_loop') {
    const generator = agentNodes.find((n) => roleOf(n) === 'generator');
    const critic = agentNodes.find((n) => roleOf(n) === 'critic');
    if (generator && critic) {
      const maxIter = definition?.critic_loop?.max_iterations ?? 0;
      const suffix = maxIter > 0 ? `（最多 ${maxIter} 轮）` : '';
      return `生成评审循环：${nameOf(generator)} ⇄ ${nameOf(critic)}${suffix}`;
    }
    return `生成评审循环：${names.join(' ⇄ ')}`;
  }
  if (mode === 'adaptive' || mode === 'swarm') {
    return `自由协作：${names.join('、')}（成员间可相互转交任务）`;
  }
  return `顺序执行：${names.join(' → ')}`;
}
