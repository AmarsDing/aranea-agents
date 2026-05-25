import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import type { TeamRunObservatory, ActivityTimelineRow } from "../orchestration/types";
import { useOrchestrationStream } from "../orchestration/useOrchestrationStream";
import { getTeamRunObservatoryTimeline } from "../orchestration/api";
import { compiledGraphToGraphDef } from "../orchestration/compileApi";
import { buildExecNodeStates } from "../orchestration/teamGraphAdapter";
import type { GraphDefinition, Task } from "../graph/types";
import type { TeamRunSummary } from "./types";
import { useOrchestrationStore } from "../../stores/orchestration";
import { useGraphRunTasks } from "../graph/useGraphRunTasks";
import { useGraphExecutionStream } from "../graph/runtime/useGraphExecutionStream";
import { getTeamRunSummary, resumeTeamRunExecution } from "./api";

export function useTeamRunObservatoryPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const orchestrationStore = useOrchestrationStore();

  const isDark = computed(() => $q.dark.isActive);
  const teamId = computed(() => String(route.params.teamId ?? ""));
  const runId = computed(() => String(route.params.runId ?? ""));

  const loading = ref(true);
  const error = ref("");
  const observatory = ref<TeamRunObservatory | null>(null);
  const selectedNodeId = ref<string | null>(null);
  const observatoryTab = ref("agents");
  const timelineRows = ref<ActivityTimelineRow[]>([]);
  const timelineLoading = ref(false);
  const timelineNodeFilter = ref<string | null>(null);
  const runSummary = ref<TeamRunSummary | null>(null);
  const summaryLoading = ref(false);
  const hitlDialogOpen = ref(false);
  const hitlReviewNodeId = ref<string | null>(null);
  const hitlAdvancedJson = ref("");
  const resumeLoading = ref(false);
  const graphExecStream = ref<ReturnType<typeof useGraphExecutionStream> | null>(null);

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

  const stream = ref<ReturnType<typeof useOrchestrationStream> | null>(null);
  const streamConnected = computed(() => stream.value?.connected ?? false);

  const graphExecutionId = computed(() => observatory.value?.graph_execution_id?.trim() ?? "");
  const taskList = computed(() => graphExecStream.value?.taskList ?? []);
  const taskStreamConnected = computed(() => graphExecStream.value?.streamConnected ?? false);

  const taskItemsSeed = (items: Task[]) => {
    graphExecStream.value?.seedTasks(items);
  };
  const taskUpsert = (task: Task) => {
    graphExecStream.value?.upsertTask(task);
  };

  const tasks = useGraphRunTasks(() => graphExecutionId.value, taskItemsSeed, taskUpsert);

  const nodeList = computed(() => {
    const map = stream.value?.nodes ?? new Map();
    return [...map.values()];
  });

  const execNodeStates = computed(() => buildExecNodeStates(stream.value?.nodes ?? new Map()));

  const hitlReviewNode = computed(() => {
    const id = hitlReviewNodeId.value;
    if (!id) return null;
    return nodeList.value.find((n) => n.node_id === id) ?? null;
  });

  const waitingReviewNodes = computed(() => nodeList.value.filter((n) => n.status === "waiting_review"));

  const timelineNodeFilterOptions = computed(() => {
    const ids = new Set<string>();
    for (const row of timelineRows.value) {
      if (row.node_id) ids.add(row.node_id);
    }
    for (const n of nodeList.value) {
      if (n.node_id) ids.add(n.node_id);
    }
    return [...ids].sort().map((id) => ({ label: id, value: id }));
  });

  const filteredTimelineRows = computed(() => {
    const filter = timelineNodeFilter.value?.trim();
    if (!filter) return timelineRows.value;
    return timelineRows.value.filter((row) => row.node_id === filter);
  });

  const runStatusColor = computed(() => {
    const s = observatory.value?.status ?? "";
    if (s === "running" || s === "pending") return "primary";
    if (s === "success") return "positive";
    if (s === "cancelled") return "grey";
    return "negative";
  });

  function applyCompiledTopology(obs: TeamRunObservatory) {
    if (obs.compiled_topology) {
      Object.assign(graphDef, compiledGraphToGraphDef(obs.compiled_topology, "team-run-orchestration"));
      return;
    }
    graphDef.nodes = [];
    graphDef.edges = [];
  }

  function connectTaskStream(obs: TeamRunObservatory) {
    graphExecStream.value?.disconnect();
    graphExecStream.value = null;
    const execId = obs.graph_execution_id?.trim();
    if (!execId || !obs.session_id) return;
    const graphId = graphDef.id || "team-run-orchestration";
    const execStream = useGraphExecutionStream(obs.session_id, graphId, execId);
    graphExecStream.value = execStream;
  }

  async function loadObservatoryTasks() {
    if (!graphExecutionId.value) return;
    await tasks.loadTasks(graphExecutionId.value);
  }

  async function loadSummary() {
    if (!runId.value) return;
    summaryLoading.value = true;
    try {
      runSummary.value = await getTeamRunSummary(runId.value);
    } catch {
      runSummary.value = null;
    } finally {
      summaryLoading.value = false;
    }
  }

  async function loadTimeline() {
    if (!runId.value) return;
    timelineLoading.value = true;
    try {
      const res = await getTeamRunObservatoryTimeline(runId.value, { limit: 100 });
      timelineRows.value = res.rows;
    } catch {
      timelineRows.value = [];
    } finally {
      timelineLoading.value = false;
    }
  }

  async function load() {
    loading.value = true;
    error.value = "";
    try {
      const obs = await orchestrationStore.fetchRunObservatory(runId.value);
      observatory.value = obs;
      applyCompiledTopology(obs);
      connectTaskStream(obs);

      stream.value?.disconnect();
      const s = useOrchestrationStream(obs.session_id, obs.run_id);
      stream.value = s;
      s.seed(obs.nodes);

      if (obs.graph_execution_id) {
        await loadObservatoryTasks();
      }
      await Promise.all([loadTimeline(), loadSummary()]);
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  function onSelectNode(nodeId: string | null) {
    selectedNodeId.value = nodeId;
    tasks.focusTaskForNode(taskList.value, nodeId);
  }

  function onSelectTask(taskId: string) {
    observatoryTab.value = "tasks";
    void tasks.openTaskDetail(taskId, (nodeId) => {
      selectedNodeId.value = nodeId;
    });
  }

  function goBack() {
    router.push({ name: "team" });
  }

  async function resumeRun(resumeValue: Record<string, unknown>) {
    if (!runId.value) return;
    resumeLoading.value = true;
    try {
      await resumeTeamRunExecution(runId.value, resumeValue);
      hitlDialogOpen.value = false;
      hitlReviewNodeId.value = null;
      hitlAdvancedJson.value = "";
      await load();
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      resumeLoading.value = false;
    }
  }

  function parseAdvancedResume(): Record<string, unknown> {
    const raw = hitlAdvancedJson.value.trim();
    if (!raw) return { action: "review_continue" };
    try {
      return JSON.parse(raw) as Record<string, unknown>;
    } catch {
      return { action: "review_continue", note: raw };
    }
  }

  async function onFailureReview(nodeId?: string) {
    observatoryTab.value = "hitl";
    hitlReviewNodeId.value = nodeId ?? waitingReviewNodes.value[0]?.node_id ?? null;
    hitlDialogOpen.value = true;
  }

  async function onFailureRetry(nodeId?: string) {
    await resumeRun({ action: "retry", node_id: nodeId ?? hitlReviewNodeId.value ?? undefined });
  }

  async function onFailureFallback(nodeId?: string) {
    await resumeRun({ action: "fallback", node_id: nodeId ?? hitlReviewNodeId.value ?? undefined });
  }

  async function onHitlApprove() {
    await resumeRun(parseAdvancedResume());
  }

  async function onHitlReject() {
    await resumeRun({ action: "halt" });
  }

  async function onHitlFallback() {
    await onFailureFallback(hitlReviewNodeId.value ?? undefined);
  }

  function onFailureHalt() {
    void resumeRun({ action: "halt" });
  }

  onMounted(load);
  watch([teamId, runId], load);
  watch(
    () => stream.value?.nodes.size,
    () => {
      if (observatoryTab.value === "timeline") {
        void loadTimeline();
      }
    },
  );
  onBeforeUnmount(() => {
    stream.value?.disconnect();
    graphExecStream.value?.disconnect();
  });

  return {
    isDark,
    teamId,
    runId,
    loading,
    error,
    observatory,
    observatoryTab,
    selectedNodeId,
    selectedTaskId: tasks.selectedTaskId,
    graphDef,
    streamConnected,
    taskStreamConnected,
    nodeList,
    taskList,
    tasksLoading: tasks.tasksLoading,
    graphExecutionId,
    execNodeStates,
    runStatusColor,
    timelineRows,
    timelineLoading,
    timelineNodeFilter,
    timelineNodeFilterOptions,
    filteredTimelineRows,
    runSummary,
    summaryLoading,
    waitingReviewNodes,
    hitlDialogOpen,
    hitlReviewNode,
    hitlReviewNodeId,
    hitlAdvancedJson,
    resumeLoading,
    loadTimeline,
    onSelectNode,
    onSelectTask,
    onKanbanAdminAction: tasks.onKanbanAdminAction,
    loadObservatoryTasks,
    goBack,
    onFailureReview,
    onFailureRetry,
    onFailureFallback,
    onFailureHalt,
    onHitlApprove,
    onHitlReject,
    onHitlFallback,
  };
}
