/**
 * useVaultExplorer：P3 资源管理器编排 composable。
 *
 * 职责（数据流铁律：组件不直接调 api/store）：
 * - 选中态：vault（与 useKnowledgePage 共享 selectedId）/ prefix / doc
 * - 文件夹树：根节点加载 + q-tree 懒加载，缓存经 store.treeChildren
 * - 中栏列表：当前 prefix 直接子节点（文件）
 * - 右栏详情：二级展开时加载正文预览 + 已解析关联（force 刷新，避免索引后脏缓存）
 * - 统一搜索框双区：即时区（instantMatch 前端索引）+ 语义区（回车走后端 Search）
 * - 降级：树接口失败时树区显式报错；中栏回退为 documents 平铺（rel_path 过滤）
 */
import { computed, ref, watch, type Ref } from 'vue';
import { useKnowledgeStore } from '../../stores/knowledge';
import { getDocumentContent } from './api';
import { classifySearchIntent, type SearchIntent } from './searchIntent';
import { instantFilter } from './instantMatch';
import type { KnowledgeChunk, KnowledgeDocument, KnowledgeLink, VaultTreeNode } from './types';

/** q-tree 节点（仅目录；文件在中栏列表展示）。 */
export interface VaultQTreeNode {
  label: string;
  key: string;
  icon: string;
  lazy?: boolean;
  children?: VaultQTreeNode[];
}

/** Quasar q-tree @lazy-load 载荷（组件透传，composable 处理）。 */
export interface VaultLazyLoadPayload {
  key: string;
  done: (children: VaultQTreeNode[]) => void;
  fail: (error: unknown) => void;
}

function dirToQNode(n: VaultTreeNode): VaultQTreeNode {
  return { label: n.name, key: n.path, icon: 'folder', lazy: true, children: [] };
}

/** relPath → 所在目录 prefix（'/' 分隔，带尾斜杠；根为 ''）。 */
export function dirPrefixOf(relPath: string): string {
  const i = relPath.lastIndexOf('/');
  return i < 0 ? '' : relPath.slice(0, i + 1);
}

/** KnowledgeDocument → VaultTreeNode（即时搜索跳转时中栏未加载该 prefix 的兜底）。 */
export function docToTreeNode(d: KnowledgeDocument): VaultTreeNode {
  return {
    name: d.source,
    path: d.rel_path || d.source,
    kind: 'file',
    doc_id: d.id,
    summary: d.summary,
    tags: d.tags ?? [],
    doc_type: d.doc_type,
    status: d.status,
    size_bytes: d.size_bytes,
    updated_at: d.updated_at,
    error_message: d.error_message,
  };
}

export function useVaultExplorer(input: {
  selectedId: Ref<string>;
  documents: Ref<KnowledgeDocument[]>;
  friendlyError: (err: unknown) => string;
  notifyError: (message: string) => void;
}) {
  const knowledgeStore = useKnowledgeStore();
  const { selectedId, documents, friendlyError, notifyError } = input;

  // ---------- 树与中栏列表 ----------

  const currentPrefix = ref('');
  const rootNodes = ref<VaultQTreeNode[]>([]);
  const currentChildren = ref<VaultTreeNode[]>([]);
  const treeLoading = ref(false);
  const treeError = ref('');

  const currentFiles = computed(() => currentChildren.value.filter((n) => n.kind === 'file'));
  const currentDirCount = computed(() => currentChildren.value.filter((n) => n.kind === 'dir').length);

  async function loadRoot(force = false) {
    if (!selectedId.value) {
      rootNodes.value = [];
      return;
    }
    try {
      const items = await knowledgeStore.loadVaultTree(selectedId.value, '', force);
      rootNodes.value = items.filter((n) => n.kind === 'dir').map(dirToQNode);
      treeError.value = '';
    } catch (e) {
      treeError.value = friendlyError(e) || 'tree load failed';
      rootNodes.value = [];
    }
  }

  async function loadCurrent(force = false) {
    if (!selectedId.value) {
      currentChildren.value = [];
      return;
    }
    treeLoading.value = true;
    try {
      currentChildren.value = await knowledgeStore.loadVaultTree(selectedId.value, currentPrefix.value, force);
      treeError.value = '';
    } catch (e) {
      // 降级：树接口失败时中栏回退为 documents 平铺（按 rel_path 过滤）。
      treeError.value = friendlyError(e) || 'tree load failed';
      currentChildren.value = documents.value
        .filter((d) => dirPrefixOf(d.rel_path) === currentPrefix.value)
        .map(docToTreeNode);
    } finally {
      treeLoading.value = false;
    }
  }

  /** q-tree 懒加载：返回 key 目录下的子目录（文件不进树）。 */
  async function onLazyLoad({ key, done, fail }: VaultLazyLoadPayload) {
    try {
      const items = await knowledgeStore.loadVaultTree(selectedId.value, key);
      done(items.filter((n) => n.kind === 'dir').map(dirToQNode));
    } catch (e) {
      fail(e);
    }
  }

  async function selectPrefix(prefix: string) {
    if (prefix === currentPrefix.value && currentChildren.value.length) return;
    currentPrefix.value = prefix;
    selectedDocId.value = '';
    detailExpanded.value = false;
    await loadCurrent();
  }

  async function refreshTree() {
    await Promise.all([loadRoot(true), loadCurrent(true)]);
  }

  // ---------- 右栏详情（两级密度） ----------

  const selectedDocId = ref('');
  const detailExpanded = ref(false);
  const previewContent = ref('');
  const previewOrganized = ref(false);
  const previewLoading = ref(false);
  const links = ref<KnowledgeLink[]>([]);
  const linksLoading = ref(false);

  /** 选中文档节点：优先中栏当前文件；兜底 documents（即时搜索跳转场景）。 */
  const selectedNode = computed<VaultTreeNode | null>(() => {
    if (!selectedDocId.value) return null;
    const fromList = currentFiles.value.find((n) => n.doc_id === selectedDocId.value);
    if (fromList) return fromList;
    const fromDocs = documents.value.find((d) => d.id === selectedDocId.value);
    return fromDocs ? docToTreeNode(fromDocs) : null;
  });

  function selectDocument(docId: string) {
    selectedDocId.value = docId;
  }

  /** 即时/语义搜索跳转：定位到文档所在目录并选中。 */
  async function navigateToDocument(docId: string, relPath: string) {
    const prefix = dirPrefixOf(relPath);
    if (prefix !== currentPrefix.value) {
      currentPrefix.value = prefix;
      await loadCurrent();
    }
    selectedDocId.value = docId;
  }

  /** 二级展开：加载正文预览 + 关联（force 刷新，索引完成后关联可能变化）。 */
  async function toggleDetail() {
    detailExpanded.value = !detailExpanded.value;
    if (!detailExpanded.value || !selectedDocId.value) return;
    previewLoading.value = true;
    linksLoading.value = true;
    try {
      const res = await getDocumentContent(selectedDocId.value);
      previewContent.value = res.content_text;
      previewOrganized.value = res.organized;
    } catch (e) {
      previewContent.value = '';
      notifyError(friendlyError(e));
    } finally {
      previewLoading.value = false;
    }
    try {
      links.value = await knowledgeStore.loadDocumentLinks(selectedDocId.value, '', true);
    } catch (e) {
      links.value = [];
      notifyError(friendlyError(e));
    } finally {
      linksLoading.value = false;
    }
  }

  // ---------- 统一搜索框双区 ----------

  const searchQuery = ref('');
  const semanticResults = ref<KnowledgeChunk[]>([]);
  const semanticLoading = ref(false);
  const semanticRan = ref(false);

  const searchIntent = computed<SearchIntent>(() => classifySearchIntent(searchQuery.value));

  /** 即时区：意图为 semantic 时不展示（强问句信号 → 只等回车走语义）。 */
  const instantResults = computed<KnowledgeDocument[]>(() => {
    if (searchIntent.value === 'semantic') return [];
    return instantFilter(documents.value, searchQuery.value, (d) => [
      d.source,
      d.rel_path,
      d.summary,
      (d.tags ?? []).join(' '),
      d.doc_type,
    ]);
  });

  const showInstantZone = computed(() => searchQuery.value.trim() !== '' && searchIntent.value !== 'semantic');
  const showSemanticZone = computed(() => searchQuery.value.trim() !== '' && searchIntent.value !== 'instant');

  const docSourceMap = computed(() => {
    const map: Record<string, string> = {};
    for (const d of documents.value) map[d.id] = d.source;
    return map;
  });

  /** 语义区：回车触发，走当前 vault 的后端 Search（亚秒）。 */
  async function runSemanticSearch() {
    const q = searchQuery.value.trim();
    if (!q || !selectedId.value) return;
    semanticLoading.value = true;
    semanticRan.value = true;
    try {
      semanticResults.value = await knowledgeStore.search({ collection_id: selectedId.value, query: q, top_k: 8 });
    } catch (e) {
      semanticResults.value = [];
      notifyError(friendlyError(e) || 'search failed');
    } finally {
      semanticLoading.value = false;
    }
  }

  function clearSearch() {
    searchQuery.value = '';
    semanticResults.value = [];
    semanticRan.value = false;
  }

  function selectInstant(doc: KnowledgeDocument) {
    clearSearch();
    void navigateToDocument(doc.id, doc.rel_path);
  }

  function selectSemanticChunk(chunk: KnowledgeChunk) {
    const doc = documents.value.find((d) => d.id === chunk.doc_id);
    clearSearch();
    void navigateToDocument(chunk.doc_id, doc?.rel_path ?? '');
  }

  // ---------- 联动 ----------

  // vault 切换：复位选中态与搜索，重载树。
  watch(selectedId, () => {
    currentPrefix.value = '';
    selectedDocId.value = '';
    detailExpanded.value = false;
    links.value = [];
    clearSearch();
    void loadRoot();
    void loadCurrent();
  });

  // 文档结构/状态变化（上传、入库完成、删除）→ 强制刷新树缓存。
  watch(
    () => documents.value.map((d) => `${d.id}:${d.status}:${d.updated_at}`).join(','),
    (sig, prev) => {
      if (sig !== prev) void refreshTree();
    },
  );

  return {
    // 树/列表
    currentPrefix,
    rootNodes,
    currentChildren,
    currentFiles,
    currentDirCount,
    treeLoading,
    treeError,
    onLazyLoad,
    selectPrefix,
    refreshTree,
    // 详情
    selectedDocId,
    selectedNode,
    detailExpanded,
    previewContent,
    previewOrganized,
    previewLoading,
    links,
    linksLoading,
    selectDocument,
    navigateToDocument,
    toggleDetail,
    // 双区搜索
    searchQuery,
    searchIntent,
    instantResults,
    showInstantZone,
    showSemanticZone,
    semanticResults,
    semanticLoading,
    semanticRan,
    docSourceMap,
    runSemanticSearch,
    clearSearch,
    selectInstant,
    selectSemanticChunk,
  };
}
