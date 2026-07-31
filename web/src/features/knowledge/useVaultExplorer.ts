/**
 * useVaultExplorer：G1 资源管理器 V2 编排 composable。
 *
 * 职责（数据流铁律：组件不直接调 api/store）：
 * - 选中态单一事实源：{selectedId（与 useKnowledgePage 共享）, currentPrefix}
 * - 融合树：一级节点=库（懒加载目录），目录节点懒加载子目录；expandedKeys 自管理，
 *   刷新重建根节点后 q-tree 对展开中的懒加载节点重新触发 lazy-load
 * - 中栏列表：当前 prefix 直接子节点（文件 + 目录下钻）
 * - 右栏详情：选中文档即加载正文预览 + 已解析关联（force 刷新，避免索引后脏缓存）；
 *   md/txt 可编辑保存（G2-B5 CAS）；image/audio/video 经 B6 原始流内联渲染（G2-F）
 * - 统一搜索框双区：即时区（instantMatch 前端索引）+ 语义区（回车走后端 Search）
 * - 降级：树接口失败时树区显式报错；中栏回退为 documents 平铺（rel_path 过滤）
 */
import { computed, onScopeDispose, ref, watch, type Ref } from 'vue';
import { useKnowledgeStore } from '../../stores/knowledge';
import { fetchDocumentAsset, getDocumentContent, updateDocumentContent } from './api';
import { classifySearchIntent, type SearchIntent } from './searchIntent';
import { instantFilter } from './instantMatch';
import { parseKratosApiError } from '../../utils/kratosError';
import {
  knowledgeMediaEditable,
  knowledgeMediaKind,
  knowledgeMediaNeedsAsset,
  type KnowledgeMediaKind,
} from './knowledgeUi';
import {
  dirNodeKey,
  isValidDropTarget,
  parseVaultTreeKey,
  vaultNodeKey,
  type DragFileRef,
  type DropTargetRef,
} from './vaultTreeUi';
import type { KnowledgeChunk, KnowledgeCollection, KnowledgeDocument, KnowledgeLink, VaultTreeNode } from './types';

/** q-tree 节点（库 + 目录；文件在中栏列表展示）。 */
export interface VaultQTreeNode {
  label: string;
  key: string;
  icon: string;
  lazy?: boolean;
  children?: VaultQTreeNode[];
  /** G1：节点类别与归属（hover 菜单判定）。 */
  kind: 'vault' | 'dir';
  vaultId: string;
  /** 目录 prefix（带尾斜杠）；库节点恒 ''。 */
  prefix: string;
  /** 库节点同步状态徽标。 */
  syncState?: string;
}

/** Quasar q-tree @lazy-load 载荷（组件透传，composable 处理）。 */
export interface VaultLazyLoadPayload {
  key: string;
  done: (children: VaultQTreeNode[]) => void;
  fail: (error: unknown) => void;
}

/** G3-F1 拖拽移动结果：moved=成功；conflict=同名冲突待页面弹窗；noop=非法/原地；error=失败已通知。 */
export type MoveDropResult = 'moved' | 'conflict' | 'noop' | 'error';

/** 库 → q-tree 节点（导出供 G4 图谱范围选择器复用）。 */
export function vaultToQNode(c: KnowledgeCollection): VaultQTreeNode {
  return {
    label: c.name || c.id,
    key: vaultNodeKey(c.id),
    icon: 'inventory_2',
    lazy: true,
    children: [],
    kind: 'vault',
    vaultId: c.id,
    prefix: '',
    syncState: c.sync_state,
  };
}

/** 目录 → q-tree 节点（导出供 G4 图谱范围选择器复用）。 */
export function dirToQNode(n: VaultTreeNode, vaultId: string): VaultQTreeNode {
  return {
    label: n.name,
    key: dirNodeKey(vaultId, n.path),
    icon: 'folder',
    lazy: true,
    children: [],
    kind: 'dir',
    vaultId,
    prefix: n.path,
  };
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
  collections: Ref<KnowledgeCollection[]>;
  documents: Ref<KnowledgeDocument[]>;
  friendlyError: (err: unknown) => string;
  notifyError: (message: string) => void;
  /** F4：语义区错误兜底文案（friendlyError 返回空串时使用），由调用方经 i18n 提供。 */
  semanticErrorFallback: () => string;
}) {
  const knowledgeStore = useKnowledgeStore();
  const { selectedId, collections, documents, friendlyError, notifyError, semanticErrorFallback } = input;

  // ---------- 融合树与中栏列表 ----------

  const currentPrefix = ref('');
  const rootNodes = ref<VaultQTreeNode[]>([]);
  const expandedKeys = ref<string[]>([]);
  const currentChildren = ref<VaultTreeNode[]>([]);
  const treeLoading = ref(false);
  const treeError = ref('');

  const currentFiles = computed(() => currentChildren.value.filter((n) => n.kind === 'file'));
  const currentDirCount = computed(() => currentChildren.value.filter((n) => n.kind === 'dir').length);

  /** 树选中 key：库根 = 库节点；子目录 = 目录节点。 */
  const selectedKey = computed<string | null>(() => {
    if (!selectedId.value) return null;
    return currentPrefix.value ? dirNodeKey(selectedId.value, currentPrefix.value) : vaultNodeKey(selectedId.value);
  });

  /** 一级节点 = 库（本地构建，无请求；目录经 lazy-load 加载）。 */
  function loadRoot() {
    rootNodes.value = collections.value.map(vaultToQNode);
  }

  function ensureVaultExpanded(collectionId: string) {
    const key = vaultNodeKey(collectionId);
    if (!expandedKeys.value.includes(key)) {
      expandedKeys.value = [...expandedKeys.value, key];
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

  /** q-tree 懒加载：库节点 → 根目录；目录节点 → 子目录（文件不进树）。 */
  async function onLazyLoad({ key, done, fail }: VaultLazyLoadPayload) {
    const ref = parseVaultTreeKey(key);
    if (!ref) {
      fail(new Error('unknown tree key'));
      return;
    }
    try {
      const items = await knowledgeStore.loadVaultTree(ref.collectionId, ref.prefix);
      done(items.filter((n) => n.kind === 'dir').map((n) => dirToQNode(n, ref.collectionId)));
    } catch (e) {
      fail(e);
    }
  }

  async function selectPrefix(prefix: string) {
    if (prefix === currentPrefix.value && currentChildren.value.length) return;
    currentPrefix.value = prefix;
    selectedDocId.value = '';
    await loadCurrent();
  }

  /** 树节点选中：库节点 = 浏览库根；目录节点 = 浏览该目录（跨库自动切换 vault）。 */
  async function selectTreeNode(key: string) {
    const ref = parseVaultTreeKey(key);
    if (!ref) return;
    if (ref.collectionId !== selectedId.value) {
      // watch(selectedId, flush:'sync') 立即复位 prefix 并加载库根，随后再定位目标目录。
      selectedId.value = ref.collectionId;
    }
    ensureVaultExpanded(ref.collectionId);
    await selectPrefix(ref.prefix);
  }

  /** 刷新：失效当前 vault 树缓存，重拉展开节点子树 + 中栏列表。 */
  async function refreshTree() {
    invalidateVault(selectedId.value);
    await Promise.all([reloadExpandedNodes(), loadCurrent(true)]);
  }

  /** 失效指定 vault 的树缓存并重建一级节点（hover 刷新任意库节点；中栏仅当前 vault 重载）。 */
  function invalidateVault(collectionId: string) {
    if (!collectionId) return;
    knowledgeStore.invalidateTree(collectionId);
    loadRoot();
  }

  /** q-tree 懒加载节点一经 done() 即被内部标记为已加载，重建 rootNodes 不会重新触发
   *  lazy-load；刷新时主动重拉所有展开节点的子目录并原地修补（递归覆盖嵌套展开）。 */
  async function reloadExpandedNodes() {
    async function patch(nodes: VaultQTreeNode[]): Promise<void> {
      await Promise.all(
        nodes.map(async (n) => {
          if (!n.lazy || !expandedKeys.value.includes(n.key)) return;
          try {
            const items = await knowledgeStore.loadVaultTree(n.vaultId, n.prefix, true);
            n.children = items.filter((x) => x.kind === 'dir').map((x) => dirToQNode(x, n.vaultId));
            await patch(n.children);
          } catch {
            // 重拉失败保留旧 children，不阻断其他分支。
          }
        }),
      );
    }
    await patch(rootNodes.value);
  }

  // ---------- 右栏详情（G2-F V12.4：选中文档即加载正文 + 关联） ----------

  const selectedDocId = ref('');
  const previewContent = ref('');
  const previewOrganized = ref(false);
  const previewLoading = ref(false);
  const links = ref<KnowledgeLink[]>([]);
  const linksLoading = ref(false);
  // 编辑（G2-B5）：rawContent/baseHash 为编辑器数据源与 CAS 凭证（仅 vault 文档非空）。
  const rawContent = ref('');
  const baseHash = ref('');
  const editing = ref(false);
  const editDraft = ref('');
  const editSaving = ref(false);
  // 原始文件流（G2-B6）：image/audio/video 内联渲染的 object URL。
  const assetUrl = ref('');
  const assetLoading = ref(false);

  /** 选中文档节点：优先中栏当前文件；兜底 documents（即时搜索跳转场景）。 */
  const selectedNode = computed<VaultTreeNode | null>(() => {
    if (!selectedDocId.value) return null;
    const fromList = currentFiles.value.find((n) => n.doc_id === selectedDocId.value);
    if (fromList) return fromList;
    const fromDocs = documents.value.find((d) => d.id === selectedDocId.value);
    return fromDocs ? docToTreeNode(fromDocs) : null;
  });

  /** 媒体区类别（按扩展名；节点就绪前为 other，就绪后 watcher 补拉 asset）。 */
  const mediaKind = computed<KnowledgeMediaKind>(() => knowledgeMediaKind(selectedNode.value?.name ?? ''));

  /** 可编辑：md/txt 且为 vault 文档（base_hash 非空 = 后端下发了编辑凭证）。 */
  const editable = computed(() => knowledgeMediaEditable(mediaKind.value) && baseHash.value !== '');

  /** 关联计数（第二行 chips）：按 link_type 聚合。 */
  const linkCounts = computed(() => {
    const counts = { explicit: 0, entity: 0, semantic: 0 };
    for (const l of links.value) {
      if (l.link_type === 'explicit') counts.explicit += 1;
      else if (l.link_type === 'entity') counts.entity += 1;
      else if (l.link_type === 'semantic') counts.semantic += 1;
    }
    return counts;
  });

  function clearDetail() {
    previewContent.value = '';
    previewOrganized.value = false;
    rawContent.value = '';
    baseHash.value = '';
    links.value = [];
    editing.value = false;
    editDraft.value = '';
  }

  /** 加载正文（content_text + raw/base_hash）与已解析关联（force 刷新，索引后可能变化）。 */
  async function loadDetail(docId: string) {
    if (!docId) return;
    previewLoading.value = true;
    linksLoading.value = true;
    try {
      const res = await getDocumentContent(docId);
      previewContent.value = res.content_text;
      previewOrganized.value = res.organized;
      rawContent.value = res.raw_content;
      baseHash.value = res.base_hash;
    } catch (e) {
      previewContent.value = '';
      rawContent.value = '';
      baseHash.value = '';
      notifyError(friendlyError(e));
    } finally {
      previewLoading.value = false;
    }
    try {
      links.value = await knowledgeStore.loadDocumentLinks(docId, '', true);
    } catch (e) {
      links.value = [];
      notifyError(friendlyError(e));
    } finally {
      linksLoading.value = false;
    }
  }

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

  // 选中即加载详情（G2-F：面板不再有展开/收起，摘要/媒体/关联同屏呈现）。
  watch(selectedDocId, (id) => {
    clearDetail();
    if (id) void loadDetail(id);
  });

  // ---------- 编辑保存（G2-B5） ----------

  function startEdit() {
    if (!editable.value) return;
    editDraft.value = rawContent.value;
    editing.value = true;
  }

  function cancelEdit() {
    editing.value = false;
    editDraft.value = '';
  }

  /** 保存编辑：CAS 冲突仍写入（留双份），返回 'saved' | 'conflict' | 'error' 由页面层提示。
   *  保存后强制重载详情（base_hash 更新 + 重索引后的 content_text）。 */
  async function saveEdit(): Promise<'saved' | 'conflict' | 'error'> {
    const docId = selectedDocId.value;
    if (!docId || !editing.value) return 'error';
    editSaving.value = true;
    try {
      const res = await updateDocumentContent(docId, editDraft.value, baseHash.value);
      editing.value = false;
      editDraft.value = '';
      await loadDetail(docId);
      return res.conflict ? 'conflict' : 'saved';
    } catch (e) {
      notifyError(friendlyError(e));
      return 'error';
    } finally {
      editSaving.value = false;
    }
  }

  // ---------- 原始文件流（G2-B6） ----------

  function revokeAssetUrl() {
    if (assetUrl.value) URL.revokeObjectURL(assetUrl.value);
    assetUrl.value = '';
  }

  // 媒体类文档：拉取 blob → object URL（<img>/<audio>/<video> 无法带 JWT 头）。
  watch([selectedDocId, mediaKind], async ([docId, kind]) => {
    revokeAssetUrl();
    if (!docId || !knowledgeMediaNeedsAsset(kind)) return;
    assetLoading.value = true;
    try {
      const { blob } = await fetchDocumentAsset(docId);
      // watcher 竞态：拉取期间又切了文档/类别则丢弃结果。
      if (selectedDocId.value !== docId || mediaKind.value !== kind) return;
      assetUrl.value = URL.createObjectURL(blob);
    } catch {
      // 拉取失败降级为正文预览（content_text 为提取文本），不弹错。
    } finally {
      assetLoading.value = false;
    }
  });

  /** 下载原文（word 等）：blob → a[download] 触发浏览器保存。 */
  async function downloadAsset(): Promise<string> {
    const docId = selectedDocId.value;
    if (!docId) return 'error';
    try {
      const { blob, filename } = await fetchDocumentAsset(docId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename || selectedNode.value?.name || 'document';
      a.click();
      URL.revokeObjectURL(url);
      return 'ok';
    } catch (e) {
      notifyError(friendlyError(e));
      return 'error';
    }
  }

  onScopeDispose(revokeAssetUrl);

  // ---------- G3-F1 拖拽移动（V12.5：HTML5 DnD，库内跨目录） ----------

  /** 拖拽中的文件（中栏文件行 dragstart 记录；dragend/drop 后清空）。 */
  const dragFile = ref<DragFileRef | null>(null);
  /** 同名冲突待决（409）：页面弹窗展示文件名，选定策略后 resolveMoveConflict 重试。 */
  const pendingMove = ref<{ docId: string; name: string; targetPrefix: string } | null>(null);

  function dragStartFile(node: VaultTreeNode) {
    if (node.kind !== 'file' || !node.doc_id) return;
    dragFile.value = {
      docId: node.doc_id,
      name: node.name,
      fromPrefix: dirPrefixOf(node.path),
      vaultId: selectedId.value,
    };
  }

  function dragEnd() {
    dragFile.value = null;
  }

  async function executeMove(docId: string, targetPrefix: string, policy: string): Promise<MoveDropResult> {
    try {
      await knowledgeStore.moveDocToDir(docId, targetPrefix, policy);
      // store 已失效树缓存；重载中栏（源目录）反映移出。
      await loadCurrent(true);
      return 'moved';
    } catch (e) {
      if (parseKratosApiError(e).status === 409) return 'conflict';
      pendingMove.value = null;
      notifyError(friendlyError(e));
      return 'error';
    }
  }

  /** drop 落点执行：校验（同库/非原地）→ 默认策略移动；409 时暂存待决返 conflict。 */
  async function dropOnTarget(target: DropTargetRef): Promise<MoveDropResult> {
    const drag = dragFile.value;
    dragFile.value = null;
    if (!drag || !isValidDropTarget(drag, target)) return 'noop';
    pendingMove.value = { docId: drag.docId, name: drag.name, targetPrefix: target.prefix };
    const res = await executeMove(drag.docId, target.prefix, '');
    if (res !== 'conflict') pendingMove.value = null;
    return res;
  }

  /** 冲突弹窗选定策略后重试（overwrite=覆盖旧版入回收站；rename=保留两份自动改名）。 */
  async function resolveMoveConflict(policy: 'overwrite' | 'rename'): Promise<MoveDropResult> {
    const p = pendingMove.value;
    if (!p) return 'noop';
    const res = await executeMove(p.docId, p.targetPrefix, policy);
    if (res !== 'conflict') pendingMove.value = null;
    return res;
  }

  /** 冲突弹窗取消：仅丢弃待决。 */
  function dismissMoveConflict() {
    pendingMove.value = null;
  }

  // ---------- 统一搜索框双区 ----------

  const searchQuery = ref('');
  const semanticResults = ref<KnowledgeChunk[]>([]);
  const semanticLoading = ref(false);
  const semanticRan = ref(false);
  // F4：语义区错误内联展示（不弹红 toast），文案经 friendlyError 映射。
  const semanticError = ref('');

  const searchIntent = computed<SearchIntent>(() => classifySearchIntent(searchQuery.value));

  // ---------- G3-F2 搜索范围选择器（V12.6） ----------

  /** 搜索范围：vault 相对目录前缀（带尾斜杠）；'' = 全库。选中后持续生效直至清除/切库。 */
  const searchScopePrefix = ref('');

  /** 范围迷你树根节点：当前 vault（选中即全库）；目录经 onLazyLoad 懒加载（仅目录）。 */
  const scopeRootNodes = computed<VaultQTreeNode[]>(() => {
    const col = collections.value.find((c) => c.id === selectedId.value);
    return col ? [vaultToQNode(col)] : [];
  });

  /** 迷你树选中 key：全库 = 库节点；目录 = 目录节点。 */
  const scopeSelectedKey = computed<string | null>(() => {
    if (!selectedId.value) return null;
    return searchScopePrefix.value
      ? dirNodeKey(selectedId.value, searchScopePrefix.value)
      : vaultNodeKey(selectedId.value);
  });

  function setSearchScope(prefix: string) {
    searchScopePrefix.value = prefix;
  }

  function clearSearchScope() {
    searchScopePrefix.value = '';
  }

  /** 即时区：意图为 semantic 时不展示（强问句信号 → 只等回车走语义）；G3-F2 范围先按 prefix 过滤。 */
  const instantResults = computed<KnowledgeDocument[]>(() => {
    if (searchIntent.value === 'semantic') return [];
    const scope = searchScopePrefix.value;
    const pool = scope ? documents.value.filter((d) => d.rel_path.startsWith(scope)) : documents.value;
    return instantFilter(pool, searchQuery.value, (d) => [
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

  /** 语义区：回车触发，走当前 vault 的后端 Search（亚秒）；G3-F2 范围经 B7 path_prefix 过滤。 */
  async function runSemanticSearch() {
    const q = searchQuery.value.trim();
    if (!q || !selectedId.value) return;
    semanticLoading.value = true;
    semanticRan.value = true;
    semanticError.value = '';
    try {
      semanticResults.value = await knowledgeStore.search({
        collection_id: selectedId.value,
        query: q,
        top_k: 8,
        path_prefix: searchScopePrefix.value,
      });
    } catch (e) {
      semanticResults.value = [];
      // F4：错误内联展示在语义区（不弹红 toast）；friendlyError 返回空串时用通用引导文案。
      semanticError.value = friendlyError(e) || semanticErrorFallback();
    } finally {
      semanticLoading.value = false;
    }
  }

  function clearSearch() {
    searchQuery.value = '';
    semanticResults.value = [];
    semanticRan.value = false;
    semanticError.value = '';
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
  // flush:'sync' —— selectTreeNode 跨库跳转依赖「先复位再定位」的确定顺序（先 sync 复位
  // prefix=''，再由 selectTreeNode 设置目标 prefix）；loadVaultTree 走 store 缓存，重复加载
  // 库根无副作用。
  watch(
    selectedId,
    () => {
      currentPrefix.value = '';
      selectedDocId.value = '';
      searchScopePrefix.value = '';
      dragFile.value = null;
      pendingMove.value = null;
      clearSearch();
      if (selectedId.value) ensureVaultExpanded(selectedId.value);
      void loadCurrent();
    },
    { flush: 'sync' },
  );

  // 库列表变化（新建/删除/同步状态翻转）→ 重建树一级节点。
  watch(
    () => collections.value.map((c) => `${c.id}:${c.name}:${c.sync_state}`).join(','),
    () => loadRoot(),
    { immediate: true },
  );

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
    expandedKeys,
    selectedKey,
    currentChildren,
    currentFiles,
    currentDirCount,
    treeLoading,
    treeError,
    onLazyLoad,
    selectPrefix,
    selectTreeNode,
    refreshTree,
    invalidateVault,
    ensureVaultExpanded,
    // 详情
    selectedDocId,
    selectedNode,
    previewContent,
    previewOrganized,
    previewLoading,
    links,
    linksLoading,
    linkCounts,
    mediaKind,
    editable,
    rawContent,
    baseHash,
    editing,
    editDraft,
    editSaving,
    assetUrl,
    assetLoading,
    selectDocument,
    navigateToDocument,
    startEdit,
    cancelEdit,
    saveEdit,
    downloadAsset,
    // 双区搜索
    searchQuery,
    searchIntent,
    instantResults,
    showInstantZone,
    showSemanticZone,
    semanticResults,
    semanticLoading,
    semanticRan,
    semanticError,
    docSourceMap,
    runSemanticSearch,
    clearSearch,
    selectInstant,
    selectSemanticChunk,
    // G3-F1 拖拽移动
    dragFile,
    pendingMove,
    dragStartFile,
    dragEnd,
    dropOnTarget,
    resolveMoveConflict,
    dismissMoveConflict,
    // G3-F2 搜索范围
    searchScopePrefix,
    scopeRootNodes,
    scopeSelectedKey,
    setSearchScope,
    clearSearchScope,
  };
}
