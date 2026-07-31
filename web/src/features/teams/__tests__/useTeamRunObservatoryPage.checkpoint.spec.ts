// useTeamRunObservatoryPage — M53 Phase 11 F6：Checkpoint tab 显隐 + API 接线 + Graph 执行视角跳转。
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { setActivePinia, createPinia } from 'pinia';
import { mount } from '@vue/test-utils';
import { defineComponent } from 'vue';
import { createI18n } from 'vue-i18n';
import { useTeamRunObservatoryPage } from '../useTeamRunObservatoryPage';

// ── 外部依赖 mock ──
const routerPush = vi.hoisted(() => vi.fn());
const notify = vi.hoisted(() => vi.fn());
vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { teamId: 'team-1', runId: 'run-1' } }),
  useRouter: () => ({ push: routerPush }),
}));
vi.mock('quasar', () => ({
  useQuasar: () => ({ dark: { isActive: false }, notify }),
}));

const graphStoreMock = vi.hoisted(() => ({
  fetchCheckpoints: vi.fn(async () => [] as unknown[]),
  fetchStateSnapshot: vi.fn(async () => ({})),
  editStateSnapshot: vi.fn(),
  timeTravelExecution: vi.fn(),
  fetchExecution: vi.fn(),
  resumeExecution: vi.fn(async () => ({ executionId: 'exec-1', status: 'running' })),
}));
vi.mock('../../../stores/graph', () => ({ useGraphStore: () => graphStoreMock }));

const orchestrationStoreMock = vi.hoisted(() => ({
  fetchRunObservatory: vi.fn(),
}));
vi.mock('../../../stores/orchestration', () => ({ useOrchestrationStore: () => orchestrationStoreMock }));

vi.mock('../../orchestration/useOrchestrationStream', () => ({
  useOrchestrationStream: () => ({
    nodes: new Map(),
    connected: { value: false },
    seed: vi.fn(),
    disconnect: vi.fn(),
  }),
}));
vi.mock('../../graph/runtime/useGraphExecutionStream', () => ({
  useGraphExecutionStream: () => ({
    taskList: { value: [] },
    streamConnected: { value: false },
    seedTasks: vi.fn(),
    upsertTask: vi.fn(),
    disconnect: vi.fn(),
  }),
}));
vi.mock('../../graph/useGraphRunTasks', () => ({
  useGraphRunTasks: () => ({
    loadTasks: vi.fn(async () => {}),
    focusTaskForNode: vi.fn(),
    openTaskDetail: vi.fn(),
    selectedTaskId: { value: null },
    tasksLoading: { value: false },
    onKanbanAdminAction: vi.fn(),
  }),
}));
vi.mock('../../orchestration/api', () => ({
  getTeamRunObservatoryTimeline: vi.fn(async () => ({ rows: [] })),
}));
vi.mock('../api', () => ({
  getTeamRunSummary: vi.fn(async () => null),
  resumeTeamRunExecution: vi.fn(async () => ({})),
}));

function mkObservatory(partial: Record<string, unknown> = {}) {
  return {
    run_id: 'run-1',
    team_id: 'team-1',
    session_id: 'sess-1',
    status: 'running',
    mode: 'sequential',
    graph_execution_id: 'exec-1',
    definition_snapshot_json: '{"enable_checkpoint": true}',
    nodes: [],
    ...partial,
  };
}

const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': {} } });

function harness() {
  let api: ReturnType<typeof useTeamRunObservatoryPage> | undefined;
  const Comp = defineComponent({
    setup() {
      api = useTeamRunObservatoryPage();
      return () => null;
    },
  });
  mount(Comp, { global: { plugins: [i18n] } });
  return api!;
}

describe('useTeamRunObservatoryPage F6 Checkpoint tab (M53 Phase 11)', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    routerPush.mockReset();
    notify.mockReset();
    graphStoreMock.fetchCheckpoints.mockClear();
    graphStoreMock.fetchExecution.mockReset();
    graphStoreMock.resumeExecution.mockClear();
    orchestrationStoreMock.fetchRunObservatory.mockReset();
  });

  it('enable_checkpoint 且有 graph_execution_id：tab 启用并预取检查点', async () => {
    orchestrationStoreMock.fetchRunObservatory.mockResolvedValue(mkObservatory());
    const api = harness();
    await vi.waitFor(() => expect(orchestrationStoreMock.fetchRunObservatory).toHaveBeenCalled());
    await vi.waitFor(() => expect(graphStoreMock.fetchCheckpoints).toHaveBeenCalledWith('exec-1'));
    expect(api.checkpointTabEnabled.value).toBe(true);
  });

  it('enable_checkpoint=false：tab 禁用且不预取检查点', async () => {
    orchestrationStoreMock.fetchRunObservatory.mockResolvedValue(
      mkObservatory({ definition_snapshot_json: '{"enable_checkpoint": false}' }),
    );
    const api = harness();
    await vi.waitFor(() => expect(orchestrationStoreMock.fetchRunObservatory).toHaveBeenCalled());
    await vi.waitFor(() => expect(api.loading.value).toBe(false));
    expect(api.checkpointTabEnabled.value).toBe(false);
    expect(graphStoreMock.fetchCheckpoints).not.toHaveBeenCalled();
  });

  it('无 graph_execution_id：tab 禁用', async () => {
    orchestrationStoreMock.fetchRunObservatory.mockResolvedValue(mkObservatory({ graph_execution_id: '' }));
    const api = harness();
    await vi.waitFor(() => expect(api.loading.value).toBe(false));
    expect(api.checkpointTabEnabled.value).toBe(false);
    expect(graphStoreMock.fetchCheckpoints).not.toHaveBeenCalled();
  });

  it('openGraphRunView：经执行记录反查 graphId 后跳 graph-run 路由', async () => {
    orchestrationStoreMock.fetchRunObservatory.mockResolvedValue(mkObservatory());
    graphStoreMock.fetchExecution.mockResolvedValue({ executionId: 'exec-1', graphId: 'g-asset-1' });
    const api = harness();
    await vi.waitFor(() => expect(api.loading.value).toBe(false));

    await api.openGraphRunView();
    expect(graphStoreMock.fetchExecution).toHaveBeenCalledWith('exec-1');
    expect(routerPush).toHaveBeenCalledWith({ name: 'graph-run', params: { id: 'g-asset-1', execId: 'exec-1' } });
  });

  it('openGraphRunView：执行记录无 graphId 时 warning 提示且不跳转', async () => {
    orchestrationStoreMock.fetchRunObservatory.mockResolvedValue(mkObservatory());
    graphStoreMock.fetchExecution.mockResolvedValue({ executionId: 'exec-1', graphId: '' });
    const api = harness();
    await vi.waitFor(() => expect(api.loading.value).toBe(false));

    await api.openGraphRunView();
    expect(routerPush).not.toHaveBeenCalled();
    expect(notify).toHaveBeenCalledWith(expect.objectContaining({ type: 'warning' }));
  });

  it('onGraphResumeExecution：调用 resumeExecution 并重新加载观测数据', async () => {
    orchestrationStoreMock.fetchRunObservatory.mockResolvedValue(mkObservatory());
    const api = harness();
    await vi.waitFor(() => expect(api.loading.value).toBe(false));
    const callsBefore = orchestrationStoreMock.fetchRunObservatory.mock.calls.length;

    await api.onGraphResumeExecution();
    expect(graphStoreMock.resumeExecution).toHaveBeenCalledWith('exec-1');
    expect(orchestrationStoreMock.fetchRunObservatory.mock.calls.length).toBeGreaterThan(callsBefore);
  });
});
