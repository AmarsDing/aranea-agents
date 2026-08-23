import { computed, onUnmounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { hasIndexingDocuments } from './knowledgeUi';
import { graphDeltaAffected, type KnowledgeGraphDelta } from './graphDelta';
import type { EmbedderConfig, UpdateEmbedderConfigInput } from './types';
import { useKnowledgeStore } from '../../stores/knowledge';
import { useKnowledgeIngestWs } from './useKnowledgeIngestWs';
import { useVaultExplorer } from './useVaultExplorer';
import { knowledgePageError } from './knowledgePageError';
import { useKnowledgeUpload } from './useKnowledgeUpload';
import { useKnowledgeVaultOps } from './useKnowledgeVaultOps';
import type { VaultQTreeNode } from './useVaultExplorer';

export function useKnowledgePage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const knowledgeStore = useKnowledgeStore();
  const selectedId = ref('');
  const docsLoading = ref(false);
  const DOCUMENTS_PAGE_LIMIT = 2000;

  const error = ref('');
  const unavailable = ref('');

  const embedderConfig = computed<EmbedderConfig | null>(() => knowledgeStore.embedderConfig);
  const collections = computed(() => knowledgeStore.collections);
  const loading = computed(() => knowledgeStore.loading);
  const documents = computed(() => knowledgeStore.documentsByCollection[selectedId.value] ?? []);
  const documentsTruncated = computed(
    () => Boolean(selectedId.value) && Boolean(knowledgeStore.documentsTruncatedByCollection[selectedId.value]),
  );
  const selectedCollection = computed(() => collections.value.find((c) => c.id === selectedId.value));
  const docSourceMap = computed(() => {
    const map: Record<string, string> = {};
    for (const d of documents.value) {
      map[d.id] = d.source;
    }
    return map;
  });

  useKnowledgeIngestWs(
    () => (hasIndexingDocuments(documents.value) ? selectedId.value : ''),
    () => {
      void loadDocuments();
      void loadCollections();
    },
  );

  function friendlyError(err: unknown): string {
    const got = knowledgePageError(err);
    if (got.unavailable) unavailable.value = got.unavailable;
    return got.message;
  }

  async function loadCollections() {
    error.value = '';
    unavailable.value = '';
    try {
      const res = await knowledgeStore.loadCollections({ limit: 100 });
      if (!selectedId.value && res.items.length) {
        selectedId.value = res.items[0].id;
      }
    } catch (e) {
      error.value = friendlyError(e) || t('knowledgePage.loadVaultsFailed');
    }
  }

  async function loadDocuments(opts?: { append?: boolean }) {
    if (!selectedId.value) return;
    docsLoading.value = true;
    try {
      const offset = opts?.append ? documents.value.length : 0;
      await knowledgeStore.loadDocuments(selectedId.value, {
        limit: DOCUMENTS_PAGE_LIMIT,
        offset,
        append: opts?.append,
      });
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || '加载文档失败' });
    } finally {
      docsLoading.value = false;
    }
  }

  function selectCollection(id: string) {
    selectedId.value = id;
  }

  const explorer = useVaultExplorer({
    selectedId,
    collections,
    documents,
    friendlyError,
    notifyError: (message: string) => {
      if (message) $q.notify({ type: 'negative', message });
    },
    semanticErrorFallback: () => t('knowledgePage.searchSemanticError'),
  });

  const upload = useKnowledgeUpload({
    selectedId,
    collections,
    loadDocuments,
    loadCollections,
    friendlyError,
    selectTreeNode: (key) => explorer.selectTreeNode(key),
  });

  const vaultOps = useKnowledgeVaultOps({
    selectedId,
    collections,
    selectedCollection,
    documents,
    embedderConfig,
    loadDocuments,
    loadCollections,
    friendlyError,
    explorer: {
      selectedDocId: explorer.selectedDocId,
      selectedNode: explorer.selectedNode,
      selectTreeNode: explorer.selectTreeNode,
      refreshTree: explorer.refreshTree,
      selectDocument: explorer.selectDocument,
      invalidateVault: explorer.invalidateVault,
    },
  });

  function onTreeNodeAction(action: string, node: VaultQTreeNode) {
    vaultOps.onTreeNodeAction(action, node, upload);
  }

  function applyGraphDelta(delta: KnowledgeGraphDelta): { docIds: string[]; collectionIds: string[] } {
    const affected = graphDeltaAffected(delta);
    knowledgeStore.invalidateLinkCaches(affected.docIds, affected.collectionIds);
    const currentDocId = explorer.selectedDocId.value;
    if (currentDocId && affected.docIds.includes(currentDocId)) {
      explorer.reloadDetail();
    }
    return affected;
  }

  let docPollTimer: ReturnType<typeof setInterval> | null = null;
  function stopDocPoll() {
    if (docPollTimer) {
      clearInterval(docPollTimer);
      docPollTimer = null;
    }
  }
  function syncDocPoll() {
    stopDocPoll();
    if (!selectedId.value || !hasIndexingDocuments(documents.value)) return;
    docPollTimer = setInterval(() => {
      if (!selectedId.value || !hasIndexingDocuments(documents.value)) {
        stopDocPoll();
        return;
      }
      void loadDocuments();
      void loadCollections();
    }, 3000);
  }

  watch(selectedId, (id) => {
    if (id) void loadDocuments();
  });
  watch(
    () => [documents.value.map((d) => d.status).join(','), selectedId.value] as const,
    () => syncDocPoll(),
  );
  onUnmounted(stopDocPoll);

  const embedderSaving = ref(false);
  async function saveEmbedderConfig(input: UpdateEmbedderConfigInput) {
    embedderSaving.value = true;
    try {
      await knowledgeStore.saveEmbedderConfig(input);
      $q.notify({ type: 'positive', message: 'Embedder 配置已保存' });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : '保存失败' });
    } finally {
      embedderSaving.value = false;
    }
  }
  async function loadEmbedderConfig() {
    try {
      await knowledgeStore.loadEmbedderConfig();
    } catch {
      $q.notify({ type: 'warning', message: 'Embedder 配置加载失败，检索功能可能不可用' });
    }
  }

  return {
    collections,
    selectedId,
    selectedCollection,
    documents,
    docSourceMap,
    documentsTruncated,
    DOCUMENTS_PAGE_LIMIT,
    friendlyError,
    loading,
    docsLoading,
    error,
    unavailable,
    embedderConfig,
    embedderSaving,
    saveEmbedderConfig,
    loadCollections,
    loadDocuments,
    loadMoreDocuments: () => loadDocuments({ append: true }),
    loadEmbedderConfig,
    selectCollection,
    applyGraphDelta,
    explorer,
    ...upload,
    ...vaultOps,
    onTreeNodeAction,
  };
}
