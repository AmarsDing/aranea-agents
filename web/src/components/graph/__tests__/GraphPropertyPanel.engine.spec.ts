// web/src/components/graph/__tests__/GraphPropertyPanel.engine.spec.ts
import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import GraphPropertyPanel from '../GraphPropertyPanel.vue';

// Mock child components
vi.mock('../GraphValidationPanel.vue', () => ({
  default: { name: 'GraphValidationPanel', template: '<div />' },
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      const translations: Record<string, string> = {
        'graphs.fieldExecutionEngine': '执行引擎',
        'graphs.engineBSP': 'BSP（默认）',
        'graphs.engineDAG': 'DAG（并行）',
        'graphs.engineBSPHint': '批量同步并行引擎：按超步同步调度，支持检查点和中断恢复',
        'graphs.engineDAGHint': '有向无环图引擎：并行执行无依赖节点，性能更优但不支持检查点',
        'graphs.fieldEnableCheckpoint': '启用检查点',
      };
      return translations[key] || key;
    },
  }),
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({
    dialog: vi.fn(),
    notify: vi.fn(),
  }),
}));

// Global stubs for Quasar components
const globalStubs = {
  'q-btn': { template: '<button><slot /></button>' },
  'q-separator': { template: '<hr />' },
  'q-input': { template: '<input />' },
  'q-select': {
    template: `
      <div class="q-select">
        <label>{{ label }}</label>
        <div v-for="opt in options" :key="opt.value" class="q-select__option">
          <slot name="option" :itemProps="{}" :opt="opt">
            <div>{{ opt.label }}</div>
            <div class="caption">{{ opt.hint }}</div>
          </slot>
        </div>
      </div>
    `,
    props: ['modelValue', 'options', 'label'],
  },
  'q-expansion-item': {
    template: '<div><div>{{ label }}</div><slot /></div>',
    props: ['label'],
  },
  'q-toggle': {
    template: '<label><input type="checkbox" :disabled="disable" /> {{ label }}</label>',
    props: ['modelValue', 'label', 'disable'],
  },
  'q-icon': { template: '<i />' },
  'q-tooltip': { template: '<span><slot /></span>' },
  'q-item': { template: '<div><slot /></div>' },
  'q-item-section': { template: '<div><slot /></div>' },
  'q-item-label': { template: '<div><slot /></div>' },
};

describe('GraphPropertyPanel - R2-4 Execution Engine UX', () => {
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
    executionEngine: 'bsp',
    enableCheckpoint: true,
  };

  const mockProps = {
    graphDef: mockGraphDef,
    selectedNode: null,
    availableTools: [],
    isDark: false,
    validationErrors: [],
    validationWarnings: [],
    validationValid: true,
    allNodes: [],
    stateFields: [],
  };

  it('displays execution engine select with tooltip hints', () => {
    const wrapper = mount(GraphPropertyPanel, {
      props: mockProps,
      global: { stubs: globalStubs },
    });

    // Check if it has engine label
    const html = wrapper.html();
    expect(html).toContain('执行引擎');
  });

  it('shows BSP engine option with descriptive tooltip', () => {
    const wrapper = mount(GraphPropertyPanel, {
      props: mockProps,
      global: { stubs: globalStubs },
    });

    const html = wrapper.html();

    // Check for BSP option with hint
    // This will fail initially as tooltips are not implemented
    expect(html).toContain('BSP');
    expect(html).toMatch(/批量同步并行|超步|检查点|中断恢复/);
  });

  it('shows DAG engine option with descriptive tooltip', () => {
    const wrapper = mount(GraphPropertyPanel, {
      props: mockProps,
      global: { stubs: globalStubs },
    });

    const html = wrapper.html();

    // Check for DAG option with hint
    // This will fail initially as tooltips are not implemented
    expect(html).toContain('DAG');
    expect(html).toMatch(/并行执行|性能更优|不支持检查点/);
  });

  it('disables checkpoint toggle when DAG engine is selected', async () => {
    const dagGraphDef = { ...mockGraphDef, executionEngine: 'dag' };
    const wrapper = mount(GraphPropertyPanel, {
      props: { ...mockProps, graphDef: dagGraphDef },
      global: { stubs: globalStubs },
    });

    // Find checkpoint toggle
    const checkpointToggle = wrapper.find('input[type="checkbox"]');

    // When DAG is selected, checkpoint should be disabled
    // This will fail initially as the logic is not implemented
    expect(checkpointToggle.attributes('disabled')).toBeDefined();
  });

  it('enables checkpoint toggle when BSP engine is selected', () => {
    const wrapper = mount(GraphPropertyPanel, {
      props: mockProps,
      global: { stubs: globalStubs },
    });

    // Find checkpoint toggle
    const checkpointToggle = wrapper.find('input[type="checkbox"]');

    // When BSP is selected, checkpoint should be enabled
    expect(checkpointToggle.attributes('disabled')).toBeUndefined();
  });
});
