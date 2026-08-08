import { computed, onUnmounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import axios from 'axios';
import { hasIndexingDocuments, promoteTargetOptions } from './knowledgeUi';
import { getDocumentContent } from './api';
import { dirNodeKey, vaultNodeKey } from './vaultTreeUi';
import type {
  KnowledgeDocument,
  KnowledgeUploadTask,
  EmbedderConfig,
  PromoteResult,
  UpdateEmbedderConfigInput,
} from './types';
import { useKnowledgeStore } from '../../stores/knowledge';
import { useKnowledgeIngestWs } from './useKnowledgeIngestWs';
import { useVaultExplorer, type VaultQTreeNode } from './useVaultExplorer';
import { parseKratosApiError } from '../../utils/kratosError';

export function useKnowledgePage() {
  const $q = useQuasar();
  const { t } = useI18n();
  const knowledgeStore = useKnowledgeStore();
  const selectedId = ref('');
  const docsLoading = ref(false);
  const error = ref('');
  const unavailable = ref('');
  // 页面级 Tab：explorer（资源管理器三栏）| graph（3D 知识图谱）| settings（Embedder 配置）
  const pageTab = ref('explorer');
  const createOpen = ref(false);
  const createLoading = ref(false);
  const ingestOpen = ref(false);
  const ingestLoading = ref(false);
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

  function confirmDeleteCollection(id?: string) {
    const col = id ? collections.value.find((c) => c.id === id) : selectedCollection.value;
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

  // 粘贴文本入库（文件统一走树 hover「上传文件到此」队列）。
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
  // G1-B3 定向上传：target 指定 {collectionId, prefix} 时，vault 集合落盘到
  // <root>/<prefix>/<文件名>（库根传 '/'）；历史集合（无 root_path）退化为空 target_dir。
  async function enqueueUploadFiles(files: File[], target?: { collectionId: string; prefix: string }) {
    if (!files.length) return;
    let targetId = selectedId.value; // 可能为空（免预选 → 默认知识库）
    let targetDir = '';
    let targetLabel: string | undefined;
    if (target) {
      const col = collections.value.find((c) => c.id === target.collectionId);
      if (col) {
        targetId = col.id;
        targetDir = col.root_path ? target.prefix || '/' : '';
        targetLabel = target.prefix ? `${col.name} / ${target.prefix}` : col.name;
        // 先定位目标目录（单一事实源 {collectionId, prefix}），用户即见文件落点。
        await explorer.selectTreeNode(target.prefix ? dirNodeKey(col.id, target.prefix) : vaultNodeKey(col.id));
      }
    }
    let firstIngestedCollectionId = '';
    for (const file of files) {
      const mime = inferMime(file);
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
        const b64 = await readFileAsBase64(file);
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

  // ---------- G1 树节点操作（hover 菜单） ----------

  /** 上传文件选择器目标（page 监听此 ref 触发隐藏 input.click()）。 */
  const pendingUploadTarget = ref<{ collectionId: string; prefix: string } | null>(null);

  /** page 隐藏 input change 后回传文件列表。 */
  async function onUploadFilesPicked(files: File[]) {
    const target = pendingUploadTarget.value;
    pendingUploadTarget.value = null;
    await enqueueUploadFiles(files, target ?? undefined);
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
          // 定位所在目录并刷新（新建的子目录经 q-tree lazy-load 重载可见）。
          await explorer.selectTreeNode(
            target.prefix ? dirNodeKey(target.collectionId, target.prefix) : vaultNodeKey(target.collectionId),
          );
          await explorer.refreshTree();
          $q.notify({ type: 'positive', message: t('knowledgePage.newDirSuccess') });
        } catch (e) {
          // UX-005：同名冲突本地化提示并重开对话框保留输入；其余错误维持原提示。
          if (parseKratosApiError(e).status === 409) {
            $q.notify({ type: 'negative', message: t('knowledgePage.dirAlreadyExists', { name: dirName }) });
            promptCreateVaultDir(target, name);
          } else {
            $q.notify({ type: 'negative', message: friendlyError(e) || t('knowledgePage.newDirFailed') });
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
          await explorer.selectTreeNode(
            target.prefix ? dirNodeKey(target.collectionId, target.prefix) : vaultNodeKey(target.collectionId),
          );
          await explorer.refreshTree();
          explorer.selectDocument(doc.id);
          $q.notify({ type: 'positive', message: t('knowledgePage.newDocSuccess') });
        } catch (e) {
          // UX-005：同名冲突本地化提示并重开对话框保留输入；其余错误维持原提示。
          if (parseKratosApiError(e).status === 409) {
            $q.notify({ type: 'negative', message: t('knowledgePage.docAlreadyExists', { name: docName }) });
            promptCreateVaultDoc(target, name);
          } else {
            $q.notify({ type: 'negative', message: friendlyError(e) || t('knowledgePage.newDocFailed') });
          }
        }
      })();
    });
  }

  /** 树节点 hover 菜单分发（G1：动作与节点类别合法性已由组件裁剪）。 */
  function onTreeNodeAction(action: string, node: VaultQTreeNode) {
    const target = { collectionId: node.vaultId, prefix: node.prefix };
    switch (action) {
      case 'new-dir':
        promptCreateVaultDir(target);
        break;
      case 'new-doc':
        promptCreateVaultDoc(target);
        break;
      case 'upload':
        pendingUploadTarget.value = target;
        break;
      case 'refresh':
        explorer.invalidateVault(node.vaultId);
        if (node.vaultId === selectedId.value) void explorer.refreshTree();
        break;
      case 'delete-vault':
        confirmDeleteCollection(node.vaultId);
        break;
    }
  }

  function removeUploadTask(id: string) {
    uploadTasks.value = uploadTasks.value.filter((t) => t.id !== id);
  }

  function clearFinishedUploadTasks() {
    uploadTasks.value = uploadTasks.value.filter((t) => t.status === 'reading' || t.status === 'uploading');
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

  // ---------- SP1-G/I-3：晋升到团队库（文档级，doc_ids 入口） ----------

  const promoteOpen = ref(false);
  const promoteLoading = ref(false);
  const promoteTargetId = ref('');
  const promoteResult = ref<PromoteResult | null>(null);
  /** 晋升源文档名（对话框副标题展示；docId 由 explorer.selectedDocId 提供）。 */
  const promoteDocName = ref('');

  // 目标库选项：仅 team 库，排除当前库（后端同库/非 team 均拒绝，前端先行过滤）。
  const promoteOptions = computed(() => promoteTargetOptions(collections.value, selectedId.value));

  // 晋升入口可见性：当前库非 team（团队库再晋升无意义）+ 已选中文件。
  const promotable = computed(
    () =>
      !!selectedCollection.value && selectedCollection.value.vault_backend !== 'team' && !!explorer.selectedDocId.value,
  );

  function openPromoteDialog() {
    const node = explorer.selectedNode.value;
    if (!node || node.kind !== 'file') return;
    promoteDocName.value = node.path || node.name;
    promoteTargetId.value = promoteOptions.value[0]?.value ?? '';
    promoteResult.value = null;
    promoteOpen.value = true;
  }

  async function submitPromote() {
    const docId = explorer.selectedDocId.value;
    if (!docId || !promoteTargetId.value) return;
    promoteLoading.value = true;
    try {
      promoteResult.value = await knowledgeStore.promoteDocs([docId], promoteTargetId.value);
      // 目标库文档计数变化（新建目标文档）；源侧谱系字段回写不改变列表。
      await loadCollections();
    } catch (e) {
      promoteOpen.value = false;
      $q.notify({ type: 'negative', message: friendlyError(e) || t('knowledgePage.promoteFailed') });
    } finally {
      promoteLoading.value = false;
    }
  }

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
    collections,
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
    friendlyError,
    loading,
    docsLoading,
    error,
    unavailable,
    pageTab,
    createOpen,
    createLoading,
    ingestOpen,
    ingestLoading,
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
    pendingUploadTarget,
    onUploadFilesPicked,
    onTreeNodeAction,
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
    explorer,
  };
}
