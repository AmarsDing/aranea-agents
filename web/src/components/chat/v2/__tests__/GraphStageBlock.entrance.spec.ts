// web/src/components/chat/v2/__tests__/GraphStageBlock.entrance.spec.ts
// P0 级联入场动画：计划发布瞬间节点按 DAG 层级 stagger 入场、边淡入跟随；
// 刷新 replay（StartedAt 久远）不播入场动画；运行中新增节点始终播。
import { describe, it, expect, beforeEach } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import { createI18n } from 'vue-i18n';
import { useChatActivityStore } from '../../../../stores/chat/activityV2Store';
import GraphStageBlock from '../GraphStageBlock.vue';
import zhCN from '../../../../i18n/locales/zh-CN';
import type { GraphStage, GraphNode } from '../../../../features/chat/v2Types';

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } });

const quasarStubs = {
  'q-icon': { template: '<i />' },
  'q-badge': { template: '<span class="q-badge-stub"><slot /></span>' },
  // onClick 经 v-bind="$attrs" 自动注册为原生监听；再显式 @click 会触发两次
  'q-btn': { template: '<button type="button" v-bind="$attrs"><slot /></button>' },
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

/** a → (b, c) → d 的 3 层 DAG。 */
function seedDiamond(store: ReturnType<typeof useChatActivityStore>) {
  store.upsertGraphNode(mkNode('a'));
  store.upsertGraphNode(mkNode('b', ['a']));
  store.upsertGraphNode(mkNode('c', ['a']));
  store.upsertGraphNode(mkNode('d', ['b', 'c']));
}

function mountBlock(stage: GraphStage) {
  return mount(GraphStageBlock, {
    props: { graphStage: stage },
    global: { plugins: [i18n], stubs: quasarStubs },
  });
}

describe('GraphStageBlock staggered entrance animation', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('plays entrance animation with per-layer increasing delays for a live graph stage', () => {
    const store = useChatActivityStore();
    seedDiamond(store);
    const wrapper = mountBlock(mkStage()); // StartedAt = now → live

    const enterNodes = wrapper.findAll('.graph-team-node--enter');
    expect(enterNodes).toHaveLength(4);

    const delayOf = (label: string) => {
      const el = enterNodes.find((n) => n.text().includes(label));
      expect(el, `node ${label} should have entrance class`).toBeTruthy();
      return el!.attributes('style') ?? '';
    };
    // layer 0: a → 0ms；layer 1: b/c → 150ms/230ms；layer 2: d → 300ms
    expect(delayOf('a')).toContain('animation-delay: 0ms');
    expect(delayOf('b')).toContain('animation-delay: 150ms');
    expect(delayOf('c')).toContain('animation-delay: 230ms');
    expect(delayOf('d')).toContain('animation-delay: 300ms');
  });

  it('plays edge fade-in following the target node for a live graph stage', () => {
    const store = useChatActivityStore();
    seedDiamond(store);
    const wrapper = mountBlock(mkStage());

    const enterEdges = wrapper.findAll('.graph-edge--enter');
    // a→b, a→c, b→d, c→d
    expect(enterEdges).toHaveLength(4);
    // 边 delay = 目标节点 delay + 节点入场时长（250ms 起）
    expect(enterEdges[0]!.attributes('style')).toContain('animation-delay:');
  });

  it('skips entrance animation when the stage started long ago (replay)', () => {
    const store = useChatActivityStore();
    seedDiamond(store);
    const wrapper = mountBlock(
      mkStage({ StartedAt: new Date(Date.now() - 3600_000).toISOString(), Status: 'completed' }),
    );

    expect(wrapper.findAll('.graph-team-node--enter')).toHaveLength(0);
    expect(wrapper.findAll('.graph-edge--enter')).toHaveLength(0);
    // 节点仍然正常渲染
    expect(wrapper.findAll('.graph-team-node')).toHaveLength(4);
  });

  it('animates nodes that appear after mount (e.g. replan inserts), even on replay', async () => {
    const store = useChatActivityStore();
    seedDiamond(store);
    const wrapper = mountBlock(mkStage({ StartedAt: new Date(Date.now() - 3600_000).toISOString() }));
    expect(wrapper.findAll('.graph-team-node--enter')).toHaveLength(0);

    store.upsertGraphNode(mkNode('e', ['d']));
    await nextTick();

    const enterNodes = wrapper.findAll('.graph-team-node--enter');
    expect(enterNodes).toHaveLength(1);
    expect(enterNodes[0]!.text()).toContain('e');
    // e 位于 layer 3 → 3*150 = 450ms
    expect(enterNodes[0]!.attributes('style')).toContain('animation-delay: 450ms');
  });
});

describe('GraphTeamNode status transition animation', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('flashes when a node transitions running → completed', async () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a'));
    store.upsertGraphNode(mkNode('b', ['a'], { Status: 'running' }));
    const wrapper = mountBlock(mkStage());

    store.upsertGraphNode(mkNode('b', ['a'], { Status: 'completed' }));
    await nextTick();

    const nodeB = wrapper.findAll('.graph-team-node').find((n) => n.text().includes('b'));
    expect(nodeB?.classes()).toContain('graph-team-node--just-completed');
  });

  it('does not flash for nodes already completed at mount (replay)', () => {
    const store = useChatActivityStore();
    store.upsertGraphNode(mkNode('a', [], { Status: 'completed' }));
    store.upsertGraphNode(mkNode('b', ['a'], { Status: 'completed' }));
    const wrapper = mountBlock(
      mkStage({ StartedAt: new Date(Date.now() - 3600_000).toISOString(), Status: 'completed' }),
    );

    expect(wrapper.findAll('.graph-team-node--just-completed')).toHaveLength(0);
  });
});
