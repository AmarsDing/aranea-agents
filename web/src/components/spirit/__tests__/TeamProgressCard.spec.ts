// D10-F1（发现#23）：TeamProgressCard 成员区 v2 词表适配。
// - member failed 在团队 running 时应渲染「失败」chip 而非「排队中」
// - v2 pending 成员在团队 running 时应显示「等待前置任务完成…」注释
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import TeamProgressCard from '../TeamProgressCard.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import type { SpiritMember, SpiritTeam } from '../../../features/spirit/types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-btn': { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
  'q-spinner': { template: '<span />' },
  'q-tooltip': { template: '<span />' },
  'q-linear-progress': { template: '<div />' },
};

function mkMember(over: Partial<SpiritMember> = {}): SpiritMember {
  return {
    agentId: 'agent-1',
    agentKey: 'coder',
    displayName: '编码员',
    role: 'coder',
    status: 'pending',
    avatarUrl: '',
    ...over,
  };
}

function mkTeam(over: Partial<SpiritTeam> = {}): SpiritTeam {
  return {
    id: 'team-1',
    teamName: '调研团队',
    taskSummary: '',
    status: 'running',
    mode: 'parallel',
    memberAvatars: [],
    completedSteps: 0,
    totalSteps: 0,
    progressPct: 0,
    durationMs: 0,
    spiritSessionId: 'sp-1',
    teamSessionId: 'ts-1',
    members: [],
    sharedAgentIds: [],
    createdAt: Date.now(),
    ...over,
  };
}

function mountCard(team: SpiritTeam) {
  return mount(TeamProgressCard, {
    props: { team },
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

describe('TeamProgressCard（D10-F1 成员状态映射）', () => {
  it('team running + member failed 渲染「失败」chip 而非「排队中」（#23 失败先行锚点）', async () => {
    const wrapper = mountCard(mkTeam({ status: 'running', members: [mkMember({ status: 'failed' })] }));
    // 展开成员明细
    await wrapper.trigger('click');

    const detail = wrapper.find('.team-progress-card__detail-item');
    expect(detail.exists()).toBe(true);
    expect(detail.text()).toContain('失败');
    expect(detail.text()).not.toContain('排队中');
  });

  it('member pending + team running 显示 waitingForPredecessor 文案行（v2 词表）', async () => {
    const wrapper = mountCard(mkTeam({ status: 'running', members: [mkMember({ status: 'pending' })] }));
    await wrapper.trigger('click');

    const waiting = wrapper.find('.team-progress-card__detail-waiting');
    expect(waiting.exists()).toBe(true);
    expect(waiting.text()).toContain('等待前置任务');
  });

  it('member idle + team running 回归：waitingForPredecessor 仍显示（legacy 词表）', async () => {
    const wrapper = mountCard(mkTeam({ status: 'running', members: [mkMember({ status: 'idle' })] }));
    await wrapper.trigger('click');

    expect(wrapper.find('.team-progress-card__detail-waiting').exists()).toBe(true);
  });

  it('member pending + team pending 不显示 waitingForPredecessor（团队未运行）', async () => {
    const wrapper = mountCard(
      mkTeam({ status: 'pending', dependsOn: ['t0'], members: [mkMember({ status: 'pending' })] }),
    );
    await wrapper.trigger('click');

    expect(wrapper.find('.team-progress-card__detail-waiting').exists()).toBe(false);
  });
});
