/**
 * Team 展示用纯函数（无网络）。与 `components/teams/*.vue` 共址，见 aranea-frontend-guide SKILL §3.3 路径硬性约定。
 * 类型来自 `features/teams/api`（仅 type import）。
 */
import type { Agent } from '../../features/agents/types';
import { findTaxonomyPath } from '../../features/platform/taxonomyTreeUtils';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import type { Team, TeamDefinition } from '../../features/teams/types';
import { buildGraphFromDefinition } from '../../features/teams/graphUtils';

export { buildGraphFromDefinition };

export type TeamIndustryGroup = {
  id: string;
  label: string;
  sortOrder: number;
  teams: Team[];
};

const UNCategorizedIndustryId = '__uncategorized__';

export const modeOptions = [
  { label: '顺序 sequential', value: 'sequential' },
  { label: '并行 parallel', value: 'parallel' },
  { label: '主控 coordinator', value: 'coordinator' },
  { label: '生成评审 critic_loop', value: 'critic_loop' },
  {
    label: '群智 adaptive（Swarm）',
    value: 'adaptive',
    description: '成员间 transfer_to_agent 协作；后端与 swarm 共用 Swarm 运行时。',
  },
];

export const statusOptions = ['draft', 'active', 'archived'].map((value) => ({ label: value, value }));

export const roleOptions = ['worker', 'coordinator', 'synthesizer', 'generator', 'critic'].map((value) => ({
  label: value,
  value,
}));

export type TeamTemplateKey = 'sequential' | 'parallel_experts' | 'critic_loop' | 'coordinator';

export const teamTemplateOptions: Array<{ label: string; value: TeamTemplateKey; description: string }> = [
  { label: '顺序协作', value: 'sequential', description: '多个 worker 按顺序接力处理任务。' },
  {
    label: '并行专家组',
    value: 'parallel_experts',
    description: '前若干成员并行产出；列表中的最后一位 Agent 固定为汇总角色（与专家槽位不同实例）。',
  },
  {
    label: '生成评审',
    value: 'critic_loop',
    description: 'generator 与 critic 顺序迭代；迭代次数取自编排里的 critic_loop.max_iterations。',
  },
  {
    label: '主控分派',
    value: 'coordinator',
    description: '成员顺序执行；当前运行时为带迭代的上屏顺序链（非独立并行拓扑），适合分步接力。',
  },
];

export const runtimeEngineOptions = [
  {
    label: 'Graph（默认，GraphAgent）',
    value: 'graph',
    description: 'CompileToGraphRuntimeConfig → GraphAgent；生产推荐。',
  },
  {
    label: 'Native（BuildTRPCTeam）',
    value: 'native',
    description: '按 mode 分发 Chain/Parallel/Swarm；仅 fallback 或调试。',
  },
];

export const failureDefaultOptions = [
  { label: '重试后阻塞 retry_then_block', value: 'retry_then_block' },
  { label: '跳过 skip', value: 'skip' },
  { label: '快速失败 fail_fast', value: 'fail_fast' },
];

export const parallelFailOptions = [
  { label: '继续 continue（分支失败可跳过）', value: 'continue' },
  { label: '中止 abort', value: 'abort' },
];

export const failureOnErrorOptions = [
  { label: '暂停等审核 await_review', value: 'await_review' },
  { label: '终止 halt', value: 'halt' },
];

export function runtimeEngineLabel(value?: string) {
  const v = String(value || 'graph').toLowerCase() === 'native' ? 'native' : 'graph';
  return runtimeEngineOptions.find((o) => o.value === v)?.label ?? v;
}

export function failurePolicySummary(def: TeamDefinition): string {
  const fp = def.failure_policy;
  if (!fp) return 'retry_then_block（默认）';
  const parts: string[] = [];
  if (fp.default) parts.push(`default: ${fp.default}`);
  if (def.mode === 'parallel' && fp.parallel_fail) parts.push(`parallel: ${fp.parallel_fail}`);
  if (fp.retry?.max_attempts != null) parts.push(`retry: ${fp.retry.max_attempts}`);
  if (fp.circuit_breaker?.failure_threshold) parts.push(`circuit: ${fp.circuit_breaker.failure_threshold}`);
  if (fp.on_error) parts.push(`on_error: ${fp.on_error}`);
  return parts.length ? parts.join(' · ') : '—';
}

export function resetDefinition(target: TeamDefinition): void {
  const fresh = defaultDefinition();
  const optionalKeys: (keyof TeamDefinition)[] = [
    'failure_policy',
    'linked_graph_id',
    'synthesizer_agent_id',
    'enable_checkpoint',
    'team_graph_runtime',
    'graph',
  ];
  for (const key of optionalKeys) {
    delete (target as Record<string, unknown>)[key];
  }
  Object.assign(target, fresh);
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

function resolveRuntimeEngine(parsed: TeamDefinition): TeamDefinition['runtime_engine'] {
  const raw = String(parsed.runtime_engine || '')
    .trim()
    .toLowerCase();
  if (raw === 'graph' || parsed.team_graph_runtime === true) return 'graph';
  if (raw === 'native' || raw === '') return 'native';
  return parsed.runtime_engine;
}

export function parseDefinition(team: Team): TeamDefinition {
  try {
    const parsed = JSON.parse(team.definition_json || '{}') as TeamDefinition;
    const linkedFromTeam = String(team.linked_graph_id || '').trim();
    return withGraph({
      ...parsed,
      version: parsed.version || 1,
      description: parsed.description || '',
      runtime_engine: resolveRuntimeEngine(parsed),
      team_graph_runtime: parsed.team_graph_runtime === true,
      linked_graph_id: String(parsed.linked_graph_id || linkedFromTeam || '').trim() || undefined,
      failure_policy: parsed.failure_policy,
      mode: parsed.mode || 'sequential',
      max_concurrency: parsed.max_concurrency || 2,
      timeout_seconds: parsed.timeout_seconds || 600,
      loop_max_iterations: typeof parsed.loop_max_iterations === 'number' ? parsed.loop_max_iterations : 0,
      intent_anchor_agent_id: typeof parsed.intent_anchor_agent_id === 'string' ? parsed.intent_anchor_agent_id : '',
      a2a: parsed.a2a || defaultA2AConfig(),
      members: Array.isArray(parsed.members) ? parsed.members : [],
      graph: parsed.graph,
      synthesizer_agent_id: parsed.synthesizer_agent_id,
      critic_loop: parsed.critic_loop || { max_iterations: 3, score_threshold: 0.8 },
    });
  } catch {
    return defaultDefinition();
  }
}

/** 序列化 definition_json 时同步 runtime_engine / team_graph_runtime 双写，避免旧后端只读其一。 */
export function definitionToJSON(definition: TeamDefinition): string {
  const engine = String(definition.runtime_engine || 'graph')
    .trim()
    .toLowerCase();
  const payload: TeamDefinition = {
    ...definition,
    runtime_engine: engine === 'graph' ? 'graph' : 'native',
    team_graph_runtime: engine === 'graph',
    graph: definition.graph?.nodes?.length ? definition.graph : buildGraphFromDefinition(definition),
  };
  return JSON.stringify(payload);
}

export function defaultA2AConfig() {
  return {
    enabled: true,
    envelope_version: 'a2a.v1',
    message_format: 'markdown_json',
    include_trace: true,
    max_payload_chars: 6000,
  };
}

export function agentName(agents: Agent[], id: string) {
  return agents.find((agent) => agent.id === id)?.display_name || id || '未选择 Agent';
}

export function memberIcon(role: string) {
  return (
    (
      {
        coordinator: 'route',
        synthesizer: 'merge_type',
        generator: 'edit_note',
        critic: 'fact_check',
        worker: 'smart_toy',
      } as Record<string, string>
    )[role] || 'smart_toy'
  );
}

export function topologyNodesFromDefinition(def: TeamDefinition) {
  const mode = def.mode || 'sequential';
  if (mode === 'adaptive')
    return [
      { icon: 'auto_awesome', label: '分析任务' },
      { icon: 'route', label: '选择拓扑' },
      { icon: 'play_arrow', label: '执行' },
    ];
  if (mode === 'parallel')
    return [
      { icon: 'call_split', label: '并行分派' },
      { icon: 'groups', label: 'Worker' },
      { icon: 'merge_type', label: '汇总' },
    ];
  if (mode === 'coordinator')
    return [
      { icon: 'route', label: '主控拆分' },
      { icon: 'smart_toy', label: '成员执行' },
      { icon: 'summarize', label: '总结' },
    ];
  if (mode === 'critic_loop')
    return [
      { icon: 'edit_note', label: '生成' },
      { icon: 'fact_check', label: '评审' },
      { icon: 'loop', label: '迭代' },
    ];
  return [
    { icon: 'looks_one', label: '顺序 1' },
    { icon: 'arrow_forward', label: '传递' },
    { icon: 'flag', label: '最终输出' },
  ];
}

export function topologyNodes(team: Team) {
  return topologyNodesFromDefinition(parseDefinition(team));
}

export function withGraph(definition: TeamDefinition): TeamDefinition {
  return {
    ...definition,
    graph: definition.graph?.nodes?.length ? definition.graph : buildGraphFromDefinition(definition),
  };
}

export function formatDate(value: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

function teamDefinitionExtras(team: Team) {
  try {
    return JSON.parse(team.definition_json || '{}') as {
      category?: string;
      group?: string;
      industry_id?: string;
    };
  } catch {
    return {};
  }
}

/** 根据成员 Agent 所属行业投票；无成员时回退 definition 中的 category / industry_id。 */
export function inferTeamIndustryId(team: Team, agents: Agent[], taxonomyTree: PlatformResourceTreeNode[]): string {
  if (team.taxonomy_industry_id && findIndustryNode(taxonomyTree, team.taxonomy_industry_id)) {
    return team.taxonomy_industry_id;
  }
  const extras = teamDefinitionExtras(team);
  if (extras.industry_id && findIndustryNode(taxonomyTree, extras.industry_id)) {
    return extras.industry_id;
  }

  const industries = taxonomyTree.filter((node) => node.level === 'industry');
  const matchByName = (name?: string) => {
    const q = String(name || '').trim();
    if (!q) return '';
    return industries.find((node) => node.name === q)?.id ?? '';
  };
  const named = matchByName(extras.category) || matchByName(extras.group);
  if (named) return named;

  const def = parseDefinition(team);
  const counts = new Map<string, number>();
  for (const member of def.members.filter((row) => row.enabled !== false)) {
    const agent = agents.find((row) => row.id === member.agent_id);
    if (!agent?.taxonomy_position_id) continue;
    const industry = findTaxonomyPath(taxonomyTree, agent.taxonomy_position_id).find(
      (node) => node.level === 'industry',
    );
    if (!industry) continue;
    counts.set(industry.id, (counts.get(industry.id) ?? 0) + 1);
  }
  if (counts.size === 0) return UNCategorizedIndustryId;

  let bestId = UNCategorizedIndustryId;
  let bestCount = 0;
  for (const [id, count] of counts) {
    if (count > bestCount) {
      bestId = id;
      bestCount = count;
    }
  }
  return bestId;
}

function findIndustryNode(taxonomyTree: PlatformResourceTreeNode[], industryId: string) {
  return taxonomyTree.find((node) => node.level === 'industry' && node.id === industryId) ?? null;
}

export function industryOptionsFromTree(taxonomyTree: PlatformResourceTreeNode[]) {
  return taxonomyTree
    .filter((node) => node.level === 'industry')
    .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.name.localeCompare(b.name, 'zh-CN'))
    .map((node) => ({
      label: node.enabled ? node.name : `${node.name}（已停用）`,
      value: node.id,
    }));
}

export const BuiltinIndustryId = '__builtin__';

export function groupTeamsByIndustry(
  teams: Team[],
  agents: Agent[],
  taxonomyTree: PlatformResourceTreeNode[],
  industryFilter = '',
): TeamIndustryGroup[] {
  const builtinTeams = teams.filter((t) => t.readonly);
  const nonBuiltinTeams = teams.filter((t) => !t.readonly);

  const industries = taxonomyTree.filter((node) => node.level === 'industry');
  const buckets = new Map<string, Team[]>();
  buckets.set(UNCategorizedIndustryId, []);
  for (const industry of industries) buckets.set(industry.id, []);

  for (const team of nonBuiltinTeams) {
    const industryId = inferTeamIndustryId(team, agents, taxonomyTree);
    if (!buckets.has(industryId)) buckets.set(industryId, []);
    buckets.get(industryId)!.push(team);
  }

  const groups: TeamIndustryGroup[] = industries
    .map((industry) => ({
      id: industry.id,
      label: industry.enabled ? industry.name : `${industry.name}（已停用）`,
      sortOrder: industry.sort_order ?? 0,
      teams: buckets.get(industry.id) ?? [],
    }))
    .filter((group) => group.teams.length > 0);

  const uncategorized = buckets.get(UNCategorizedIndustryId) ?? [];
  const assigned = new Set(groups.flatMap((group) => group.teams.map((team) => team.id)));
  for (const team of nonBuiltinTeams) {
    if (!assigned.has(team.id)) uncategorized.push(team);
  }
  if (uncategorized.length > 0) {
    groups.push({
      id: UNCategorizedIndustryId,
      label: '未分类',
      sortOrder: 9999,
      teams: uncategorized,
    });
  }

  if (builtinTeams.length > 0) {
    groups.unshift({
      id: BuiltinIndustryId,
      label: '系统内置',
      sortOrder: -1,
      teams: builtinTeams,
    });
  }

  groups.sort((a, b) => a.sortOrder - b.sortOrder || a.label.localeCompare(b.label, 'zh-CN'));
  if (!industryFilter) return groups;
  return groups.filter((group) => group.id === industryFilter || group.id === BuiltinIndustryId);
}
