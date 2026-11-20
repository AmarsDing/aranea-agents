/**
 * 组织架构「编制图」的纯函数口径（无网络）：公司 → 部门 → 岗位，岗位上坐着 Agent，团队挂在部门上。
 * 页面与 Team 页共用这里的部门归属与统计，保证两边数字一致。
 */
import type { Agent } from '../agents/types';
import type { PlatformResourceTreeNode } from '../platform/types';
import type { Team } from '../teams/types';
import { inferTeamDepartmentId } from '../../components/teams/teamUtils';

export type OrgNode = PlatformResourceTreeNode;

const bySortThenName = (a: OrgNode, b: OrgNode) =>
  (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.name.localeCompare(b.name, 'zh-CN');

export function companiesOf(tree: OrgNode[]): OrgNode[] {
  return tree.filter((n) => n.level === 'company').sort(bySortThenName);
}

/** 总经理办公室：后端为每家公司自动建的治理部门（key = 公司 key + `_office`）。 */
export function isOfficeDept(dept: OrgNode, company: OrgNode | null | undefined): boolean {
  return Boolean(company) && dept.key === `${company!.key}_office`;
}

/** 部门按排序值排，总经理办公室固定在最前。 */
export function departmentsOf(company: OrgNode | null | undefined): OrgNode[] {
  if (!company) return [];
  const depts = (company.children ?? []).filter((n) => n.level === 'department').sort(bySortThenName);
  const office = depts.filter((d) => isOfficeDept(d, company));
  return [...office, ...depts.filter((d) => !isOfficeDept(d, company))];
}

export function positionsOf(dept: OrgNode | null | undefined): OrgNode[] {
  return (dept?.children ?? []).filter((n) => n.level === 'position').sort(bySortThenName);
}

/** 节点 id → 挂在它上面的 Agent（岗位上的专项、部门上的部门主管）。 */
export function agentsByNode(agents: Agent[]): Map<string, Agent[]> {
  const map = new Map<string, Agent[]>();
  for (const agent of agents) {
    const id = agent.taxonomy_position_id;
    if (!id) continue;
    map.set(id, [...(map.get(id) ?? []), agent]);
  }
  return map;
}

/** 部门主管：部门节点上记录的 Agent，退一步取挂在部门节点上、变体为 dept_lead 的 Agent。 */
export function deptLeadOf(dept: OrgNode, agents: Agent[], byNode: Map<string, Agent[]>): Agent | null {
  const id = dept.dept_lead_agent_id;
  return (
    (id && agents.find((a) => a.id === id)) ||
    (byNode.get(dept.id) ?? []).find((a) => a.agent_variant === 'dept_lead') ||
    null
  );
}

/** 公司总经理：公司 metadata_json.company_lead_agent_id。 */
export function companyLeadOf(company: OrgNode, agents: Agent[]): Agent | null {
  let id = '';
  try {
    id = String(
      (JSON.parse(company.metadata_json || '{}') as { company_lead_agent_id?: string }).company_lead_agent_id ?? '',
    );
  } catch {
    id = '';
  }
  return (id && agents.find((a) => a.id === id)) || null;
}

/** 部门 id → 团队；归属规则与 Team 页一致（显式部门优先，其次按成员所在部门投票）。 */
export function teamsByDepartment(teams: Team[], agents: Agent[], tree: OrgNode[]): Map<string, Team[]> {
  const map = new Map<string, Team[]>();
  for (const team of teams) {
    const id = inferTeamDepartmentId(team, agents, tree);
    map.set(id, [...(map.get(id) ?? []), team]);
  }
  return map;
}

export type OrgCounts = { departments: number; positions: number; agents: number; vacant: number; teams: number };

export function deptCounts(dept: OrgNode, byNode: Map<string, Agent[]>, byDept: Map<string, Team[]>): OrgCounts {
  const positions = positionsOf(dept);
  const seated = positions.filter((p) => (byNode.get(p.id)?.length ?? 0) > 0).length;
  return {
    departments: 1,
    positions: positions.length,
    agents: positions.reduce((sum, p) => sum + (byNode.get(p.id)?.length ?? 0), 0),
    vacant: positions.length - seated,
    teams: byDept.get(dept.id)?.length ?? 0,
  };
}

export function companyCounts(company: OrgNode, byNode: Map<string, Agent[]>, byDept: Map<string, Team[]>): OrgCounts {
  const total: OrgCounts = { departments: 0, positions: 0, agents: 0, vacant: 0, teams: 0 };
  for (const dept of departmentsOf(company)) {
    const c = deptCounts(dept, byNode, byDept);
    total.departments += 1;
    total.positions += c.positions;
    total.agents += c.agents;
    total.vacant += c.vacant;
    total.teams += c.teams;
  }
  return total;
}

const norm = (s: string | undefined) => (s ?? '').toLowerCase();

/** 岗位是否命中关键词：岗位名 / 描述 / 坐在上面的 Agent 名。空关键词恒为真。 */
export function positionMatches(pos: OrgNode, q: string, byNode: Map<string, Agent[]>): boolean {
  if (!q) return true;
  return (
    norm(pos.name).includes(q) ||
    norm(pos.description).includes(q) ||
    (byNode.get(pos.id) ?? []).some((a) => norm(a.display_name).includes(q) || norm(a.agent_key).includes(q))
  );
}

/** 部门命中数：部门名本身命中时算全部岗位，否则数命中的岗位。 */
export function deptMatchCount(dept: OrgNode, q: string, byNode: Map<string, Agent[]>): number {
  const positions = positionsOf(dept);
  if (!q || norm(dept.name).includes(q)) return positions.length || 1;
  return positions.filter((p) => positionMatches(p, q, byNode)).length;
}

export function companyMatchCount(company: OrgNode, q: string, byNode: Map<string, Agent[]>): number {
  return departmentsOf(company).reduce((sum, d) => sum + deptMatchCount(d, q, byNode), 0);
}

export type DeleteImpact = { departments: number; positions: number; unseated: number; leads: number; teams: number };

/** 删除一个节点会波及多少东西（与后端级联一致：岗位上的 Agent 变无岗、主管 Agent 删除、团队归档）。 */
export function deleteImpact(node: OrgNode, byNode: Map<string, Agent[]>, byDept: Map<string, Team[]>): DeleteImpact {
  const impact: DeleteImpact = { departments: 0, positions: 0, unseated: 0, leads: 0, teams: 0 };
  const addDept = (dept: OrgNode) => {
    impact.departments += 1;
    impact.leads += dept.dept_lead_agent_id ? 1 : 0;
    impact.teams += (byDept.get(dept.id) ?? []).filter((t) => t.status !== 'archived').length;
    for (const pos of positionsOf(dept)) {
      impact.positions += 1;
      impact.unseated += byNode.get(pos.id)?.length ?? 0;
    }
  };
  if (node.level === 'position') {
    impact.positions = 1;
    impact.unseated = byNode.get(node.id)?.length ?? 0;
  } else if (node.level === 'department') {
    addDept(node);
  } else {
    (node.children ?? []).filter((n) => n.level === 'department').forEach(addDept);
    impact.leads += 1; // 总经理
  }
  return impact;
}

/** 挡住删除的团队（后端 cascadeBlockActiveTeams 同口径：运行中 / 待执行）的名字。 */
export function blockingTeams(node: OrgNode, byDept: Map<string, Team[]>): string[] {
  const depts = node.level === 'department' ? [node] : node.level === 'company' ? departmentsOf(node) : [];
  return depts
    .flatMap((d) => byDept.get(d.id) ?? [])
    .filter((t) => t.status === 'running' || t.status === 'pending')
    .map((t) => t.display_name);
}

/** 同级上移 / 下移后的完整 id 顺序；到头返回 null。 */
export function moveSibling(siblings: OrgNode[], id: string, delta: -1 | 1): string[] | null {
  const ids = siblings.map((n) => n.id);
  const from = ids.indexOf(id);
  const to = from + delta;
  if (from < 0 || to < 0 || to >= ids.length) return null;
  [ids[from], ids[to]] = [ids[to], ids[from]];
  return ids;
}

export type DeptRailGroup = {
  id: string;
  name: string;
  items: Array<{ id: string; name: string; enabled: boolean; count: number }>;
};

/**
 * Team 页部门栏：按公司分组列出有团队的部门（计数来自已按其他条件筛过的团队）。
 * keepId 是当前选中的部门——即使它下面没有团队也要留在栏里（从组织页跳过来时常见）。
 */
export function teamDepartmentRail(deptIds: string[], tree: OrgNode[], keepId = ''): DeptRailGroup[] {
  const counts = new Map<string, number>();
  for (const id of deptIds) counts.set(id, (counts.get(id) ?? 0) + 1);
  return companiesOf(tree)
    .map((company) => ({
      id: company.id,
      name: company.name,
      items: departmentsOf(company)
        .filter((d) => (counts.get(d.id) ?? 0) > 0 || d.id === keepId)
        .map((d) => ({ id: d.id, name: d.name, enabled: d.enabled, count: counts.get(d.id) ?? 0 })),
    }))
    .filter((g) => g.items.length > 0);
}

/** 「公司 / 部门」；找不到返回空串。 */
export function departmentPath(tree: OrgNode[], deptId: string): string {
  for (const company of tree) {
    const dept = (company.children ?? []).find((d) => d.id === deptId);
    if (dept) return `${company.name} / ${dept.name}`;
  }
  return '';
}

/** Agent 自带的 emoji 图标（icon 字段）；不是 emoji（空、URL、图标名）返回空串 */
export function agentGlyph(agent: Agent | null | undefined): string {
  const icon = (agent?.icon ?? '').trim();
  return icon && icon.length <= 8 && /\p{Extended_Pictographic}/u.test(icon) ? icon : '';
}

export function findNode(tree: OrgNode[], id: string): OrgNode | null {
  for (const node of tree) {
    if (node.id === id) return node;
    const hit = findNode(node.children ?? [], id);
    if (hit) return hit;
  }
  return null;
}
