import { describe, expect, it } from 'vitest';
import { buildBoard, companyLeadId, shortTime, type AgentInput, type OrgTreeInput, type WorkInput } from '../tree';

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
const work = (id: string, assignee: string, state: WorkInput['state'], kind = 'agent'): WorkInput => ({
  id,
  title: id,
  assignee_kind: kind,
  assignee_id: assignee,
  state,
  at: at(9),
  to: '/chat',
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
  node('co-2', 'company', { name: '二号公司', sortOrder: 1 }, [
    node('d-x', 'department', {}, [node('p-x', 'position')]),
  ]),
];
const agents: AgentInput[] = [
  agent('ceo', ''),
  agent('lead-c', 'd-content', { last_run_at: at(8) }),
  agent('w1', 'p-write', { last_run_at: at(9) }),
  agent('w2', 'p-write', { last_run_at: at(9, 15) }),
  agent('r1', 'p-review', { last_run_status: 'failed', last_run_at: at(7) }),
  agent('q1', 'p-quant', { last_run_at: at(9, 14) }),
  agent('dead', 'p-quant', { deleted_at: at(1) }),
  agent('x1', 'p-x'),
  agent('loose', ''),
];

function build() {
  return buildBoard({
    orgTree,
    agents,
    inbox: [
      { id: 'i1', title: '调仓', agent_key: 'key-w1', due_at: at(18), risk_level: 'high' },
      { id: 'i2', title: '选题', agent_key: 'key-w1', due_at: at(12), risk_level: 'low' },
      { id: 'i3', title: '没归属', agent_key: '', due_at: '', risk_level: 'medium' },
    ],
    inboxTotal: 5,
    work: [
      work('t1', 'w2', 'running'),
      work('t2', 'lead-c', 'review'),
      work('t3', 'x1', 'stuck'),
      work('t4', 'w2', 'running', 'team'),
    ],
    cron: [
      { agentId: 'w1', nextRunAt: at(14) },
      { agentId: 'w2', nextRunAt: at(12) },
      { agentId: 'w2', nextRunAt: at(6) },
    ],
    usage: [{ agentId: 'q1', calls: 3, costMicro: 120_000 }],
    now: NOW,
  });
}

describe('buildBoard（一张图管全部公司）', () => {
  it('所有公司都在图上，按组织树顺序；总经理并进公司卡', () => {
    const board = build();
    expect(board.companies.map((c) => c.name)).toEqual(['一号公司', '二号公司']);
    expect(board.companies[0]?.ceo?.id).toBe('ceo');
    expect(board.companies[1]?.ceo).toBeNull();
    expect(board.companies[1]?.ceoConfigured).toBe(false);
  });

  it('部门按组织树顺序；成员 = 岗位上的 + 直接挂部门的 + 负责人，按 id 去重；停用岗位和已删 Agent 不算', () => {
    const [co] = build().companies;
    expect(co?.depts.map((d) => d.name)).toEqual(['交易部', '内容部', '空部门']);
    const content = co?.depts[1];
    expect(content?.agents.map((a) => a.id).sort()).toEqual(['lead-c', 'r1', 'w1', 'w2']);
    expect(content?.positions).toBe(2);
    expect(content?.emptyPositions).toBe(0);
    expect(content?.lead?.id).toBe('lead-c');
    const trade = co?.depts[0];
    expect(trade?.leadMissing).toBe(true);
    expect(trade?.agents.map((a) => a.id)).toEqual(['q1']);
    expect(co?.depts[2]?.state).toBe('empty');
  });

  it('闭环四格逐层汇总：等你拍板按 agent_key、交办按指派的 Agent；派给团队的不归到 Agent', () => {
    const board = build();
    const [co1, co2] = board.companies;
    const content = co1?.depts[1];
    expect(content?.loop).toEqual({ waiting: 2, running: 1, stuck: 0, review: 1 });
    expect(co1?.loop).toEqual({ waiting: 2, running: 1, stuck: 0, review: 1 });
    expect(co2?.loop).toEqual({ waiting: 0, running: 0, stuck: 1, review: 0 });
    // 我：待办用总数（分页外还有），交办算全部（含派给团队的）
    expect(board.me.loop).toEqual({ waiting: 5, running: 2, stuck: 1, review: 1 });
    expect(board.me.highRisk).toBe(1);
    expect(board.me.earliestDue).toBe(at(12));
    expect(board.me.unattributed).toBe(1);
    expect(board.me.unassigned).toBe(1);
  });

  it('节点状态：有事等我或失败 → 要看一眼；今天跑过或有事在做 → 正常；在岗没动静 → 今天没动静', () => {
    const [co1, co2] = build().companies;
    expect(co1?.depts[1]?.state).toBe('attention');
    // 交易部：q1 不是今天跑的，但今天有用量 → 今天跑过
    expect(co1?.depts[0]?.state).toBe('ok');
    expect(co1?.depts[0]?.activeToday).toBe(1);
    expect(co1?.depts[0]?.calls).toBe(3);
    expect(co1?.costMicro).toBe(120_000);
    expect(co2?.state).toBe('attention');
    const content = co1?.depts[1];
    // 有问题的 Agent 排前面
    expect(content?.agents[0]?.state).toBe('attention');
    expect(content?.agents.find((a) => a.id === 'w2')?.state).toBe('ok');
  });

  it('上次 / 下次：上次取最近一次运行，下次取最早的未来定时（过去的忽略）', () => {
    const content = build().companies[0]?.depts[1];
    expect(content?.lastRunAt).toBe(at(9));
    expect(content?.nextRunAt).toBe(at(12));
    expect(content?.agents.find((a) => a.id === 'w1')?.position).toBe('写手');
  });

  it('没有公司时也能出图（只有「我」）', () => {
    const board = buildBoard({
      orgTree: [],
      agents: [],
      inbox: [],
      inboxTotal: 0,
      work: [],
      cron: [],
      usage: [],
      now: NOW,
    });
    expect(board.companies).toEqual([]);
    expect(board.me.loop).toEqual({ waiting: 0, running: 0, stuck: 0, review: 0 });
  });
});

describe('companyLeadId / shortTime', () => {
  it('从 metadata 里读总经理 id，读不了返回空串', () => {
    expect(companyLeadId(JSON.stringify({ company_lead_agent_id: ' a ' }))).toBe('a');
    expect(companyLeadId('not json')).toBe('');
    expect(companyLeadId(undefined)).toBe('');
  });

  it('今天只写时分，不是今天带上月-日', () => {
    expect(shortTime(at(9), NOW)).toBe('09:00');
    expect(shortTime(at(9, 14), NOW)).toBe('11-14 09:00');
    expect(shortTime('', NOW)).toBe('');
  });
});
