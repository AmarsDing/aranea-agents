/**
 * Team 展示用纯函数（无网络）。与 `components/teams/*.vue` 共址，见 aranea-frontend-guide SKILL §3.3 路径硬性约定。
 * 类型来自 `features/teams/api`（仅 type import）。
 *
 * 常量 → teamConstants.ts | 模板 → teamTemplates.ts | 本文件 → 解析/序列化/格式化/分组
 */
import type { Agent } from '../../features/agents/types';
import { findTaxonomyPath } from '../../features/platform/taxonomyTreeUtils';
import type { PlatformResourceTreeNode } from '../../features/platform/types';
import type { Team, TeamDefinition } from '../../features/teams/types';
import { buildGraphFromDefinition } from '../../features/teams/graphUtils';
import {
  runtimeEngineOptions,
  BuiltinIndustryId,
  PresetIndustryId,
  teamRoleLabel,
  teamModeLabel,
  failurePolicyValueLabel,
} from './teamConstants';
import { defaultDefinition, defaultA2AConfig, withGraph } from './teamTemplates';

// Re-export from split modules
export { buildGraphFromDefinition } from '../../features/teams/graphUtils';
export {
  teamStatusMap,
  modeOptions,
  statusOptions,
  roleOptions,
  teamTemplateOptions,
  runtimeEngineOptions,
  failureDefaultOptions,
  parallelFailOptions,
  failureOnErrorOptions,
  BuiltinIndustryId,
  PresetIndustryId,
  validStatusTransitions,
  isValidStatusTransition,
  teamModeMap,
  teamModeLabel,
  teamRoleMap,
  teamRoleLabel,
  failurePolicyValueMap,
  failurePolicyValueLabel,
  teamRunStatusLabel,
} from './teamConstants';
export type { TeamTemplateKey } from './teamConstants';
export { defaultDefinition, definitionFromTemplate, defaultA2AConfig, withGraph } from './teamTemplates';

export type TeamIndustryGroup = {
  id: string;
  label: string;
  sortOrder: number;
  teams: Team[];
};

const UNCategorizedIndustryId = '__uncategorized__';

// ── Role-mode validation (mirrors backend validRolesForMode) ──

/**
 * 后端 biz/validRolesForMode 的前端镜像。
 * 返回当前 mode 允许的角色集合；null 表示任意角色均允许（swarm/adaptive）。
 */
export function validRolesForMode(mode: string): Map<string, boolean> | null {
  switch (mode) {
    case 'critic_loop':
      return new Map([
        ['generator', true],
        ['critic', true],
        ['synthesizer', true],
      ]);
    case 'parallel':
      return new Map([
        ['synthesizer', true],
        ['worker', true],
      ]);
    case 'coordinator':
      return new Map([
        ['coordinator', true],
        ['worker', true],
        ['synthesizer', true],
      ]);
    case 'sequential':
      return new Map([['worker', true]]);
    case 'swarm':
    case 'adaptive':
      return null; // any role allowed
    default:
      return new Map(); // empty = no role allowed
  }
}

/** 根据 mode 过滤可选角色列表（供 TeamEditorDialog 角色下拉使用） */
export function roleOptionsForMode(mode: string): Array<{ label: string; value: string }> {
  const allowed = validRolesForMode(mode);
  if (allowed === null) {
    // swarm/adaptive: all roles
    return ['worker', 'coordinator', 'synthesizer', 'generator', 'critic'].map((v) => ({
      label: teamRoleLabel(v),
      value: v,
    }));
  }
  return Array.from(allowed.keys()).map((v) => ({ label: teamRoleLabel(v), value: v }));
}

/**
 * 前端 Team 定义校验，对齐后端 biz/validateTeamDefinition。
 * 返回错误提示字符串；null 表示通过。
 *
 * ADR-08 A4：definition 携带 embedded graph（custom 路径）时，拓扑以 graph 为
 * 唯一真相源，跳过 role-mode 耦合校验（角色兼容 / parallel 汇总 / coordinator /
 * critic_loop 角色要求）；结构问题由 CompileTeamGraph 编译期校验报告。
 */
export function validateTeamDefinition(definition: TeamDefinition): string | null {
  const mode = String(definition.mode || 'sequential').toLowerCase();
  const enabled = definition.members.filter((m) => m.enabled !== false && String(m.agent_id || '').trim() !== '');

  // 至少一个 enabled 成员
  if (enabled.length === 0) {
    return '请至少启用一名成员并选择 Agent';
  }

  // 成员 agent_id 必填
  const missingAgent = definition.members.find((m) => m.enabled !== false && !String(m.agent_id || '').trim());
  if (missingAgent) {
    return '所有启用成员必须选择 Agent';
  }

  // A4：custom graph 路径跳过 role-mode 耦合校验
  if (definition.graph?.nodes?.length) {
    return null;
  }

  // 角色-模式兼容性
  const allowedRoles = validRolesForMode(mode);
  if (allowedRoles !== null) {
    const invalid = enabled.find((m) => {
      const role = String(m.role || '').trim();
      return role && !allowedRoles.has(role);
    });
    if (invalid) {
      return `角色「${teamRoleLabel(invalid.role)}」与模式「${teamModeLabel(mode)}」不兼容`;
    }
  }

  // parallel: 需要 synthesizer
  if (mode === 'parallel') {
    const synthRaw = String(definition.synthesizer_agent_id || '').trim();
    const synthFromRole = enabled.find((m) => String(m.role || '').toLowerCase() === 'synthesizer')?.agent_id?.trim();
    const synth = synthRaw || synthFromRole || '';
    if (!synth) {
      return '并行模式需要指定汇总 Agent（synthesizer_agent_id 或「汇总」角色成员）';
    }
    const workers = enabled.filter((m) => String(m.agent_id).trim() !== synth);
    if (workers.length === 0) {
      return '并行模式至少需要一名与汇总 Agent 不同的并行成员';
    }
  }

  // coordinator: 需要 synthesizer 或 coordinator
  if (mode === 'coordinator') {
    const hasSynth = enabled.some((m) => String(m.role || '').toLowerCase() === 'synthesizer');
    const hasCoord = enabled.some((m) => String(m.role || '').toLowerCase() === 'coordinator');
    const hasSynthAgent = String(definition.synthesizer_agent_id || '').trim() !== '';
    if (!hasSynth && !hasCoord && !hasSynthAgent) {
      return '主控模式需要「汇总」或「协调」角色成员，或指定 synthesizer_agent_id';
    }
  }

  // critic_loop: 需要 generator + critic
  if (mode === 'critic_loop') {
    const hasGenerator = enabled.some((m) => String(m.role || '').toLowerCase() === 'generator');
    const hasCritic = enabled.some((m) => String(m.role || '').toLowerCase() === 'critic');
    if (!hasGenerator || !hasCritic) {
      return '生成评审模式需要「生成」和「评审」角色成员';
    }
  }

  return null;
}

// ── Label helpers ──

export function runtimeEngineLabel(value?: string) {
  const v = String(value || 'graph').toLowerCase() === 'native' ? 'native' : 'graph';
  return runtimeEngineOptions.find((o) => o.value === v)?.label ?? v;
}

export function failurePolicySummary(def: TeamDefinition): string {
  const fp = def.failure_policy;
  if (!fp) return '重试后阻塞（默认）';
  const parts: string[] = [];
  if (fp.default) parts.push(`默认: ${failurePolicyValueLabel(fp.default)}`);
  if (def.mode === 'parallel' && fp.parallel_fail) parts.push(`并行: ${failurePolicyValueLabel(fp.parallel_fail)}`);
  if (fp.retry?.max_attempts != null) parts.push(`重试: ${fp.retry.max_attempts}`);
  if (fp.circuit_breaker?.failure_threshold) parts.push(`熔断: ${fp.circuit_breaker.failure_threshold}`);
  if (fp.on_error) parts.push(`错误接管: ${failurePolicyValueLabel(fp.on_error)}`);
  return parts.length ? parts.join(' · ') : '—';
}

// ── Definition reset ──

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

// ── Definition parse / serialize ──

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
    let members = Array.isArray(parsed.members) ? parsed.members : [];
    // Backfill members from graph.nodes when members is empty but graph has agent nodes.
    // This handles data inconsistency where some teams were created with graph.nodes
    // but empty members array (e.g. certain pack import paths).
    if (members.length === 0 && parsed.graph?.nodes?.length) {
      members = parsed.graph.nodes
        .filter((n) => n.type === 'agent' && n.agent_id)
        .map((n, i) => ({
          agent_id: n.agent_id!,
          role: n.role || 'worker',
          name: n.label || `Agent ${i + 1}`,
          task_prompt: n.task_prompt || '',
          enabled: n.enabled !== undefined ? n.enabled : true,
          sort_order: i + 1,
        }));
    }
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
      members,
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

// ── ADR-08 A1 派生同步：拓扑字段 → graph 单向派生 ──

/**
 * 拓扑字段指纹：仅覆盖驱动 graph 结构的字段（mode / synthesizer_agent_id / members 拓扑）。
 * 非拓扑字段（description / timeout / failure_policy 等）变化不影响返回值；
 * members 数组顺序不影响（按 sort_order 稳定排序后取指纹）。
 */
export function definitionTopologyKey(definition: TeamDefinition): string {
  const members = (definition.members || [])
    .map((member, index) => ({ member, index }))
    .sort((a, b) => (a.member.sort_order ?? 0) - (b.member.sort_order ?? 0) || a.index - b.index)
    .map(({ member }) =>
      [
        String(member.agent_id || '').trim(),
        String(member.role || '')
          .trim()
          .toLowerCase(),
        String(member.name || '').trim(),
        member.enabled === false ? '0' : '1',
        String(member.sort_order ?? 0),
      ].join(''),
    )
    .join('');
  return [
    String(definition.mode || 'sequential')
      .trim()
      .toLowerCase(),
    String(definition.synthesizer_agent_id || '').trim(),
    members,
  ].join('');
}

/**
 * 从拓扑字段重建 embedded graph（ADR-08 A1：杜绝陈旧 graph 覆盖 mode/members 改动）。
 * layout 未变时保留旧 graph 中存活节点的 x/y，避免画布坐标因重建漂移。
 * 注意：本地 builder 仅为离线/失败回退（ADR-08 A2）；在线时应以后端
 * compile 返回的 definition_graph_json 为准（见 definitionGraphFromCompileJSON）。
 */
export function rebuildDefinitionGraph(definition: TeamDefinition): NonNullable<TeamDefinition['graph']> {
  const next = buildGraphFromDefinition(definition);
  const previous = definition.graph;
  if (!previous?.nodes?.length || previous.layout !== next.layout) return next;
  const positions = new Map(previous.nodes.map((node) => [node.id, { x: node.x, y: node.y }]));
  next.nodes = next.nodes.map((node) => {
    const position = positions.get(node.id);
    return position ? { ...node, x: position.x, y: position.y } : node;
  });
  return next;
}

// ── ADR-08 A3 角色派生：mode + 成员顺序 → role / synthesizer_agent_id ──

/** 角色由编排模式派生的模式集合（其余模式角色自由，编辑器中可人工编辑）。 */
export const derivedRoleModes: ReadonlySet<string> = new Set(['sequential', 'parallel', 'coordinator', 'critic_loop']);

/**
 * 按 mode + 成员顺序派生启用成员的 role，并同步 synthesizer_agent_id（位置制，
 * 与 teamTemplates 模板语义一致）：
 * - sequential：全部 worker
 * - parallel：sort_order 最后的启用成员 = synthesizer（回写 synthesizer_agent_id），其余 worker
 * - coordinator：sort_order 首位启用成员 = coordinator，其余 worker
 * - critic_loop：按 sort_order 交替 generator / critic
 * - adaptive / swarm：不派生（角色自由）；仅清理残留的 synthesizer_agent_id
 *
 * 直接修改 definition；返回是否有任何字段被改动（幂等，二次调用返回 false）。
 */
export function deriveMemberRolesForMode(definition: TeamDefinition): boolean {
  const mode = String(definition.mode || 'sequential')
    .trim()
    .toLowerCase();
  let changed = false;
  const setRole = (member: TeamDefinition['members'][number], role: string) => {
    if (member.role !== role) {
      member.role = role;
      changed = true;
    }
  };

  const enabledSorted = definition.members
    .map((member, index) => ({ member, index }))
    .filter(({ member }) => member.enabled !== false)
    .sort((a, b) => (a.member.sort_order ?? 0) - (b.member.sort_order ?? 0) || a.index - b.index)
    .map(({ member }) => member);

  if (mode === 'parallel') {
    const synth = enabledSorted[enabledSorted.length - 1];
    for (const member of enabledSorted) setRole(member, member === synth ? 'synthesizer' : 'worker');
    const synthAgentId = String(synth?.agent_id || '').trim();
    if (String(definition.synthesizer_agent_id || '').trim() !== synthAgentId) {
      definition.synthesizer_agent_id = synthAgentId || undefined;
      changed = true;
    }
    return changed;
  }

  if (derivedRoleModes.has(mode)) {
    enabledSorted.forEach((member, position) => {
      if (mode === 'sequential') setRole(member, 'worker');
      else if (mode === 'coordinator') setRole(member, position === 0 ? 'coordinator' : 'worker');
      else setRole(member, position % 2 === 0 ? 'generator' : 'critic');
    });
  }
  // synthesizer_agent_id 仅 parallel 有意义；其余模式清理残留，保持拓扑指纹干净。
  if (String(definition.synthesizer_agent_id || '').trim() !== '') {
    delete definition.synthesizer_agent_id;
    changed = true;
  }
  return changed;
}

/**
 * ADR-08 A2：把后端 CompileTeamGraph 返回的 definition_graph_json（模板生成的
 * canonical embedded spec，含 start/end 装饰节点、无坐标）转换为 definition.graph。
 * 前端只负责坐标：layout 与 prior 一致时按节点 id 保留旧坐标，新节点给网格坐标。
 * 返回 null 表示规格不可用（空/非法/无节点），调用方回退 rebuildDefinitionGraph。
 */
export function definitionGraphFromCompileJSON(
  specJSON: string,
  prior?: TeamDefinition['graph'],
): NonNullable<TeamDefinition['graph']> | null {
  const raw = String(specJSON || '').trim();
  if (!raw) return null;
  let spec: {
    version?: number;
    layout?: string;
    nodes?: Array<{ id?: string; type?: string; label?: string; agent_id?: string; role?: string }>;
    edges?: Array<{ id?: string; source?: string; target?: string; label?: string; condition?: string }>;
  };
  try {
    spec = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!spec || !Array.isArray(spec.nodes) || spec.nodes.length === 0) return null;

  const layout = String(spec.layout || '');
  const keepPositions = Boolean(prior?.nodes?.length) && prior?.layout === layout;
  const positions = new Map((keepPositions ? prior?.nodes : undefined)?.map((node) => [node.id, node]) ?? []);
  let bodyIndex = 0;
  const bodyCount = spec.nodes.filter((node) => {
    const type = String(node.type || '').toLowerCase();
    return type !== 'start' && type !== 'end';
  }).length;
  const nodes = spec.nodes.map((node) => {
    const id = String(node.id || '').trim();
    const type = String(node.type || 'agent').toLowerCase();
    const isStart = type === 'start';
    const isEnd = type === 'end';
    const grid = {
      x: isStart ? 0 : isEnd ? 160 + bodyCount * 150 : 160 + bodyIndex++ * 150,
      y: 80,
    };
    const kept = positions.get(id);
    return {
      id,
      type: node.type || 'agent',
      label: node.label || id,
      ...(node.agent_id ? { agent_id: node.agent_id } : {}),
      ...(node.role ? { role: node.role } : {}),
      x: kept?.x ?? grid.x,
      y: kept?.y ?? grid.y,
    };
  });
  const edges = (Array.isArray(spec.edges) ? spec.edges : [])
    .map((edge) => {
      const source = String(edge.source || '').trim();
      const target = String(edge.target || '').trim();
      if (!source || !target) return null;
      return {
        id: String(edge.id || '').trim() || `${source}-${target}`,
        source,
        target,
        ...(edge.label ? { label: edge.label } : {}),
        ...(edge.condition ? { condition: edge.condition } : {}),
      };
    })
    .filter((edge): edge is NonNullable<typeof edge> => edge !== null);
  return { version: spec.version || 1, layout, nodes, edges };
}

// ── Agent helpers ──

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

// ── Topology ──

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
      { icon: 'groups', label: '并行执行' },
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

// ── Date ──

export function formatDate(value: string) {
  if (!value) return '-';
  return new Date(value).toLocaleString();
}

// ── Industry grouping ──

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
  if (team.taxonomy_industry_id) {
    const found = findIndustryNode(taxonomyTree, team.taxonomy_industry_id);
    if (found) return found.id; // normalize key → UUID
  }
  const extras = teamDefinitionExtras(team);
  if (extras.industry_id) {
    const found = findIndustryNode(taxonomyTree, extras.industry_id);
    if (found) return found.id; // normalize key → UUID
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
  return (
    taxonomyTree.find((node) => node.level === 'industry' && (node.id === industryId || node.key === industryId)) ??
    null
  );
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

export function groupTeamsByIndustry(
  teams: Team[],
  agents: Agent[],
  taxonomyTree: PlatformResourceTreeNode[],
  industryFilter = '',
): TeamIndustryGroup[] {
  const builtinTeams = teams.filter((t) => t.readonly);
  const presetTeams = teams.filter((t) => !t.readonly && t.kind === 'ecosystem_preset');
  const userTeams = teams.filter((t) => !t.readonly && t.kind !== 'ecosystem_preset');

  const industries = taxonomyTree.filter((node) => node.level === 'industry');
  const buckets = new Map<string, Team[]>();
  buckets.set(UNCategorizedIndustryId, []);
  for (const industry of industries) buckets.set(industry.id, []);

  for (const team of userTeams) {
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

  const assigned = new Set(groups.flatMap((group) => group.teams.map((team) => team.id)));
  // Uncategorized = user teams not claimed by any visible industry group.
  // Filtering from userTeams (instead of appending to the bucket) prevents
  // double-adding teams already placed in the uncategorized bucket above.
  const uncategorized = userTeams.filter((team) => !assigned.has(team.id));
  if (uncategorized.length > 0) {
    groups.push({
      id: UNCategorizedIndustryId,
      label: '未分类',
      sortOrder: 9999,
      teams: uncategorized,
    });
  }

  if (presetTeams.length > 0) {
    groups.unshift({
      id: PresetIndustryId,
      label: '预设模板',
      sortOrder: 0,
      teams: presetTeams,
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
  return groups.filter(
    (group) => group.id === industryFilter || group.id === BuiltinIndustryId || group.id === PresetIndustryId,
  );
}
