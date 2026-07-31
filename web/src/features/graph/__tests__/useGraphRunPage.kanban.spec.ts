// useGraphRunPage — M53 Phase 11 F7：team 执行 Kanban 视角 + 悬空 graph_id 友好降级。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { defineComponent, ref } from 'vue';
import { useGraphRunPage } from '../useGraphRunPage';
import type { GraphDefinition, GraphExecution } from '../types';

// ── 外部依赖 mock ──
const routerPush = vi.hoisted(() => vi.fn());
const notify = vi.hoisted(() => vi.fn());
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'g-1', execId: 'exec-1' } }),
  useRouter: () => ({ push: routerPush }),
}));
vi.mock('quasar', () => ({
  useQuasar: () => ({ dark: { isActive: false }, notify }),
}));

const graphStoreMock = vi.hoisted(() => ({
  fetchGraph: vi.fn(),
  fetchExecution: vi.fn(),
  cancelExecution: vi.fn(),
}));
vi.mock('../../../stores/graph', () => ({ useGraphStore: () => graphStoreMock }));

// hoisted 阶段无法访问 vue 的 ref，使用带 .value 的 plain 对象（composable 仅读 .value）
const streamMock = vi.hoisted(() => ({
  liveStatus: { value: 'completed' },
  execNodeStates: { value: new Map() },
  executionSummary: { value: null },
  streamConnected: { value: false },
  interrupt: { value: null },
  taskList: { value: [] },
  liveSteps: { value: [] as unknown[] },
  connectStream: vi.fn(),
  disconnectStream: vi.fn(),
  seedTasks: vi.fn(),
  upsertTask: vi.fn(),
  clearInterrupt: vi.fn(),
}));
vi.mock('../runtime/useGraphRunStream', () => ({ useGraphRunStream: () => streamMock }));

vi.mock('../runtime/useGraphTimeTravel', () => ({
  useGraphTimeTravel: () => ({
    checkpoints: ref([]),
    checkpointsLoading: ref(false),
    selectedCheckpoint: ref(null),
    stateSnapshot: ref(null),
    statePatchJson: ref(''),
    snapshotLoading: ref(false),
    editLoading: ref(false),
    timeTravelLoading: ref(false),
    stepIndexInput: ref(0),
    loadCheckpoints: vi.fn(async () => {}),
    selectCheckpoint: vi.fn(),
    applyEditState: vi.fn(),
    travelToStep: vi.fn(),
  }),
}));

vi.mock('../useGraphRunTasks', () => ({
  useGraphRunTasks: () => ({
    tasksLoading: ref(false),
    selectedTaskId: ref(null),
    taskDrawerOpen: ref(false),
    activeTask: ref(null),
    taskComments: ref([]),
    taskLogs: ref([]),
    taskRuns: ref([]),
    taskEvents: ref([]),
    taskDetailLoading: ref(false),
    taskActionLoading: ref(false),
    loadTasks: vi.fn(async () => {}),
    openTaskDetail: vi.fn(),
    focusTaskForNode: vi.fn(),
    onClaimTask: vi.fn(),
    onSubmitTask: vi.fn(),
    onReportBlocked: vi.fn(),
    onUnblockTask: vi.fn(),
    onReviewTask: vi.fn(),
    onAddTaskComment: vi.fn(),
    onKanbanAdminAction: vi.fn(),
  }),
}));

vi.mock('../useGraphRunHitl', () => ({
  useGraphRunHitl: () => ({
    hitlDialogOpen: ref(false),
    hitlAdvancedJson: ref(''),
    resumeLoading: ref(false),
    resumeExec: vi.fn(),
    submitHitlResume: vi.fn(),
  }),
}));

function mkGraph(partial: Partial<GraphDefinition> = {}): GraphDefinition {
  return {
    id: 'g-1',
    name: 'Team 图',
    description: '',
    stateFields: [],
    nodes: [],
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
    ...partial,
  } as GraphDefinition;
}

function mkExecution(partial: Partial<GraphExecution> = {}): GraphExecution {
  return {
    executionId: 'exec-1',
    graphId: 'g-1',
    sessionId: 'sess-1',
    status: 'completed',
    currentState: {},
    steps: [
      {
        nodeId: 'agent_a',
        stepIndex: 0,
        inputState: { q: 'hi' },
        outputState: { r: 'ok' },
        status: 'completed',
        error: '',
        timestamp: '',
      },
    ],
    interruptNode: '',
    startedAt: '',
    finishedAt: '',
    ...partial,
  };
}

function harness() {
  let api: ReturnType<typeof useGraphRunPage> | undefined;
  const Comp = defineComponent({
    setup() {
      api = useGraphRunPage();
      return () => null;
    },
  });
  mount(Comp);
  return api!;
}

describe('useGraphRunPage F7 Kanban 视角 (M53 Phase 11)', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    routerPush.mockReset();
    notify.mockReset();
    graphStoreMock.fetchGraph.mockReset();
    graphStoreMock.fetchExecution.mockReset();
    streamMock.execNodeStates.value = new Map();
    streamMock.liveSteps.value = [];
  });

  it('team_id 非空 → showKanbanTab=true；独立图 → false', async () => {
    graphStoreMock.fetchGraph.mockResolvedValue(mkGraph({ teamId: 'team-1' }));
    graphStoreMock.fetchExecution.mockResolvedValue(mkExecution());
    const api = harness();
    await vi.waitFor(() => expect(graphStoreMock.fetchExecution).toHaveBeenCalled());
    await vi.waitFor(() => expect(api.graphDef.teamId).toBe('team-1'));
    expect(api.showKanbanTab.value).toBe(true);
  });

  it('team_id 为空 → showKanbanTab=false', async () => {
    graphStoreMock.fetchGraph.mockResolvedValue(mkGraph({ teamId: '' }));
    graphStoreMock.fetchExecution.mockResolvedValue(mkExecution());
    const api = harness();
    await vi.waitFor(() => expect(graphStoreMock.fetchExecution).toHaveBeenCalled());
    await vi.waitFor(() => expect(api.execution.value).not.toBeNull());
    expect(api.showKanbanTab.value).toBe(false);
  });

  it('kanbanNodes 从执行 steps + 图定义节点派生', async () => {
    graphStoreMock.fetchGraph.mockResolvedValue(
      mkGraph({
        teamId: 'team-1',
        nodes: [{ id: 'agent_a', agentName: '阿尔法' } as GraphDefinition['nodes'][number]],
      }),
    );
    graphStoreMock.fetchExecution.mockResolvedValue(mkExecution());
    const api = harness();
    await vi.waitFor(() => expect(api.kanbanNodes.value.length).toBe(1));
    const node = api.kanbanNodes.value[0];
    expect(node).toMatchObject({
      node_id: 'agent_a',
      agent_name: '阿尔法',
      status: 'success',
      display_status: 'success',
    });
    expect(node.input_preview).toContain('hi');
    expect(node.output_preview).toContain('ok');
  });

  it('fetchGraph 404 → graphAssetMissing=true、合成只读节点、无 negative 通知、Kanban 可用', async () => {
    graphStoreMock.fetchGraph.mockRejectedValue({ response: { status: 404 } });
    graphStoreMock.fetchExecution.mockResolvedValue(mkExecution());
    const api = harness();
    await vi.waitFor(() => expect(api.graphAssetMissing.value).toBe(true));
    await vi.waitFor(() => expect(api.graphDef.nodes.length).toBe(1));
    expect(api.graphDef.nodes[0].id).toBe('agent_a');
    expect(notify).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'negative' }));
    expect(api.showKanbanTab.value).toBe(true);
    expect(api.kanbanNodes.value.map((n) => n.node_id)).toEqual(['agent_a']);
  });

  it('fetchGraph 非 404 错误 → negative 通知且不进入降级', async () => {
    graphStoreMock.fetchGraph.mockRejectedValue(new Error('network down'));
    graphStoreMock.fetchExecution.mockResolvedValue(mkExecution());
    const api = harness();
    await vi.waitFor(() =>
      expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'negative', message: '加载 Graph 失败' })),
    );
    expect(api.graphAssetMissing.value).toBe(false);
    expect(api.showKanbanTab.value).toBe(false);
  });
});
