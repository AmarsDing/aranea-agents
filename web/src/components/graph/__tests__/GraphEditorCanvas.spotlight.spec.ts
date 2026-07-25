// web/src/components/graph/__tests__/GraphEditorCanvas.spotlight.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import GraphEditorCanvas from '../GraphEditorCanvas.vue';

const { fitViewSpy } = vi.hoisted(() => ({ fitViewSpy: vi.fn() }));

vi.mock('@vue-flow/core', () => ({
  VueFlow: {
    name: 'VueFlow',
    template: '<div class="vue-flow-mock"><slot /></div>',
  },
  useVueFlow: () => ({
    project: vi.fn(),
    fitView: fitViewSpy,
    getSelectedNodes: { value: [] },
    onViewportChange: vi.fn(),
    zoomTo: vi.fn(),
    getNodes: { value: [] },
  }),
  SelectionMode: { Partial: 'partial' },
}));

vi.mock('@vue-flow/background', () => ({
  Background: { name: 'Background', template: '<div class="background-mock" />' },
}));

vi.mock('../GraphFlowNode.vue', () => ({
  default: { name: 'GraphFlowNode', template: '<div />' },
}));
vi.mock('../GraphFlowDiamond.vue', () => ({
  default: { name: 'GraphFlowDiamond', template: '<div />' },
}));
vi.mock('../GraphFlowEdge.vue', () => ({
  default: { name: 'GraphFlowEdge', template: '<div />' },
}));
vi.mock('../GraphConnectionLine.vue', () => ({
  default: { name: 'GraphConnectionLine', template: '<div />' },
}));
vi.mock('../GraphContextMenu.vue', () => ({
  default: { name: 'GraphContextMenu', template: '<div />' },
}));
vi.mock('../GraphNodeSearch.vue', () => ({
  default: { name: 'GraphNodeSearch', template: '<div />' },
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({ dialog: vi.fn(), notify: vi.fn() }),
}));

describe('GraphEditorCanvas - R2-7 validation spotlight', () => {
  const mockGraphDef = {
    id: 'g1',
    name: 'G',
    version: 1,
    nodes: [
      { id: 'n1', type: 'agent', agentName: 'A' },
      { id: 'n2', type: 'llm' },
    ],
    edges: [],
    conditionalEdges: [],
    entryPoint: 'n1',
    finishPoint: '',
    stateFields: [],
    description: '',
  };

  beforeEach(() => {
    fitViewSpy.mockClear();
  });

  it('pans/zooms to spotlighted node when spotlightNodeId is set', async () => {
    const wrapper = mount(GraphEditorCanvas, {
      props: { graphDef: mockGraphDef, isDark: false, spotlightNodeId: null },
    });
    await wrapper.setProps({ spotlightNodeId: 'n1' });
    expect(fitViewSpy).toHaveBeenCalledWith(
      expect.objectContaining({ nodes: ['n1'], padding: 0.4, duration: 280, maxZoom: 1.2 }),
    );
  });

  it('does not refit when spotlightNodeId stays the same', async () => {
    const wrapper = mount(GraphEditorCanvas, {
      props: { graphDef: mockGraphDef, isDark: false, spotlightNodeId: 'n1' },
    });
    fitViewSpy.mockClear();
    await wrapper.setProps({ spotlightNodeId: 'n1' });
    expect(fitViewSpy).not.toHaveBeenCalled();
  });

  it('emits clear-spotlight on pane click', async () => {
    const wrapper = mount(GraphEditorCanvas, {
      props: { graphDef: mockGraphDef, isDark: false, spotlightNodeId: 'n1' },
    });
    wrapper.findComponent({ name: 'VueFlow' }).vm.$emit('pane-click');
    await wrapper.vm.$nextTick();
    expect(wrapper.emitted('clearSpotlight')).toHaveLength(1);
  });
});
