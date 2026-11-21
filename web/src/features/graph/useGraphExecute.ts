import { ref } from 'vue';
import type { Router } from 'vue-router';
import { useQuasar } from 'quasar';
import { useGraphStore } from '../../stores/graph';
import { recordHarnessRun } from '../tasks/recordRun';

/** Shared Graph run dialog + execute navigation (Graphs list + Editor). */
export function useGraphExecute(router: Router) {
  const $q = useQuasar();
  const graphStore = useGraphStore();

  const runDialogOpen = ref(false);
  const runTargetGraphId = ref('');
  const runSessionId = ref('');
  const runInitialState = ref('');
  const runLoading = ref(false);

  function openRunDialog(graphId: string) {
    runTargetGraphId.value = graphId;
    runSessionId.value = `graph-${Date.now()}`;
    runInitialState.value = '';
    runDialogOpen.value = true;
  }

  /** record：从流程列表正式运行时传流程名，这次运行会登记成一条工作台任务；编辑器里的试跑不传 */
  async function executeRun(graphId?: string, record?: { name: string }) {
    const id = graphId ?? runTargetGraphId.value;
    if (!id) return;
    runLoading.value = true;
    try {
      let initialState: Record<string, unknown> | undefined;
      if (runInitialState.value.trim()) {
        try {
          initialState = JSON.parse(runInitialState.value);
        } catch {
          $q.notify({ type: 'negative', message: '初始状态 JSON 格式无效，请检查输入' });
          return;
        }
      }
      const result = await graphStore.runGraph(id, runSessionId.value, initialState);
      if (record?.name && result.executionId)
        void recordHarnessRun({
          title: record.name,
          body: runInitialState.value.trim(),
          assigneeKind: 'flow',
          assigneeId: id,
          assigneeName: record.name,
          exec: { kind: 'graph_execution', ref_id: result.executionId, executor: `flow:${id}` },
        });
      runDialogOpen.value = false;
      $q.notify({ type: 'positive', message: `Graph 已开始执行：${result.executionId}` });
      await router.push({
        name: 'graph-run',
        params: { id, execId: result.executionId },
      });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : '执行失败' });
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
