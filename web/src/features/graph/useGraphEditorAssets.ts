import { ref } from 'vue';
import { useQuasar } from 'quasar';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import type { GraphDefinition, GraphVersionInfo } from './types';
import { useGraphStore } from '../../stores/graph';

export function useGraphEditorAssets(graphDef: GraphDefinition, _isNew: () => boolean) {
  const $q = useQuasar();
  const router = useRouter();
  const graphStore = useGraphStore();
  const { t } = useI18n();

  const versionDialogOpen = ref(false);
  const versions = ref<GraphVersionInfo[]>([]);
  const versionsLoading = ref(false);
  const rollingBackVersion = ref<number | null>(null);
  const templateDialogOpen = ref(false);
  const templateName = ref('');
  const templateCategory = ref('custom');
  const templateSaving = ref(false);
  const importInputRef = ref<HTMLInputElement | null>(null);

  async function openVersionDialog() {
    if (!graphDef.id) {
      $q.notify({ type: 'warning', message: t('graphs.assetSaveGraphFirst') });
      return;
    }
    versionDialogOpen.value = true;
    versionsLoading.value = true;
    try {
      versions.value = await graphStore.fetchGraphVersions(graphDef.id);
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.assetLoadVersionsFailed') });
    } finally {
      versionsLoading.value = false;
    }
  }

  async function rollbackVersion(version: number, onRestored?: () => Promise<void>) {
    if (!graphDef.id) return;
    rollingBackVersion.value = version;
    try {
      const restored = await graphStore.rollbackGraph(graphDef.id, version);
      Object.assign(graphDef, restored);
      versionDialogOpen.value = false;
      $q.notify({ type: 'positive', message: t('graphs.assetRolledBack', { version }) });
      await onRestored?.();
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.assetRollbackFailed') });
    } finally {
      rollingBackVersion.value = null;
    }
  }

  async function exportCurrentGraph() {
    if (!graphDef.id) {
      $q.notify({ type: 'warning', message: t('graphs.assetSaveGraphFirst') });
      return;
    }
    try {
      const result = await graphStore.exportGraphDefinition(graphDef.id);
      const blob = new Blob([result.json], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const anchor = document.createElement('a');
      anchor.href = url;
      anchor.download = `${graphDef.name || graphDef.id}.graph.json`;
      anchor.click();
      URL.revokeObjectURL(url);
      $q.notify({ type: 'positive', message: t('graphs.assetExported') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.assetExportFailed') });
    }
  }

  function triggerImport() {
    importInputRef.value?.click();
  }

  async function onImportFile(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    if (!file) return;

    try {
      const json = await file.text();

      // Show dialog to choose import destination
      $q.dialog({
        title: t('graphs.assetImportTitle'),
        message: t('graphs.assetImportMessage'),
        options: {
          type: 'radio',
          model: 'current',
          items: [
            { value: 'current', label: t('graphs.assetImportToCurrent') },
            { value: 'new', label: t('graphs.assetImportCreateNew') },
          ],
        },
        cancel: true,
        persistent: true,
      }).onOk(async (choice: string) => {
        const created = await graphStore.importGraphDefinition(
          json,
          graphDef.name || file.name.replace(/\.json$/i, ''),
          graphDef.description,
        );

        if (choice === 'current') {
          // Import to current canvas - merge content
          Object.assign(graphDef, created);
          $q.notify({ type: 'positive', message: t('graphs.assetImportedToCurrent') });
        } else {
          // Create new graph - navigate to it
          Object.assign(graphDef, created);
          $q.notify({ type: 'positive', message: t('graphs.assetImportSuccess') });
          router.replace({ name: 'graph-editor', params: { id: created.id } });
        }
      });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.assetImportFailed') });
    }
  }

  function openTemplateDialog() {
    if (!graphDef.id) {
      $q.notify({ type: 'warning', message: t('graphs.assetSaveGraphFirst') });
      return;
    }
    templateName.value = graphDef.name ? t('graphs.assetTemplateNameSuffix', { name: graphDef.name }) : '';
    templateCategory.value = 'custom';
    templateDialogOpen.value = true;
  }

  async function saveTemplate() {
    if (!graphDef.id || !templateName.value.trim()) return;
    templateSaving.value = true;
    try {
      await graphStore.saveAsTemplate(
        graphDef.id,
        templateName.value.trim(),
        templateCategory.value,
        graphDef.description,
      );
      templateDialogOpen.value = false;
      $q.notify({ type: 'positive', message: t('graphs.assetTemplateSaved') });
    } catch (err) {
      $q.notify({ type: 'negative', message: err instanceof Error ? err.message : t('graphs.assetTemplateSaveFailed') });
    } finally {
      templateSaving.value = false;
    }
  }

  return {
    versionDialogOpen,
    versions,
    versionsLoading,
    rollingBackVersion,
    templateDialogOpen,
    templateName,
    templateCategory,
    templateSaving,
    importInputRef,
    openVersionDialog,
    rollbackVersion,
    exportCurrentGraph,
    triggerImport,
    onImportFile,
    openTemplateDialog,
    saveTemplate,
  };
}
