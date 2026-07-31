import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useRoute, useRouter } from 'vue-router';
import type { CheckpointInfo, GraphDefinition, GraphExecution } from './types';
import { useGraphStore } from '../../stores/graph';
import { useGraphTimeTravel } from './runtime/useGraphTimeTravel';
import { useGraphRunStream } from './runtime/useGraphRunStream';
import { buildGraphRunKanbanNodes, synthesizeGraphNodesFromSteps } from './runtime/graphExecutionProjection';
import { useGraphRunTasks } from './useGraphRunTasks';
import { useGraphRunHitl } from './useGraphRunHitl';

/** Kratos NotFound 经 axios 暴露为 HTTP 404（拦截器对 404 不弹全局 toast）。 */
function isNotFoundError(err: unknown): boolean {
  return (err as { response?: { status?: number } } | null)?.response?.status === 404;
}

export function useGraphRunPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const graphStore = useGraphStore();
  let loadSeq = 0;

  const isDark = computed(() => $q.dark.isActive);
  const graphId = computed(() => (route.params.id as string) ?? '');
  const execId = computed(() => (route.params.execId as string) ?? '');

  const graphDef = reactive<GraphDefinition>({
    id: '',
    name: '',
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
    version: 0,
    sortOrder: 0,
    createdAt: '',
    updatedAt: '',
  });

  const execution = ref<GraphExecution | null>(null);
  const selectedNodeId = ref<string | null>(null);
  // ── M53 Phase 11 F7：悬空 graph_id（资产已删除）降级标记 ──
  const graphAssetMissing = ref(false);

  const stream = useGraphRunStream(graphId, execId, execution);
  const timeTravel = useGraphTimeTravel(execId);

  const tasks = useGraphRunTasks(() => execId.value, stream.seedTasks, stream.upsertTask);

  const displayStatus = computed(() => stream.liveStatus.value);

  const statusColor = computed(() => {
    const s = displayStatus.value;
    if (s === 'completed') return 'positive';
    if (s === 'running') return 'blue';
    if (s === 'failed') return 'negative';
    if (s === 'waiting_human') return 'warning';
    if (s === 'cancelled') return 'grey';
    return 'grey';
  });

  async function refreshExecution() {
    if (!execId.value) return;
    execution.value = await graphStore.fetchExecution(execId.value);
    stream.connectStream(execution.value?.steps ?? []);
    await tasks.loadTasks(execId.value);
  }

  const inspectorTab = ref('overview');

  const hitl = useGraphRunHitl(execId, stream.interrupt, displayStatus, stream.clearInterrupt, refreshExecution);

  async function loadPageData() {
    const seq = ++loadSeq;
    graphAssetMissing.value = false;
    if (graphId.value) {
      try {
        const graph = await graphStore.fetchGraph(graphId.value);
        if (seq !== loadSeq) return;
        Object.assign(graphDef, graph);
      } catch (err) {
        if (seq !== loadSeq) return;
        // F7：资产已删除（换绑 external 级联删除）→ 友好降级，不弹错误
        if (isNotFoundError(err)) {
          graphAssetMissing.value = true;
        } else {
          $q.notify({ type: 'negative', message: '加载 Graph 失败' });
        }
      }
    }
    if (execId.value) {
      try {
        const exec = await graphStore.fetchExecution(execId.value);
        if (seq !== loadSeq) return;
        execution.value = exec;
        stream.connectStream(exec.steps);
        await Promise.all([tasks.loadTasks(execId.value), timeTravel.loadCheckpoints()]);
      } catch {
        if (seq !== loadSeq) return;
        $q.notify({ type: 'negative', message: '加载执行记录失败' });
      }
    }
    // F7：降级渲染——从执行 steps 合成只读拓扑
    if (graphAssetMissing.value) {
      graphDef.id = graphId.value;
      graphDef.name = '';
      graphDef.nodes = synthesizeGraphNodesFromSteps(execution.value?.steps ?? []);
      graphDef.edges = [];
      graphDef.conditionalEdges = [];
    }
  }

  watch(
    () => stream.liveSteps.value,
    (steps) => {
      if (!execution.value || steps.length === 0) return;
      execution.value = { ...execution.value, steps: [...steps] };
    },
    { deep: true },
  );

  onMounted(loadPageData);
  watch([graphId, execId], loadPageData);
  onBeforeUnmount(() => stream.disconnectStream());

  function onSelectNode(nodeId: string | null) {
    selectedNodeId.value = nodeId;
    tasks.focusTaskForNode(stream.taskList.value, nodeId);
    if (nodeId && stream.taskList.value.some((task) => task.nodeId === nodeId)) {
      inspectorTab.value = 'tasks';
    }
  }

  function onSelectTask(taskId: string) {
    inspectorTab.value = 'tasks';
    void tasks.openTaskDetail(taskId, (nodeId) => {
      selectedNodeId.value = nodeId;
    });
  }

  function confirmCancelExec() {
    $q.dialog({
      title: '取消执行',
      message: '确定取消当前执行？正在运行的节点将被中断。',
      cancel: true,
      persistent: true,
    }).onOk(() => void cancelExec());
  }

  async function cancelExec() {
    if (!execId.value) return;
    try {
      await graphStore.cancelExecution(execId.value);
      $q.notify({ type: 'info', message: '已请求取消执行' });
      await refreshExecution();
    } catch {
      $q.notify({ type: 'negative', message: '取消失败' });
    }
  }

  async function onSelectCheckpoint(checkpoint: CheckpointInfo) {
    await timeTravel.selectCheckpoint(checkpoint);
  }

  async function onTimeTravel() {
    try {
      await timeTravel.travelToStep(timeTravel.stepIndexInput.value);
      $q.notify({ type: 'positive', message: '已加载步骤状态快照' });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '回溯失败' });
    }
  }

  async function onApplyEditState() {
    try {
      const result = await timeTravel.applyEditState();
      if (result) {
        $q.notify({ type: 'positive', message: `已创建检查点 ${result.newCheckpointId}` });
        await timeTravel.loadCheckpoints();
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '编辑状态失败' });
    }
  }

  async function onRestoreCheckpoint(_checkpoint: CheckpointInfo) {
    try {
      const result = await timeTravel.applyEditState();
      if (result) {
        $q.notify({ type: 'positive', message: `已回退至检查点 ${result.newCheckpointId}` });
        await timeTravel.loadCheckpoints();
        await refreshExecution();
      }
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '回退检查点失败' });
    }
  }

  function updateStatePatchJson(value: string) {
    timeTravel.statePatchJson.value = value;
  }

  function updateStepIndex(value: number) {
    timeTravel.stepIndexInput.value = value;
  }

  function goBack() {
    router.push({ name: 'graphs' });
  }

  const progressCompleted = computed(() => {
    let count = 0;
    for (const state of stream.execNodeStates.value.values()) {
      if (state.status === 'completed') count++;
    }
    return count;
  });

  const progressRunning = computed(() => {
    let count = 0;
    for (const state of stream.execNodeStates.value.values()) {
      if (state.status === 'running') count++;
    }
    return count;
  });

  const progressWaiting = computed(() => {
    let count = 0;
    for (const state of stream.execNodeStates.value.values()) {
      if (state.status === 'waiting' || state.status === 'idle') count++;
    }
    return count;
  });

  const progressTotal = computed(() => {
    const fromStates = stream.execNodeStates.value.size;
    const fromDef = graphDef.nodes?.length ?? 0;
    return Math.max(fromStates, fromDef);
  });

  const progressPercent = computed(() => {
    if (progressTotal.value === 0) return 0;
    return Math.round((progressCompleted.value / progressTotal.value) * 100);
  });

  const progressStepLabel = computed(() => {
    if (stream.executionSummary.value?.totalSteps) {
      return `Step ${progressCompleted.value}/${stream.executionSummary.value.totalSteps}`;
    }
    if (progressTotal.value > 0) {
      return `Step ${progressCompleted.value}/${progressTotal.value}`;
    }
    return '';
  });

  const progressDurationSec = computed(() => {
    const ms = stream.executionSummary.value?.durationMs;
    if (ms && ms > 0) return (ms / 1000).toFixed(1);
    return '';
  });

  const showProgressBar = computed(() => stream.execNodeStates.value.size > 0);

  // ── M53 Phase 11 F7：team 执行 Kanban 视角 ──
  const kanbanNodes = computed(() =>
    buildGraphRunKanbanNodes(execution.value?.steps ?? [], stream.execNodeStates.value, graphDef.nodes),
  );
  /** team 执行（图 team_id 非空）或资产删除降级时显示 Kanban tab。 */
  const showKanbanTab = computed(() => Boolean(graphDef.teamId) || graphAssetMissing.value);

  return {
    isDark,
    graphDef,
    execution,
    execNodeStates: stream.execNodeStates,
    executionSummary: stream.executionSummary,
    streamConnected: stream.streamConnected,
    interrupt: stream.interrupt,
    selectedNodeId,
    hitlDialogOpen: hitl.hitlDialogOpen,
    hitlAdvancedJson: hitl.hitlAdvancedJson,
    resumeLoading: hitl.resumeLoading,
    displayStatus,
    statusColor,
    taskList: stream.taskList,
    tasksLoading: tasks.tasksLoading,
    selectedTaskId: tasks.selectedTaskId,
    taskDrawerOpen: tasks.taskDrawerOpen,
    activeTask: tasks.activeTask,
    taskComments: tasks.taskComments,
    taskLogs: tasks.taskLogs,
    taskRuns: tasks.taskRuns,
    taskEvents: tasks.taskEvents,
    taskDetailLoading: tasks.taskDetailLoading,
    taskActionLoading: tasks.taskActionLoading,
    checkpoints: timeTravel.checkpoints,
    checkpointsLoading: timeTravel.checkpointsLoading,
    selectedCheckpoint: timeTravel.selectedCheckpoint,
    stateSnapshot: timeTravel.stateSnapshot,
    statePatchJson: timeTravel.statePatchJson,
    snapshotLoading: timeTravel.snapshotLoading,
    editLoading: timeTravel.editLoading,
    timeTravelLoading: timeTravel.timeTravelLoading,
    stepIndex: timeTravel.stepIndexInput,
    onSelectNode,
    confirmCancelExec,
    resumeExec: hitl.resumeExec,
    submitHitlResume: hitl.submitHitlResume,
    onSelectCheckpoint,
    onTimeTravel,
    onApplyEditState,
    onRestoreCheckpoint,
    loadTasks: () => tasks.loadTasks(execId.value),
    timeTravelLoadCheckpoints: timeTravel.loadCheckpoints,
    updateStatePatchJson,
    updateStepIndex,
    openTaskDetail: onSelectTask,
    onClaimTask: tasks.onClaimTask,
    onSubmitTask: tasks.onSubmitTask,
    onReportBlocked: tasks.onReportBlocked,
    onUnblockTask: tasks.onUnblockTask,
    onReviewTask: tasks.onReviewTask,
    onAddTaskComment: tasks.onAddTaskComment,
    onKanbanAdminAction: tasks.onKanbanAdminAction,
    inspectorTab,
    goBack,
    progressCompleted,
    progressRunning,
    progressWaiting,
    progressPercent,
    progressStepLabel,
    progressDurationSec,
    showProgressBar,
    kanbanNodes,
    showKanbanTab,
    graphAssetMissing,
  };
}
