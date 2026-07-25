// web/src/components/graph/__tests__/GraphDetailPanel.spec.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import GraphDetailPanel from '../GraphDetailPanel.vue';
import type { GraphDefinition, NodeDef, GraphExecutionSummary } from '../../../features/graph/types';

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${JSON.stringify(params)}` : key,
  }),
}));

const globalStubs = {
  'q-btn': {
    template: '<button class="q-btn" @click="$emit(\'click\', $event)"><slot /></button>',
    props: ['loading', 'icon', 'label', 'disable'],
    emits: ['click'],
  },
  'q-icon': { template: '<i class="q-icon" />', props: ['name'] },
  'q-tooltip': { template: '<span class="q-tooltip"><slot /></span>' },
  'q-input': {
    template:
      '<input class="q-input" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
    props: ['modelValue', 'placeholder'],
    emits: ['update:modelValue'],
  },
};

function node(id: string, type: NodeDef['type'] = 'llm'): NodeDef {
  return {
    id,
    funcRef: '',
    interruptBefore: false,
    interruptAfter: false,
    type,
    description: '',
    instruction: '',
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

function makeGraph(overrides: Partial<GraphDefinition> = {}): GraphDefinition {
  return {
    id: 'g1',
    name: '订单流水线',
    description: '测试图',
    stateFields: [],
    nodes: [node('fetch_order', 'function'), node('summarize', 'llm'), node('review', 'hitl')],
    edges: [{ from: 'fetch_order', to: 'summarize', kind: 'normal' }],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: 'fetch_order',
    finishPoint: 'review',
    enableCheckpoint: true,
    executionEngine: 'bsp',
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 3,
    sortOrder: 0,
    createdAt: '2026-07-20T08:00:00Z',
    updatedAt: '2026-07-24T08:00:00Z',
    ...overrides,
  };
}

function makeExec(id: string, status: string): GraphExecutionSummary {
  return {
    executionId: id,
    graphId: 'g1',
    sessionId: 's1',
    status,
    currentNode: '',
    lineageId: 'l1',
    errorMessage: '',
    startedAt: '2026-07-24T08:00:00Z',
    finishedAt: '2026-07-24T08:01:00Z',
  };
}

function mountPanel(graph: GraphDefinition | null, extra: Record<string, unknown> = {}) {
  return mount(GraphDetailPanel, {
    props: {
      graph,
      isDark: true,
      nodeCounts: graph ? Object.fromEntries(graph.nodes.map((n) => [n.type, 1])) : {},
      nodeTypeBorderColor: () => '#fff',
      ...extra,
    },
    global: { stubs: globalStubs },
  });
}

beforeEach(() => {
  localStorage.clear();
});

describe('GraphDetailPanel - R2-6 重设计', () => {
  it('R2-B.1 操作条：编辑/执行/复制/导出/删除 5 个紧凑操作', async () => {
    const graph = makeGraph();
    const wrapper = mountPanel(graph);
    const bar = wrapper.find('.graph-detail-panel__actionbar');
    expect(bar.exists()).toBe(true);

    await bar.find('[data-action="edit"]').trigger('click');
    expect(wrapper.emitted('edit')).toEqual([['g1']]);

    await bar.find('[data-action="run"]').trigger('click');
    expect(wrapper.emitted('run')).toEqual([[graph]]);

    await bar.find('[data-action="duplicate"]').trigger('click');
    expect(wrapper.emitted('duplicate')).toEqual([[graph]]);

    await bar.find('[data-action="export"]').trigger('click');
    expect(wrapper.emitted('export')).toEqual([[graph]]);

    await bar.find('[data-action="delete"]').trigger('click');
    expect(wrapper.emitted('delete')).toEqual([[graph]]);
  });

  it('R2-B.2 统计行：节点/连线/状态字段/执行次数 4 格', () => {
    const graph = makeGraph({ stateFields: [{ name: 'f1', type: 'string', reducer: 'append', required: false, disableDeepCopy: false }] });
    const wrapper = mountPanel(graph, {
      executions: [makeExec('e1', 'completed'), makeExec('e2', 'failed')],
      executionsHasMore: true,
    });
    const stats = wrapper.find('.graph-detail-panel__stats-row');
    expect(stats.exists()).toBe(true);
    const cells = stats.findAll('.graph-detail-panel__stat-cell');
    expect(cells.length).toBe(4);
    const text = stats.text();
    expect(text).toContain('3'); // 节点
    expect(text).toContain('1'); // 连线
    expect(text).toContain('2+'); // 执行次数（hasMore）
  });

  it('R2-B.3/B.5 节点搜索 + 虚拟滚动：512 节点仅渲染窗口行', async () => {
    const manyNodes = Array.from({ length: 512 }, (_, i) => node(`node_${String(i).padStart(3, '0')}`, 'llm'));
    manyNodes.push(node('special_hitl_node', 'hitl'));
    const graph = makeGraph({ nodes: manyNodes });
    const wrapper = mountPanel(graph, {
      nodeCounts: { llm: 512, hitl: 1 },
    });
    // 默认折叠，先展开 nodes section
    await wrapper.find('[data-section-head="nodes"]').trigger('click');

    // 虚拟滚动：jsdom 视口高 0 → 仅渲染 buffer 行，远少于 513
    const rows = wrapper.findAll('.graph-detail-panel__node-row');
    expect(rows.length).toBeLessThan(30);
    // 占位高度 = 全部行高
    const spacer = wrapper.find('.graph-detail-panel__vlist-spacer');
    expect(spacer.attributes('style')).toContain(`${513 * 32}px`);

    // 搜索过滤（q-input stub 根元素即 input）
    await wrapper.find('.graph-detail-panel__node-search').setValue('special_hitl');
    const filtered = wrapper.findAll('.graph-detail-panel__node-row');
    expect(filtered.length).toBe(1);
    expect(filtered[0].text()).toContain('special_hitl_node');
  });

  it('R2-B.4 类型 chips 多选过滤', async () => {
    const graph = makeGraph();
    const wrapper = mountPanel(graph, {
      nodeCounts: { function: 1, llm: 1, hitl: 1 },
    });
    await wrapper.find('[data-section-head="nodes"]').trigger('click');
    // 默认全选/无过滤 → 3 行
    expect(wrapper.findAll('.graph-detail-panel__node-row').length).toBe(3);

    // 只选 llm
    const chips = wrapper.findAll('.graph-detail-panel__type-chip');
    const llmChip = chips.find((c) => c.attributes('data-type') === 'llm');
    expect(llmChip).toBeDefined();
    await llmChip!.trigger('click');
    const rows = wrapper.findAll('.graph-detail-panel__node-row');
    expect(rows.length).toBe(1);
    expect(rows[0].text()).toContain('summarize');
  });

  it('R2-B.7 行内定位：点击定位按钮 emit locateNode', async () => {
    const graph = makeGraph();
    const wrapper = mountPanel(graph);
    await wrapper.find('[data-section-head="nodes"]').trigger('click');
    const locateBtn = wrapper.find('.graph-detail-panel__node-row [data-action="locate"]');
    expect(locateBtn.exists()).toBe(true);
    await locateBtn.trigger('click');
    expect(wrapper.emitted('locateNode')).toEqual([['fetch_order']]);
  });

  it('R2-B.6 三段默认折叠 + 展开态持久化', async () => {
    const graph = makeGraph({
      stateFields: [{ name: 'f1', type: 'string', reducer: 'append', required: false, disableDeepCopy: false }],
    });
    const wrapper = mountPanel(graph, { executions: [makeExec('e1', 'completed')] });

    // 默认全部折叠
    expect(wrapper.find('.graph-detail-panel__node-search').exists()).toBe(false);
    expect(wrapper.findAll('.graph-detail-panel__field-row').length).toBe(0);
    expect(wrapper.findAll('.graph-detail-panel__exec-row').length).toBe(0);

    // 展开 nodes section
    await wrapper.find('[data-section-head="nodes"]').trigger('click');
    expect(wrapper.find('.graph-detail-panel__node-search').exists()).toBe(true);
    expect(localStorage.getItem('graph.detail.section.nodes')).toBe('open');

    // 重挂载后保持展开
    const wrapper2 = mountPanel(graph);
    expect(wrapper2.find('.graph-detail-panel__node-search').exists()).toBe(true);
  });

  it('状态字段 section：超过 20 条截断 + 管理全部入口', async () => {
    const fields = Array.from({ length: 42 }, (_, i) => ({
      name: `field_${i}`,
      type: 'string',
      reducer: 'append' as const,
      required: false,
      disableDeepCopy: false,
    }));
    const graph = makeGraph({ stateFields: fields });
    const wrapper = mountPanel(graph);
    await wrapper.find('[data-section-head="fields"]').trigger('click');

    const rows = wrapper.findAll('.graph-detail-panel__field-row');
    expect(rows.length).toBe(20);
    // 枚举值走 i18n 映射（mock t 返回 key 本身）
    expect(rows[0].text()).toContain('graphs.stateTypeString');
    expect(rows[0].text()).toContain('graphs.reducerAppend');
    expect(rows[0].text()).not.toContain('append"');

    const manageBtn = wrapper.find('[data-action="manage-schema"]');
    expect(manageBtn.exists()).toBe(true);
    expect(manageBtn.text()).toContain('42');
    await manageBtn.trigger('click');
    expect(wrapper.emitted('manageSchema')).toBeTruthy();
  });

  it('执行历史 section：最近 5 条 + 全部入口', async () => {
    const execs = Array.from({ length: 8 }, (_, i) => makeExec(`e${i}`, 'completed'));
    const graph = makeGraph();
    const wrapper = mountPanel(graph, { executions: execs });
    await wrapper.find('[data-section-head="runs"]').trigger('click');

    const rows = wrapper.findAll('.graph-detail-panel__exec-row');
    expect(rows.length).toBe(5);
    // 执行状态走 EXECUTION_STATUS_STYLES i18n 映射
    expect(rows[0].text()).toContain('graphs.executionStatusCompleted');

    const allBtn = wrapper.find('[data-action="view-executions"]');
    expect(allBtn.exists()).toBe(true);
    await allBtn.trigger('click');
    expect(wrapper.emitted('viewExecutions')).toBeTruthy();
  });

  it('空态：无 graph 时显示提示', () => {
    const wrapper = mountPanel(null);
    expect(wrapper.find('.graph-detail-panel__empty').exists()).toBe(true);
    expect(wrapper.find('.graph-detail-panel__actionbar').exists()).toBe(false);
  });

  it('详情面板用到的 graphs.detail* i18n key 在双语中存在（防 key 缺失回归）', async () => {
    const zh = (await import('../../../i18n/locales/zh-CN')).default as Record<string, Record<string, string>>;
    const en = (await import('../../../i18n/locales/en-US')).default as Record<string, Record<string, string>>;
    const usedKeys = [
      'detailActionEdit',
      'detailActionRun',
      'detailActionDuplicate',
      'detailActionExport',
      'detailActionDelete',
      'detailStatNodes',
      'detailStatEdges',
      'detailStatFields',
      'detailStatRuns',
      'detailLabelEngine',
      'detailLabelCheckpoint',
      'detailLabelEntry',
      'detailLabelFinish',
      'detailLabelUpdatedAt',
      'detailCheckpointEnabled',
      'detailCheckpointDisabled',
      'detailSectionNodes',
      'detailSectionStateFields',
      'detailSectionRuns',
      'detailNodeSearchPlaceholder',
      'detailNodeLocate',
      'detailNodesEmpty',
      'detailManageAllFields',
      'detailViewAllRuns',
      'detailRunsEmpty',
      'detailEmptyHint',
      'detailEmptySubHint',
    ];
    for (const key of usedKeys) {
      expect(zh.graphs[key], `zh-CN 缺失 graphs.${key}`).toBeTruthy();
      expect(en.graphs[key], `en-US 缺失 graphs.${key}`).toBeTruthy();
    }
  });
});
