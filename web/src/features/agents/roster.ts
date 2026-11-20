/**
 * 编制花名册（/agents）的口径：把 Agent 挂到组织树上（公司 → 部门 → 岗位），算在编数、缺编、汇报关系。
 * 纯函数，不发请求；页面一次拉全量 Agent + 组织树后在前端归组、筛选。
 *
 * 业务约定见 docs/development/org-invariants.md：
 *  - 总经理（company_lead）挂在「总经理办公室」的岗位上；部门主管（dept_lead）直接挂在部门节点上；
 *  - 专项（行业包导入的 ecosystem_preset）挂在岗位上；管家（system_builtin 且不是领导岗）不挂岗。
 */
import type { Agent } from './types';
import type { PlatformResourceTreeNode as OrgNode } from '../platform/types';

export type RosterRole = 'butler' | 'company_lead' | 'dept_lead' | 'specialist' | 'custom';

export const ROSTER_ROLES: RosterRole[] = ['company_lead', 'dept_lead', 'specialist', 'custom', 'butler'];

export function rosterRole(agent: Agent): RosterRole {
  if (agent.agent_variant === 'company_lead') return 'company_lead';
  if (agent.agent_variant === 'dept_lead') return 'dept_lead';
  if (agent.kind === 'system_builtin') return 'butler';
  if (agent.kind === 'ecosystem_preset') return 'specialist';
  return 'custom';
}

/** 与后端 canDeleteAgent（internal/biz/agent_usecase.go）一致：内置、行业包导入、只读的都不能直接删。 */
export function canDeleteAgent(agent: Agent): boolean {
  return agent.kind !== 'system_builtin' && agent.kind !== 'ecosystem_preset' && !agent.readonly;
}

/** 批量启停只对非只读的生效（后端 BatchUpdateAgents 同样跳过只读）。 */
export function canToggleAgent(agent: Agent): boolean {
  return !agent.readonly;
}

export type OrgIndex = {
  nodes: Map<string, OrgNode>;
  parentOf: Map<string, string>;
  companies: OrgNode[];
};

const bySort = (a: OrgNode, b: OrgNode) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.name.localeCompare(b.name);

export function buildOrgIndex(tree: OrgNode[]): OrgIndex {
  const nodes = new Map<string, OrgNode>();
  const parentOf = new Map<string, string>();
  const walk = (node: OrgNode, parent: string) => {
    nodes.set(node.id, node);
    if (parent) parentOf.set(node.id, parent);
    for (const child of node.children ?? []) walk(child, node.id);
  };
  for (const root of tree) walk(root, '');
  return { nodes, parentOf, companies: tree.filter((n) => n.level === 'company').sort(bySort) };
}

/** 从公司到该节点的路径；节点不在树上时返回空数组。 */
export function orgPath(index: OrgIndex, id: string): OrgNode[] {
  const path: OrgNode[] = [];
  let cur = id ? index.nodes.get(id) : undefined;
  while (cur) {
    path.unshift(cur);
    const parent = index.parentOf.get(cur.id);
    cur = parent ? index.nodes.get(parent) : undefined;
  }
  return path;
}

export type Placement = { company?: OrgNode; department?: OrgNode; position?: OrgNode };

export function placementOf(index: OrgIndex, agent: Agent): Placement {
  const out: Placement = {};
  for (const node of orgPath(index, agent.taxonomy_position_id)) {
    if (node.level === 'company') out.company = node;
    else if (node.level === 'department') out.department = node;
    else if (node.level === 'position') out.position = node;
  }
  return out;
}

export type RailDept = {
  node: OrgNode;
  count: number;
  lead?: Agent;
  positions: OrgNode[];
  vacant: OrgNode[];
};
export type RailCompany = { node: OrgNode; count: number; lead?: Agent; departments: RailDept[] };
export type Rail = { companies: RailCompany[]; butlers: Agent[]; unassigned: Agent[]; favorites: Agent[] };

/** 左侧编制树：每个公司 / 部门的在编人数、负责人、岗位与缺编岗位。 */
export function buildRail(index: OrgIndex, agents: Agent[]): Rail {
  const deptCount = new Map<string, number>();
  const companyCount = new Map<string, number>();
  const filledPositions = new Set<string>();
  const deptLead = new Map<string, Agent>();
  const companyLead = new Map<string, Agent>();
  const butlers: Agent[] = [];
  const unassigned: Agent[] = [];

  for (const agent of agents) {
    const role = rosterRole(agent);
    const place = placementOf(index, agent);
    if (!place.company) {
      (role === 'butler' ? butlers : unassigned).push(agent);
      continue;
    }
    companyCount.set(place.company.id, (companyCount.get(place.company.id) ?? 0) + 1);
    if (place.department) deptCount.set(place.department.id, (deptCount.get(place.department.id) ?? 0) + 1);
    if (place.position) filledPositions.add(place.position.id);
    if (role === 'dept_lead' && place.department) deptLead.set(place.department.id, agent);
    if (role === 'company_lead') {
      companyLead.set(place.company.id, agent);
      // 总经理办公室没有部门主管，由总经理本人坐镇
      if (place.department && !deptLead.has(place.department.id)) deptLead.set(place.department.id, agent);
    }
  }

  const companies = index.companies.map((company) => ({
    node: company,
    count: companyCount.get(company.id) ?? 0,
    lead: companyLead.get(company.id),
    departments: (company.children ?? [])
      .filter((d) => d.level === 'department')
      .sort(bySort)
      .map((dept) => {
        const positions = (dept.children ?? []).filter((p) => p.level === 'position').sort(bySort);
        return {
          node: dept,
          count: deptCount.get(dept.id) ?? 0,
          lead: deptLead.get(dept.id),
          positions,
          vacant: positions.filter((p) => !filledPositions.has(p.id)),
        };
      }),
  }));
  return { companies, butlers, unassigned, favorites: agents.filter((a) => a.is_favorite) };
}

/** 左栏的固定入口；其余取值是组织节点 id（公司或部门）。 */
export type RosterScope = 'all' | 'butlers' | 'unassigned' | 'favorites' | (string & {});
export type RosterStatus = '' | 'active' | 'inactive' | 'failed';

export type RosterFilter = {
  scope: RosterScope;
  q: string;
  role: RosterRole | '';
  status: RosterStatus;
};

function inScope(index: OrgIndex, agent: Agent, scope: RosterScope): boolean {
  if (scope === 'all') return true;
  if (scope === 'favorites') return agent.is_favorite;
  const place = placementOf(index, agent);
  if (scope === 'butlers') return !place.company && rosterRole(agent) === 'butler';
  if (scope === 'unassigned') return !place.company && rosterRole(agent) !== 'butler';
  return place.company?.id === scope || place.department?.id === scope || place.position?.id === scope;
}

function matchesText(index: OrgIndex, agent: Agent, q: string): boolean {
  if (!q) return true;
  const place = placementOf(index, agent);
  return [
    agent.display_name,
    agent.agent_key,
    agent.agent_description,
    agent.provider,
    agent.model,
    place.position?.name,
    place.department?.name,
  ].some((v) => (v ?? '').toLowerCase().includes(q));
}

function matchesStatus(agent: Agent, status: RosterStatus): boolean {
  if (!status) return true;
  if (status === 'failed') return agent.last_run_status === 'failed';
  return agent.status === status;
}

/** 范围 + 文本 + 状态，不含角色（角色页签上的计数要在这一层算）。 */
export function scopeRoster(index: OrgIndex, agents: Agent[], f: Omit<RosterFilter, 'role'>): Agent[] {
  const q = f.q.trim().toLowerCase();
  return agents.filter((a) => inScope(index, a, f.scope) && matchesText(index, a, q) && matchesStatus(a, f.status));
}

export function countByRole(agents: Agent[]): Record<RosterRole | 'all', number> {
  const out = { all: agents.length, butler: 0, company_lead: 0, dept_lead: 0, specialist: 0, custom: 0 };
  for (const a of agents) out[rosterRole(a)] += 1;
  return out;
}

export type RosterGroup = {
  key: string;
  /** 部门分组：公司名；其余为空 */
  company: string;
  title: string;
  lead?: Agent;
  agents: Agent[];
};

const ROLE_ORDER: Record<RosterRole, number> = { company_lead: 0, dept_lead: 1, specialist: 2, custom: 3, butler: 4 };

/** 按部门归组（管家一组在最前，未挂岗一组在最后），组内领导在前、再按岗位顺序与名称。 */
export function groupRoster(index: OrgIndex, agents: Agent[]): RosterGroup[] {
  const groups = new Map<string, RosterGroup & { order: number }>();
  const companyOrder = new Map(index.companies.map((c, i) => [c.id, i]));
  for (const agent of agents) {
    const place = placementOf(index, agent);
    const role = rosterRole(agent);
    let key: string;
    let order: number;
    let company = '';
    let title: string;
    if (!place.company) {
      key = role === 'butler' ? 'butlers' : 'unassigned';
      order = role === 'butler' ? -1 : Number.MAX_SAFE_INTEGER;
      title = key;
    } else {
      const holder = place.department ?? place.company;
      key = holder.id;
      order = (companyOrder.get(place.company.id) ?? 0) * 10_000 + (place.department?.sort_order ?? 0);
      company = place.company.name;
      title = holder.name;
    }
    const group = groups.get(key) ?? { key, company, title, agents: [], order };
    group.agents.push(agent);
    if (role === 'dept_lead' || (role === 'company_lead' && !group.lead)) group.lead = agent;
    groups.set(key, group);
  }
  const posOrder = (a: Agent) => index.nodes.get(a.taxonomy_position_id)?.sort_order ?? 0;
  return [...groups.values()]
    .sort((a, b) => a.order - b.order)
    .map(({ order: _order, ...g }) => ({
      ...g,
      agents: g.agents.sort(
        (a, b) =>
          ROLE_ORDER[rosterRole(a)] - ROLE_ORDER[rosterRole(b)] ||
          posOrder(a) - posOrder(b) ||
          a.display_name.localeCompare(b.display_name),
      ),
    }));
}

/** 汇报对象：专项 → 本部门主管 → 本公司总经理；领导岗往上一级。 */
export function reportsTo(rail: Rail, index: OrgIndex, agent: Agent): Agent | undefined {
  const place = placementOf(index, agent);
  if (!place.company) return undefined;
  const company = rail.companies.find((c) => c.node.id === place.company?.id);
  if (!company) return undefined;
  const role = rosterRole(agent);
  if (role === 'company_lead') return undefined;
  const dept = company.departments.find((d) => d.node.id === place.department?.id);
  if (role !== 'dept_lead' && dept?.lead && dept.lead.id !== agent.id) return dept.lead;
  return company.lead && company.lead.id !== agent.id ? company.lead : undefined;
}

/** 补人时的默认标识：岗位 key 转成后端允许的小写 + 连字符，再加 -custom 避免与行业包的 key 撞车。 */
export function vacancyAgentKey(position: OrgNode): string {
  const base = (position.key || position.name)
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return base ? `${base}-custom` : '';
}
