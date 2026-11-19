import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { describe, expect, it } from 'vitest';
import zh from '../../../i18n/locales/zh-CN';
import CompanyTree from '../CompanyTree.vue';
import type { CompanyTree as Tree, DeptNode } from '../../../features/company/tree';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zh } });
const now = new Date('2026-11-18T09:00:00');

const dept = (id: string, extra: Partial<DeptNode> = {}): DeptNode => ({
  id,
  name: id,
  lead: null,
  leadMissing: false,
  agentCount: 2,
  activeToday: 1,
  failed: 0,
  waiting: 0,
  delegated: 0,
  state: 'ok',
  lastRunAt: '',
  nextRunAt: '',
  positions: ['甲', '乙', '丙', '丁', '戊'].map((name) => ({
    id: `${id}-${name}`,
    name,
    agents: [],
    state: 'empty',
    waiting: 0,
    failed: 0,
  })),
  ...extra,
});
const tree = (extra: Partial<Tree> = {}): Tree => ({
  companies: [{ id: 'c1', name: '示例公司' }],
  company: { id: 'c1', name: '示例公司' },
  me: { waiting: 2, highRisk: 0, earliestDue: '2026-11-18T18:00:00' },
  ceo: null,
  ceoConfigured: false,
  depts: [dept('电商部'), dept('内容部', { waiting: 3, delegated: 4, state: 'attention' })],
  totals: { agents: 4, activeToday: 2, failed: 0, unassigned: 0 },
  ...extra,
});
const mountTree = (value = tree()) =>
  mount(CompanyTree, { props: { tree: value, now, selected: 'me' }, global: { plugins: [i18n] } });

describe('公司树（设计稿画法）', () => {
  it('每个部门一个连线标签，写清往下交办几件、往上等我几件；标签和卡片一一对应', () => {
    const wrapper = mountTree();
    const labels = wrapper.findAll('.co-fan .co-lab').map((el) => el.text().replace(/\s+/g, ''));
    expect(labels).toEqual(['↓交办0件↑没有上报', '↓交办4件↑等你3件']);
    expect(wrapper.findAll('.co-node--dept')).toHaveLength(2);
    expect(wrapper.find('.co-fan .co-lab__hot').text()).toContain('等你 3 件');
  });

  it('部门识别色取业务线色令牌（深空皮肤时期的 --nt-domain-* 已经没有了）', () => {
    const styles = mountTree()
      .findAll('.co-dept')
      .map((el) => el.attributes('style'));
    expect(styles[0]).toContain('var(--nt-line-1)');
    expect(styles[1]).toContain('var(--nt-line-2)');
    expect(styles.join(' ')).not.toContain('--nt-domain-');
  });

  it('我和总经理之间：左边写等我几件和最早到期，右边写总经理的情况；没指定总经理时如实写', () => {
    const wrapper = mountTree();
    const [down, up] = wrapper.findAll('.co-lens .co-lab').map((el) => el.text().replace(/\s+/g, ' '));
    expect(down).toContain('等你 2 件 · 最早 18:00');
    expect(up).toContain('还没指定总经理');
    expect(mountTree(tree({ me: { waiting: 0, highRisk: 0, earliestDue: '' } })).text()).toContain('没有等你拍板的事');
  });

  it('岗位默认露出 3 个，其余收在「还有 N 个岗位」里；点节点发出选中', async () => {
    const wrapper = mountTree();
    const first = wrapper.findAll('.co-dept')[0];
    expect(first.findAll('.co-leaf')).toHaveLength(3);
    expect(first.text()).toContain('还有 2 个岗位');
    await first.get('.co-link').trigger('click');
    expect(wrapper.findAll('.co-dept')[0].findAll('.co-leaf')).toHaveLength(5);
    await wrapper.get('.co-node--dept').trigger('click');
    await wrapper.get('.co-ceo').trigger('click');
    expect(wrapper.emitted('select')).toEqual([['dept:电商部'], ['ceo']]);
  });
});
