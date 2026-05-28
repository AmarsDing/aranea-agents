import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import { useRouter } from "vue-router";
import { storeToRefs } from "pinia";
import { useGraphStore } from "../../stores/graph";
import type { GraphDefinition, NodeType } from "./types";
import { NODE_TYPE_STYLES } from "./types";
import { useGraphExecute } from "./useGraphExecute";

const SORT_OPTIONS = [
  { label: "更新时间", value: "updatedAt" },
  { label: "名称", value: "name" },
  { label: "节点数", value: "nodes" },
];

const ENGINE_FILTER_OPTIONS = [
  { label: "全部引擎", value: "" },
  { label: "BSP（默认）", value: "bsp" },
  { label: "DAG（并行）", value: "dag" },
];

const NODE_TYPE_EMOJI: Record<NodeType, string> = {
  agent: "🤖",
  llm: "🧠",
  router: "🔀",
  function: "⚙️",
  tool: "🔧",
  join: "🔗",
  hitl: "✋",
};

export function useGraphsPage() {
  const $q = useQuasar();
  const router = useRouter();
  const graphStore = useGraphStore();
  const graphExecute = useGraphExecute(router);
  const { graphs: rows, loading } = storeToRefs(graphStore);

  const isDark = computed(() => $q.dark.isActive);
  const error = ref("");
  const runDialogGraph = ref<GraphDefinition | null>(null);

  const searchQuery = ref("");
  const engineFilter = ref("");
  const sortKey = ref("updatedAt");
  const sortOrder = ref("desc");

  const filteredRows = computed(() => {
    let list = rows.value.slice();
    const q = searchQuery.value.trim().toLowerCase();
    if (q) {
      list = list.filter(
        (g) =>
          g.name.toLowerCase().includes(q) ||
          g.description.toLowerCase().includes(q),
      );
    }
    if (engineFilter.value) {
      list = list.filter((g) => g.executionEngine === engineFilter.value);
    }
    const dir = sortOrder.value === "asc" ? 1 : -1;
    list.sort((a, b) => {
      switch (sortKey.value) {
        case "name":
          return dir * a.name.localeCompare(b.name);
        case "nodes":
          return dir * ((a.nodes?.length ?? 0) - (b.nodes?.length ?? 0));
        case "updatedAt":
        default:
          return dir * (new Date(a.updatedAt).getTime() - new Date(b.updatedAt).getTime());
      }
    });
    return list;
  });

  function countNodesByType(graph: GraphDefinition) {
    const counts: Partial<Record<NodeType, number>> = {};
    for (const node of graph.nodes ?? []) {
      counts[node.type] = (counts[node.type] ?? 0) + 1;
    }
    return counts;
  }

  function relativeTime(dateStr: string) {
    const diff = Date.now() - new Date(dateStr).getTime();
    const minutes = Math.floor(diff / 60000);
    if (minutes < 1) return "刚刚";
    if (minutes < 60) return `${minutes}分钟前`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}小时前`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}天前`;
    const months = Math.floor(days / 30);
    return `${months}个月前`;
  }

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

  function confirmRemoveGraph(graph: GraphDefinition) {
    $q.dialog({
      title: "删除 Graph",
      message: `确定删除「${graph.name}」？此操作不可撤销。`,
      cancel: true,
      persistent: true,
    }).onOk(() => void doRemoveGraph(graph));
  }

  async function doRemoveGraph(graph: GraphDefinition) {
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
    filteredRows,
    loading,
    error,
    searchQuery,
    engineFilter,
    sortKey,
    sortOrder,
    SORT_OPTIONS,
    ENGINE_FILTER_OPTIONS,
    NODE_TYPE_EMOJI,
    NODE_TYPE_STYLES,
    countNodesByType,
    relativeTime,
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
    confirmRemoveGraph,
  };
}
