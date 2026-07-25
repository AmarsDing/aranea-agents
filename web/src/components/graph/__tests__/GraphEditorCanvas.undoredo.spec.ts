// web/src/components/graph/__tests__/GraphEditorCanvas.undoredo.spec.ts
// 画布编辑操作经 undoRedo 命令栈的回归测试：
// 根因契约 —— execute() 会通过 redo() 完成首次应用，调用方不得预改 graphDef。
// 拖入/复制/连线 都必须恰好产生一个实体，undo 后全量回收。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { reactive } from 'vue';
import GraphEditorCanvas from '../GraphEditorCanvas.vue';
import { useGraphUndoRedo } from '../../../features/graph/useGraphUndoRedo';
import type { GraphDefinition, NodeDef } from '../../../features/graph/types';

const selectedNodesRef = { value: [] as { id: string }[] };

vi.mock('@vue-flow/core', () => ({
  VueFlow: {
    name: 'VueFlow',
    template: '<div class="vue-flow-mock"><slot /></div>',
  },
  useVueFlow: () => ({
    project: vi.fn(() => ({ x: 50, y: 60 })),
    fitView: vi.fn(),
    getSelectedNodes: selectedNodesRef,
    onViewportChange: vi.fn(),
    zoomTo: vi.fn(),
    getNodes: { value: [] },
  }),
  SelectionMode: { Partial: 'partial' },
}));

vi.mock('@vue-flow/background', () => ({
  Background: { name: 'Background', template: '<div class="background-mock" />' },
}));

vi.mock('../GraphFlowNode.vue', () => ({ default: { name: 'GraphFlowNode', template: '<div />' } }));
vi.mock('../GraphFlowDiamond.vue', () => ({ default: { name: 'GraphFlowDiamond', template: '<div />' } }));
vi.mock('../GraphFlowEdge.vue', () => ({ default: { name: 'GraphFlowEdge', template: '<div />' } }));
vi.mock('../GraphConnectionLine.vue', () => ({ default: { name: 'GraphConnectionLine', template: '<div />' } }));
vi.mock('../GraphContextMenu.vue', () => ({ default: { name: 'GraphContextMenu', template: '<div />' } }));
vi.mock('../GraphNodeSearch.vue', () => ({ default: { name: 'GraphNodeSearch', template: '<div />' } }));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({ dialog: vi.fn(), notify: vi.fn() }),
}));

function makeNode(id: string): NodeDef {
  return {
    id,
    funcRef: '',
    interruptBefore: false,
    interruptAfter: false,
    type: 'function',
    description: '',
    instruction: '',
    modelName: '',
    toolNames: [],
    agentName: '',
    destinations: [],
    requiredRole: '',
    assignmentMode: 'static',
    assignmentStrategy: '',
    reviewerAgent: '',
    reviewRules: '',
    timeoutSeconds: 0,
    heartbeatIntervalSeconds: 0,
    enableLeaseExtension: false,
    retryMaxAttempts: 0,
    failureAction: '',
    fallbackAgent: '',
    inputMapperJson: '',
    outputMapperJson: '',
    isolatedMessages: false,
    inputFromLastResponse: false,
    cacheEnabled: false,
    cacheTtlSeconds: 0,
  };
}

function setup(initialNodes: NodeDef[] = []) {
  const graphDef = reactive<GraphDefinition>({
    id: 'g1',
    name: 'G',
    description: '',
    stateFields: [],
    nodes: initialNodes,
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: '',
    finishPoint: '',
    enableCheckpoint: true,
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 0,
    sortOrder: 0,
    createdAt: '',
    updatedAt: '',
  }) as GraphDefinition;
  const undoRedo = useGraphUndoRedo(graphDef, vi.fn());
  const wrapper = mount(GraphEditorCanvas, {
    props: { graphDef, isDark: false, undoRedo },
  });
  return { graphDef, undoRedo, wrapper };
}

describe('GraphEditorCanvas - 编辑操作恰好应用一次', () => {
  beforeEach(() => {
    selectedNodesRef.value = [];
  });

  it('拖入节点：graphDef 中恰好新增一个节点，undo 后为空', async () => {
    const { graphDef, undoRedo, wrapper } = setup();

    await wrapper.find('.graph-editor-canvas').trigger('drop', {
      dataTransfer: { getData: () => 'function' },
      clientX: 10,
      clientY: 10,
    });

    expect(graphDef.nodes).toHaveLength(1);
    expect(undoRedo.canUndo.value).toBe(true);

    undoRedo.undo();
    expect(graphDef.nodes).toHaveLength(0);
  });

  it('Ctrl+D 复制节点：恰好新增一个副本，undo 后只剩原节点', async () => {
    selectedNodesRef.value = [{ id: 'n1' }];
    const { graphDef, undoRedo } = setup([makeNode('n1')]);

    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'd', ctrlKey: true }));
    await new Promise((r) => setTimeout(r, 0));

    expect(graphDef.nodes).toHaveLength(2);
    const copies = graphDef.nodes.filter((n) => n.id !== 'n1');
    expect(copies).toHaveLength(1);

    undoRedo.undo();
    expect(graphDef.nodes).toHaveLength(1);
    expect(graphDef.nodes[0].id).toBe('n1');
  });

  it('连线：恰好新增一条边，undo 后边为空', async () => {
    const { graphDef, undoRedo, wrapper } = setup([makeNode('n1'), makeNode('n2')]);

    wrapper.findComponent({ name: 'VueFlow' }).vm.$emit('connect', { source: 'n1', target: 'n2' });
    await new Promise((r) => setTimeout(r, 0));

    expect(graphDef.edges).toHaveLength(1);
    expect(graphDef.edges[0]).toMatchObject({ from: 'n1', to: 'n2' });

    undoRedo.undo();
    expect(graphDef.edges).toHaveLength(0);
  });
});
