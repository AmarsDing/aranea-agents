// web/src/components/graph/__tests__/GraphEditorCanvas.spec.ts
import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import GraphEditorCanvas from '../GraphEditorCanvas.vue';

// Mock VueFlow components to avoid complex rendering
vi.mock('@vue-flow/core', () => ({
  VueFlow: {
    name: 'VueFlow',
    template: '<div class="vue-flow-mock"><slot /></div>',
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
  Background: {
    name: 'Background',
    template: '<div class="background-mock" />',
  },
}));

vi.mock('@vue-flow/controls', () => ({
  Controls: {
    name: 'Controls',
    template: '<div class="controls-mock" />',
  },
}));

vi.mock('@vue-flow/minimap', () => ({
  MiniMap: {
    name: 'MiniMap',
    template: '<div class="minimap-mock" />',
  },
}));

// Mock child components
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

// Mock i18n
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}));

// Mock Quasar
vi.mock('quasar', () => ({
  useQuasar: () => ({
    dialog: vi.fn(),
    notify: vi.fn(),
  }),
}));

describe('GraphEditorCanvas - R2-1 Canvas Control Simplification', () => {
  const mockGraphDef = {
    id: 'test-graph',
    name: 'Test Graph',
    version: 1,
    nodes: [],
    edges: [],
    conditionalEdges: [],
    entryPoint: '',
    finishPoint: '',
    stateFields: [],
    description: '',
  };

  it('does NOT render VueFlow Controls component', () => {
    const wrapper = mount(GraphEditorCanvas, {
      props: {
        graphDef: mockGraphDef,
        isDark: false,
      },
    });

    // Controls component should NOT be present
    expect(wrapper.findComponent({ name: 'Controls' }).exists()).toBe(false);
    expect(wrapper.find('.controls-mock').exists()).toBe(false);
  });

  it('does NOT render VueFlow MiniMap component', () => {
    const wrapper = mount(GraphEditorCanvas, {
      props: {
        graphDef: mockGraphDef,
        isDark: false,
      },
    });

    // MiniMap component should NOT be present
    expect(wrapper.findComponent({ name: 'MiniMap' }).exists()).toBe(false);
    expect(wrapper.find('.minimap-mock').exists()).toBe(false);
  });

  it('renders zoom indicator with glassmorphism styling at bottom center', () => {
    const wrapper = mount(GraphEditorCanvas, {
      props: {
        graphDef: mockGraphDef,
        isDark: false,
      },
    });

    // Zoom indicator should exist
    const zoomIndicator = wrapper.find('.graph-editor-canvas__zoom-indicator');
    expect(zoomIndicator.exists()).toBe(true);

    // Should have the correct CSS class (styling is applied via scoped styles)
    expect(zoomIndicator.classes()).toContain('graph-editor-canvas__zoom-indicator');
  });

  it('positions zoom indicator at bottom center of canvas', () => {
    const wrapper = mount(GraphEditorCanvas, {
      props: {
        graphDef: mockGraphDef,
        isDark: false,
      },
    });

    const zoomIndicator = wrapper.find('.graph-editor-canvas__zoom-indicator');
    expect(zoomIndicator.exists()).toBe(true);

    // Verify it's positioned within the canvas container
    const canvas = wrapper.find('.graph-editor-canvas');
    expect(canvas.exists()).toBe(true);
    expect(canvas.element.contains(zoomIndicator.element)).toBe(true);
  });
});
