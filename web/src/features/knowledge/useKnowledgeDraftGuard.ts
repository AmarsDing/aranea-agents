import { onMounted, onUnmounted } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { KnowledgeWorkbench } from './useKnowledgeWorkbench';

/** 页面离开与浏览器关闭共用未保存判断；编辑器防抖内容须先回写。 */
export function useKnowledgeDraftGuard(workbench: KnowledgeWorkbench, flush: () => void) {
  const $q = useQuasar();
  const { t } = useI18n();
  function beforeUnload(event: BeforeUnloadEvent) {
    flush();
    workbench.persistDrafts();
    if (!workbench.hasUnsaved.value) return;
    event.preventDefault();
    event.returnValue = '';
  }
  onMounted(() => window.addEventListener('beforeunload', beforeUnload));
  onUnmounted(() => window.removeEventListener('beforeunload', beforeUnload));

  onBeforeRouteLeave(async () => {
    flush();
    if (!workbench.hasUnsaved.value) return true;
    return new Promise<boolean>((resolve) => {
      $q.dialog({
        title: t('knowledgeWorkflow.unsavedTitle'),
        message: t('knowledgeWorkflow.unsavedHint'),
        options: {
          type: 'radio',
          model: 'save',
          items: [
            { label: t('knowledgeWorkflow.saveLeave'), value: 'save' },
            {
              label: t('knowledgeWorkflow.draftLeave'),
              value: 'draft',
              disable: !workbench.draftsAvailable || workbench.draftError.value,
            },
          ],
        },
        cancel: true,
        persistent: true,
      })
        .onOk(async (choice: string) => {
          if (choice === 'draft') {
            if (!workbench.draftsAvailable) {
              resolve(false);
              return;
            }
            workbench.persistDrafts();
            resolve(!workbench.draftError.value);
            return;
          }
          try {
            for (const tab of workbench.tabs.value.filter((tab) => tab.dirty || tab.saving)) {
              if (!(await workbench.saveTab(tab.docId))) {
                resolve(false);
                return;
              }
            }
            resolve(!workbench.hasUnsaved.value);
          } catch (error) {
            $q.notify({
              type: 'negative',
              message: error instanceof Error ? error.message : t('knowledgeWorkflow.saveFailed'),
            });
            resolve(false);
          }
        })
        .onCancel(() => resolve(false));
    });
  });
}
