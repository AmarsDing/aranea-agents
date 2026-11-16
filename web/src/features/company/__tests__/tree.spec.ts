import { describe, expect, it } from 'vitest';
import { buildCompanyTree, companyLeadId, shortTime, type AgentInput, type OrgTreeInput } from '../tree';

// 本地时间 2026-11-16 10:00；用本地构造避免依赖运行机时区。
const NOW = new Date(2026, 10, 16, 10, 0, 0);
const at = (h: number, day = 16) => new Date(2026, 10, day, h, 0, 0).toISOString();

const node = (
  id: string,
  level: string,
  extra: Partial<NonNullable<OrgTreeInput['node']>> = {},
  children: OrgTreeInput[] = [],
) => ({
  node: { id, name: id, level, enabled: true, sortOrder: 0, ...extra },
  children,
});
const agent = (id: string, position: string, extra: Partial<AgentInput> = {}): AgentInput => ({
  id,
  agent_key: `key-${id}`,
  display_name: `名-${id}`,
  taxonomy_position_id: position,
  ...extra,
});

const orgTree: OrgTreeInput[] = [
  node('co-1', 'company', { name: '一号公司', metadataJson: JSON.stringify({ company_lead_agent_id: 'ceo' }) }, [
    node('d-content', 'department', { name: '内容部', deptLeadAgentId: 'lead-c', sortOrder: 2 }, [
      node('p-write', 'position', { name: '写手', sortOrder: 1 }),
      node('p-review', 'position', { name: '评审', sortOrder: 2 }),
      node('p-off', 'position', { name: '停用岗', enabled: false }),
    ]),
    node('d-trade', 'department', { name: '交易部', deptLeadAgentId: 'gone', sortOrder: 1 }, [
      node('p-quant', 'position'),
    ]),
    node('d-empty', 'department', { name: '空部门', sortOrder: 3 }),
  ]),
  node('co-2', 'company', { name: '二号公司' }, [node('d-x', 'department', {}, [node('p-x', 'position')])]),
];
const agents: AgentInput[] = [
  agent('ceo', ''),
  agent('lead-c', '', { last_run_at: at(8) }),
  agent('w1', 'p-write', { last_run_at: at(9) }),
  agent('w2', 'p-write', { last_run_at: at(9, 15) }),
  agent('r1', 'p-review', { last_run_status: 'failed', last_run_at: at(7) }),
  agent('q1', 'p-quant', { last_run_at: at(9, 14) }),
  agent('dead', 'p-quant', { deleted_at: at(1) }),
  agent('other', 'p-x'),
];

function build(companyId = '') {
  return buildCompanyTree({
    orgTree,
    agents,
    inbox: [
      { agent_key: 'key-w1', due_at: at(18), risk_level: 'high' },
      { agent_key: 'key-w1', due_at: at(12), risk_level: 'low' },
      { agent_key: '', due_at: '', risk_level: 'medium' },
    ],
    inboxTotal: 5,
    delegated: [
      { assignee_kind: 'agent', assignee_id: 'w2' },
      { assignee_kind: 'agent', assignee_id: 'lead-c' },
      { assignee_kind: 'team', assignee_id: 'w2' },
    ],
    cron: [
      { agentId: 'w1', nextRunAt: at(14) },
      { agentId: 'w2', nextRunAt: at(12) },
      { agentId: 'w2', nextRunAt: at(6) },
    ],
    companyId,
    now: NOW,
  });
}

describe('buildCompanyTree', () => {
  it('uses the first company by default and resolves its lead agent', () => {
    const tree = build();
    expect(tree.companies.map((c) => c.name)).toEqual(['一号公司', '二号公司']);
    expect(tree.company?.id).toBe('co-1');
    expect(tree.ceo?.id).toBe('ceo');
    expect(tree.ceoConfigured).toBe(true);
    expect(tree.me).toEqual({ waiting: 5, highRisk: 1, earliestDue: at(12) });
  });

  it('orders departments by the org tree and rolls agents up through positions and the lead', () => {
    const tree = build();
    expect(tree.depts.map((d) => d.name)).toEqual(['交易部', '内容部', '空部门']);
    const content = tree.depts[1];
    expect(content).toMatchObject({
      agentCount: 4,
      activeToday: 3,
      failed: 1,
      waiting: 2,
      delegated: 2,
      state: 'attention',
      lastRunAt: at(9),
      nextRunAt: at(12),
    });
    expect(content?.lead?.id).toBe('lead-c');
    // 停用岗不出现；有问题的岗位排前面
    expect(content?.positions.map((p) => `${p.name}:${p.state}`)).toEqual(['写手:attention', '评审:attention']);
  });

  it('tells idle, empty and a missing lead apart', () => {
    const tree = build();
    expect(tree.depts[0]).toMatchObject({
      name: '交易部',
      state: 'idle',
      agentCount: 1,
      lead: null,
      leadMissing: true,
    });
    expect(tree.depts[2]).toMatchObject({ name: '空部门', state: 'empty', agentCount: 0, leadMissing: false });
    expect(tree.totals).toEqual({ agents: 6, activeToday: 3, failed: 1, unassigned: 0 });
  });

  it('switches company and reports a company without a lead', () => {
    const tree = build('co-2');
    expect(tree.company?.name).toBe('二号公司');
    expect(tree.ceo).toBeNull();
    expect(tree.ceoConfigured).toBe(false);
    expect(tree.depts).toHaveLength(1);
  });

  it('degrades to an empty tree when there is no company', () => {
    const tree = buildCompanyTree({
      orgTree: [],
      agents,
      inbox: [],
      inboxTotal: 0,
      delegated: [],
      cron: [],
      companyId: '',
      now: NOW,
    });
    expect(tree.company).toBeNull();
    expect(tree.depts).toEqual([]);
  });

  it('counts unassigned agents only when there is a single company', () => {
    const tree = buildCompanyTree({
      orgTree: orgTree.slice(0, 1),
      agents,
      inbox: [],
      inboxTotal: 0,
      delegated: [],
      cron: [],
      companyId: '',
      now: NOW,
    });
    expect(tree.totals.unassigned).toBe(1);
  });
});

describe('agents attached straight to a department', () => {
  it('counts them as department members', () => {
    const tree = buildCompanyTree({
      orgTree: [node('co', 'company', {}, [node('d', 'department', {}, [node('p', 'position')])])],
      agents: [agent('float', 'd', { last_run_at: at(9) }), agent('seated', 'p')],
      inbox: [],
      inboxTotal: 0,
      delegated: [],
      cron: [],
      companyId: '',
      now: NOW,
    });
    expect(tree.depts[0]).toMatchObject({ agentCount: 2, activeToday: 1, state: 'ok' });
    expect(tree.totals.unassigned).toBe(0);
  });
});

describe('helpers', () => {
  it('reads the company lead id defensively', () => {
    expect(companyLeadId('{"company_lead_agent_id":" a1 "}')).toBe('a1');
    expect(companyLeadId('not json')).toBe('');
    expect(companyLeadId(undefined)).toBe('');
  });

  it('formats times relative to today', () => {
    expect(shortTime(at(9), NOW)).toBe('09:00');
    expect(shortTime(at(9, 14), NOW)).toBe('11-14 09:00');
    expect(shortTime('', NOW)).toBe('');
  });
});
