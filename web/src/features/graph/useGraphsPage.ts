import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import { useGraphStore } from "../../stores/graph";
import type { GraphDefinition } from "./types";
import { useGraphExecute } from "./useGraphExecute";

export function useGraphsPage() {
  const $q = useQuasar();
  const router = useRouter();
  const graphStore = useGraphStore();
  const graphExecute = useGraphExecute(router);
  const { graphs: rows, loading } = storeToRefs(graphStore);

  const isDark = computed(() => $q.dark.isActive);
  const error = ref("");
  const runDialogGraph = ref<GraphDefinition | null>(null);

  onMounted(() => void loadRows());

  async function loadRows() {
    error.value = "";
    try {
      await graphStore.loadGraphs();
    } catch (err) {
      error.value = err instanceof Error ? err.message : "加载 Graph 列表失败";
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
    graphExecute.openRunDialog(graph.id);
  }

  async function executeRun() {
    if (!runDialogGraph.value) return;
    await graphExecute.executeRun(runDialogGraph.value.id);
  }

  async function duplicateGraph(graph: GraphDefinition) {
    try {
      await graphStore.addGraph({
        name: `${graph.name} (副本)`,
        description: graph.description,
        stateFields: graph.stateFields,
        nodes: graph.nodes,
        edges: graph.edges,
        conditionalEdges: graph.conditionalEdges,
        subgraphs: graph.subgraphs,
        entryPoint: graph.entryPoint,
        finishPoint: graph.finishPoint,
        enableCheckpoint: graph.enableCheckpoint,
        executionEngine: graph.executionEngine,
        interruptBefore: graph.interruptBefore,
        interruptAfter: graph.interruptAfter,
        metadata: graph.metadata,
      });
      $q.notify({ type: "positive", message: "Graph 已复制" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "复制失败" });
    }
  }

  async function removeGraph(graph: GraphDefinition) {
    try {
      await graphStore.removeGraph(graph.id);
      $q.notify({ type: "info", message: "Graph 已删除" });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "删除失败" });
    }
  }

  return {
    isDark,
    rows,
    loading,
    error,
    runDialogOpen: graphExecute.runDialogOpen,
    runDialogGraph,
    runSessionId: graphExecute.runSessionId,
    runInitialState: graphExecute.runInitialState,
    runLoading: graphExecute.runLoading,
    loadRows,
    openCreate,
    openEditor,
    openRunDialog,
    executeRun,
    duplicateGraph,
    removeGraph,
  };
}
