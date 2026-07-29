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
import { useVaultExplorer } from './useVaultExplorer';

export function useKnowledgePage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const knowledgeStore = useKnowledgeStore();
  const selectedId = ref('');
  const docsLoading = ref(false);
  const error = ref('');
  const unavailable = ref('');
  // 页面级 Tab：explorer（资源管理器三栏）| search（全库检索调试）| settings（Embedder 配置）
  const pageTab = ref('explorer');
  const createOpen = ref(false);
  const createLoading = ref(false);
  const ingestOpen = ref(false);
  const ingestLoading = ref(false);
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
  const createForm = ref({ name: '', description: '', embedding_model: '', root_path: '' });
  const ingestForm = ref({
    source: '',
    mime_type: 'text/plain',
    text: '',
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
      // P3-2 即时区前端索引：上限放宽到 2000（设计上限 10k 内存索引；树导航为主时 2000 足够覆盖常见 vault）。
      await knowledgeStore.loadDocuments(selectedId.value, { limit: 2000 });
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || '加载文档失败' });
    } finally {
      docsLoading.value = false;
    }
  }

  function selectCollection(id: string) {
    selectedId.value = id;
  }

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
      await loadCollections();
      selectedId.value = col.id;
      $q.notify({ type: 'positive', message: t('knowledgePage.createSuccess') });
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || t('knowledgePage.createFailed') });
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
  // Phase 9：image/png|jpeg|webp 经 VisionExtractor（多模态 LLM）入库。
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
    'image/png',
    'image/jpeg',
    'image/webp',
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
    // Phase 9：图片经 VisionExtractor 入库。
    if (lower.endsWith('.png')) return 'image/png';
    if (lower.endsWith('.jpg') || lower.endsWith('.jpeg')) return 'image/jpeg';
    if (lower.endsWith('.webp')) return 'image/webp';
    return 'application/octet-stream';
  }

  // 粘贴文本入库（文件统一走 DropZone 拖拽队列）。
  async function submitIngest() {
    if (!selectedId.value) return;
    const text = ingestForm.value.text.trim();
    if (!text) {
      $q.notify({ type: 'warning', message: '请粘贴文本内容' });
      return;
    }
    ingestLoading.value = true;
    try {
      await knowledgeStore.ingest({
        collection_id: selectedId.value,
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
      await loadDocuments();
      await loadCollections();
      $q.notify({ type: 'positive', message: '文档已提交入库，正在后台解析...' });
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

  // 不支持的格式（含未放开的图片类型如 gif）给出明确错误，不静默失败。
  function validateUploadMime(mime: string): string | null {
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
  // US-14 免预选：未选中集合时不传 collection_id，后端自动落入「默认知识库」；
  // 上传完成后自动选中该库并刷新列表（存储可分类，使用免选）。
  async function enqueueUploadFiles(files: File[]) {
    if (!files.length) return;
    const targetId = selectedId.value; // 可能为空（免预选 → 默认知识库）
    let firstIngestedCollectionId = '';
    for (const file of files) {
      const mime = inferMime(file);
      const task: KnowledgeUploadTask = {
        id: crypto.randomUUID(),
        name: file.name,
        size: file.size,
        mime_type: mime,
        status: 'reading',
        collection_label: targetId ? undefined : t('knowledgePage.defaultCollectionBadge'),
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
        const doc = await knowledgeStore.ingest({
          collection_id: targetId,
          source: file.name,
          mime_type: mime,
          content_base64: b64,
          organize_to_markdown: true,
        });
        if (!firstIngestedCollectionId) firstIngestedCollectionId = doc.collection_id;
        patchUploadTask(task.id, { status: 'success', message: t('knowledgePage.uploadSubmitted') });
      } catch (e) {
        patchUploadTask(task.id, { status: 'error', message: friendlyError(e) || t('knowledgePage.uploadFailed') });
      }
    }
    await loadCollections();
    if (targetId) {
      await loadDocuments();
    } else if (firstIngestedCollectionId) {
      // 免预选上传：自动选中默认知识库（watch(selectedId) 触发 loadDocuments）。
      selectedId.value = firstIngestedCollectionId;
    }
  }

  function removeUploadTask(id: string) {
    uploadTasks.value = uploadTasks.value.filter((t) => t.id !== id);
  }

  function clearFinishedUploadTasks() {
    uploadTasks.value = uploadTasks.value.filter((t) => t.status === 'reading' || t.status === 'uploading');
  }

  // US-14 检索免选择：searchScopeId 空 = 全部知识库（后端 FederatedRetriever 智能路由），默认全库。
  const searchScopeId = ref('');
  const searchScopeOptions = computed(() => [
    { label: t('knowledgePage.searchScopeAll'), value: '' },
    ...collections.value.map((c) => ({ label: c.name || c.id, value: c.id })),
  ]);

  async function runSearch() {
    if (!searchQuery.value.trim()) return;
    searchLoading.value = true;
    searchRan.value = true;
    try {
      searchResults.value = await knowledgeStore.search({
        collection_id: searchScopeId.value,
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

  // ---------- 文档跨库移动（US-14 整理归档） ----------

  const moveOpen = ref(false);
  const moveLoading = ref(false);
  const movingDoc = ref<KnowledgeDocument | null>(null);
  const moveTargetId = ref('');

  // 目标库选项：排除当前库；dim 不一致的禁用（pgvector 列维度固定，跨 dim 移动向量不可检索）。
  const moveTargetOptions = computed(() => {
    const current = selectedCollection.value;
    return collections.value
      .filter((c) => c.id !== selectedId.value)
      .map((c) => ({
        label: c.name || c.id,
        value: c.id,
        disable: !!current && c.dim !== current.dim,
        dim: c.dim,
      }));
  });

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
      const target = collections.value.find((c) => c.id === moveTargetId.value);
      await loadDocuments();
      await loadCollections();
      $q.notify({ type: 'positive', message: t('knowledgePage.moveSuccess', { name: target?.name ?? '' }) });
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) || t('knowledgePage.moveFailed') });
    } finally {
      moveLoading.value = false;
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
    if (!selectedId.value || pageTab.value !== 'explorer' || !hasIndexingDocuments(documents.value)) return;
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
    () => [documents.value.map((d) => d.status).join(','), pageTab.value, selectedId.value] as const,
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

  // P3 资源管理器：三栏编排（树/列表/详情/双区搜索），与上方共享 selectedId 与 documents。
  const explorer = useVaultExplorer({
    selectedId,
    documents,
    friendlyError,
    notifyError: (message: string) => {
      if (message) $q.notify({ type: 'negative', message });
    },
    semanticErrorFallback: () => t('knowledgePage.searchSemanticError'),
  });

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
    pageTab,
    createOpen,
    createLoading,
    ingestOpen,
    ingestLoading,
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
    searchScopeId,
    searchScopeOptions,
    runSearch,
    moveOpen,
    moveLoading,
    movingDoc,
    moveTargetId,
    moveTargetOptions,
    openMoveDialog,
    submitMove,
    explorer,
  };
}
