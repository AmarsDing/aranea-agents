/**
 * useKnowledgeGraph：G4 3D 知识图谱编排 composable（数据流铁律：组件不直接调 api/store）。
 *
 * 职责：
 * - 图谱数据：库/边类型/目录前缀三维过滤 → ListCollectionGraph（G4-B8）一次性全量
 * - 渲染裁剪：>2k 节点默认仅渲染有连接节点（「显示孤立节点」开关放开，V12.7 规模条款）
 * - 操作台状态：节点搜索定位、节点列表（连接度排序）、选中节点、画布聚焦信号
 * - 范围选择器：迷你目录树（仅目录懒加载，复用 V12.6 语义；切换库自动重置范围）
 */
import { computed, ref, watch, type Ref } from 'vue';
import { useKnowledgeStore } from '../../stores/knowledge';
import { listEntityMergeSuggestions, mergeKnowledgeEntities } from './api';
import {
  buildNeighborhoodGraph,
  buildRenderGraph,
  filterGraphNodes,
  sortedGraphNodes,
  GRAPH_LINK_TYPES,
  type RenderGraph,
} from './graphUi';
import { parseVaultTreeKey } from './vaultTreeUi';
import { dirToQNode, vaultToQNode, type VaultLazyLoadPayload, type VaultQTreeNode } from './useVaultExplorer';
import type {
  CollectionGraphEdge,
  CollectionGraphNode,
  EntityMergeSuggestion,
  KnowledgeCollection,
  MergeEntitiesResult,
} from './types';

export function useKnowledgeGraph(input: {
  collections: Ref<KnowledgeCollection[]>;
  friendlyError: (err: unknown) => string;
}) {
  const knowledgeStore = useKnowledgeStore();
  const { collections, friendlyError } = input;

  // ---------- 过滤维度 ----------

  /** 当前库（操作台库 select；空 = 未选择）。 */
  const collectionId = ref('');
  /** 边类型过滤 chips（默认全选；全选/全不选语义 = 全部，传空给后端）。 */
  const linkTypes = ref<string[]>([...GRAPH_LINK_TYPES]);
  /** 目录前缀过滤（范围选择器；'' = 全库）。 */
  const pathPrefix = ref('');

  // ---------- 图谱数据 ----------

  const nodes = ref<CollectionGraphNode[]>([]);
  const edges = ref<CollectionGraphEdge[]>([]);
  const loading = ref(false);
  const error = ref('');
  /** 数据代际：每次成功加载 +1（画布据此重置相机）。 */
  const generation = ref(0);

  /** 传给后端的有效类型集：全选/全不选 = 全部（空数组）。 */
  function effectiveLinkTypes(): string[] {
    const sel = linkTypes.value.filter((t) => (GRAPH_LINK_TYPES as readonly string[]).includes(t));
    return sel.length === 0 || sel.length === GRAPH_LINK_TYPES.length ? [] : sel;
  }

  /** 全量加载经 store 共享缓存（无过滤时命中缓存零请求；右栏局部图走服务端邻域 RPC，不复用本缓存）。
   *  force=true 强制回源（实体合并/图谱增量失效后）。 */
  async function loadGraph(force = false) {
    if (!collectionId.value) {
      nodes.value = [];
      edges.value = [];
      return;
    }
    loading.value = true;
    try {
      const g = await knowledgeStore.loadCollectionGraph(collectionId.value, effectiveLinkTypes(), pathPrefix.value, force);
      nodes.value = g.nodes;
      edges.value = g.edges;
      error.value = '';
      generation.value++;
      resetGlobalView(); // 数据重载后邻域根可能已不存在，回全局
    } catch (e) {
      nodes.value = [];
      edges.value = [];
      error.value = friendlyError(e) || 'graph load failed';
    } finally {
      loading.value = false;
    }
  }

  function selectCollection(id: string) {
    if (collectionId.value === id) return;
    collectionId.value = id;
    // 切库重置范围与选中（范围语义绑定具体 vault 目录）。
    pathPrefix.value = '';
    selectedNodeId.value = '';
    // 切库清空合并反馈（反馈语义绑定具体库）。
    lastMergeResult.value = null;
  }

  function toggleLinkType(type: string) {
    const i = linkTypes.value.indexOf(type);
    linkTypes.value = i >= 0 ? linkTypes.value.filter((t) => t !== type) : [...linkTypes.value, type];
  }

  function setPathPrefix(prefix: string) {
    pathPrefix.value = prefix;
    selectedNodeId.value = '';
  }

  // 三维过滤任一变化 → 重新拉取（后端一次性全量，前端不做二次裁剪）。
  watch([collectionId, linkTypes, pathPrefix], () => void loadGraph());

  // 集合列表就绪后默认选中首库（当前库为空时）。
  watch(
    collections,
    (cols) => {
      if (!collectionId.value && cols.length) collectionId.value = cols[0].id;
    },
    { immediate: true },
  );

  // ---------- 渲染裁剪与操作台派生 ----------

  /** 显示孤立节点开关（节点数 ≤2k 时无实际效果——全量渲染）。 */
  const showIsolated = ref(false);
  const renderGraph = computed<RenderGraph>(() => buildRenderGraph(nodes.value, edges.value, showIsolated.value));

  // ---------- 局部图谱（G5-D D-5：聚焦邻域 + 返回全局） ----------

  /** 聚焦邻域跳数：0 = 全局图（1-4 步进由 HUD 控件约束）。 */
  const neighborhoodHops = ref(0);
  /** 邻域根 doc_id（进入邻域模式时锁定，不随选中漂移——避免视图突然重建）。 */
  const neighborhoodRootId = ref('');

  /** 画布视图：hops>0 且有根时按 BFS 邻域裁剪；doc_type 不变 → 跨视图颜色一致（反模式⑥）。 */
  const viewGraph = computed<RenderGraph>(() => {
    const g = renderGraph.value;
    if (neighborhoodHops.value <= 0 || !neighborhoodRootId.value) return g;
    const sub = buildNeighborhoodGraph(g.nodes, g.edges, neighborhoodRootId.value, neighborhoodHops.value);
    return { nodes: sub.nodes, edges: sub.edges, hiddenIsolated: g.hiddenIsolated };
  });

  /** 聚焦邻域：以 docId 为根、跳数 hops（钳制 1-4）。 */
  function focusNeighborhood(docId: string, hops: number) {
    if (!docId) return;
    neighborhoodRootId.value = docId;
    neighborhoodHops.value = Math.min(4, Math.max(1, Math.round(hops)));
  }

  /** 返回全局。 */
  function resetGlobalView() {
    neighborhoodHops.value = 0;
    neighborhoodRootId.value = '';
  }

  /** 节点搜索定位关键字。 */
  const nodeQuery = ref('');
  /** 节点列表：连接度降序 + 搜索过滤（基于全量节点，不受孤立裁剪影响）。 */
  const nodeList = computed(() => filterGraphNodes(sortedGraphNodes(nodes.value), nodeQuery.value));

  // ---------- 选中与聚焦 ----------

  const selectedNodeId = ref('');
  /** 聚焦信号：+1 触发画布相机飞往选中节点。 */
  const focusSignal = ref(0);

  const selectedNode = computed<CollectionGraphNode | null>(
    () => nodes.value.find((n) => n.doc_id === selectedNodeId.value) ?? null,
  );

  function selectNode(id: string) {
    selectedNodeId.value = id;
  }

  /** 节点列表/搜索点击：选中 + 画布聚焦。 */
  function focusNode(id: string) {
    selectedNodeId.value = id;
    focusSignal.value++;
  }

  // ---------- 实体治理（G5-G G-1：合并建议 + 一键合并） ----------

  /** 合并建议列表（norm 冲突组在前 + embedding 高相似对；随库加载）。 */
  const mergeSuggestions = ref<EntityMergeSuggestion[]>([]);
  /** 合并进行中（按钮 loading/防重入）。 */
  const merging = ref(false);
  /** 最近一次合并重写反馈（内联展示；切库清空）。 */
  const lastMergeResult = ref<MergeEntitiesResult | null>(null);

  /** 拉取合并建议：失败降级空列表（辅助数据不阻断图谱主流程）。 */
  async function loadMergeSuggestions() {
    if (!collectionId.value) {
      mergeSuggestions.value = [];
      return;
    }
    try {
      mergeSuggestions.value = await listEntityMergeSuggestions(collectionId.value);
    } catch (e) {
      console.warn('[knowledge-graph] load merge suggestions failed', e);
      mergeSuggestions.value = [];
    }
  }

  // 建议只随库变化（与边类型/目录过滤无关）。
  watch(collectionId, () => void loadMergeSuggestions(), { immediate: true });

  /** 一键合并：mergee 并入 keeper → 重拉图谱（边可能变化）与建议列表 → 内联反馈重写条数。 */
  async function mergeEntities(keeperId: number, mergeeId: number) {
    if (!collectionId.value || merging.value) return;
    merging.value = true;
    try {
      lastMergeResult.value = await mergeKnowledgeEntities({
        collectionId: collectionId.value,
        keeperId,
        mergeeIds: [mergeeId],
      });
      await Promise.all([loadGraph(true), loadMergeSuggestions()]);
    } catch (e) {
      error.value = friendlyError(e) || 'merge failed';
    } finally {
      merging.value = false;
    }
  }

  // ---------- 范围选择器迷你树（仅目录懒加载） ----------

  /** 迷你树根节点：当前库（选中即全库）。 */
  const scopeNodes = computed<VaultQTreeNode[]>(() => {
    const col = collections.value.find((c) => c.id === collectionId.value);
    return col ? [vaultToQNode(col)] : [];
  });

  /** 迷你树懒加载：库节点 → 根目录；目录节点 → 子目录（文件不进树）。 */
  async function onScopeLazyLoad({ key, done, fail }: VaultLazyLoadPayload) {
    const ref0 = parseVaultTreeKey(key);
    if (!ref0) {
      fail(new Error('unknown tree key'));
      return;
    }
    try {
      const items = await knowledgeStore.loadVaultTree(ref0.collectionId, ref0.prefix);
      done(items.filter((n) => n.kind === 'dir').map((n) => dirToQNode(n, ref0.collectionId)));
    } catch (e) {
      fail(e);
    }
  }

  return {
    // 过滤维度
    collectionId,
    linkTypes,
    pathPrefix,
    selectCollection,
    toggleLinkType,
    setPathPrefix,
    // 数据
    nodes,
    edges,
    loading,
    error,
    generation,
    loadGraph,
    // 渲染与列表
    showIsolated,
    renderGraph,
    nodeQuery,
    nodeList,
    // 局部图谱（G5-D）
    neighborhoodHops,
    neighborhoodRootId,
    viewGraph,
    focusNeighborhood,
    resetGlobalView,
    // 选中与聚焦
    selectedNodeId,
    selectedNode,
    focusSignal,
    selectNode,
    focusNode,
    // 实体治理（G5-G）
    mergeSuggestions,
    merging,
    lastMergeResult,
    loadMergeSuggestions,
    mergeEntities,
    // 范围选择器
    scopeNodes,
    onScopeLazyLoad,
  };
}

export type KnowledgeGraphContext = ReturnType<typeof useKnowledgeGraph>;
