// web/src/components/graph/__tests__/GraphStateSchemaDrawer.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import GraphStateSchemaDrawer from '../GraphStateSchemaDrawer.vue';
import type { GraphDefinition, NodeDef, StateFieldDef } from '../../../features/graph/types';

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
  }),
}));

// $q.dialog 链式 onOk 立即执行（测试删除路径）
const dialogOnOk = vi.hoisted(() => ({ immediate: true }));
vi.mock('quasar', () => ({
  useQuasar: () => ({
    dialog: () => ({
      onOk: (cb: () => void) => {
        if (dialogOnOk.immediate) cb();
        return { onCancel: () => ({}) };
      },
    }),
  }),
}));

const globalStubs = {
  Teleport: { template: '<div><slot /></div>' },
  'q-btn': {
    template: '<button class="q-btn" @click="$emit(\'click\', $event)"><slot /></button>',
    props: ['loading', 'icon', 'label', 'disable'],
    emits: ['click'],
  },
  'q-icon': { template: '<i class="q-icon" />', props: ['name'] },
  'q-input': {
    template:
      '<input class="q-input" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder', 'label'],
    emits: ['update:modelValue'],
  },
  'q-select': {
    template:
      '<select class="q-select" :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option></select>',
    props: ['modelValue', 'options', 'label'],
    emits: ['update:modelValue'],
  },
  'q-toggle': {
    template:
      '<label class="q-toggle"><input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" /></label>',
    props: ['modelValue', 'label'],
    emits: ['update:modelValue'],
  },
};

function field(name: string, type = 'string', defaultValue?: unknown): StateFieldDef {
  return { name, type, reducer: 'append', defaultValue, required: false, disableDeepCopy: false };
}

function llmNode(id: string, instruction: string): NodeDef {
  return {
    id,
    funcRef: '',
    interruptBefore: false,
    interruptAfter: false,
    type: 'llm',
    description: '',
    instruction,
    modelName: '',
    toolNames: [],
    agentName: '',
    destinations: [],
    requiredRole: '',
    assignmentMode: '',
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

function makeGraph(stateFields: StateFieldDef[], nodes: NodeDef[] = []): GraphDefinition {
  return {
    id: 'g1',
    name: 'g',
    description: '',
    stateFields,
    nodes,
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: '',
    finishPoint: '',
    enableCheckpoint: false,
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 1,
    sortOrder: 0,
    createdAt: '',
    updatedAt: '',
  };
}

function mountDrawer(graph: GraphDefinition, open = true) {
  return mount(GraphStateSchemaDrawer, {
    props: { open, isDark: true, graphDef: graph, 'onUpdate:graphDef': () => {} },
    global: { stubs: globalStubs },
  });
}

beforeEach(() => {
  dialogOnOk.immediate = true;
});

describe('GraphStateSchemaDrawer - R2-8 State Schema 独立抽屉', () => {
  it('R2-G.1 头部：字段总数 + 新建字段；新建追加唯一名字段并 emit change', async () => {
    const graph = makeGraph([field('a'), field('b')]);
    const wrapper = mountDrawer(graph);
    expect(wrapper.find('.state-schema-drawer__count').text()).toBe('2');

    await wrapper.find('[data-action="add-field"]').trigger('click');
    expect(graph.stateFields).toHaveLength(3);
    expect(graph.stateFields[2].name).toBe('field_3');
    expect(wrapper.emitted('change')).toBeTruthy();

    // 名称冲突时递增
    graph.stateFields.push(field('field_4'));
    await wrapper.find('[data-action="add-field"]').trigger('click');
    expect(graph.stateFields[4].name).toBe('field_5');
  });

  it('R2-G.2 搜索：按字段名与默认值过滤', async () => {
    const graph = makeGraph([field('order_id'), field('user_name'), field('retry_count', 'integer', 3)]);
    const wrapper = mountDrawer(graph);
    expect(wrapper.findAll('.state-schema-drawer__field-row')).toHaveLength(3);

    await wrapper.find('.state-schema-drawer__search').setValue('order');
    expect(wrapper.findAll('.state-schema-drawer__field-row')).toHaveLength(1);

    await wrapper.find('.state-schema-drawer__search').setValue('3');
    const rows = wrapper.findAll('.state-schema-drawer__field-row');
    expect(rows).toHaveLength(1);
    expect(rows[0].attributes('data-field')).toBe('retry_count');
  });

  it('R2-G.4 类型 chips 带计数，多选过滤', async () => {
    const graph = makeGraph([field('a', 'string'), field('b', 'integer'), field('c', 'integer')]);
    const wrapper = mountDrawer(graph);
    const chips = wrapper.findAll('.state-schema-drawer__type-chip');
    expect(chips.map((c) => c.attributes('data-type')).sort()).toEqual(['integer', 'string']);
    expect(chips.find((c) => c.attributes('data-type') === 'integer')!.text()).toContain('2');

    await chips.find((c) => c.attributes('data-type') === 'integer')!.trigger('click');
    expect(wrapper.findAll('.state-schema-drawer__field-row')).toHaveLength(2);
  });

  it('R2-G.6 虚拟滚动：512 字段仅渲染窗口行，占位高度完整', () => {
    const fields = Array.from({ length: 512 }, (_, i) => field(`f_${String(i).padStart(3, '0')}`));
    const wrapper = mountDrawer(makeGraph(fields));
    const rows = wrapper.findAll('.state-schema-drawer__field-row');
    expect(rows.length).toBeLessThan(30);
    expect(wrapper.find('.state-schema-drawer__vlist-spacer').attributes('style')).toContain(`${512 * 36}px`);
  });

  it('R2-G.7 未使用警告 + 未使用行置灰 + 被引用行复选框禁用', () => {
    // n1 模板读取 user_name；last_response 被 n1 写入
    const graph = makeGraph(
      [field('user_name'), field('last_response'), field('orphan_field')],
      [llmNode('n1', '你好 ${user_name}')],
    );
    const wrapper = mountDrawer(graph);

    const warning = wrapper.find('[data-testid="unused-warning"]');
    expect(warning.exists()).toBe(true);
    expect(warning.text()).toContain('1');

    const orphan = wrapper.find('[data-field="orphan_field"]');
    expect(orphan.classes()).toContain('is-unused');
    // 未被引用 → 可勾选删除（复选框可用）
    expect(orphan.find('input[type="checkbox"]').attributes('disabled')).toBeUndefined();

    const used = wrapper.find('[data-field="user_name"]');
    expect(used.classes()).not.toContain('is-unused');
    // 被引用 → 禁止删除（复选框禁用）
    expect(used.find('input[type="checkbox"]').attributes('disabled')).toBeDefined();

    // 被写入字段也不算未使用
    expect(wrapper.find('[data-field="last_response"]').classes()).not.toContain('is-unused');
  });

  it('R2-G.3 前缀分组视图：多字段前缀成组、单字段归其他、折叠生效', async () => {
    const graph = makeGraph([
      field('order_id'),
      field('order_total'),
      field('order_status'),
      field('user_name'),
      field('standalone'),
    ]);
    const wrapper = mountDrawer(graph);
    await wrapper.find('[data-view="prefix"]').trigger('click');

    let groups = wrapper.findAll('.state-schema-drawer__group-row');
    expect(groups.map((g) => g.attributes('data-group'))).toEqual(['order', '__other__']);
    // 总行 = 2 组头 + 5 字段（jsdom 视口高 0 → 仅渲染 buffer 窗口）
    expect(wrapper.find('.state-schema-drawer__vlist-spacer').attributes('style')).toContain(`${7 * 36}px`);
    expect(wrapper.findAll('.state-schema-drawer__field-row').length).toBeLessThanOrEqual(5);

    // 折叠 order 组 → 总行 = 2 组头 + other 组 2 字段
    await wrapper.find('[data-group="order"]').trigger('click');
    expect(wrapper.find('.state-schema-drawer__vlist-spacer').attributes('style')).toContain(`${4 * 36}px`);
    expect(wrapper.findAll('.state-schema-drawer__field-row')).toHaveLength(2);
    groups = wrapper.findAll('.state-schema-drawer__group-row');
    expect(groups).toHaveLength(2);
  });

  it('R2-G.3 读写关系视图：有写入 / 仅被读取 / 未使用 三组', async () => {
    const graph = makeGraph(
      [field('user_name'), field('last_response'), field('orphan_field')],
      [llmNode('n1', '读取 ${user_name}')],
    );
    const wrapper = mountDrawer(graph);
    await wrapper.find('[data-view="usage"]').trigger('click');

    const groups = wrapper.findAll('.state-schema-drawer__group-row');
    expect(groups.map((g) => g.attributes('data-group'))).toEqual(['written', 'readonly', 'unused']);
    expect(groups[0].text()).toContain('1'); // last_response
    expect(groups[1].text()).toContain('1'); // user_name
    expect(groups[2].text()).toContain('1'); // orphan_field
  });

  it('R2-G.8 内联编辑：点击行打开编辑区，改 reducer / 默认值写回并 emit change', async () => {
    const graph = makeGraph([field('order_id')]);
    const wrapper = mountDrawer(graph);

    await wrapper.find('[data-field="order_id"]').trigger('click');
    const editor = wrapper.find('[data-testid="field-editor"]');
    expect(editor.exists()).toBe(true);
    expect(editor.text()).toContain('order_id');

    // reducer 下拉切换
    await editor.find('.q-select').setValue('cover');
    expect(graph.stateFields[0].reducer).toBe('cover');
    expect(wrapper.emitted('change')).toBeTruthy();

    // 默认值 JSON 解析（reducer 变更触发编辑区重渲染，缓存的 editor 引用已失效，需经 wrapper 重新查找）
    await wrapper.find('[data-field-input="defaultValue"]').setValue('{"a":1}');
    expect(graph.stateFields[0].defaultValue).toEqual({ a: 1 });

    // 非法 JSON 回退为字符串
    await wrapper.find('[data-field-input="defaultValue"]').setValue('raw text');
    expect(graph.stateFields[0].defaultValue).toBe('raw text');

    // 关闭编辑区
    await wrapper.find('[data-action="close-editor"]').trigger('click');
    expect(wrapper.find('[data-testid="field-editor"]').exists()).toBe(false);
  });

  it('R2-G.9 批量操作：勾选未使用字段 → 批量条计数 → 删除确认后移除', async () => {
    const graph = makeGraph(
      [field('user_name'), field('junk_1'), field('junk_2')],
      [llmNode('n1', '${user_name}')],
    );
    const wrapper = mountDrawer(graph);
    expect(wrapper.find('[data-testid="batch-bar"]').exists()).toBe(false);

    // 勾选两个未使用字段
    await wrapper.find('[data-select="junk_1"]').setValue(true);
    await wrapper.find('[data-select="junk_2"]').setValue(true);
    const bar = wrapper.find('[data-testid="batch-bar"]');
    expect(bar.exists()).toBe(true);
    expect(bar.text()).toContain('2');

    // 删除（dialog onOk 立即执行）
    await wrapper.find('[data-action="delete-selected"]').trigger('click');
    expect(graph.stateFields.map((f) => f.name)).toEqual(['user_name']);
    expect(wrapper.find('[data-testid="batch-bar"]').exists()).toBe(false);
    expect(wrapper.emitted('change')).toBeTruthy();
  });

  it('R2-G.9 导出选中字段为 JSON 下载', async () => {
    const graph = makeGraph([field('junk_1'), field('junk_2')]);
    const wrapper = mountDrawer(graph);

    const createObjectURL = vi.fn(() => 'blob:mock');
    const revokeObjectURL = vi.fn();
    vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL });
    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});

    await wrapper.find('[data-select="junk_1"]').setValue(true);
    await wrapper.find('[data-action="export-selected"]').trigger('click');

    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(clickSpy).toHaveBeenCalledOnce();

    clickSpy.mockRestore();
    vi.unstubAllGlobals();
  });

  it('open=false 不渲染', () => {
    const wrapper = mountDrawer(makeGraph([field('a')]), false);
    expect(wrapper.find('.state-schema-drawer__panel').exists()).toBe(false);
  });
});
