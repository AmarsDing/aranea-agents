// web/src/components/chat/v2/__tests__/GraphTeamNode.spec.ts
// GraphTeamNode 富卡片：节点头（标题+状态徽章）/ 成员行（状态点+名称+耗时/状态）/ 底部进度条。
// 成员数据：GraphNode → TeamStage → TeamRun → MemberSession（与 TeamRunCard 同一解析链路）。
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import { useAppStore } from '../../../../stores/app';
import GraphTeamNode from '../GraphTeamNode.vue';
import { graphTeamNodeHeight, GTN_WIDTH } from '../graphTeamNodeUi';
import zhCN from '../../../../i18n/locales/zh-CN';
import type { GraphNode, TeamStage, TeamRun, MemberSession, PlanStep } from '../../../../features/chat/v2Types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-badge': { template: '<span class="q-badge-stub"><slot /></span>' },
  'q-avatar': { template: '<span class="q-avatar-stub" />' },
};

function mkNode(over: Partial<GraphNode> = {}): GraphNode {
  return {
    ID: 'gn1',
    GraphStageID: 'gs1',
    Label: '调研阶段',
    DagNodeID: 'ps1',
    TeamStageID: 'ts1',
    Status: 'running',
    DependsOn: [],
    ...over,
  };
}

function mkTeamStage(over: Partial<TeamStage> = {}): TeamStage {
  return {
    ID: 'ts1',
    TaskID: 'tk1',
    TurnID: 'tn1',
    SessionID: 's1',
    TeamID: 'team1',
    TeamName: '调研组',
    DagNodeID: 'ps1',
    DependsOn: [],
    Status: 'running',
    Stage: 'assembled',
    Members: [],
    Strategy: 'parallel',
    StartedAt: new Date().toISOString(),
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    ...over,
  };
}

function mkTeamRun(over: Partial<TeamRun> = {}): TeamRun {
  return {
    ID: 'tr1',
    TeamStageID: 'ts1',
    TaskID: 'tk1',
    SessionID: 's1',
    SpiritSessionID: 'sp1',
    DagNodeID: 'ps1',
    DependsOn: [],
    Status: 'running',
    StartedAt: new Date().toISOString(),
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    Error: '',
    ...over,
  };
}

function mkMember(id: string, over: Partial<MemberSession> = {}): MemberSession {
  return {
    ID: id,
    TeamRunID: 'tr1',
    TeamStageID: 'ts1',
    TaskID: 'tk1',
    SessionID: `sess-${id}`,
    SpiritSessionID: 'sp1',
    AgentKey: `agent-${id}`,
    AgentName: `成员${id}`,
    AvatarURL: '',
    Status: 'running',
    Seq: 1,
    Version: 1,
    StartedAt: new Date().toISOString(),
    FinishedAt: null,
    Error: '',
    ...over,
  };
}

function seedTeam(store: ReturnType<typeof useChatActivityStore>, members: MemberSession[]) {
  store.upsertTeamStage(mkTeamStage());
  store.upsertTeamRun(mkTeamRun());
  for (const ms of members) store.upsertMemberSession(ms);
}

function mountNode(node: GraphNode, extraProps: Record<string, unknown> = {}) {
  return mount(GraphTeamNode, {
    props: { node, pos: { x: 10, y: 20 }, ...extraProps },
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

describe('GraphTeamNode', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('renders node label and status badge with layout position/size', () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1')]);
    const wrapper = mountNode(mkNode());

    expect(wrapper.text()).toContain('调研阶段');
    expect(wrapper.text()).toContain('执行中');
    const el = wrapper.find('.graph-team-node');
    expect(el.exists()).toBe(true);
    expect(el.attributes('style')).toContain('left: 10px');
    expect(el.attributes('style')).toContain('top: 20px');
    expect(el.attributes('style')).toContain(`width: ${GTN_WIDTH}px`);
  });

  it('renders member rows with status dot, name and duration/status', () => {
    const store = useChatActivityStore();
    seedTeam(store, [
      mkMember('m1', { AgentName: '阿尔法', Status: 'completed', StartedAt: '2026-07-26T10:00:00Z', FinishedAt: '2026-07-26T10:00:12.4Z' }),
      mkMember('m2', { AgentName: '贝塔', Status: 'running' }),
    ]);
    const wrapper = mountNode(mkNode());

    const rows = wrapper.findAll('.gtn-member');
    expect(rows).toHaveLength(2);
    expect(rows[0]!.text()).toContain('阿尔法');
    // 完成成员显示耗时（12.4s）
    expect(rows[0]!.text()).toContain('12.4s');
    expect(rows[0]!.find('.gtn-member__dot--success').exists()).toBe(true);
    expect(rows[1]!.text()).toContain('贝塔');
    expect(rows[1]!.find('.gtn-member__dot--accent').exists()).toBe(true);
  });

  it('renders one placeholder row when the team has no members yet', () => {
    const wrapper = mountNode(mkNode({ TeamStageID: '' }));
    const rows = wrapper.findAll('.gtn-member');
    expect(rows).toHaveLength(1);
    expect(wrapper.text()).toContain('暂无成员');
  });

  it('renders planned members from PlanStep.AgentKeys before the team is dispatched', async () => {
    const store = useChatActivityStore();
    // 编排期已定员：planStep（ID=DagNodeID）带 AgentKeys，但尚无 TeamStage/TeamRun/MemberSession。
    store.upsertPlanStep({
      ID: 'ps1',
      PlanID: 'pb1',
      TaskID: 'tk1',
      Label: '媒体制作',
      Description: '',
      DependsOn: [],
      MappedTeamStageID: '',
      Status: 'pending',
      AutoSynthesis: false,
      StartedAt: '',
      CompletedAt: null,
      Seq: 1,
      Version: 1,
      Result: null,
      Error: null,
      AgentKeys: ['content_creator__general', 'brand_guardian__general'],
    } as PlanStep);
    const appStore = useAppStore();
    appStore.agents = [
      { id: 'a1', agent_key: 'content_creator__general', display_name: '内容创作者' },
      { id: 'a2', agent_key: 'brand_guardian__general', display_name: '品牌守护者' },
    ] as never;

    const wrapper = mountNode(mkNode({ TeamStageID: '', Status: 'pending' }));

    const rows = wrapper.findAll('.gtn-member');
    expect(rows).toHaveLength(2);
    expect(wrapper.text()).toContain('内容创作者');
    expect(wrapper.text()).toContain('品牌守护者');
    expect(wrapper.text()).not.toContain('暂无成员');
    expect(wrapper.text()).toContain('0/2');
    // 计划行：muted 状态点 + 等待中文案 + 不可点击（无 MemberSession 可弹窗）
    expect(rows[0]!.find('.gtn-member__dot--muted').exists()).toBe(true);
    expect(rows[0]!.text()).toContain('等待中');
    await rows[0]!.trigger('click');
    expect(wrapper.emitted('select-member')).toBeUndefined();
  });

  it('switches planned rows to runtime member sessions once the team is dispatched', () => {
    const store = useChatActivityStore();
    store.upsertPlanStep({
      ID: 'ps1',
      PlanID: 'pb1',
      TaskID: 'tk1',
      Label: '媒体制作',
      Description: '',
      DependsOn: [],
      MappedTeamStageID: '',
      Status: 'running',
      AutoSynthesis: false,
      StartedAt: '',
      CompletedAt: null,
      Seq: 1,
      Version: 1,
      Result: null,
      Error: null,
      AgentKeys: ['agent-m1'],
    } as PlanStep);
    seedTeam(store, [mkMember('m1', { AgentName: '运行时成员' })]);

    const wrapper = mountNode(mkNode({ TeamStageID: '' }));
    // 运行时成员接管后不再出现计划行
    expect(wrapper.text()).toContain('运行时成员');
    expect(wrapper.findAll('.gtn-member--planned')).toHaveLength(0);
  });

  it('falls back to DagNodeID matching when TeamStageID is empty', () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1', { AgentName: '阿尔法' })]);
    const wrapper = mountNode(mkNode({ TeamStageID: '' }));
    expect(wrapper.text()).toContain('阿尔法');
  });

  it('emits select-member with the member session when a member row is clicked', async () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1'), mkMember('m2')]);
    const wrapper = mountNode(mkNode());

    await wrapper.findAll('.gtn-member')[1]!.trigger('click');
    const events = wrapper.emitted('select-member');
    expect(events).toBeTruthy();
    expect(events![0]![0]).toMatchObject({ ID: 'm2', SessionID: 'sess-m2' });
    // 成员点击不应触发节点选中
    expect(wrapper.emitted('select')).toBeFalsy();
  });

  it('emits select and hover for the node itself', async () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1')]);
    const wrapper = mountNode(mkNode());

    await wrapper.find('.graph-team-node').trigger('mouseenter');
    expect(wrapper.emitted('hover')![0]).toEqual(['gn1']);
    await wrapper.find('.graph-team-node').trigger('mouseleave');
    expect(wrapper.emitted('hover')![1]).toEqual([null]);
    await wrapper.find('.gtn-header').trigger('click');
    expect(wrapper.emitted('select')![0]).toEqual(['gn1']);
  });

  it('renders bottom progress bar reflecting completed member ratio', () => {
    const store = useChatActivityStore();
    seedTeam(store, [
      mkMember('m1', { Status: 'completed' }),
      mkMember('m2', { Status: 'running' }),
      mkMember('m3', { Status: 'pending' }),
    ]);
    const wrapper = mountNode(mkNode());

    const fill = wrapper.find('.gtn-progress__fill');
    expect(fill.attributes('style')).toContain('width: 33%');
    expect(wrapper.find('.gtn-progress__text').text()).toBe('1/3');
  });

  it('applies tone classes for node status (failed → danger border)', () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1')]);
    const wrapper = mountNode(mkNode({ Status: 'failed' }));
    expect(wrapper.find('.graph-team-node--failed').exists()).toBe(true);
  });

  it('keeps layout height in sync with graphTeamNodeHeight(memberCount)', () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1'), mkMember('m2'), mkMember('m3')]);
    const wrapper = mountNode(mkNode());
    const style = wrapper.find('.graph-team-node').attributes('style') ?? '';
    expect(style).toContain(`height: ${graphTeamNodeHeight(3)}px`);
  });

  // ── F 任务：视觉层次 / 动效 / 状态感知增强（2026-07-27） ──

  it('renders running ripple ring on member status dot while running', () => {
    const store = useChatActivityStore();
    seedTeam(store, [
      mkMember('m1', { Status: 'running' }),
      mkMember('m2', { Status: 'completed' }),
    ]);
    const wrapper = mountNode(mkNode());
    const dots = wrapper.findAll('.gtn-member__dot');
    expect(dots[0]!.classes()).toContain('gtn-member__dot--ripple');
    expect(dots[1]!.classes()).not.toContain('gtn-member__dot--ripple');
  });

  it('shows latest action of the running member', () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1', { Status: 'running' })]);
    const wrapper = mountNode(mkNode());
    const action = wrapper.find('.gtn-action');
    expect(action.exists()).toBe(true);
    expect(action.text()).toContain('执行中');
  });

  it('shows inline error row with full text in title when a member failed', () => {
    const store = useChatActivityStore();
    seedTeam(store, [
      mkMember('m1', { Status: 'failed', Error: 'LLM timeout: context deadline exceeded' }),
    ]);
    const wrapper = mountNode(mkNode({ Status: 'failed' }));
    const err = wrapper.find('.gtn-error');
    expect(err.exists()).toBe(true);
    expect(err.attributes('title')).toBe('LLM timeout: context deadline exceeded');
    expect(err.text()).toContain('LLM timeout');
    // 有错误时不再渲染动作行
    expect(wrapper.find('.gtn-action').exists()).toBe(false);
  });

  it('falls back to generic failed label when failed member has no error text', () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1', { Status: 'failed', Error: '' })]);
    const wrapper = mountNode(mkNode({ Status: 'failed' }));
    const err = wrapper.find('.gtn-error');
    expect(err.exists()).toBe(true);
    expect(err.text()).toContain('失败');
  });

  it('keeps status row height equal to member row height in layout constants', async () => {
    const { GTN_ROW_H, GTN_STATUS_ROW_H } = await import('../graphTeamNodeUi');
    expect(GTN_STATUS_ROW_H).toBe(GTN_ROW_H);
  });

  // ── 待确认成员黄色慢闪（2026-08-07） ──

  function mkConfirmStep(memberId: string, status: 'tool_blocked' | 'completed') {
    return {
      ID: `step-confirm-${memberId}`,
      TurnID: 'tn1',
      TaskID: 'tk1',
      SessionID: `sess-${memberId}`,
      SpiritSessionID: 'sp1',
      Kind: 'confirm' as const,
      AuthorAgentKey: `agent-${memberId}`,
      Seq: 1,
      Version: 1,
      Content: '工具 shell 需要确认后执行',
      Reasoning: '',
      ToolName: 'shell',
      ToolCallID: 'call-1',
      ToolArgs: null,
      ToolResult: null,
      ToolDurationMs: 0,
      ToolErrorCode: '',
      Status: status,
      IsFinal: false,
      StartedAt: new Date().toISOString(),
      CompletedAt: null,
    };
  }

  it('blinks member row in warning yellow while a confirm step is tool_blocked', async () => {
    const store = useChatActivityStore();
    seedTeam(store, [mkMember('m1', { AgentName: '阿尔法' }), mkMember('m2', { AgentName: '贝塔' })]);
    store.upsertStep(mkConfirmStep('m1', 'tool_blocked'));
    const wrapper = mountNode(mkNode());

    const rows = wrapper.findAll('.gtn-member');
    expect(rows[0]!.classes()).toContain('gtn-member--confirm-pending');
    expect(rows[0]!.find('.gtn-member__dot--confirm-blink').exists()).toBe(true);
    expect(rows[0]!.find('.gtn-member__dot--warning').exists()).toBe(true);
    expect(rows[0]!.text()).toContain('待确认');
    // 无待确认 step 的成员不闪烁
    expect(rows[1]!.classes()).not.toContain('gtn-member--confirm-pending');

    // 确认处理完毕（step 完成）后闪烁消失
    store.upsertStep(mkConfirmStep('m1', 'completed'));
    await wrapper.vm.$nextTick();
    expect(wrapper.findAll('.gtn-member')[0]!.classes()).not.toContain('gtn-member--confirm-pending');
  });
});