import { ref, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { KnowledgeCollection, KnowledgeUploadTask } from './types';
import { useKnowledgeStore } from '../../stores/knowledge';
import { dirNodeKey, vaultNodeKey } from './vaultTreeUi';
import { inferUploadMime, isExtractSupported, readFileAsBase64, utf8ToBase64 } from './knowledgeUploadCodec';

export type KnowledgeUploadDeps = {
  selectedId: Ref<string>;
  collections: { value: KnowledgeCollection[] };
  loadDocuments: () => Promise<void>;
  loadCollections: () => Promise<void>;
  friendlyError: (err: unknown) => string;
  selectTreeNode: (key: string) => Promise<void> | void;
};

export function useKnowledgeUpload(deps: KnowledgeUploadDeps) {
  const $q = useQuasar();
  const { t } = useI18n();
  const knowledgeStore = useKnowledgeStore();

  const ingestOpen = ref(false);
  const ingestLoading = ref(false);
  const ingestForm = ref({
    source: '',
    mime_type: 'text/plain',
    text: '',
    chunk_strategy: '',
    chunk_size: 0,
    chunk_overlap: 0,
  });
  const uploadTasks = ref<KnowledgeUploadTask[]>([]);
  const pendingUploadTarget = ref<{ collectionId: string; prefix: string } | null>(null);

  function validateUploadMime(mime: string): string | null {
    if (!isExtractSupported(mime)) {
      return t('knowledgePage.uploadUnsupportedFormat', { mime });
    }
    return null;
  }

  function patchUploadTask(id: string, patch: Partial<KnowledgeUploadTask>) {
    const i = uploadTasks.value.findIndex((task) => task.id === id);
    if (i >= 0) {
      uploadTasks.value[i] = { ...uploadTasks.value[i], ...patch };
    }
  }

  async function submitIngest() {
    const text = ingestForm.value.text.trim();
    if (!text) {
      $q.notify({ type: 'warning', message: '请粘贴文本内容' });
      return;
    }
    ingestLoading.value = true;
    try {
      const doc = await knowledgeStore.ingest({
        collection_id: deps.selectedId.value || undefined,
        source: ingestForm.value.source || 'paste',
        mime_type: ingestForm.value.mime_type || 'text/plain',
        content_base64: utf8ToBase64(text),
        chunk_strategy: ingestForm.value.chunk_strategy || undefined,
        chunk_size: ingestForm.value.chunk_size || undefined,
        chunk_overlap: ingestForm.value.chunk_overlap || undefined,
        organize_to_markdown: true,
      });
      ingestOpen.value = false;
      ingestForm.value = {
        source: '',
        mime_type: 'text/plain',
        text: '',
        chunk_strategy: '',
        chunk_size: 0,
        chunk_overlap: 0,
      };
      if (!deps.selectedId.value && doc.collection_id) {
        deps.selectedId.value = doc.collection_id;
      }
      await deps.loadDocuments();
      await deps.loadCollections();
      $q.notify({ type: 'positive', message: '文档已提交入库，正在后台解析...' });
    } catch (e) {
      $q.notify({ type: 'negative', message: deps.friendlyError(e) || '入库失败' });
    } finally {
      ingestLoading.value = false;
    }
  }

  async function enqueueUploadFiles(files: File[], target?: { collectionId: string; prefix: string }) {
    if (!files.length) return;
    let targetId = deps.selectedId.value;
    let targetDir = '';
    let targetLabel: string | undefined;
    if (target) {
      const col = deps.collections.value.find((c) => c.id === target.collectionId);
      if (col) {
        targetId = col.id;
        targetDir = col.root_path ? target.prefix || '/' : '';
        targetLabel = target.prefix ? `${col.name} / ${target.prefix}` : col.name;
        await deps.selectTreeNode(target.prefix ? dirNodeKey(col.id, target.prefix) : vaultNodeKey(col.id));
      }
    }
    let firstIngestedCollectionId = '';
    for (const file of files) {
      const mime = inferUploadMime(file);
      const task: KnowledgeUploadTask = {
        id: crypto.randomUUID(),
        name: file.name,
        size: file.size,
        mime_type: mime,
        status: 'reading',
        collection_label: targetLabel ?? (targetId ? undefined : t('knowledgePage.defaultCollectionBadge')),
      };
      uploadTasks.value.push(task);
      const invalid = validateUploadMime(mime);
      if (invalid) {
        patchUploadTask(task.id, { status: 'error', message: invalid });
        continue;
      }
      try {
        const b64 = await readFileAsBase64(file).catch(() => {
          throw new Error(t('knowledgePage.uploadReadFailed'));
        });
        patchUploadTask(task.id, { status: 'uploading' });
        const doc = await knowledgeStore.ingest({
          collection_id: targetId,
          source: file.name,
          mime_type: mime,
          content_base64: b64,
          organize_to_markdown: true,
          target_dir: targetDir || undefined,
        });
        if (!firstIngestedCollectionId) firstIngestedCollectionId = doc.collection_id;
        patchUploadTask(task.id, { status: 'success', message: t('knowledgePage.uploadSubmitted') });
      } catch (e) {
        patchUploadTask(task.id, { status: 'error', message: deps.friendlyError(e) || t('knowledgePage.uploadFailed') });
      }
    }
    await deps.loadCollections();
    if (targetId) {
      await deps.loadDocuments();
    } else if (firstIngestedCollectionId) {
      deps.selectedId.value = firstIngestedCollectionId;
    }
  }

  async function onUploadFilesPicked(files: File[]) {
    const target = pendingUploadTarget.value;
    pendingUploadTarget.value = null;
    await enqueueUploadFiles(files, target ?? undefined);
  }

  function removeUploadTask(id: string) {
    uploadTasks.value = uploadTasks.value.filter((task) => task.id !== id);
  }

  function clearFinishedUploadTasks() {
    uploadTasks.value = uploadTasks.value.filter((task) => task.status === 'reading' || task.status === 'uploading');
  }

  return {
    ingestOpen,
    ingestLoading,
    ingestForm,
    submitIngest,
    uploadTasks,
    pendingUploadTarget,
    enqueueUploadFiles,
    onUploadFilesPicked,
    removeUploadTask,
    clearFinishedUploadTasks,
  };
}
