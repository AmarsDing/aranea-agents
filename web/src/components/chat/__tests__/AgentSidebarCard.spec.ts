// D10-F2（发现#24）：AgentSidebarCard displayStatus 终态分支重设计。
// 优先级契约：blockedInfo > 成员自身终态 > 团队终态回填 > 成员运行/暂停 > 团队运行 > pending。
// 重试按钮出现的充要条件 = displayStatus === 'failed'。
import { describe, it, expect } from 'vitest';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import AgentSidebarCard from '../AgentSidebarCard.vue';
import zhCN from '../../../i18n/locales/zh-CN';
import type { SpiritMember, SpiritTeamStatus } from '../../../features/spirit/types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-btn': { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
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

function mountCard(opts: { memberStatus?: string; teamStatus?: SpiritTeamStatus; blocked?: boolean } = {}) {
  return mount(AgentSidebarCard, {
    props: {
      member: mkMember({ status: opts.memberStatus ?? 'pending' }),
      teamName: '调研团队',
      teamSessionId: 'ts-1',
      teamId: 'team-1',
      ...(opts.teamStatus ? { teamStatus: opts.teamStatus } : {}),
      ...(opts.blocked ? { blockedInfo: { blocked: true, type: 'tool' as const, agentKey: 'coder' } } : {}),
    },
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

function retryBtn(wrapper: ReturnType<typeof mountCard>) {
  return wrapper.find('button[icon="refresh"]');
}

function resumeBtn(wrapper: ReturnType<typeof mountCard>) {
  return wrapper.find('button[icon="play_arrow"]');
}

function pauseBtn(wrapper: ReturnType<typeof mountCard>) {
  return wrapper.find('button[icon="pause"]');
}

describe('AgentSidebarCard（D10-F2 displayStatus 优先级契约）', () => {
  it('member pending + teamStatus cancelled →「已取消」灰样式且无重试（#24 失败先行锚点）', () => {
    const wrapper = mountCard({ memberStatus: 'pending', teamStatus: 'cancelled' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--cancelled');
    expect(wrapper.find('.agent-sidebar-card__status-text').text()).toBe('已取消');
    expect(retryBtn(wrapper).exists()).toBe(false);
  });

  it('member pending + teamStatus interrupted →「已中断」样式且无重试', () => {
    const wrapper = mountCard({ memberStatus: 'pending', teamStatus: 'interrupted' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--interrupted');
    expect(wrapper.find('.agent-sidebar-card__status-text').text()).toBe('已中断');
    expect(retryBtn(wrapper).exists()).toBe(false);
  });

  it('member pending + teamStatus partial_failure →「部分失败」警示且无重试（修团队终态永久待开始）', () => {
    const wrapper = mountCard({ memberStatus: 'pending', teamStatus: 'partial_failure' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--partial_failure');
    expect(wrapper.find('.agent-sidebar-card__status-text').text()).toBe('部分失败');
    expect(retryBtn(wrapper).exists()).toBe(false);
  });

  it('member failed + teamStatus cancelled → 成员终态优先 = failed 且有重试', () => {
    const wrapper = mountCard({ memberStatus: 'failed', teamStatus: 'cancelled' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--failed');
    expect(wrapper.find('.agent-sidebar-card__status-text').text()).toBe('失败');
    expect(retryBtn(wrapper).exists()).toBe(true);
  });

  it('member error（legacy）+ teamStatus cancelled → 成员终态优先 = failed 且有重试', () => {
    const wrapper = mountCard({ memberStatus: 'error', teamStatus: 'cancelled' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--failed');
    expect(retryBtn(wrapper).exists()).toBe(true);
  });

  it('member skipped →「已跳过」中性终态，无重试无暂停', () => {
    const wrapper = mountCard({ memberStatus: 'skipped', teamStatus: 'running' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--skipped');
    expect(wrapper.find('.agent-sidebar-card__status-text').text()).toBe('已跳过');
    expect(retryBtn(wrapper).exists()).toBe(false);
    expect(pauseBtn(wrapper).exists()).toBe(false);
  });

  it('member paused → 视为可恢复挂起（blocked 样式）+ resume 按钮', async () => {
    const wrapper = mountCard({ memberStatus: 'paused', teamStatus: 'running' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--blocked');
    expect(resumeBtn(wrapper).exists()).toBe(true);
    expect(retryBtn(wrapper).exists()).toBe(false);

    await resumeBtn(wrapper).trigger('click');
    expect(wrapper.emitted('resume')?.[0]).toEqual(['coder']);
  });

  it('member running → pause 按钮', () => {
    const wrapper = mountCard({ memberStatus: 'running', teamStatus: 'running' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--running');
    expect(pauseBtn(wrapper).exists()).toBe(true);
  });

  it('blockedInfo.blocked=true 覆盖一切 → blocked（工具确认 UX 优先权不变）', () => {
    const wrapper = mountCard({ memberStatus: 'failed', teamStatus: 'failed', blocked: true });

    expect(wrapper.classes()).toContain('agent-sidebar-card--blocked');
    expect(resumeBtn(wrapper).exists()).toBe(true);
    expect(retryBtn(wrapper).exists()).toBe(false);
  });

  it('member pending + teamStatus failed → 团队终态回填 failed 且有重试', () => {
    const wrapper = mountCard({ memberStatus: 'pending', teamStatus: 'failed' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--failed');
    expect(retryBtn(wrapper).exists()).toBe(true);
  });

  it('member pending + teamStatus completed → 团队终态回填 completed', () => {
    const wrapper = mountCard({ memberStatus: 'pending', teamStatus: 'completed' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--completed');
    expect(wrapper.find('.agent-sidebar-card__status-text').text()).toBe('已完成');
    expect(retryBtn(wrapper).exists()).toBe(false);
  });

  it('member pending 且无 teamStatus → pending 兜底', () => {
    const wrapper = mountCard({ memberStatus: 'pending' });

    expect(wrapper.classes()).toContain('agent-sidebar-card--pending');
    expect(wrapper.find('.agent-sidebar-card__status-text').text()).toBe('待开始');
    expect(retryBtn(wrapper).exists()).toBe(false);
  });
});
