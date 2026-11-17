import { storeToRefs } from 'pinia';
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useAgentsPageStore } from '../../stores/agents';

/** Agent 列表页批量操作：选择状态 + 批量启停/删除，通知与确认对话框状态内聚于此。 */
export function useAgentBatch() {
  const $q = useQuasar();
  const { t } = useI18n();
  const pageStore = useAgentsPageStore();
  const { selectedAgentIds } = storeToRefs(pageStore);

  const batchBusy = ref(false);
  /** 正在执行的批量动作键，供批量条做按钮级 loading。 */
  const batchBusyAction = ref('');
  /** 批量动作失败信息，就地展示在批量条内（失败不得只 toast）。 */
  const batchError = ref('');

  function toggleAgentSelected(id: string) {
    pageStore.toggleAgentSelected(id);
  }

  function clearAgentSelection() {
    batchError.value = '';
    pageStore.clearAgentSelection();
  }

  async function runBatchSetStatus(status: 'active' | 'inactive') {
    if (batchBusy.value) return;
    batchBusy.value = true;
    batchBusyAction.value = status === 'active' ? 'enable' : 'disable';
    batchError.value = '';
    try {
      const affected = await pageStore.batchUpdateListedAgents({ status });
      $q.notify({ type: 'positive', message: t('agentsPage.batch.statusOk', { n: affected }) });
    } catch (error) {
      // 就地展示可重试错误（批量条 role=alert），不再只弹 toast 后当作无变化。
      batchError.value = error instanceof Error ? error.message : t('agentsPage.batch.failed');
    } finally {
      batchBusy.value = false;
      batchBusyAction.value = '';
    }
  }

  async function runBatchDelete() {
    if (batchBusy.value) return;
    batchBusy.value = true;
    batchBusyAction.value = 'delete';
    batchError.value = '';
    try {
      const affected = await pageStore.batchUpdateListedAgents({ delete: true });
      $q.notify({ type: 'positive', message: t('agentsPage.batch.deleteOk', { n: affected }) });
    } catch (error) {
      batchError.value = error instanceof Error ? error.message : t('agentsPage.batch.failed');
    } finally {
      batchBusy.value = false;
      batchBusyAction.value = '';
    }
  }

  return {
    selectedAgentIds,
    toggleAgentSelected,
    clearAgentSelection,
    batchBusy,
    batchBusyAction,
    batchError,
    runBatchSetStatus,
    runBatchDelete,
  };
}
