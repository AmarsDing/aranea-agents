import { computed, ref, type ComputedRef, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { promoteTargetOptions } from './knowledgeUi';
import { fetchDocumentAsset } from './api';
import { dirNodeKey, vaultNodeKey } from './vaultTreeUi';
import type { EmbedderConfig, KnowledgeCollection, KnowledgeDocument, PromoteResult } from './types';
import { useKnowledgeStore } from '../../stores/knowledge';
import { parseKratosApiError } from '../../utils/kratosError';
import type { VaultQTreeNode } from './useVaultExplorer';

export type KnowledgeVaultOpsDeps = {
  selectedId: Ref<string>;
  collections: ComputedRef<KnowledgeCollection[]>;
  selectedCollection: ComputedRef<KnowledgeCollection | undefined>;
  documents: ComputedRef<KnowledgeDocument[]>;
  embedderConfig: ComputedRef<EmbedderConfig | null>;
  loadDocuments: () => Promise<void>;
  loadCollections: () => Promise<void>;
  friendlyError: (err: unknown) => string;
  explorer: {
    selectedDocId: Ref<string>;
    selectedNode: { value: { kind?: string; path?: string; name?: string } | null };
    selectTreeNode: (key: string) => Promise<void> | void;
    refreshTree: () => Promise<void> | void;
    selectDocument: (id: string) => void;
    invalidateVault: (id: string) => void;
  };
};

export function useKnowledgeVaultOps(deps: KnowledgeVaultOpsDeps) {
  const $q = useQuasar();
  const { t } = useI18n();
  const knowledgeStore = useKnowledgeStore();

  const createOpen = ref(false);
  const createLoading = ref(false);
  const createForm = ref({ name: '', description: '', embedding_model: '', root_path: '' });
  const removedDocId = ref('');
  const moveOpen = ref(false);
  const moveLoading = ref(false);
  const movingDoc = ref<KnowledgeDocument | null>(null);
  const moveTargetId = ref('');
  const promoteOpen = ref(false);
  const promoteLoading = ref(false);
  const promoteTargetId = ref('');
  const promoteResult = ref<PromoteResult | null>(null);
  const promoteDocId = ref('');
  const promoteDocName = ref('');

  const moveTargetOptions = computed(() => {
    const current = deps.selectedCollection.value;
    return deps.collections.value
      .filter((c) => c.id !== deps.selectedId.value)
      .map((c) => ({
        label: c.name || c.id,
        value: c.id,
        disable: !!current && c.dim !== current.dim,
        dim: c.dim,
      }));
  });
  const promoteOptions = computed(() => promoteTargetOptions(deps.collections.value, deps.selectedId.value));
  const promotable = computed(
    () => !!deps.selectedCollection.value && deps.selectedCollection.value.vault_backend !== 'team',
  );

  function openCreateCollection() {
    createForm.value = { name: '', description: '', embedding_model: '', root_path: '' };
    createOpen.value = true;
  }

  async function submitCreateCollection() {
    if (!createForm.value.name.trim()) {
      $q.notify({ type: 'warning', message: t('knowledgePage.createNameRequired') });
      return;
    }
    if (!createForm.value.root_path.trim()) {
      $q.notify({ type: 'warning', message: t('knowledgePage.createRootPathRequired') });
      return;
    }
    createLoading.value = true;
    try {
      const col = await knowledgeStore.addCollection({
        name: createForm.value.name.trim(),
        description: createForm.value.description.trim(),
        embedding_model: createForm.value.embedding_model.trim(),
        root_path: createForm.value.root_path.trim(),
      });
      createOpen.value = false;
      await deps.loadCollections();
      deps.selectedId.value = col.id;
      $q.notify({ type: 'positive', message: t('knowledgePage.createSuccess') });
    } catch (e) {
      $q.notify({ type: 'negative', message: deps.friendlyError(e) || t('knowledgePage.createFailed') });
    } finally {
      createLoading.value = false;
    }
  }

  function confirmDeleteCollection(id?: string) {
    const col = id ? deps.collections.value.find((c) => c.id === id) : deps.selectedCollection.value;
    if (!col) return;
    $q.dialog({
      title: t('knowledgePage.deleteVaultTitle'),
      message: t('knowledgePage.deleteVaultMessage', { name: col.name }),
      cancel: true,
      persistent: true,
    }).onOk(() => void deleteCollectionConfirmed(col.id));
  }

  async function deleteCollectionConfirmed(id: string) {
    try {
      await knowledgeStore.removeCollection(id);
      if (deps.selectedId.value === id) {
        deps.selectedId.value = '';
      }
      await deps.loadCollections();
      $q.notify({ type: 'positive', message: t('knowledgePage.deleted', 'Deleted') });
    } catch (e) {
      $q.notify({
        type: 'negative',
        message: deps.friendlyError(e) || t('knowledgePage.deleteFailed', 'Delete failed'),
      });
    }
  }

  function confirmReembed(docIds: string[]) {
    if (!deps.selectedId.value || !docIds.length) return;
    $q.dialog({
      title: t('knowledgePage.reembedConfirmTitle'),
      message: t('knowledgePage.reembedConfirmBody', { n: docIds.length }),
      cancel: true,
      persistent: true,
    }).onOk(() => {
      void knowledgeStore
        .reembedDocuments(deps.selectedId.value, docIds)
        .then((r) => {
          $q.notify({ type: 'positive', message: t('knowledgePage.reembedAccepted', { n: r.accepted_count }) });
          void deps.loadDocuments();
        })
        .catch((e) => $q.notify({ type: 'negative', message: deps.friendlyError(e) }));
    });
  }

  function confirmDeleteDocument(doc: KnowledgeDocument) {
    $q.dialog({
      title: t('knowledgePage.deleteDocTitle', 'Delete document'),
      message: t('knowledgePage.deleteDocMessage', { name: doc.source }, 'Delete "{name}"?'),
      cancel: true,
    }).onOk(async () => {
      try {
        await knowledgeStore.removeDocument(doc.id, deps.selectedId.value);
        await deps.loadDocuments();
        await deps.loadCollections();
        removedDocId.value = doc.id;
        $q.notify({ type: 'positive', message: t('knowledgePage.deleted', 'Deleted') });
      } catch (e) {
        $q.notify({
          type: 'negative',
          message: deps.friendlyError(e) || t('knowledgePage.deleteFailed', 'Delete failed'),
        });
      }
    });
  }

  async function downloadDocument(doc: Pick<KnowledgeDocument, 'id' | 'source'>) {
    try {
      const { blob, filename } = await fetchDocumentAsset(doc.id);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename || doc.source || 'document';
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      $q.notify({ type: 'negative', message: deps.friendlyError(e) || t('knowledgePage.downloadFailed') });
    }
  }

  async function setDocumentVisibility(doc: Pick<KnowledgeDocument, 'id'>, visibility: 'collection' | 'private') {
    try {
      await knowledgeStore.setDocumentVisibility(doc.id, visibility);
    } catch (e) {
      $q.notify({ type: 'negative', message: deps.friendlyError(e) || t('knowledgePage.workbench.filePrivate') });
    }
  }

  function openMoveDialog(doc: KnowledgeDocument) {
    movingDoc.value = doc;
    moveTargetId.value = '';
    moveOpen.value = true;
  }

  async function submitMove() {
    const doc = movingDoc.value;
    if (!doc || !moveTargetId.value) return;
    moveLoading.value = true;
    try {
      await knowledgeStore.moveDoc(doc.id, moveTargetId.value);
      moveOpen.value = false;
      const target = deps.collections.value.find((c) => c.id === moveTargetId.value);
      await deps.loadDocuments();
      await deps.loadCollections();
      $q.notify({ type: 'positive', message: t('knowledgePage.moveSuccess', { name: target?.name ?? '' }) });
    } catch (e) {
      $q.notify({ type: 'negative', message: deps.friendlyError(e) || t('knowledgePage.moveFailed') });
    } finally {
      moveLoading.value = false;
    }
  }

  function openPromoteDialog(docId?: string) {
    const id = docId || deps.explorer.selectedDocId.value;
    if (!id) return;
    const doc = deps.documents.value.find((d) => d.id === id);
    const node = deps.explorer.selectedNode.value;
    promoteDocId.value = id;
    promoteDocName.value = doc?.rel_path || doc?.source || (node?.kind === 'file' ? node.path || node.name : id);
    promoteTargetId.value = promoteOptions.value[0]?.value ?? '';
    promoteResult.value = null;
    promoteOpen.value = true;
  }

  async function submitPromote() {
    const docId = promoteDocId.value;
    if (!docId || !promoteTargetId.value) return;
    promoteLoading.value = true;
    try {
      promoteResult.value = await knowledgeStore.promoteDocs([docId], promoteTargetId.value);
      await deps.loadCollections();
    } catch (e) {
      promoteOpen.value = false;
      $q.notify({ type: 'negative', message: deps.friendlyError(e) || t('knowledgePage.promoteFailed') });
    } finally {
      promoteLoading.value = false;
    }
  }

  function promptCreateVaultDir(target: { collectionId: string; prefix: string }, preset = '') {
    $q.dialog({
      title: t('knowledgePage.newDirTitle'),
      prompt: {
        model: preset,
        type: 'text',
        label: t('knowledgePage.newDirLabel'),
        isValid: (v: string) => !!v.trim(),
      },
      cancel: true,
      persistent: true,
    }).onOk((name: string) => {
      void (async () => {
        const dirName = name.trim().replace(/^\/+|\/+$/g, '');
        if (!dirName) return;
        try {
          await knowledgeStore.addVaultDir(target.collectionId, `${target.prefix}${dirName}`);
          await deps.explorer.selectTreeNode(
            target.prefix ? dirNodeKey(target.collectionId, target.prefix) : vaultNodeKey(target.collectionId),
          );
          await deps.explorer.refreshTree();
          $q.notify({ type: 'positive', message: t('knowledgePage.newDirSuccess') });
        } catch (e) {
          if (parseKratosApiError(e).status === 409) {
            $q.notify({ type: 'negative', message: t('knowledgePage.dirAlreadyExists', { name: dirName }) });
            promptCreateVaultDir(target, name);
          } else {
            $q.notify({ type: 'negative', message: deps.friendlyError(e) || t('knowledgePage.newDirFailed') });
          }
        }
      })();
    });
  }

  function promptCreateVaultDoc(target: { collectionId: string; prefix: string }, preset = '') {
    $q.dialog({
      title: t('knowledgePage.newDocTitle'),
      prompt: {
        model: preset,
        type: 'text',
        label: t('knowledgePage.newDocLabel'),
        isValid: (v: string) => !!v.trim(),
      },
      cancel: true,
      persistent: true,
    }).onOk((name: string) => {
      void (async () => {
        let docName = name.trim().replace(/^\/+|\/+$/g, '');
        if (!docName) return;
        if (!docName.toLowerCase().endsWith('.md')) docName += '.md';
        try {
          const doc = await knowledgeStore.addVaultDocument(target.collectionId, `${target.prefix}${docName}`);
          await deps.explorer.selectTreeNode(
            target.prefix ? dirNodeKey(target.collectionId, target.prefix) : vaultNodeKey(target.collectionId),
          );
          await deps.explorer.refreshTree();
          deps.explorer.selectDocument(doc.id);
          $q.notify({ type: 'positive', message: t('knowledgePage.newDocSuccess') });
        } catch (e) {
          if (parseKratosApiError(e).status === 409) {
            $q.notify({ type: 'negative', message: t('knowledgePage.docAlreadyExists', { name: docName }) });
            promptCreateVaultDoc(target, name);
          } else {
            $q.notify({ type: 'negative', message: deps.friendlyError(e) || t('knowledgePage.newDocFailed') });
          }
        }
      })();
    });
  }

  function confirmEnableSemantic(collectionId: string) {
    const col = deps.collections.value.find((c) => c.id === collectionId);
    if (!col || col.embedding_model) return;
    $q.dialog({
      title: t('knowledgePage.enableSemanticTitle'),
      message: t('knowledgePage.enableSemanticBody', {
        model: deps.embedderConfig.value?.model ?? '',
        dim: deps.embedderConfig.value?.dim ?? 0,
      }),
      cancel: true,
      persistent: true,
    }).onOk(() => {
      void knowledgeStore
        .enableCollectionSemantic(collectionId)
        .then((r) => {
          $q.notify({ type: 'positive', message: t('knowledgePage.enableSemanticAccepted', { n: r.enqueued_docs }) });
          return deps.loadCollections();
        })
        .catch((e) => $q.notify({ type: 'negative', message: deps.friendlyError(e) }));
    });
  }

  function onTreeNodeAction(
    action: string,
    node: VaultQTreeNode,
    upload: { pendingUploadTarget: Ref<{ collectionId: string; prefix: string } | null> },
  ) {
    const target = { collectionId: node.vaultId, prefix: node.prefix };
    switch (action) {
      case 'new-dir':
        promptCreateVaultDir(target);
        break;
      case 'new-doc':
        promptCreateVaultDoc(target);
        break;
      case 'upload':
        upload.pendingUploadTarget.value = target;
        break;
      case 'refresh':
        deps.explorer.invalidateVault(node.vaultId);
        if (node.vaultId === deps.selectedId.value) void deps.explorer.refreshTree();
        break;
      case 'delete-vault':
        confirmDeleteCollection(node.vaultId);
        break;
      case 'enable-semantic':
        confirmEnableSemantic(node.vaultId);
        break;
    }
  }

  return {
    createOpen,
    createLoading,
    createForm,
    openCreateCollection,
    submitCreateCollection,
    confirmDeleteCollection,
    removedDocId,
    confirmReembed,
    confirmDeleteDocument,
    downloadDocument,
    setDocumentVisibility,
    moveOpen,
    moveLoading,
    movingDoc,
    moveTargetId,
    moveTargetOptions,
    openMoveDialog,
    submitMove,
    promoteOpen,
    promoteLoading,
    promoteTargetId,
    promoteResult,
    promoteDocName,
    promoteOptions,
    promotable,
    openPromoteDialog,
    submitPromote,
    onTreeNodeAction,
  };
}
