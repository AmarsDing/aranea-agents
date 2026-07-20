import { computed, onUnmounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { hasIndexingDocuments } from './knowledgeUi';
import { getDocumentContent } from './api';
import type {
  KnowledgeChunk,
  KnowledgeDocument,
  KnowledgeUploadTask,
  EmbedderConfig,
  UpdateEmbedderConfigInput,
} from './types';
import { useKnowledgeStore } from '../../stores/knowledge';
import { useKnowledgeIngestWs } from './useKnowledgeIngestWs';

export function useKnowledgePage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const knowledgeStore = useKnowledgeStore();
  const selectedId = ref('');
  const docsLoading = ref(false);
  const error = ref('');
  const unavailable = ref('');
  const tab = ref('documents');
  const createOpen = ref(false);
  const createLoading = ref(false);
  const ingestOpen = ref(false);
  const ingestLoading = ref(false);
  const ingestFile = ref<File | null>(null);
  const searchQuery = ref('');
  const searchTopK = ref(5);
  const searchMinScore = ref(0);
  const searchHybridMode = ref('auto');
  const searchRewriteStrategy = ref('');
  const searchUseRerank = ref(false);
  const searchResults = ref<KnowledgeChunk[]>([]);
  const searchLoading = ref(false);
  const searchRan = ref(false);
  const embedderSaving = ref(false);
  const createForm = ref({ name: '', description: '', embedding_model: 'text-embedding-3-small' });
  const ingestForm = ref({
    source: '',
    mime_type: 'text/plain',
    text: '',
    fileContentBase64: '',
    chunk_strategy: '',
    chunk_size: 0,
    chunk_overlap: 0,
  });

  const embedderConfig = computed<EmbedderConfig | null>(() => knowledgeStore.embedderConfig);
  const collections = computed(() => knowledgeStore.collections);
  const loading = computed(() => knowledgeStore.loading);
  const documents = computed(() => knowledgeStore.documentsByCollection[selectedId.value] ?? []);
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
    if (axios.isAxiosError(err)) {
      const status = err.response?.status;
      if (status === 404) {
        return '知识库 API 未找到 (404)。请确认：① 页面使用 http://localhost:9001/knowledge（勿直接打开 :8000）；② 已重启 admin（go run ./cmd/admin）；③ 请求路径为 /v1/knowledge/... 而非 /api/v1/...。';
      }
      if (!err.response && (err.code === 'ERR_NETWORK' || err.message === 'Network Error')) {
        return '无法连接后端。请确认 admin 在 :8000 运行，并通过 http://localhost:9001 打开页面。';
      }
      const data = err.response?.data as { message?: string } | undefined;
      if (typeof data?.message === 'string' && data.message.trim()) {
        return data.message;
      }
    }
    const msg = err instanceof Error ? err.message : String(err);
    if (/unavailable|pgvector|postgres|not configured/i.test(msg)) {
      unavailable.value = msg;
      return '';
    }
    return msg;
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
      error.value = friendlyError(e) || '加载集合失败';
    }
  }

  async function loadDocuments() {
    if (!selectedId.value) return;
    docsLoading.value = true;
    try {
      await knowledgeStore.loadDocuments(selectedId.value, { limit: 100 });
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || '加载文档失败' });
    } finally {
      docsLoading.value = false;
    }
  }

  function selectCollection(id: string) {
    selectedId.value = id;
    tab.value = 'documents';
  }

  function openCreateCollection() {
    createForm.value = { name: '', description: '', embedding_model: 'text-embedding-3-small' };
    createOpen.value = true;
  }

  async function submitCreateCollection() {
    if (!createForm.value.name.trim()) {
      $q.notify({ type: 'warning', message: '请填写名称' });
      return;
    }
    createLoading.value = true;
    try {
      const col = await knowledgeStore.addCollection({
        name: createForm.value.name.trim(),
        description: createForm.value.description.trim(),
        embedding_model: createForm.value.embedding_model.trim(),
      });
      createOpen.value = false;
      await loadCollections();
      selectedId.value = col.id;
      $q.notify({ type: 'positive', message: '集合已创建' });
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || '创建失败' });
    } finally {
      createLoading.value = false;
    }
  }

  function confirmDeleteCollection() {
    const col = selectedCollection.value;
    if (!col) return;
    $q.dialog({
      title: '删除集合',
      message: `确定删除「${col.name}」及其全部文档？`,
      cancel: true,
      persistent: true,
    }).onOk(() => void deleteCollectionConfirmed(col.id));
  }

  async function deleteCollectionConfirmed(id: string) {
    try {
      await knowledgeStore.removeCollection(id);
      if (selectedId.value === id) {
        selectedId.value = '';
      }
      await loadCollections();
      $q.notify({ type: 'positive', message: '已删除' });
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || '删除失败' });
    }
  }

  // DAT-04 / KB-01 helpers: byte-faithful base64 for both file uploads (any
  // binary format) and pasted text (UTF-8). Prior code went through
  // FileReader.readAsText, which mangled non-ASCII / binary payloads.
  function bytesToBase64(bytes: Uint8Array): string {
    const CHUNK = 0x8000;
    let binary = '';
    for (let i = 0; i < bytes.length; i += CHUNK) {
      const slice = bytes.subarray(i, Math.min(i + CHUNK, bytes.length));
      binary += String.fromCharCode.apply(null, slice as unknown as number[]);
    }
    return btoa(binary);
  }

  function utf8ToBase64(str: string): string {
    return bytesToBase64(new TextEncoder().encode(str));
  }

  // REV-D: mirrors backend extractSupportedMimes — keep in sync with
  // internal/service/knowledge.go:extractSupportedMimes.
  const EXTRACT_SUPPORTED_MIMES = new Set([
    'text/plain',
    'text/markdown',
    'text/csv',
    'text/html',
    'text/xml',
    'application/json',
    'application/xml',
    'application/pdf',
    'application/msword',
    'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    'application/vnd.openxmlformats-officedocument.presentationml.presentation',
  ]);

  function isExtractSupported(mimeType: string): boolean {
    const m = mimeType.trim().toLowerCase();
    return EXTRACT_SUPPORTED_MIMES.has(m) || m.startsWith('text/');
  }

  function inferMime(file: File): string {
    if (file.type && file.type.trim()) return file.type;
    const lower = file.name.toLowerCase();
    if (lower.endsWith('.md')) return 'text/markdown';
    if (lower.endsWith('.txt') || lower.endsWith('.log')) return 'text/plain';
    if (lower.endsWith('.json')) return 'application/json';
    if (lower.endsWith('.csv')) return 'text/csv';
    if (lower.endsWith('.html') || lower.endsWith('.htm')) return 'text/html';
    if (lower.endsWith('.xml')) return 'application/xml';
    if (lower.endsWith('.yaml') || lower.endsWith('.yml')) return 'text/yaml';
    if (lower.endsWith('.toml')) return 'text/toml';
    if (lower.endsWith('.pdf')) return 'application/pdf';
    if (lower.endsWith('.docx')) return 'application/vnd.openxmlformats-officedocument.wordprocessingml.document';
    if (lower.endsWith('.doc')) return 'application/msword';
    if (lower.endsWith('.pptx')) return 'application/vnd.openxmlformats-officedocument.presentationml.presentation';
    if (lower.endsWith('.xlsx')) return 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet';
    // TODO(debt): image MIME removed until OCR is implemented on the backend.
    return 'application/octet-stream';
  }

  function onIngestFile(file: File | File[] | null) {
    const picked = Array.isArray(file) ? file[0] : file;
    ingestForm.value.fileContentBase64 = '';
    if (!picked) {
      ingestFile.value = null;
      return;
    }
    ingestFile.value = picked;
    ingestForm.value.source = ingestForm.value.source || picked.name;
    const inferred = inferMime(picked);
    ingestForm.value.mime_type = inferred;
    const reader = new FileReader();
    // ArrayBuffer keeps binary payloads (PDF/DOCX/…) intact; we still surface a
    // textarea preview for text-like MIME so the user can review before submit.
    reader.onload = () => {
      const buf = reader.result as ArrayBuffer | null;
      if (!buf) return;
      const bytes = new Uint8Array(buf);
      ingestForm.value.fileContentBase64 = bytesToBase64(bytes);
      const textLike = /^text\//i.test(inferred) || inferred === 'application/json' || inferred === 'application/xml';
      if (textLike) {
        try {
          ingestForm.value.text = new TextDecoder('utf-8', { fatal: false }).decode(buf);
        } catch (_e) {
          ingestForm.value.text = '';
        }
      } else {
        ingestForm.value.text = '';
      }
    };
    reader.readAsArrayBuffer(picked);
  }

  async function submitIngest() {
    if (!selectedId.value) return;
    const fileBase64 = ingestForm.value.fileContentBase64;
    const text = ingestForm.value.text.trim();
    if (!fileBase64 && !text) {
      $q.notify({ type: 'warning', message: '请提供文件或文本内容' });
      return;
    }
    ingestLoading.value = true;
    try {
      // Prefer pre-encoded file base64 (binary-safe); fall back to UTF-8 text encoding.
      const b64 = fileBase64 || utf8ToBase64(text);
      await knowledgeStore.ingest({
        collection_id: selectedId.value,
        source: ingestForm.value.source || 'upload',
        mime_type: ingestForm.value.mime_type || 'text/plain',
        content_base64: b64,
        chunk_strategy: ingestForm.value.chunk_strategy || undefined,
        chunk_size: ingestForm.value.chunk_size || undefined,
        chunk_overlap: ingestForm.value.chunk_overlap || undefined,
        organize_to_markdown: true,
      });
      ingestOpen.value = false;
      const submittedMime = ingestForm.value.mime_type || 'text/plain';
      ingestForm.value = {
        source: '',
        mime_type: 'text/plain',
        text: '',
        fileContentBase64: '',
        chunk_strategy: '',
        chunk_size: 0,
        chunk_overlap: 0,
      };
      ingestFile.value = null;
      await loadDocuments();
      await loadCollections();
      // REV-D: binary formats that require server-side extraction get a softer
      // success message so users know search availability depends on parsing.
      if (isExtractSupported(submittedMime)) {
        $q.notify({ type: 'positive', message: '文档已提交入库，正在后台解析...' });
      } else {
        $q.notify({
          type: 'warning',
          message: `文档已上传（${submittedMime}），但当前后端可能不支持解析该格式，检索内容可能为空。`,
          timeout: 8000,
        });
      }
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || '入库失败' });
    } finally {
      ingestLoading.value = false;
    }
  }

  function confirmDeleteDocument(doc: KnowledgeDocument) {
    $q.dialog({
      title: '删除文档',
      message: `删除「${doc.source}」？`,
      cancel: true,
    }).onOk(async () => {
      try {
        await knowledgeStore.removeDocument(doc.id, selectedId.value);
        await loadDocuments();
        await loadCollections();
        $q.notify({ type: 'positive', message: '已删除' });
      } catch (e) {
        $q.notify({ type: 'negative', message: friendlyError(e) || '删除失败' });
      }
    });
  }

  // ---------- 文档预览（GetDocumentContent） ----------

  const previewOpen = ref(false);
  const previewLoading = ref(false);
  const previewDoc = ref<KnowledgeDocument | null>(null);
  const previewContent = ref('');
  const previewOrganized = ref(false);

  async function openDocPreview(doc: KnowledgeDocument) {
    previewDoc.value = doc;
    previewContent.value = '';
    previewOrganized.value = false;
    previewOpen.value = true;
    previewLoading.value = true;
    try {
      const res = await getDocumentContent(doc.id);
      previewContent.value = res.content_text;
      previewOrganized.value = res.organized;
    } catch (e) {
      previewOpen.value = false;
      $q.notify({ type: 'negative', message: friendlyError(e) || t('knowledgePage.previewLoadFailed') });
    } finally {
      previewLoading.value = false;
    }
  }

  // ---------- 拖拽批量上传队列 ----------

  const uploadTasks = ref<KnowledgeUploadTask[]>([]);

  function patchUploadTask(id: string, patch: Partial<KnowledgeUploadTask>) {
    const i = uploadTasks.value.findIndex((t) => t.id === id);
    if (i >= 0) {
      uploadTasks.value[i] = { ...uploadTasks.value[i], ...patch };
    }
  }

  // Phase 8 验收：图片等多模态格式必须给出明确错误，不静默失败。
  function validateUploadMime(mime: string): string | null {
    if (mime.startsWith('image/')) {
      return t('knowledgePage.uploadImageUnsupported');
    }
    if (!isExtractSupported(mime)) {
      return t('knowledgePage.uploadUnsupportedFormat', { mime });
    }
    return null;
  }

  function readFileAsBase64(file: File): Promise<string> {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => {
        const buf = reader.result as ArrayBuffer | null;
        if (!buf) {
          reject(new Error(t('knowledgePage.uploadReadFailed')));
          return;
        }
        resolve(bytesToBase64(new Uint8Array(buf)));
      };
      reader.onerror = () => reject(new Error(t('knowledgePage.uploadReadFailed')));
      reader.readAsArrayBuffer(file);
    });
  }

  // 顺序上传：逐文件读取 → 入库，避免并发冲击后端；WS 事件并行刷新文档列表。
  async function enqueueUploadFiles(files: File[]) {
    if (!selectedId.value || !files.length) return;
    const collectionId = selectedId.value;
    for (const file of files) {
      const mime = inferMime(file);
      const task: KnowledgeUploadTask = {
        id: crypto.randomUUID(),
        name: file.name,
        size: file.size,
        mime_type: mime,
        status: 'reading',
      };
      uploadTasks.value.push(task);
      const invalid = validateUploadMime(mime);
      if (invalid) {
        patchUploadTask(task.id, { status: 'error', message: invalid });
        continue;
      }
      try {
        const b64 = await readFileAsBase64(file);
        patchUploadTask(task.id, { status: 'uploading' });
        await knowledgeStore.ingest({
          collection_id: collectionId,
          source: file.name,
          mime_type: mime,
          content_base64: b64,
          organize_to_markdown: true,
        });
        patchUploadTask(task.id, { status: 'success', message: t('knowledgePage.uploadSubmitted') });
      } catch (e) {
        patchUploadTask(task.id, { status: 'error', message: friendlyError(e) || t('knowledgePage.uploadFailed') });
      }
    }
    await loadDocuments();
    await loadCollections();
  }

  function removeUploadTask(id: string) {
    uploadTasks.value = uploadTasks.value.filter((t) => t.id !== id);
  }

  function clearFinishedUploadTasks() {
    uploadTasks.value = uploadTasks.value.filter((t) => t.status === 'reading' || t.status === 'uploading');
  }

  async function runSearch() {
    if (!selectedId.value || !searchQuery.value.trim()) return;
    searchLoading.value = true;
    searchRan.value = true;
    try {
      searchResults.value = await knowledgeStore.search({
        collection_id: selectedId.value,
        query: searchQuery.value.trim(),
        top_k: searchTopK.value,
        min_score: searchMinScore.value || undefined,
        hybrid_search: searchHybridMode.value || undefined,
        rewrite_strategy: searchRewriteStrategy.value || undefined,
        use_rerank: searchUseRerank.value ? true : undefined,
      });
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || '检索失败' });
    } finally {
      searchLoading.value = false;
    }
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
    if (!selectedId.value || tab.value !== 'documents' || !hasIndexingDocuments(documents.value)) return;
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
    () => [documents.value.map((d) => d.status).join(','), tab.value, selectedId.value] as const,
    () => syncDocPoll(),
  );

  onUnmounted(stopDocPoll);

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
    } catch (_e) {
      $q.notify({ type: 'warning', message: 'Embedder 配置加载失败，检索功能可能不可用' });
    }
  }

  return {
    collections,
    selectedId,
    selectedCollection,
    documents,
    docSourceMap,
    loading,
    docsLoading,
    error,
    unavailable,
    tab,
    createOpen,
    createLoading,
    ingestOpen,
    ingestLoading,
    ingestFile,
    searchQuery,
    searchTopK,
    searchMinScore,
    searchHybridMode,
    searchRewriteStrategy,
    searchUseRerank,
    searchResults,
    searchLoading,
    searchRan,
    createForm,
    ingestForm,
    embedderConfig,
    embedderSaving,
    saveEmbedderConfig,
    loadCollections,
    loadDocuments,
    loadEmbedderConfig,
    selectCollection,
    openCreateCollection,
    submitCreateCollection,
    confirmDeleteCollection,
    onIngestFile,
    submitIngest,
    confirmDeleteDocument,
    previewOpen,
    previewLoading,
    previewDoc,
    previewContent,
    previewOrganized,
    openDocPreview,
    uploadTasks,
    enqueueUploadFiles,
    removeUploadTask,
    clearFinishedUploadTasks,
    runSearch,
  };
}
