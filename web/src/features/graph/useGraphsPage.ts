import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import { useGraphStore } from "../../stores/graph";
import type { GraphDefinition } from "./types";

export function useGraphsPage() {
  const $q = useQuasar();
  const router = useRouter();
  const graphStore = useGraphStore();
  const { graphs: rows, loading } = storeToRefs(graphStore);

  const isDark = computed(() => $q.dark.isActive);
  const error = ref("");
  const runDialogOpen = ref(false);
  const runDialogGraph = ref<GraphDefinition | null>(null);
  const runSessionId = ref("");
  const runInitialState = ref("");
  const runLoading = ref(false);

  onMounted(() => void loadRows());

  async function loadRows() {
    error.value = "";
    try {
      await graphStore.loadGraphs();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "?? Graph ????";
    }
  }

  function openCreate() {
    router.push({ name: "graph-editor-new" });
  }

  function openEditor(id: string) {
    router.push({ name: "graph-editor", params: { id } });
  }

  function openRunDialog(graph: GraphDefinition) {
    runDialogGraph.value = graph;
    runSessionId.value = `graph-${Date.now()}`;
    runInitialState.value = "";
    runDialogOpen.value = true;
  }

  async function executeRun() {
    if (!runDialogGraph.value) return;
    runLoading.value = true;
    try {
      let initialState: Record<string, unknown> | undefined;
      if (runInitialState.value.trim()) {
        initialState = JSON.parse(runInitialState.value);
      }
      const result = await graphStore.runGraph(runDialogGraph.value.id, runSessionId.value, initialState);
      runDialogOpen.value = false;
      $q.notify({ type: "positive", message: `Graph ??????${result.executionId}` });
      router.push({
        name: "graph-run",
        params: { id: runDialogGraph.value.id, execId: result.executionId }
      });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    } finally {
      runLoading.value = false;
    }
  }

  async function duplicateGraph(graph: GraphDefinition) {
    try {
      const created = await graphStore.addGraph({
        name: `${graph.name} (??)`,
        description: graph.description,
        stateFields: graph.stateFields,
        nodes: graph.nodes,
        edges: graph.edges,
        conditionalEdges: graph.conditionalEdges,
        subgraphs: graph.subgraphs,
        entryPoint: graph.entryPoint,
        finishPoint: graph.finishPoint,
        enableCheckpoint: graph.enableCheckpoint,
        executionEngine: graph.executionEngine
      });
      $q.notify({ type: "positive", message: "Graph ???" });
      return created;
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    }
  }

  async function removeGraph(graph: GraphDefinition) {
    try {
      await graphStore.removeGraph(graph.id);
      $q.notify({ type: "info", message: "Graph ???" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    }
  }

  return {
    isDark,
    rows,
    loading,
    error,
    runDialogOpen,
    runDialogGraph,
    runSessionId,
    runInitialState,
    runLoading,
    loadRows,
    openCreate,
    openEditor,
    openRunDialog,
    executeRun,
    duplicateGraph,
    removeGraph
  };
}
