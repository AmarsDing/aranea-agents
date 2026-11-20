import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { describe, expect, it } from 'vitest';
import zh from '../../../i18n/locales/zh-CN';
import BoardInspector from '../BoardInspector.vue';
import CompanyTree from '../CompanyTree.vue';
import { buildBoard, type OrgTreeInput } from '../../../features/company/tree';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } });
const now = new Date(2026, 10, 18, 9, 0, 0);
const stubs = { 'q-icon': true, 'q-btn': true, 'router-link': { template: '<a><slot /></a>' } };

const node = (id: string, level: string, children: OrgTreeInput[] = [], extra = {}): OrgTreeInput => ({
  node: { id, name: id, level, enabled: true, ...extra },
  children,
});
const board = buildBoard({
  orgTree: [
    node('电商公司', 'company', [
      node('运营部', 'department', [node('店长', 'position'), node('客服', 'position')]),
      node('选品部', 'department', [node('买手', 'position')]),
    ]),
    node('内容公司', 'company', [node('编辑部', 'department', [node('编辑', 'position')])], { sortOrder: 1 }),
  ],
  agents: [
    { id: 'a1', agent_key: 'k1', display_name: '店长甲', taxonomy_position_id: '店长' },
    { id: 'a2', agent_key: 'k2', display_name: '编辑乙', taxonomy_position_id: '编辑' },
  ],
  inbox: [{ id: 'i1', title: '上新方案', agent_key: 'k1', due_at: '', risk_level: 'high' }],
  inboxTotal: 1,
  work: [
    { id: 'w1', title: '写周报', assignee_kind: 'agent', assignee_id: 'a2', state: 'running', at: '', to: '/chat' },
  ],
  cron: [],
  usage: [],
  now,
});

const mountTree = (folded: string[] = [], opened: string[] = []) =>
  mount(CompanyTree, {
    props: { board, now, selected: '', folded: new Set(folded), opened: new Set(opened) },
    global: { plugins: [i18n], stubs },
  });

describe('看板总览（一张图管全部公司）', () => {
  it('每家公司一列：连线标签写清往下在办几件、往上等我几件；公司识别色取业务线色令牌', () => {
    const wrapper = mountTree();
    const labels = wrapper.findAll('.kb-lab').map((el) => el.text().replace(/\s+/g, ''));
    expect(labels).toEqual(['↓在办0件↑等你1件', '↓在办1件↑没有上报']);
    expect(wrapper.find('.kb-lab__hot').text()).toContain('等你 1 件');
    const styles = wrapper.findAll('.kb-col').map((el) => el.attributes('style'));
    expect(styles[0]).toContain('var(--nt-line-1)');
    expect(styles[1]).toContain('var(--nt-line-2)');
  });

  it('部门挂在公司下面，要我拍板的部门带「拍板 N」；收起公司后只剩一条说明', async () => {
    const wrapper = mountTree();
    expect(wrapper.findAll('.kb-dept__name').map((el) => el.text())).toEqual(['运营部', '选品部', '编辑部']);
    expect(wrapper.find('.kb-badge--waiting').text()).toBe('拍板 1');
    const folded = mountTree(['电商公司']);
    expect(folded.findAll('.kb-dept__name').map((el) => el.text())).toEqual(['编辑部']);
    expect(folded.find('.kb-folded').text()).toContain('2 个部门已收起 · 1 个要看一眼');
    await folded.get('.kb-folded').trigger('click');
    expect(folded.emitted('toggleCompany')).toEqual([['电商公司']]);
  });

  it('部门展开后列出 Agent；点公司、部门、Agent 都发出选中', async () => {
    const wrapper = mountTree([], ['运营部']);
    expect(wrapper.findAll('.kb-agent__name').map((el) => el.text())).toEqual(['店长甲']);
    expect(wrapper.find('.kb-agent__val').text()).toBe('拍板 1');
    await wrapper.get('.kb-co__body').trigger('click');
    await wrapper.get('.kb-dept__body').trigger('click');
    await wrapper.get('.kb-agent').trigger('click');
    await wrapper.get('.kb-me').trigger('click');
    expect(wrapper.emitted('select')).toEqual([
      ['co:电商公司'],
      ['dept:电商公司:运营部'],
      ['agent:电商公司:运营部:a1'],
      ['me'],
    ]);
  });
});

describe('右侧检视', () => {
  const mountInsp = (selected: string) =>
    mount(BoardInspector, {
      props: { board, selected, now, signals: [], partial: [] },
      global: { plugins: [i18n], stubs },
    });

  it('我：列出等你拍板的事和各公司；点公司跳到公司', async () => {
    const wrapper = mountInsp('me');
    expect(wrapper.find('.kb-insp__title').text()).toBe('我');
    expect(wrapper.text()).toContain('等你拍板 · 1');
    expect(wrapper.text()).toContain('上新方案');
    const companies = wrapper.findAll('button.kb-insp__item');
    expect(companies.map((el) => el.find('.kb-insp__item-title').text())).toEqual(['电商公司', '内容公司']);
    await companies[1]?.trigger('click');
    expect(wrapper.emitted('select')).toEqual([['co:内容公司']]);
  });

  it('部门：写清负责人、岗位与空岗、交出去的事；Esc 关闭', async () => {
    const wrapper = mountInsp('dept:内容公司:编辑部');
    expect(wrapper.find('.kb-insp__kicker').text()).toBe('部门 · 内容公司');
    expect(wrapper.text()).toContain('1 个，空岗 0');
    expect(wrapper.text()).toContain('交出去的事 · 1');
    expect(wrapper.text()).toContain('写周报');
    await wrapper.get('.kb-insp').trigger('keydown', { key: 'Escape' });
    expect(wrapper.emitted('close')).toHaveLength(1);
  });

  it('选中的节点不在了（刷新后被删）就回到「我」', () => {
    expect(mountInsp('dept:电商公司:不存在').find('.kb-insp__title').text()).toBe('我');
  });
});

describe('总经理巡视 / 汇报（后端 org-graph 给出 gm 时）', () => {
  const ceo = { ...board.companies[0]!.depts[0]!.agents[0]!, id: 'gm', name: '总经理' };
  const report = {
    runId: 'r1',
    at: new Date(2026, 10, 18, 8, 20).toISOString(),
    conclusion: '要看一眼：等你拍板 1 件',
    reasons: ['上新方案是高风险'],
    writer: 'model',
    acked: false,
  };
  const withGm = {
    ...board,
    companies: [
      {
        ...board.companies[0]!,
        ceo,
        gm: {
          enabled: true,
          lastPatrolAt: new Date(2026, 10, 18, 8, 10).toISOString(),
          nextPatrolAt: new Date(2026, 10, 18, 9, 10).toISOString(),
          lastReport: report,
          nextReportAt: new Date(2026, 10, 18, 12, 20).toISOString(),
        },
      },
      {
        ...board.companies[1]!,
        ceo: { ...ceo, id: 'gm2' },
        gm: { enabled: false, lastPatrolAt: '', nextPatrolAt: '', nextReportAt: '' },
      },
    ],
  };

  it('公司卡写巡视与汇报的时间，两小时内巡视过雷达才转；定时任务停了写「没在巡视」', () => {
    const wrapper = mount(CompanyTree, {
      props: { board: withGm, now, selected: '', folded: new Set<string>(), opened: new Set<string>() },
      global: { plugins: [i18n], stubs },
    });
    const lines = wrapper.findAll('.kb-ceo__time').map((el) => el.text());
    expect(lines).toEqual(['巡视 08:10 · 汇报 08:20 → 12:20', '没在巡视']);
    const radars = wrapper.findAll('.kb-radar');
    expect(radars[0]!.classes()).toContain('kb-radar--on');
    expect(radars[1]!.classes()).not.toContain('kb-radar--on');
  });

  it('公司检视栏顶部是总经理最近汇报：结论、理由，未读时指向收件箱', () => {
    const wrapper = mount(BoardInspector, {
      props: { board: withGm, selected: `co:${withGm.companies[0]!.id}`, now, signals: [], partial: [] },
      global: { plugins: [i18n], stubs },
    });
    const block = wrapper.find('.kb-insp__report');
    expect(block.text()).toContain('总经理最近汇报 · 08:20');
    expect(block.find('.kb-insp__report-conclusion').text()).toBe('要看一眼：等你拍板 1 件');
    expect(block.findAll('.kb-insp__reasons li').map((li) => li.text())).toEqual(['上新方案是高风险']);
    expect(block.text()).toContain('总经理写的');
    expect(block.text()).toContain('去收件箱点「知道了」');
  });
});
