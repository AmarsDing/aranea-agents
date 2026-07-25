// web/src/components/graph/__tests__/GraphEditorCanvas.layout.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick } from 'vue';
import GraphEditorCanvas from '../GraphEditorCanvas.vue';
import { writeGraphNodePosition } from '../../../features/graph/editor/graphLayout';

// Mock VueFlow components
vi.mock('@vue-flow/core', () => ({
  VueFlow: {
    name: 'VueFlow',
    template: '<div class="vue-flow-mock"><slot /></div>',
    props: ['nodes', 'edges'],
  },
  useVueFlow: () => ({
    project: vi.fn(),
    fitView: vi.fn(),
    getSelectedNodes: { value: [] },
    onViewportChange: vi.fn(),
    zoomTo: vi.fn(),
    getNodes: { value: [] },
  }),
  SelectionMode: { Partial: 'partial' },
}));

vi.mock('@vue-flow/background', () => ({
  Background: { name: 'Background', template: '<div />' },
}));

// Mock child components
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

describe('GraphEditorCanvas - R2-2 Layout Priority', () => {
  const createMockGraphDef = () => ({
    id: 'test-graph',
    name: 'Test Graph',
    version: 1,
    nodes: [
      { id: 'node1', type: 'function', description: 'Node 1' },
      { id: 'node2', type: 'function', description: 'Node 2' },
    ],
    edges: [{ from: 'node1', to: 'node2', kind: '' }],
    conditionalEdges: [],
    entryPoint: 'node1',
    finishPoint: 'node2',
    stateFields: [],
    description: '',
    metadata: {},
  });

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('prioritizes saved layout over existing positions when layout changes', async () => {
    const graphDef = createMockGraphDef();

    // Set initial saved layout
    writeGraphNodePosition(graphDef, 'node1', { x: 100, y: 100 });
    writeGraphNodePosition(graphDef, 'node2', { x: 300, y: 100 });

    const wrapper = mount(GraphEditorCanvas, {
      props: { graphDef, isDark: false },
    });

    // Simulate existing in-memory positions (different from saved)
    const vm = wrapper.vm as any;
    vm.internalNodes = [
      { id: 'node1', position: { x: 150, y: 150 }, type: 'function', data: {} },
      { id: 'node2', position: { x: 350, y: 150 }, type: 'function', data: {} },
    ];

    // Now update the saved layout (simulating auto-layout or version rollback)
    writeGraphNodePosition(graphDef, 'node1', { x: 200, y: 200 });
    writeGraphNodePosition(graphDef, 'node2', { x: 400, y: 200 });

    // Simulate layout watcher trigger by setting preferSavedLayout flag
    vm.preferSavedLayout = true;

    // Rebuild nodes
    const rebuiltNodes = vm.buildNodes();

    // When preferSavedLayout is true, saved layout should take priority
    expect(rebuiltNodes[0].position).toEqual({ x: 200, y: 200 });
    expect(rebuiltNodes[1].position).toEqual({ x: 400, y: 200 });
  });

  it('preserves existing positions during normal rebuild (drag operations)', async () => {
    const graphDef = createMockGraphDef();

    // Set initial saved layout
    writeGraphNodePosition(graphDef, 'node1', { x: 100, y: 100 });
    writeGraphNodePosition(graphDef, 'node2', { x: 300, y: 100 });

    const wrapper = mount(GraphEditorCanvas, {
      props: { graphDef, isDark: false },
    });

    // Simulate existing in-memory positions (user dragged nodes)
    const vm = wrapper.vm as any;
    vm.internalNodes = [
      { id: 'node1', position: { x: 150, y: 150 }, type: 'function', data: {} },
      { id: 'node2', position: { x: 350, y: 150 }, type: 'function', data: {} },
    ];

    // Trigger normal rebuild (e.g., node data changed, not layout)
    const rebuiltNodes = vm.buildNodes();

    // During normal rebuild, existing positions should be preserved
    expect(rebuiltNodes[0].position).toEqual({ x: 150, y: 150 });
    expect(rebuiltNodes[1].position).toEqual({ x: 350, y: 150 });
  });

  it('initializes preferSavedLayout flag as false', () => {
    const graphDef = createMockGraphDef();

    const wrapper = mount(GraphEditorCanvas, {
      props: { graphDef, isDark: false },
    });

    const vm = wrapper.vm as any;

    // Initially should be false
    expect(vm.preferSavedLayout).toBe(false);
  });
});
