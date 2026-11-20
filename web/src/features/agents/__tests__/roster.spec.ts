import { describe, expect, it } from 'vitest';
import type { Agent } from '../types';
import type { PlatformResourceTreeNode } from '../../platform/types';
import {
  buildOrgIndex,
  buildRail,
  canDeleteAgent,
  countByRole,
  groupRoster,
  reportsTo,
  rosterRole,
  scopeRoster,
  vacancyAgentKey,
} from '../roster';

const node = (id: string, level: string, name: string, children: PlatformResourceTreeNode[] = [], sort = 0) =>
  ({ id, level, name, key: id, sort_order: sort, children }) as unknown as PlatformResourceTreeNode;

const agent = (id: string, pos: string, extra: Partial<Agent> = {}) =>
  ({
    id,
    agent_key: id,
    display_name: id,
    agent_description: '',
    provider: 'deepseek',
    model: 'chat',
    status: 'active',
    is_favorite: false,
    taxonomy_position_id: pos,
    kind: 'ecosystem_preset',
    ...extra,
  }) as Agent;

// 公司 C：总经理办公室 GMO（岗位 gm）、财务部 FIN（岗位 acct、budget）
const tree = [
  node('C', 'company', '公司', [
    node('GMO', 'department', '总经理办公室', [node('gm', 'position', '总经理')], 0),
    node(
      'FIN',
      'department',
      '财务部',
      [node('acct', 'position', '会计', [], 1), node('budget_planner', 'position', '预算', [], 2)],
      1,
    ),
  ]),
];
const ceo = agent('ceo', 'gm', { agent_variant: 'company_lead', kind: 'system_builtin', readonly: true });
const finLead = agent('fin-lead', 'FIN', { agent_variant: 'dept_lead', kind: 'system_builtin', readonly: true });
const acct = agent('acct-a', 'acct');
const mine = agent('mine', 'acct', { kind: 'user', last_run_status: 'failed', is_favorite: true });
const spirit = agent('spirit', '', { kind: 'system_builtin', readonly: true });
const stray = agent('stray', 'gone');
const all = [stray, acct, mine, finLead, spirit, ceo];

describe('roster', () => {
  const index = buildOrgIndex(tree);
  const rail = buildRail(index, all);

  it('classifies roles and delete rights like the backend', () => {
    expect([ceo, finLead, acct, mine, spirit].map(rosterRole)).toEqual([
      'company_lead',
      'dept_lead',
      'specialist',
      'custom',
      'butler',
    ]);
    expect([ceo, finLead, acct, mine, spirit].map(canDeleteAgent)).toEqual([false, false, false, true, false]);
  });

  it('counts headcount, leads and vacancies per department', () => {
    const [company] = rail.companies;
    expect(company.count).toBe(4);
    expect(company.lead?.id).toBe('ceo');
    const fin = company.departments.find((d) => d.node.id === 'FIN')!;
    expect(fin.count).toBe(3); // 主管挂在部门节点上也算在编
    expect(fin.lead?.id).toBe('fin-lead');
    expect(fin.vacant.map((p) => p.id)).toEqual(['budget_planner']);
    const gmo = company.departments.find((d) => d.node.id === 'GMO')!;
    expect(gmo.vacant).toEqual([]);
    expect(gmo.lead?.id).toBe('ceo');
    expect(rail.butlers.map((a) => a.id)).toEqual(['spirit']);
    expect(rail.unassigned.map((a) => a.id)).toEqual(['stray']);
    expect(rail.favorites.map((a) => a.id)).toEqual(['mine']);
  });

  it('scopes by department including the lead, and by text / status', () => {
    const base = { q: '', status: '' as const };
    expect(
      scopeRoster(index, all, { ...base, scope: 'FIN' })
        .map((a) => a.id)
        .sort(),
    ).toEqual(['acct-a', 'fin-lead', 'mine']);
    expect(scopeRoster(index, all, { ...base, scope: 'C' })).toHaveLength(4);
    expect(scopeRoster(index, all, { ...base, scope: 'butlers' }).map((a) => a.id)).toEqual(['spirit']);
    expect(scopeRoster(index, all, { ...base, scope: 'all', q: '会计' }).map((a) => a.id)).toEqual(['acct-a', 'mine']);
    expect(scopeRoster(index, all, { ...base, scope: 'all', status: 'failed' }).map((a) => a.id)).toEqual(['mine']);
    expect(countByRole(all)).toMatchObject({ all: 6, specialist: 2, custom: 1, butler: 1 });
  });

  it('groups butlers first, unassigned last, leads first inside a department', () => {
    const groups = groupRoster(index, all);
    expect(groups.map((g) => g.key)).toEqual(['butlers', 'GMO', 'FIN', 'unassigned']);
    const fin = groups.find((g) => g.key === 'FIN')!;
    expect(fin.agents[0].id).toBe('fin-lead');
    expect(fin.lead?.id).toBe('fin-lead');
    expect(fin.company).toBe('公司');
  });

  it('derives the reporting line', () => {
    expect(reportsTo(rail, index, acct)?.id).toBe('fin-lead');
    expect(reportsTo(rail, index, finLead)?.id).toBe('ceo');
    expect(reportsTo(rail, index, ceo)).toBeUndefined();
    expect(reportsTo(rail, index, spirit)).toBeUndefined();
  });

  it('builds a valid agent key for a vacancy', () => {
    expect(vacancyAgentKey(node('budget_planner', 'position', '预算'))).toBe('budget-planner-custom');
    expect(vacancyAgentKey(node('', 'position', '预算'))).toBe('');
  });
});
