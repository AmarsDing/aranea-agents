// web/src/components/graph/__tests__/GraphValidationPanel.spec.ts
import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import GraphValidationPanel from '../GraphValidationPanel.vue';
import type { ValidationIssue } from '../../../features/graph/types';

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}));

const globalStubs = {
  'q-btn': {
    template: '<button class="q-btn" @click="$emit(\'click\', $event)"><slot /></button>',
    props: ['loading', 'icon', 'label'],
    emits: ['click'],
  },
  'q-icon': { template: '<i class="q-icon" />', props: ['name'] },
  'q-tooltip': { template: '<span><slot /></span>' },
  'q-space': { template: '<span class="q-space" />' },
};

const errorIssue: ValidationIssue = {
  nodeId: 'n1',
  nodeLabel: '研究助手',
  level: 'error',
  code: 'unreachable_node',
  field: '',
  message: '节点不可达: n1',
};

const warningIssue: ValidationIssue = {
  nodeId: 'n2',
  nodeLabel: 'n2',
  level: 'warning',
  code: 'orphan_node',
  field: '',
  message: '孤立节点（无连接）: n2',
};

const graphLevelIssue: ValidationIssue = {
  nodeId: '',
  nodeLabel: '',
  level: 'error',
  code: 'no_entry_point',
  field: 'entryPoint',
  message: '缺少入口节点',
};

function mountPanel(issues: ValidationIssue[], open = true) {
  return mount(GraphValidationPanel, {
    props: { open, issues, validating: false },
    global: { stubs: globalStubs },
  });
}

describe('GraphValidationPanel - R2-7 bottom dock redesign', () => {
  it('renders nothing when open is false', () => {
    const wrapper = mountPanel([errorIssue], false);
    expect(wrapper.find('.graph-validation-dock').exists()).toBe(false);
  });

  it('renders dock with level filter counts', () => {
    const wrapper = mountPanel([errorIssue, warningIssue]);
    const dock = wrapper.find('.graph-validation-dock');
    expect(dock.exists()).toBe(true);
    const html = dock.html();
    expect(html).toContain('graphs.validationFilterAll');
    expect(html).toContain('graphs.validationFilterErrors');
    expect(html).toContain('graphs.validationFilterWarnings');
  });

  it('lists issues with node label, code chip and message', () => {
    const wrapper = mountPanel([errorIssue]);
    const html = wrapper.find('.graph-validation-dock__list').html();
    expect(html).toContain('研究助手');
    expect(html).toContain('unreachable_node');
    expect(html).toContain('节点不可达: n1');
  });

  it('shows repair suggestion for known codes', () => {
    const wrapper = mountPanel([errorIssue]);
    expect(wrapper.html()).toContain('graphs.suggestionUnreachable');
  });

  it('filters issues by level', async () => {
    const wrapper = mountPanel([errorIssue, warningIssue]);
    expect(wrapper.findAll('.graph-validation-dock__row')).toHaveLength(2);

    await wrapper.find('[data-testid="filter-error"]').trigger('click');
    let rows = wrapper.findAll('.graph-validation-dock__row');
    expect(rows).toHaveLength(1);
    expect(rows[0].html()).toContain('unreachable_node');

    await wrapper.find('[data-testid="filter-warning"]').trigger('click');
    rows = wrapper.findAll('.graph-validation-dock__row');
    expect(rows).toHaveLength(1);
    expect(rows[0].html()).toContain('orphan_node');

    await wrapper.find('[data-testid="filter-all"]').trigger('click');
    expect(wrapper.findAll('.graph-validation-dock__row')).toHaveLength(2);
  });

  it('emits locate with nodeId when locate button clicked', async () => {
    const wrapper = mountPanel([errorIssue]);
    await wrapper.find('[data-testid="locate-btn"]').trigger('click');
    expect(wrapper.emitted('locate')).toEqual([['n1']]);
  });

  it('emits locate when row clicked', async () => {
    const wrapper = mountPanel([errorIssue]);
    await wrapper.find('.graph-validation-dock__row').trigger('click');
    expect(wrapper.emitted('locate')).toEqual([['n1']]);
  });

  it('hides locate button for graph-level issues without nodeId', () => {
    const wrapper = mountPanel([graphLevelIssue]);
    expect(wrapper.find('[data-testid="locate-btn"]').exists()).toBe(false);
    expect(wrapper.html()).toContain('graphs.validationGraphLevel');
  });

  it('emits close and revalidate from header buttons', async () => {
    const wrapper = mountPanel([errorIssue]);
    await wrapper.find('[data-testid="revalidate-btn"]').trigger('click');
    await wrapper.find('[data-testid="close-btn"]').trigger('click');
    expect(wrapper.emitted('revalidate')).toHaveLength(1);
    expect(wrapper.emitted('close')).toHaveLength(1);
  });
});
