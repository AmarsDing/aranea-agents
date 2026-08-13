// M5 运行时缺陷回归：图例「全部隐藏」后 —— 图例必须保持可见（恢复路径），且不得误显「库为空」覆盖层。
// 根因：GraphLegend/空态 v-if 误用过滤后 nodes；应使用未过滤 legendNodes。
import { describe, expect, it } from 'vitest';
import { shallowMount } from '@vue/test-utils';
import { createI18n } from 'vue-i18n';
import KnowledgeGraph3D from '../KnowledgeGraph3D.vue';
import GraphLegend from '../graph3d/GraphLegend.vue';
import type { CollectionGraphNode } from '../../../features/knowledge/types';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  missing: (_l, k) => k,
  messages: { 'zh-CN': {} },
});

const N = (id: string): CollectionGraphNode => ({
  doc_id: id,
  name: id,
  rel_path: `${id}.md`,
  doc_type: '',
  degree: 1,
});

function makeProps(over: Partial<Record<string, unknown>> = {}) {
  return {
    collections: [],
    collectionId: 'c1',
    linkTypes: [],
    pathPrefix: '',
    nodes: [N('a'), N('b')],
    edges: [],
    legendNodes: [N('a'), N('b')],
    hiddenGroups: [],
    totalNodes: 2,
    totalEdges: 0,
    hiddenIsolated: 0,
    loading: false,
    error: '',
    generation: 0,
    showIsolated: true,
    nodeQuery: '',
    nodeList: [],
    selectedNodeId: '',
    selectedNode: null,
    focusSignal: 0,
    scopeNodes: [],
    autoRotate: false,
    showLabels: true,
    neighborhoodHops: 0,
    neighborhoodRootName: '',
    mergeSuggestions: [],
    merging: false,
    lastMergeResult: null,
    fullscreen: false,
    ...over,
  };
}

const mountOpts = { global: { plugins: [i18n] } };

describe('KnowledgeGraph3D（M5 图例陷阱状态回归）', () => {
  it('全部组被隐藏（nodes=[] 但 legendNodes 非空）：图例仍渲染，无「库为空」覆盖层', () => {
    const w = shallowMount(KnowledgeGraph3D, {
      props: makeProps({ nodes: [], hiddenGroups: [''] }),
      ...mountOpts,
    });
    expect(w.findComponent(GraphLegend).exists()).toBe(true);
    expect(w.find('.knowledge-graph__overlay').exists()).toBe(false);
  });

  it('真空库（legendNodes=[]）：显示「库为空」覆盖层，不渲染图例', () => {
    const w = shallowMount(KnowledgeGraph3D, {
      props: makeProps({ nodes: [], legendNodes: [] }),
      ...mountOpts,
    });
    expect(w.find('.knowledge-graph__overlay').exists()).toBe(true);
    expect(w.findComponent(GraphLegend).exists()).toBe(false);
  });
});
