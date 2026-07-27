// web/src/components/chat/v2/__tests__/GraphStageBlock.spec.ts
// 方案A GraphStageBlock 重写：
// - GraphTeamNode 富卡片（成员行 + 进度条）
// - 视口缩放/平移（按钮/滚轮/拖拽，拖拽抑制节点点击）
// - 始终显示（单节点也渲染，替代原 TeamStagePanel）
// - 成员点击 → MemberSessionDialog（事件向上透传）
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import GraphStageBlock from '../GraphStageBlock.vue';
import MemberSessionDialog from '../MemberSessionDialog.vue';
import { graphTeamNodeHeight } from '../graphTeamNodeUi';
import zhCN from '../../../../i18n/locales/zh-CN';
import type { GraphStage, GraphNode, TeamStage, TeamRun, MemberSession } from '../../../../features/chat/v2Types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-badge': { template: '<span class="q-badge-stub"><slot /></span>' },
  'q-avatar': { template: '<span />' },
  'q-btn': {
    // onClick 经 v-bind="$attrs" 自动注册为原生监听；再显式 @click 会触发两次
    template: '<button type="button" v-bind="$attrs"><slot /></button>',
  },
  'q-input': {
    props: ['modelValue'],
    emits: ['update:modelValue', 'keyup'],
    template:
      '<div class="q-input-stub"><input :value="modelValue" @input="$emit(\'update:modelValue\', ($event.target).value)" /><slot name="append" /></div>',
  },
  'q-dialog': {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<div v-if="modelValue" class="q-dialog-stub"><slot /></div>',
  },
  'q-card': { template: '<div class="q-card-stub"><slot /></div>' },
  'q-card-section': { template: '<div class="q-card-section-stub"><slot /></div>' },
};

function mkNode(id: string, deps: string[] = [], over: Partial<GraphNode> = {}): GraphNode {
  return {
    ID: id,
    GraphStageID: 'gs1',
    Label: id,
    DagNodeID: id,
    TeamStageID: '',
    Status: 'pending',
    DependsOn: deps,
    ...over,
  };
}

function mkStage(over: Partial<GraphStage> = {}): GraphStage {
  return {
    ID: 'gs1',
    TaskID: 'tk1',
    TurnID: 'tn1',
    SessionID: 's1',
    PlanBoardID: 'pb1',
    Nodes: [],
    Status: 'running',
    StartedAt: new Date().toISOString(),
    CompletedAt: null,
    Seq: 1,
    Version: 1,
    ...over,
  };
}

function mkTeamStage(nodeId: string, over: Partial<TeamStage> = {}): TeamStage {
  return {
    ID: `ts-${nodeId}`,
    TaskID: 'tk1',
    TurnID: 'tn1',
    SessionID: 's1',
    TeamID: `team-${nodeId}`,
    TeamName: `团队${nodeId}`,
    DagNodeID: nodeId,
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

function mkTeamRun(nodeId: string, over: Partial<TeamRun> = {}): TeamRun {
  return {
    ID: `tr-${nodeId}`,
    TeamStageID: `ts-${nodeId}`,
    TaskID: 'tk1',
    SessionID: 's1',
    SpiritSessionID: 'sp1',
    DagNodeID: nodeId,
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

function mkMember(nodeId: string, memberId: string, over: Partial<MemberSession> = {}): MemberSession {
  return {
    ID: `${nodeId}-${memberId}`,
    TeamRunID: `tr-${nodeId}`,
    TeamStageID: `ts-${nodeId}`,
    TaskID: 'tk1',
    SessionID: `sess-${nodeId}-${memberId}`,
    SpiritSessionID: 'sp1',
    AgentKey: `agent-${memberId}`,
    AgentName: `成员${memberId}`,
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

/** 为节点挂上团队 + 指定数量成员。 */
function seedTeam(store: ReturnType<typeof useChatActivityStore>, nodeId: string, memberCount: number) {
  store.upsertTeamStage(mkTeamStage(nodeId));
  store.upsertTeamRun(mkTeamRun(nodeId));
  for (let i = 0; i < memberCount; i++) {
    store.upsertMemberSession(mkMember(nodeId, `m${i}`));
  }
}

function mountBlock(stage: GraphStage) {
  return mount(GraphStageBlock, {
    props: { graphStage: stage },
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

describe('GraphStageBlock (方案A rich cards + viewport)', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('always renders, even for a single node (replaces TeamStagePanel)', () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    const wrapper = mountBlock(mkStage());
    expect(wrapper.find('.graph-stage-block').exists()).toBe(true);
    expect(wrapper.findAll('.graph-team-node')).toHaveLength(1);
  });

  it('renders GraphTeamNode rich cards with member rows', () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    seedTeam(store, 'a', 2);
    const wrapper = mountBlock(mkStage());
    expect(wrapper.findAll('.gtn-member')).toHaveLength(2);
    expect(wrapper.text()).toContain('成员m0');
  });

  it('lays out nodes with per-node heights driven by member count (heightOf)', () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    store.upsertGraphNode(mkNode('b', ['a']));
    store.upsertGraphNode(mkNode('c', ['a']));
    seedTeam(store, 'b', 3);
    seedTeam(store, 'c', 1);
    const wrapper = mountBlock(mkStage());

    const top = (id: string) => {
      const el = wrapper
        .findAll('.graph-team-node')
        .find((n) => n.find('.gtn-header__label').text() === id);
      expect(el, `node ${id}`).toBeTruthy();
      const m = /top: (\d+(?:\.\d+)?)px/.exec(el!.attributes('style') ?? '');
      return m ? parseFloat(m[1]!) : NaN;
    };
    // b/c 同列：b 从 padY 开始，c 紧跟 b（高度由 3 成员决定）
    expect(top('b')).toBe(12);
    expect(top('c')).toBe(12 + graphTeamNodeHeight(3) + 16);
  });

  it('zoom buttons update scale display around 100% → 115% → 100%', async () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    const wrapper = mountBlock(mkStage());

    const scaleText = () => wrapper.find('.graph-viewport-controls__scale').text();
    expect(scaleText()).toBe('100%');
    await wrapper.find('[data-testid="zoom-in"]').trigger('click');
    expect(scaleText()).toBe('115%');
    await wrapper.find('[data-testid="zoom-reset"]').trigger('click');
    expect(scaleText()).toBe('100%');
    await wrapper.find('[data-testid="zoom-out"]').trigger('click');
    expect(scaleText()).toBe('87%');
  });

  it('wheel zooms content transform', async () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    const wrapper = mountBlock(mkStage());

    await wrapper.find('.graph-stage-viewport').trigger('wheel', { deltaY: -100, clientX: 10, clientY: 10 });
    expect(wrapper.find('.graph-stage-canvas__inner').attributes('style')).toContain('scale(1.15');
  });

  it('pointer drag pans content and suppresses node/member click once', async () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    seedTeam(store, 'a', 1);
    const wrapper = mountBlock(mkStage());
    const viewport = wrapper.find('.graph-stage-viewport');

    // 拖拽 60px/40px → translate 跟随
    await viewport.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 });
    await viewport.trigger('pointermove', { clientX: 160, clientY: 140 });
    await viewport.trigger('pointerup', { clientX: 160, clientY: 140 });
    expect(wrapper.find('.graph-stage-canvas__inner').attributes('style')).toContain('translate(60px, 40px)');

    // pan 后的 click 被抑制（节点不选中、弹框不开）
    await wrapper.find('.gtn-header').trigger('click');
    expect(wrapper.find('.graph-team-node--selected').exists()).toBe(false);
    await wrapper.find('.gtn-member').trigger('click');
    expect(wrapper.find('.q-dialog-stub').exists()).toBe(false);

    // 下一轮干净点击恢复可选中
    await viewport.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 });
    await viewport.trigger('pointerup', { clientX: 100, clientY: 100 });
    await wrapper.find('.gtn-header').trigger('click');
    expect(wrapper.find('.graph-team-node--selected').exists()).toBe(true);
  });

  it('defers setPointerCapture until pan threshold so plain clicks reach members', async () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    seedTeam(store, 'a', 1);
    const wrapper = mountBlock(mkStage());
    const viewport = wrapper.find('.graph-stage-viewport');
    const captureSpy = vi.fn();
    (viewport.element as HTMLElement).setPointerCapture = captureSpy;

    // 纯点击（pointerdown+pointerup 无位移）：不得捕获指针，
    // 否则真实浏览器中 click 被重定向到视口，成员弹框打不开
    await viewport.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 });
    await viewport.trigger('pointerup', { clientX: 100, clientY: 100 });
    expect(captureSpy).not.toHaveBeenCalled();
    await wrapper.find('.gtn-member').trigger('click');
    expect(wrapper.findComponent(MemberSessionDialog).props('open')).toBe(true);

    // 关闭弹框后拖拽超阈值：此时才捕获指针（保证拖出视口仍能平移）
    await wrapper.findComponent(MemberSessionDialog).vm.$emit('update:open', false);
    await viewport.trigger('pointerdown', { button: 0, clientX: 100, clientY: 100 });
    await viewport.trigger('pointermove', { clientX: 160, clientY: 140 });
    expect(captureSpy).toHaveBeenCalledTimes(1);
    await viewport.trigger('pointerup', { clientX: 160, clientY: 140 });
  });

  it('opens MemberSessionDialog on member row click and passes through action events', async () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    seedTeam(store, 'a', 1);
    const wrapper = mountBlock(mkStage());

    await wrapper.find('.gtn-member').trigger('click');
    const dialog = wrapper.findComponent(MemberSessionDialog);
    expect(dialog.props('open')).toBe(true);
    expect(wrapper.find('.member-session-dialog__title').text()).toContain('成员m0');

    // 弹框操作事件向上透传（暂停 / 注入）
    dialog.vm.$emit('pause-agent', 'sess-a-m0');
    dialog.vm.$emit('inject-agent', { sessionId: 'sess-a-m0', message: 'hi' });
    expect(wrapper.emitted('pause-agent')?.[0]).toEqual(['sess-a-m0']);
    expect(wrapper.emitted('inject-agent')?.[0]).toEqual([{ sessionId: 'sess-a-m0', message: 'hi' }]);
  });

  it('closes the dialog via update:open', async () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    seedTeam(store, 'a', 1);
    const wrapper = mountBlock(mkStage());

    await wrapper.find('.gtn-member').trigger('click');
    expect(wrapper.find('.q-dialog-stub').exists()).toBe(true);
    await wrapper.find('.member-session-dialog__close').trigger('click');
    expect(wrapper.find('.q-dialog-stub').exists()).toBe(false);
  });
});
