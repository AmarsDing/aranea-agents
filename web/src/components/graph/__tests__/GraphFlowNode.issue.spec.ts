// web/src/components/graph/__tests__/GraphFlowNode.issue.spec.ts
import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import GraphFlowNode from '../GraphFlowNode.vue';
import GraphFlowDiamond from '../GraphFlowDiamond.vue';

vi.mock('@vue-flow/core', () => ({
  Handle: { name: 'Handle', template: '<div class="handle-mock" />' },
  Position: { Left: 'left', Right: 'right', Top: 'top', Bottom: 'bottom' },
}));

vi.mock('../orchestration/OrchestrationStatusChip.vue', () => ({
  default: { name: 'OrchestrationStatusChip', template: '<div />' },
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}));

const globalStubs = {
  'q-icon': { template: '<i class="q-icon" />', props: ['name', 'size'] },
  'q-badge': { template: '<span class="q-badge"><slot /></span>' },
  'q-tooltip': { template: '<span class="q-tooltip"><slot /></span>' },
};

const baseData = {
  nodeId: 'n1',
  nodeType: 'agent' as const,
  label: '研究助手',
};

const errorIssue = { level: 'error' as const, code: 'unreachable_node', message: '节点不可达: n1' };
const warningIssue = { level: 'warning' as const, code: 'orphan_node', message: '孤立节点（无连接）: n1' };

function mountNode(data: Record<string, unknown>) {
  return mount(GraphFlowNode, {
    props: { id: 'n1', data: { ...baseData, ...data } },
    global: { stubs: globalStubs },
  });
}

describe('GraphFlowNode - R2-7 validation issue state', () => {
  it('applies error class and pulse marker when issue level is error', () => {
    const wrapper = mountNode({ issue: errorIssue });
    expect(wrapper.find('.graph-flow-node').classes()).toContain('graph-flow-node--issue-error');
  });

  it('applies warning class when issue level is warning', () => {
    const wrapper = mountNode({ issue: warningIssue });
    expect(wrapper.find('.graph-flow-node').classes()).toContain('graph-flow-node--issue-warning');
  });

  it('renders inline issue bar with truncated message', () => {
    const wrapper = mountNode({ issue: errorIssue });
    const bar = wrapper.find('.graph-flow-node__issue-bar');
    expect(bar.exists()).toBe(true);
    expect(bar.text()).toContain('节点不可达: n1');
  });

  it('does not render issue bar when no issue', () => {
    const wrapper = mountNode({});
    expect(wrapper.find('.graph-flow-node__issue-bar').exists()).toBe(false);
  });

  it('renders spotlight bubble with code, message and suggestion when spotlighted', () => {
    const wrapper = mountNode({ issue: errorIssue, spotlighted: true });
    const bubble = wrapper.find('.graph-flow-node__bubble');
    expect(bubble.exists()).toBe(true);
    expect(bubble.text()).toContain('unreachable_node');
    expect(bubble.text()).toContain('节点不可达: n1');
    expect(bubble.text()).toContain('graphs.suggestionUnreachable');
  });

  it('hides spotlight bubble when not spotlighted even with issue', () => {
    const wrapper = mountNode({ issue: errorIssue, spotlighted: false });
    expect(wrapper.find('.graph-flow-node__bubble').exists()).toBe(false);
  });
});

describe('GraphFlowDiamond - R2-7 validation issue state', () => {
  function mountDiamond(data: Record<string, unknown>) {
    return mount(GraphFlowDiamond, {
      props: { id: 'r1', data: { nodeId: 'r1', nodeType: 'router', label: 'r1', ...data } },
      global: { stubs: globalStubs },
    });
  }

  it('applies error class when issue level is error', () => {
    const wrapper = mountDiamond({ issue: errorIssue });
    expect(wrapper.find('.graph-flow-diamond').classes()).toContain('graph-flow-diamond--issue-error');
  });

  it('applies warning class when issue level is warning', () => {
    const wrapper = mountDiamond({ issue: warningIssue });
    expect(wrapper.find('.graph-flow-diamond').classes()).toContain('graph-flow-diamond--issue-warning');
  });

  it('renders spotlight bubble when spotlighted with issue', () => {
    const wrapper = mountDiamond({ issue: errorIssue, spotlighted: true });
    const bubble = wrapper.find('.graph-flow-diamond__bubble');
    expect(bubble.exists()).toBe(true);
    expect(bubble.text()).toContain('unreachable_node');
  });
});
