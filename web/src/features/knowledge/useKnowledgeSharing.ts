import { computed, ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useKnowledgeStore } from '../../stores/knowledge';
import { usePlatformStore } from '../../stores/platform';

export function useKnowledgeSharing() {
  const store = useKnowledgeStore();
  const platform = usePlatformStore();
  const $q = useQuasar();
  const { t } = useI18n();
  const open = ref(false);
  const loading = ref(false);
  const saving = ref(false);
  const loadError = ref('');
  const targetError = ref('');
  const docId = ref('');
  const title = ref('');
  const visibility = ref<'collection' | 'private' | 'shared'>('collection');
  const sharedWith = ref<string[]>([]);
  const error = computed(() => loadError.value || (visibility.value === 'shared' ? targetError.value : ''));
  const options = ref<Array<{ label: string; value: string }>>([]);

  async function show(id: string) {
    docId.value = id;
    open.value = true;
    loading.value = true;
    loadError.value = '';
    targetError.value = '';
    try {
      const doc = await store.loadDocument(id);
      title.value = doc.rel_path || doc.source;
      visibility.value = doc.visibility === 'shared' || doc.visibility === 'private' ? doc.visibility : 'collection';
      sharedWith.value = [...(doc.shared_with ?? [])];
      // 只列部门/岗位，不把公司当共享目标，更不创建组织节点。
      const nodes = await platform.loadResource('organization').catch((err: unknown) => {
        targetError.value = err instanceof Error ? err.message : t('knowledgeWorkflow.openFailed');
        return [];
      });
      const byId = new Map(nodes.map((node) => [node.id, node]));
      options.value = nodes
        .filter((node) => node.level === 'department' || node.level === 'position')
        .map((node) => ({
          value: node.id,
          label: `${byId.get(node.parent_id)?.name ? `${byId.get(node.parent_id)?.name} / ` : ''}${node.name} (${node.level === 'department' ? t('knowledgeWorkflow.department') : t('knowledgeWorkflow.position')})`,
        }));
    } catch (err) {
      loadError.value = err instanceof Error ? err.message : t('knowledgeWorkflow.openFailed');
    } finally {
      loading.value = false;
    }
  }

  async function save() {
    if (loading.value || error.value || !docId.value) return;
    if (visibility.value === 'shared' && !sharedWith.value.length) return;
    saving.value = true;
    try {
      await store.setDocumentVisibility(docId.value, visibility.value, sharedWith.value);
      open.value = false;
      $q.notify({ type: 'positive', message: t('knowledgeWorkflow.visibilitySaved') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('knowledgeWorkflow.saveFailed') });
    } finally {
      saving.value = false;
    }
  }
  return { open, loading, saving, error, title, visibility, sharedWith, options, show, save };
}
