// web/src/components/graph/__tests__/GraphPropertyPanel.undoredo.spec.ts
// State 字段增删经 undoRedo 命令栈的回归测试：
// 添加必须恰好一条、删除必须只删目标（不误伤相邻字段），undo 精确还原。
import { describe, it, expect, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { reactive } from 'vue';
import GraphPropertyPanel from '../GraphPropertyPanel.vue';
import { useGraphUndoRedo } from '../../../features/graph/useGraphUndoRedo';
import type { GraphDefinition, StateFieldDef } from '../../../features/graph/types';

vi.mock('../GraphValidationPanel.vue', () => ({
  default: { name: 'GraphValidationPanel', template: '<div />' },
}));

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({ dialog: vi.fn(), notify: vi.fn() }),
}));

const globalStubs = {
  'q-btn': { template: '<button v-bind="$attrs">{{ label }}<slot /></button>', props: ['label'] },
  'q-separator': { template: '<hr />' },
  'q-input': { template: '<input />', props: ['modelValue', 'label'] },
  'q-select': { template: '<div />', props: ['modelValue', 'options', 'label'] },
  'q-expansion-item': {
    template: '<div><div>{{ label }}</div><slot /></div>',
    props: ['label'],
  },
  'q-toggle': { template: '<label />', props: ['modelValue', 'label', 'disable'] },
  'q-icon': { template: '<i />' },
  'q-tooltip': { template: '<span />' },
  'q-item': { template: '<div><slot /></div>' },
  'q-item-section': { template: '<div><slot /></div>' },
  'q-item-label': { template: '<div><slot /></div>' },
};

function makeField(name: string): StateFieldDef {
  return { name, type: 'string', reducer: 'cover', required: false, disableDeepCopy: false };
}

function setup(initialFields: StateFieldDef[] = []) {
  const graphDef = reactive<GraphDefinition>({
    id: 'g1',
    name: 'G',
    description: '',
    stateFields: initialFields,
    nodes: [],
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
  const wrapper = mount(GraphPropertyPanel, {
    props: {
      graphDef,
      selectedNode: null,
      availableTools: [],
      isDark: false,
      validationErrors: [],
      validationWarnings: [],
      validationValid: true,
      allNodes: [],
      stateFields: initialFields,
      undoRedo,
    },
    global: { stubs: globalStubs },
  });
  return { graphDef, undoRedo, wrapper };
}

describe('GraphPropertyPanel - State 字段增删恰好应用一次', () => {
  it('添加 State 字段：恰好新增一条，undo 后为空', async () => {
    const { graphDef, undoRedo, wrapper } = setup();

    const addBtn = wrapper.findAll('button').find((b) => b.text().includes('graphs.stateFieldAddButton'));
    expect(addBtn).toBeTruthy();
    await addBtn!.trigger('click');

    expect(graphDef.stateFields).toHaveLength(1);

    undoRedo.undo();
    expect(graphDef.stateFields).toHaveLength(0);
  });

  it('删除 State 字段：只删除目标字段，相邻字段不受影响；undo 精确还原', async () => {
    const { graphDef, undoRedo, wrapper } = setup([makeField('a'), makeField('b'), makeField('c')]);

    const rows = wrapper.findAll('.state-field-row');
    expect(rows).toHaveLength(3);
    // 删除第 2 行（b）
    await rows[1].find('button').trigger('click');

    expect(graphDef.stateFields.map((f) => f.name)).toEqual(['a', 'c']);

    undoRedo.undo();
    expect(graphDef.stateFields.map((f) => f.name)).toEqual(['a', 'b', 'c']);
  });
});
