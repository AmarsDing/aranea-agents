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

  const batchDeleteOpen = ref(false);
  const batchBusy = ref(false);

  function toggleAgentSelected(id: string) {
    pageStore.toggleAgentSelected(id);
  }

  function clearAgentSelection() {
    pageStore.clearAgentSelection();
  }

  function requestBatchDelete() {
    if (selectedAgentIds.value.length === 0) return;
    batchDeleteOpen.value = true;
  }

  async function runBatchSetStatus(status: 'active' | 'inactive') {
    if (batchBusy.value) return;
    batchBusy.value = true;
    try {
      const affected = await pageStore.batchUpdateListedAgents({ status });
      $q.notify({ type: 'positive', message: t('agentsPage.batch.statusOk', { n: affected }) });
    } catch (error) {
      $q.notify({
        type: 'negative',
        message: error instanceof Error ? error.message : t('agentsPage.batch.failed'),
      });
    } finally {
      batchBusy.value = false;
    }
  }

  async function runBatchDelete() {
    batchDeleteOpen.value = false;
    if (batchBusy.value) return;
    batchBusy.value = true;
    try {
      const affected = await pageStore.batchUpdateListedAgents({ delete: true });
      $q.notify({ type: 'positive', message: t('agentsPage.batch.deleteOk', { n: affected }) });
    } catch (error) {
      $q.notify({
        type: 'negative',
        message: error instanceof Error ? error.message : t('agentsPage.batch.failed'),
      });
    } finally {
      batchBusy.value = false;
    }
  }

  return {
    selectedAgentIds,
    toggleAgentSelected,
    clearAgentSelection,
    batchDeleteOpen,
    batchBusy,
    requestBatchDelete,
    runBatchSetStatus,
    runBatchDelete,
  };
}
