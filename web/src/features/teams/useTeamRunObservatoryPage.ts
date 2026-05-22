import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import type { TeamRunObservatory } from "../orchestration/types";
import { useOrchestrationStream } from "../orchestration/useOrchestrationStream";
import { compiledGraphToGraphDef } from "../orchestration/compileApi";
import { buildExecNodeStates } from "../orchestration/teamGraphAdapter";
import type { GraphDefinition } from "../graph/types";
import { useOrchestrationStore } from "../../stores/orchestration";

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
    updatedAt: ""
  });

  const stream = ref<ReturnType<typeof useOrchestrationStream> | null>(null);
  const streamConnected = computed(() => stream.value?.connected.value ?? false);

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

  async function load() {
    loading.value = true;
    error.value = "";
    try {
      const obs = await orchestrationStore.fetchRunObservatory(runId.value);
      observatory.value = obs;
      applyCompiledTopology(obs);

      stream.value?.disconnect();
      const s = useOrchestrationStream(obs.session_id, obs.run_id);
      stream.value = s;
      s.seed(obs.nodes);
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
    } finally {
      loading.value = false;
    }
  }

  function onSelectNode(nodeId: string | null) {
    selectedNodeId.value = nodeId;
  }

  function goBack() {
    router.push({ name: "team" });
  }

  onMounted(load);
  watch([teamId, runId], load);
  onBeforeUnmount(() => stream.value?.disconnect());

  return {
    isDark,
    teamId,
    runId,
    loading,
    error,
    observatory,
    selectedNodeId,
    graphDef,
    streamConnected,
    nodeList,
    execNodeStates,
    runStatusColor,
    onSelectNode,
    goBack
  };
}
