import { computed, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import { useQuasar } from "quasar";
import { useRoute, useRouter } from "vue-router";
import type { GraphDefinition, GraphExecution } from "./types";
import { useGraphStream } from "../chat/useEnvelopeStream";
import { useGraphStore } from "../../stores/graph";

export function useGraphRunPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const graphStore = useGraphStore();

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
    createdAt: "",
    updatedAt: ""
  });

  const execution = ref<GraphExecution | null>(null);
  const execNodeStates = ref<Map<string, { status: string }>>(new Map());
  const resumeDialogOpen = ref(false);
  const resumeValue = ref("");
  const resumeLoading = ref(false);

  let graphStream: ReturnType<typeof useGraphStream> | null = null;

  const statusColor = computed(() => {
    const s = execution.value?.status ?? "";
    if (s === "completed") return "positive";
    if (s === "running") return "blue";
    if (s === "failed") return "negative";
    if (s === "waiting_human") return "warning";
    return "grey";
  });

  onMounted(async () => {
    if (graphId.value) {
      try {
        Object.assign(graphDef, await graphStore.fetchGraph(graphId.value));
      } catch {
        $q.notify({ type: "negative", message: "?? Graph ??" });
      }
    }
    if (execId.value) {
      try {
        execution.value = await graphStore.fetchExecution(execId.value);
        syncNodeStates();
      } catch {
        $q.notify({ type: "negative", message: "????????" });
      }
    }
    if (graphId.value && execId.value) {
      graphStream = useGraphStream("graph-monitor", graphId.value, execId.value);
    }
  });

  onBeforeUnmount(() => {
    graphStream?.disconnect();
  });

  function syncNodeStates() {
    const map = new Map<string, { status: string }>();
    if (execution.value) {
      for (const step of execution.value.steps) {
        map.set(step.nodeId, { status: step.status });
      }
    }
    execNodeStates.value = map;
  }

  function onSelectNode(_nodeId: string | null) {}

  async function cancelExec() {
    if (!execId.value) return;
    try {
      await graphStore.cancelExecution(execId.value);
      $q.notify({ type: "info", message: "?????" });
      execution.value = await graphStore.fetchExecution(execId.value);
      syncNodeStates();
    } catch {
      $q.notify({ type: "negative", message: "????" });
    }
  }

  function resumeExec() {
    resumeValue.value = "";
    resumeDialogOpen.value = true;
  }

  async function doResume() {
    if (!execId.value) return;
    resumeLoading.value = true;
    try {
      let value: Record<string, unknown> | undefined;
      if (resumeValue.value.trim()) {
        value = JSON.parse(resumeValue.value);
      }
      await graphStore.resumeExecution(execId.value, value);
      resumeDialogOpen.value = false;
      $q.notify({ type: "positive", message: "?????" });
      execution.value = await graphStore.fetchExecution(execId.value);
      syncNodeStates();
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "????" });
    } finally {
      resumeLoading.value = false;
    }
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
    execNodeStates,
    resumeDialogOpen,
    resumeValue,
    resumeLoading,
    statusColor,
    onSelectNode,
    cancelExec,
    resumeExec,
    doResume,
    stepIcon,
    stepColor,
    formatTime,
    goBack
  };
}
