import { computed, onMounted, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import { onBeforeRouteLeave, useRoute, useRouter } from "vue-router";
import type { GraphDefinition, NodeDef } from "./types";
import { useGraphStore } from "../../stores/graph";

export function useGraphEditorPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const graphStore = useGraphStore();

  const isDark = computed(() => $q.dark.isActive);
  const isNew = computed(() => route.name === "graph-editor-new");
  const graphId = computed(() => (route.params.id as string) ?? "");

  const saving = ref(false);
  const dirty = ref(false);
  const runDialogOpen = ref(false);
  const runSessionId = ref("");
  const runInitialState = ref("");
  const runLoading = ref(false);
  const selectedNodeId = ref<string | null>(null);
  const availableTools = ref<string[]>([]);
  const execNodeStates = ref<Map<string, { status: string }>>(new Map());

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
    enableCheckpoint: true,
    executionEngine: "bsp",
    interruptBefore: [],
    interruptAfter: [],
    metadata: {},
    createdAt: "",
    updatedAt: ""
  });

  const selectedNode = computed<NodeDef | null>(() => {
    if (!selectedNodeId.value) return null;
    return graphDef.nodes.find((n) => n.id === selectedNodeId.value) ?? null;
  });

  const canSave = computed(() => Boolean(graphDef.name && graphDef.nodes.length > 0));

  onMounted(async () => {
    if (!isNew.value && graphId.value) {
      try {
        const g = await graphStore.fetchGraph(graphId.value);
        Object.assign(graphDef, g);
      } catch {
        $q.notify({ type: "negative", message: "?? Graph ??" });
      }
    }
  });

  function onSelectNode(nodeId: string | null) {
    selectedNodeId.value = nodeId;
  }

  function markDirty() {
    dirty.value = true;
  }

  async function save() {
    saving.value = true;
    try {
      if (isNew.value || !graphDef.id) {
        const created = await graphStore.addGraph(graphDef);
        Object.assign(graphDef, created);
        dirty.value = false;
        $q.notify({ type: "positive", message: "Graph ???" });
        router.replace({ name: "graph-editor", params: { id: created.id } });
      } else {
        const updated = await graphStore.editGraph(graphDef.id, graphDef);
        Object.assign(graphDef, updated);
        dirty.value = false;
        $q.notify({ type: "positive", message: "Graph ???" });
      }
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    } finally {
      saving.value = false;
    }
  }

  function openRunDialog() {
    runSessionId.value = `graph-${Date.now()}`;
    runInitialState.value = "";
    runDialogOpen.value = true;
  }

  async function executeRun() {
    runLoading.value = true;
    try {
      let initialState: Record<string, unknown> | undefined;
      if (runInitialState.value.trim()) {
        initialState = JSON.parse(runInitialState.value);
      }
      const result = await graphStore.runGraph(graphDef.id, runSessionId.value, initialState);
      runDialogOpen.value = false;
      $q.notify({ type: "positive", message: `Graph ??????${result.executionId}` });
      router.push({
        name: "graph-run",
        params: { id: graphDef.id, execId: result.executionId }
      });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    } finally {
      runLoading.value = false;
    }
  }

  function goBack() {
    router.push({ name: "graphs" });
  }

  onBeforeRouteLeave((_to, _from, next) => {
    if (dirty.value) {
      $q
        .dialog({
          title: "??????",
          message: "?? Graph ???????????????",
          cancel: true,
          persistent: true
        })
        .onOk(() => next())
        .onCancel(() => next(false));
    } else {
      next();
    }
  });

  return {
    isDark,
    isNew,
    saving,
    dirty,
    runDialogOpen,
    runSessionId,
    runInitialState,
    runLoading,
    selectedNodeId,
    availableTools,
    execNodeStates,
    graphDef,
    selectedNode,
    canSave,
    onSelectNode,
    markDirty,
    save,
    openRunDialog,
    executeRun,
    goBack
  };
}
