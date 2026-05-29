import { ref } from "vue";
import type { Router } from "vue-router";
import { useQuasar } from "quasar";
import { useGraphStore } from "../../stores/graph";

/** Shared Graph run dialog + execute navigation (Graphs list + Editor). */
export function useGraphExecute(router: Router) {
  const $q = useQuasar();
  const graphStore = useGraphStore();

  const runDialogOpen = ref(false);
  const runTargetGraphId = ref("");
  const runSessionId = ref("");
  const runInitialState = ref("");
  const runLoading = ref(false);

  function openRunDialog(graphId: string) {
    runTargetGraphId.value = graphId;
    runSessionId.value = `graph-${Date.now()}`;
    runInitialState.value = "";
    runDialogOpen.value = true;
  }

  async function executeRun(graphId?: string) {
    const id = graphId ?? runTargetGraphId.value;
    if (!id) return;
    runLoading.value = true;
    try {
      let initialState: Record<string, unknown> | undefined;
      if (runInitialState.value.trim()) {
        try {
          initialState = JSON.parse(runInitialState.value);
        } catch {
          $q.notify({ type: "negative", message: "初始状态 JSON 格式无效，请检查输入" });
          return;
        }
      }
      const result = await graphStore.runGraph(id, runSessionId.value, initialState);
      runDialogOpen.value = false;
      $q.notify({ type: "positive", message: `Graph 已开始执行：${result.executionId}` });
      await router.push({
        name: "graph-run",
        params: { id, execId: result.executionId },
      });
    } catch (err) {
      $q.notify({ type: "negative", message: err instanceof Error ? err.message : "执行失败" });
    } finally {
      runLoading.value = false;
    }
  }

  return {
    runDialogOpen,
    runTargetGraphId,
    runSessionId,
    runInitialState,
    runLoading,
    openRunDialog,
    executeRun,
  };
}
