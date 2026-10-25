// D10-F3（发现#25）：ObserveNodeDetail 成员状态 paused/cancelled 文案键与样式补全。
// 成员 status 数据源为 v2 MemberSessionStatus（useObserveGraph.extractMembers），
// paused 此前直出英文原文。
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import ObserveNodeDetail from '../ObserveNodeDetail.vue';
import zhCN from '../../../../i18n/locales/zh-CN';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import type { GraphNode, TeamStage } from '../../../../features/chat/v2Types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-btn': { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
  'q-icon': { template: '<i />' },
};

function mkNode(over: Partial<GraphNode> = {}): GraphNode {
  return {
    ID: 'n1',
    GraphStageID: 'gs1',
    Label: '调研节点',
    DagNodeID: 'n1',
    TeamStageID: 'ts-n1',
    Status: 'running',
    DependsOn: [],
    ...over,
  };
}

function mkTeamStage(memberStatus: string): TeamStage {
  return {
    ID: 'ts-n1',
    TaskID: 'tk1',
    TurnID: 'tn1',
    SessionID: 's1',
    TeamID: 'team-1',
    TeamName: '调研团队',
    DagNodeID: 'n1',
    DependsOn: [],
    Status: 'running',
    Stage: 'assembled',
    Members: [{ AgentKey: 'coder', AgentName: '编码员', AvatarURL: '', ChildSessionID: '', Status: memberStatus }],
    Strategy: 'parallel',
    StartedAt: new Date().toISOString(),
    CompletedAt: null,
    Seq: 1,
    Version: 1,
  };
}

function mountDetail(memberStatus: string) {
  const store = useChatActivityStore();
  store.upsertTeamStage(mkTeamStage(memberStatus));
  return mount(ObserveNodeDetail, {
    props: { node: mkNode() },
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

describe('ObserveNodeDetail（D10-F3 成员状态 paused/cancelled）', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('member paused → 文案 = observe.statusPaused（失败先行：此前直出英文原文）', () => {
    const wrapper = mountDetail('paused');

    const statusEl = wrapper.find('.observe-node-detail__member-status');
    expect(statusEl.exists()).toBe(true);
    expect(statusEl.text()).toBe('已暂停');
    expect(statusEl.text()).not.toBe('paused');
    // dot / status 两个 CSS 类均存在
    expect(wrapper.find('.observe-node-detail__member-dot--paused').exists()).toBe(true);
    expect(wrapper.find('.observe-node-detail__member-status--paused').exists()).toBe(true);
  });

  it('member cancelled → 文案 = observe.statusCancelled + CSS 类存在（防御性补全）', () => {
    const wrapper = mountDetail('cancelled');

    const statusEl = wrapper.find('.observe-node-detail__member-status');
    expect(statusEl.text()).toBe('已取消');
    expect(wrapper.find('.observe-node-detail__member-dot--cancelled').exists()).toBe(true);
    expect(wrapper.find('.observe-node-detail__member-status--cancelled').exists()).toBe(true);
  });

  it('member skipped 回归：仍显示「已跳过」（其余状态渲染不变）', () => {
    const wrapper = mountDetail('skipped');

    const statusEl = wrapper.find('.observe-node-detail__member-status');
    expect(statusEl.text()).toBe('已跳过');
    expect(wrapper.find('.observe-node-detail__member-status--skipped').exists()).toBe(true);
  });

  it('未知成员状态仍走原文透传兜底（memberStatusLabel 行为不变）', () => {
    const wrapper = mountDetail('some_future_state');

    expect(wrapper.find('.observe-node-detail__member-status').text()).toBe('some_future_state');
  });
});
