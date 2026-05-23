import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import type { TeamRunObservatory } from "../orchestration/types";
import { useOrchestrationStream } from "../orchestration/useOrchestrationStream";
import { compiledGraphToGraphDef } from "../orchestration/compileApi";
import { buildExecNodeStates } from "../orchestration/teamGraphAdapter";
import type { GraphDefinition, Task } from "../graph/types";
import { useOrchestrationStore } from "../../stores/orchestration";
import { useGraphRunTasks } from "../graph/useGraphRunTasks";
import { useGraphExecutionStream } from "../graph/runtime/useGraphExecutionStream";

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
    createdAt: "",
    updatedAt: "",
  });

  const stream = ref<ReturnType<typeof useOrchestrationStream> | null>(null);
  const streamConnected = computed(() => stream.value?.connected.value ?? false);

  const graphExecutionId = computed(() => observatory.value?.graph_execution_id?.trim() ?? "");
  const taskList = computed(() => graphExecStream.value?.taskList.value ?? []);
  const taskStreamConnected = computed(() => graphExecStream.value?.streamConnected.value ?? false);

  const taskItemsSeed = (items: Task[]) => {
    graphExecStream.value?.seedTasks(items);
  };
  const taskUpsert = (task: Task) => {
    graphExecStream.value?.upsertTask(task);
  };

  const tasks = useGraphRunTasks(() => graphExecutionId.value, taskItemsSeed, taskUpsert);

  const nodeList = computed(() => {
    const map = stream.value?.nodes.value ?? new Map();
    return [...map.values()];
  });

  const execNodeStates = computed(() => buildExecNodeStates(stream.value?.nodes.value ?? new Map()));

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

  onMounted(load);
  watch([teamId, runId], load);
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
    onSelectNode,
    onSelectTask,
    onKanbanAdminAction: tasks.onKanbanAdminAction,
    loadObservatoryTasks,
    goBack,
  };
}
