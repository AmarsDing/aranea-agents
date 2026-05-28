import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import type { CheckpointInfo, GraphDefinition, GraphExecution } from "./types";
import { useGraphStore } from "../../stores/graph";
import { useGraphTimeTravel } from "./runtime/useGraphTimeTravel";
import { useGraphRunStream } from "./runtime/useGraphRunStream";
import { useGraphRunTasks } from "./useGraphRunTasks";
import { useGraphRunHitl } from "./useGraphRunHitl";

export function useGraphRunPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const graphStore = useGraphStore();
  let loadSeq = 0;

  const isDark = computed(() => $q.dark.isActive);
  const graphId = computed(() => (route.params.id as string) ?? "");
  const execId = computed(() => (route.params.execId as string) ?? "");

  const graphDef = reactive<GraphDefinition>({
    id: "",
    name: "",
    description: "",
    stateFields: [],
    nodes: [],
    edges: [],
    conditionalEdges: [],
    subgraphs: [],
    entryPoint: "",
    finishPoint: "",
    enableCheckpoint: false,
    executionEngine: "bsp",
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    version: 0,
    createdAt: "",
    updatedAt: "",
  });

  const execution = ref<GraphExecution | null>(null);
  const selectedNodeId = ref<string | null>(null);

  const stream = useGraphRunStream(graphId, execId, execution);
  const timeTravel = useGraphTimeTravel(execId);

  const tasks = useGraphRunTasks(() => execId.value, stream.seedTasks, stream.upsertTask);

  const displayStatus = computed(() => stream.liveStatus.value);

  const statusColor = computed(() => {
    const s = displayStatus.value;
    if (s === "completed") return "positive";
    if (s === "running") return "blue";
    if (s === "failed") return "negative";
    if (s === "waiting_human") return "warning";
    if (s === "cancelled") return "grey";
    return "grey";
  });

  async function refreshExecution() {
    if (!execId.value) return;
    execution.value = await graphStore.fetchExecution(execId.value);
    stream.connectStream(execution.value?.steps ?? []);
    await tasks.loadTasks(execId.value);
  }

  const inspectorTab = ref("overview");

  const hitl = useGraphRunHitl(
    execId,
    stream.interrupt,
    displayStatus,
    stream.clearInterrupt,
    refreshExecution,
  );

  async function loadPageData() {
    const seq = ++loadSeq;
    if (graphId.value) {
      try {
        const graph = await graphStore.fetchGraph(graphId.value);
        if (seq !== loadSeq) return;
        Object.assign(graphDef, graph);
      } catch {
        if (seq !== loadSeq) return;
        $q.notify({ type: "negative", message: "加载 Graph 失败" });
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
        $q.notify({ type: "negative", message: "加载执行记录失败" });
      }
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
      inspectorTab.value = "tasks";
    }
  }

  function onSelectTask(taskId: string) {
    inspectorTab.value = "tasks";
    void tasks.openTaskDetail(taskId, (nodeId) => {
      selectedNodeId.value = nodeId;
    });
  }

  function confirmCancelExec() {
    $q.dialog({
      title: "取消执行",
      message: "确定取消当前执行？正在运行的节点将被中断。",
      cancel: true,
      persistent: true,
    }).onOk(() => void cancelExec());
  }

  async function cancelExec() {
    if (!execId.value) return;
    try {
      await graphStore.cancelExecution(execId.value);
      $q.notify({ type: "info", message: "已请求取消执行" });
      await refreshExecution();
    } catch {
      $q.notify({ type: "negative", message: "取消失败" });
    }
  }

  async function onSelectCheckpoint(checkpoint: CheckpointInfo) {
    await timeTravel.selectCheckpoint(checkpoint);
  }

  async function onTimeTravel() {
    try {
      await timeTravel.travelToStep(timeTravel.stepIndexInput.value);
      $q.notify({ type: "positive", message: "已加载步骤状态快照" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "回溯失败" });
    }
  }

  async function onApplyEditState() {
    try {
      const result = await timeTravel.applyEditState();
      if (result) {
        $q.notify({ type: "positive", message: `已创建检查点 ${result.newCheckpointId}` });
        await timeTravel.loadCheckpoints();
      }
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "编辑状态失败" });
    }
  }

  function updateStatePatchJson(value: string) {
    timeTravel.statePatchJson.value = value;
  }

  function updateStepIndex(value: number) {
    timeTravel.stepIndexInput.value = value;
  }

  function stepIcon(status: string) {
    if (status === "completed") return "check_circle";
    if (status === "error" || status === "failed") return "error";
    if (status === "running") return "sync";
    return "radio_button_unchecked";
  }

  function stepColor(status: string) {
    if (status === "completed") return "positive";
    if (status === "error" || status === "failed") return "negative";
    if (status === "running") return "blue";
    return "grey";
  }

  function formatTime(ts: string) {
    if (!ts) return "";
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  }

  function goBack() {
    router.push({ name: "graphs" });
  }

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
    statePatchJson: timeTravel.statePatchJson,
    snapshotLoading: timeTravel.snapshotLoading,
    editLoading: timeTravel.editLoading,
    timeTravelLoading: timeTravel.timeTravelLoading,
    stepIndex: timeTravel.stepIndexInput,
    onSelectNode,
    confirmCancelExec,
    cancelExec,
    resumeExec: hitl.resumeExec,
    submitHitlResume: hitl.submitHitlResume,
    onSelectCheckpoint,
    onTimeTravel,
    onApplyEditState,
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
    stepIcon,
    stepColor,
    formatTime,
    goBack,
  };
}
