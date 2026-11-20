import { describe, expect, it } from 'vitest';
import {
  agentsByNode,
  blockingTeams,
  companyCounts,
  deleteImpact,
  departmentsOf,
  deptMatchCount,
  moveSibling,
  teamDepartmentRail,
  teamsByDepartment,
  type OrgNode,
} from '../orgChart';
import type { Agent } from '../../agents/types';
import type { Team } from '../../teams/types';

function node(id: string, level: string, extra: Partial<OrgNode> = {}): OrgNode {
  return {
    id,
    key: id,
    name: id,
    level,
    description: '',
    enabled: true,
    sort_order: 10,
    parent_id: '',
    is_system: false,
    metadata_json: '{}',
    dept_lead_agent_id: '',
    children: [],
    ...extra,
  } as OrgNode;
}

const pos1 = node('p1', 'position', { name: '编辑', parent_id: 'd1', sort_order: 20 });
const pos2 = node('p2', 'position', { name: '设计', parent_id: 'd1', sort_order: 10 });
const dept1 = node('d1', 'department', {
  name: '内容部',
  parent_id: 'c1',
  sort_order: 20,
  dept_lead_agent_id: 'lead',
  children: [pos1, pos2],
});
const office = node('acme_office', 'department', { key: 'acme_office', parent_id: 'c1', sort_order: 99 });
const company = node('c1', 'company', { key: 'acme', children: [dept1, office] });
const tree = [company];

const agents = [
  { id: 'a1', display_name: '写手 Agent', agent_key: 'writer', taxonomy_position_id: 'p1' },
  { id: 'lead', display_name: '主管', agent_key: 'lead', taxonomy_position_id: 'd1', agent_variant: 'dept_lead' },
] as Agent[];

const team = (id: string, members: string[], dept = '') =>
  ({
    id,
    status: 'pending',
    taxonomy_industry_id: dept,
    definition_json: JSON.stringify({
      members: members.map((agent_id, i) => ({ agent_id, enabled: true, sort_order: i })),
    }),
  }) as Team;

describe('orgChart', () => {
  const byNode = agentsByNode(agents);

  it('puts the 总经理办公室 first regardless of sort order', () => {
    expect(departmentsOf(company).map((d) => d.id)).toEqual(['acme_office', 'd1']);
  });

  it('counts positions, seated agents and vacancies per company', () => {
    const byDept = teamsByDepartment([team('t1', ['a1'])], agents, tree);
    expect(companyCounts(company, byNode, byDept)).toEqual({
      departments: 2,
      positions: 2,
      agents: 1,
      vacant: 1,
      teams: 1,
    });
  });

  it('matches positions by the agent sitting on them', () => {
    expect(deptMatchCount(dept1, '写手', byNode)).toBe(1);
    expect(deptMatchCount(dept1, '内容', byNode)).toBe(2); // 部门名命中 = 全部岗位
    expect(deptMatchCount(dept1, 'nothing', byNode)).toBe(0);
  });

  it('describes what a delete takes down (same cascade as the backend)', () => {
    const byDept = teamsByDepartment([team('t1', ['a1'])], agents, tree);
    expect(deleteImpact(pos1, byNode, byDept)).toMatchObject({ positions: 1, unseated: 1 });
    expect(deleteImpact(dept1, byNode, byDept)).toEqual({
      departments: 1,
      positions: 2,
      unseated: 1,
      leads: 1,
      teams: 1,
    });
    expect(deleteImpact(company, byNode, byDept)).toMatchObject({ departments: 2, positions: 2, leads: 2, teams: 1 });
  });

  it('lists running/pending teams that block a delete, at department and company level', () => {
    const byDept = teamsByDepartment(
      [
        { ...team('t1', ['a1']), display_name: 'T1' },
        { ...team('t2', ['a1']), display_name: 'T2', status: 'completed' },
      ],
      agents,
      tree,
    );
    expect(blockingTeams(dept1, byDept)).toEqual(['T1']);
    expect(blockingTeams(company, byDept)).toEqual(['T1']);
    expect(blockingTeams(pos1, byDept)).toEqual([]);
  });

  it('moves a sibling and refuses to move past the ends', () => {
    expect(moveSibling([pos2, pos1], 'p1', -1)).toEqual(['p1', 'p2']);
    expect(moveSibling([pos2, pos1], 'p2', -1)).toBeNull();
  });

  it('team rail lists only departments with teams, plus the selected one', () => {
    expect(teamDepartmentRail(['d1', 'd1'], tree)).toEqual([
      { id: 'c1', name: 'c1', items: [{ id: 'd1', name: '内容部', enabled: true, count: 2 }] },
    ]);
    expect(teamDepartmentRail([], tree, 'acme_office')[0]?.items.map((i) => i.id)).toEqual(['acme_office']);
  });
});
