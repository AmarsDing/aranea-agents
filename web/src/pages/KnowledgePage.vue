<template>
  <q-page class="knowledge-page-wb column no-wrap">
    <q-banner v-if="unavailable" rounded class="app-banner-warning q-ma-sm flex-none">
      知识库服务不可用：{{ unavailable }}。请确认 Postgres / pgvector 已配置。
    </q-banner>
    <q-banner v-if="error" rounded class="bg-negative text-white q-ma-sm flex-none">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadCollections" />
      </template>
    </q-banner>
    <!-- B5：文档列表超上限截断提示（树导航不受影响，图谱/搜索可能不完整） -->
    <q-banner v-if="documentsTruncated" rounded class="app-banner-warning q-ma-sm flex-none">
      当前库文档超过 {{ DOCUMENTS_PAGE_LIMIT }} 条上限，仅加载前 {{ DOCUMENTS_PAGE_LIMIT }}
      条；目录树导航不受影响，图谱与搜索结果可能不完整。
    </q-banner>

    <!-- SP2-8：深空液态玻璃工作台（薄壳页面唯一的常驻主体） -->
    <KnowledgeWorkbench
      class="knowledge-page-wb__main"
      :workbench="workbench"
      :collections="collections"
      :documents="documents"
      :current-vault-id="selectedId"
      :nodes="explorerRootNodes"
      :selected-key="explorerSelectedKey"
      :expanded-keys="explorerExpandedKeys"
      :tree-loading="explorerTreeLoading"
      :tree-error="explorerTreeError"
      :drag-file="explorerDragFile"
      :files="explorerFiles"
      :current-prefix="explorerPrefix"
      :panels-refresh-nonce="panelsRefreshNonce"
      :performance-mode="performanceMode"
      @switch-vault="onSwitchVault"
      @toggle-performance-mode="togglePerformanceMode"
      @select-node="selectExplorerTreeNode"
      @update:expanded-keys="(v: string[]) => (explorerExpandedKeys = v)"
      @lazy-load="onExplorerLazyLoad"
      @node-action="onTreeNodeAction"
      @create-vault="openCreateCollection"
      @drop-node="onExplorerDropNode"
      @retry="refreshExplorerTree"
      @refresh-tree="refreshExplorerTree"
      @open-graph="onOpenGraph"
      @open-settings="settingsOpen = true"
      @promote-active="openPromoteDialog"
      @ingest-text="ingestOpen = true"
      @file-action="onFileAction"
      @file-drag-start="explorerDragStart"
      @file-drag-end="explorerDragEnd"
      @drop-current-dir="onDropCurrentDir"
    >
      <!-- 上传队列收纳进左栏底部（SP2-8 插槽） -->
      <template #left-footer>
        <knowledge-upload-queue
          v-if="uploadTasks.length"
          :tasks="uploadTasks"
          @clear-finished="clearFinishedUploadTasks"
          @remove-task="removeUploadTask"
        />
      </template>
    </KnowledgeWorkbench>

    <!-- G5 图谱全屏覆盖（SP2-8）：顶栏 / ⌘K / 局部图谱「展开全屏」进入，ESC 或关闭按钮退出 -->
    <knowledge-graph-3-d
      v-if="graphFullscreen"
      v-model:fullscreen="graphFullscreen"
      :collections="collections"
      :collection-id="graphCollectionId"
      :link-types="graphLinkTypes"
      :path-prefix="graphPathPrefix"
      :nodes="graphRenderNodes"
      :edges="graphRenderEdges"
      :legend-nodes="graphViewGraph.nodes"
      :hidden-groups="graphHiddenGroups"
      :total-nodes="graphAllNodes.length"
      :total-edges="graphAllEdges.length"
      :hidden-isolated="graphHiddenIsolated"
      :loading="graphLoading"
      :error="graphError"
      :generation="graphGeneration"
      :show-isolated="graphShowIsolated"
      :node-query="graphNodeQuery"
      :node-list="graphNodeList"
      :selected-node-id="graphSelectedNodeId"
      :selected-node="graphSelectedNode"
      :focus-signal="graphFocusSignal"
      :scope-nodes="graphScopeNodes"
      :auto-rotate="graphAutoRotate"
      :show-labels="graphShowLabels"
      :neighborhood-hops="graphNeighborhoodHops"
      :neighborhood-root-name="graphNeighborhoodRootName"
      :merge-suggestions="graphMergeSuggestions"
      :merging="graphMerging"
      :last-merge-result="graphLastMergeResult"
      @select-collection="graph.selectCollection"
      @toggle-link-type="graph.toggleLinkType"
      @set-path-prefix="graph.setPathPrefix"
      @update:show-isolated="(v: boolean) => (graphShowIsolated = v)"
      @update:node-query="(v: string) => (graphNodeQuery = v)"
      @select-node="graph.selectNode"
      @focus-node="graph.focusNode"
      @open-in-explorer="onGraphOpenInWorkbench"
      @scope-lazy-load="graph.onScopeLazyLoad"
      @update:auto-rotate="(v: boolean) => (graphAutoRotate = v)"
      @update:show-labels="(v: boolean) => (graphShowLabels = v)"
      @focus-neighborhood="onGraphFocusNeighborhood"
      @reset-global-view="graph.resetGlobalView"
      @merge-entities="(p: { keeperId: number; mergeeId: number }) => graph.mergeEntities(p.keeperId, p.mergeeId)"
      @reembed-node="(docId: string) => confirmReembed([docId])"
      @toggle-group="onGraphToggleGroup"
    />

    <!-- 设置浮层（SP2-8）：Embedder 配置；kb-portal 在 body 重挂深空令牌 -->
    <q-dialog v-model="settingsOpen" content-class="kb-portal">
      <GlassPanel strong icon="tune" :title="t('knowledgePage.tabSettings')" class="knowledge-page-wb__settings">
        <knowledge-embedder-panel :config="embedderConfig" :saving="embedderSaving" @save="saveEmbedderConfig" />
      </GlassPanel>
    </q-dialog>

    <knowledge-create-dialog
      v-model:open="createOpen"
      v-model:name="createForm.name"
      v-model:root-path="createForm.root_path"
      v-model:description="createForm.description"
      v-model:embedding-model="createForm.embedding_model"
      :loading="createLoading"
      @submit="submitCreateCollection"
    />
    <knowledge-ingest-dialog
      v-model:open="ingestOpen"
      v-model:source="ingestForm.source"
      v-model:mime-type="ingestForm.mime_type"
      v-model:text="ingestForm.text"
      v-model:chunk-strategy="ingestForm.chunk_strategy"
      v-model:chunk-size="ingestForm.chunk_size"
      v-model:chunk-overlap="ingestForm.chunk_overlap"
      :loading="ingestLoading"
      @submit="submitIngest"
    />
    <knowledge-move-dialog
      v-model:open="moveOpen"
      v-model:target-id="moveTargetId"
      :doc-source="movingDoc?.source ?? ''"
      :options="moveTargetOptions"
      :loading="moveLoading"
      @submit="submitMove"
    />
    <!-- SP1-G/I-3 晋升到团队库：目标库选择 → 结果反馈（新建块数 + 级联提示） -->
    <knowledge-promote-dialog
      v-model:open="promoteOpen"
      v-model:target-id="promoteTargetId"
      :doc-name="promoteDocName"
      :options="promoteOptions"
      :loading="promoteLoading"
      :result="promoteResult"
      @submit="submitPromote"
    />
    <!-- G3-F1 拖拽移动同名冲突（V12.5）：覆盖 / 保留两份 / 取消 -->
    <knowledge-move-conflict-dialog
      v-model:open="moveConflictOpen"
      :file-name="explorerPendingMove?.name ?? ''"
      :target-label="moveConflictTargetLabel"
      :loading="moveConflictLoading"
      @resolve="onMoveConflictResolve"
    />
    <!-- G1 树 hover「上传文件到此」：隐藏文件输入，pendingUploadTarget 置位时触发 -->
    <input ref="uploadFileInput" type="file" multiple class="hidden" @change="onUploadInputChange" />
  </q-page>
</template>

<script setup lang="ts">
// SP2-8：薄壳页面——持有 composable（页面级状态）+ 装配 KnowledgeWorkbench / 全屏图谱 / 设置浮层 / 既有对话框。
// 旧 Tab 管理后台（浏览/图谱/设置三页签）已退役，职责分别由工作台、全屏覆盖图谱、设置浮层吸收。
import { computed, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import KnowledgeEmbedderPanel from '../components/knowledge/KnowledgeEmbedderPanel.vue';
import KnowledgeWorkbench from '../components/knowledge/workbench/KnowledgeWorkbench.vue';
import GlassPanel from '../components/knowledge/effects/GlassPanel.vue';
import KnowledgeGraph3D from '../components/knowledge/KnowledgeGraph3D.vue';
import KnowledgeCreateDialog from '../components/knowledge/KnowledgeCreateDialog.vue';
import KnowledgeIngestDialog from '../components/knowledge/KnowledgeIngestDialog.vue';
import KnowledgeUploadQueue from '../components/knowledge/KnowledgeUploadQueue.vue';
import KnowledgeMoveDialog from '../components/knowledge/KnowledgeMoveDialog.vue';
import KnowledgeMoveConflictDialog from '../components/knowledge/KnowledgeMoveConflictDialog.vue';
import KnowledgePromoteDialog from '../components/knowledge/KnowledgePromoteDialog.vue';
import { useKnowledgePage } from '../features/knowledge/useKnowledgePage';
import { filterGraphByGroups } from '../features/knowledge/graph3d/model';
import { useKnowledgeGraph } from '../features/knowledge/useKnowledgeGraph';
import { useKnowledgeGraphDeltaWs } from '../features/knowledge/useKnowledgeGraphDeltaWs';
import { createKnowledgeWorkbench } from '../features/knowledge/useKnowledgeWorkbench';
import type { MoveDropResult, VaultQTreeNode } from '../features/knowledge/useVaultExplorer';
import type { KnowledgeDocument, VaultTreeNode } from '../features/knowledge/types';

const {
  collections,
  selectedId,
  documents,
  documentsTruncated,
  DOCUMENTS_PAGE_LIMIT,
  performanceMode,
  togglePerformanceMode,
  error,
  unavailable,
  removedDocId,
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
  loadEmbedderConfig,
  openCreateCollection,
  submitCreateCollection,
  submitIngest,
  confirmDeleteDocument,
  confirmReembed,
  downloadDocument,
  setDocumentVisibility,
  uploadTasks,
  removeUploadTask,
  clearFinishedUploadTasks,
  pendingUploadTarget,
  onUploadFilesPicked,
  onTreeNodeAction,
  friendlyError,
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
  openPromoteDialog,
  submitPromote,
  applyGraphDelta,
  explorer,
} = useKnowledgePage();

// 工作台消费的 explorer 子集（树状态 + 拖拽；旧详情区/双区搜索已随 SP2-8 退役）。
const {
  currentPrefix: explorerPrefix,
  rootNodes: explorerRootNodes,
  expandedKeys: explorerExpandedKeys,
  selectedKey: explorerSelectedKey,
  currentFiles: explorerFiles,
  treeLoading: explorerTreeLoading,
  treeError: explorerTreeError,
  onLazyLoad: onExplorerLazyLoad,
  selectTreeNode: selectExplorerTreeNode,
  refreshTree: refreshExplorerTree,
  dragFile: explorerDragFile,
  pendingMove: explorerPendingMove,
} = explorer;

const $q = useQuasar();
const { t } = useI18n();

// ---------- SP2-8：工作台状态机（tabs/脏标记/CAS 保存） ----------

const workbench = createKnowledgeWorkbench();
const route = useRoute();

watch(
  () => route.query.doc,
  (id) => {
    if (typeof id !== 'string' || !id.trim()) return;
    const found = documents.value.find((d) => d.id === id);
    const doc =
      found ??
      ({
        id,
        source: id,
        rel_path: '',
        collection_id: selectedId.value,
        mime_type: 'text/markdown',
      } as KnowledgeDocument);
    void workbench.openDoc(doc);
  },
  { immediate: true },
);

// FR-SP2-2：文档删除广播 → 关闭对应 tab（无确认，数据已删）。
watch(removedDocId, (id) => {
  if (id) workbench.onDocRemoved(id);
});

function onSwitchVault(id: string) {
  if (id && id !== selectedId.value) selectedId.value = id;
}

/** 文件行菜单/详情操作需要完整 KnowledgeDocument；列表未覆盖时按节点合成。 */
function resolveWorkbenchDoc(node: VaultTreeNode): KnowledgeDocument {
  const found = documents.value.find((d) => d.id === node.doc_id);
  if (found) return found;
  return { id: node.doc_id, source: node.name, rel_path: node.path, collection_id: selectedId.value } as KnowledgeDocument;
}

/** 侧栏文件行操作（移动跨库 / 下载原文 / 重嵌入 / 删除），复用既有对话框与逻辑。 */
function onFileAction(action: string, node: VaultTreeNode) {
  const doc = resolveWorkbenchDoc(node);
  if (action === 'move') openMoveDialog(doc);
  else if (action === 'download') void downloadDocument(doc);
  else if (action === 'reembed') confirmReembed([node.doc_id]);
  else if (action === 'delete') confirmDeleteDocument(doc);
  else if (action === 'private' || action === 'collection') void setDocumentVisibility(doc, action);
}

// ---------- G5 深空知识图谱（全屏覆盖模式） ----------

const graph = useKnowledgeGraph({ collections, friendlyError });

const {
  collectionId: graphCollectionId,
  linkTypes: graphLinkTypes,
  pathPrefix: graphPathPrefix,
  nodes: graphAllNodes,
  edges: graphAllEdges,
  loading: graphLoading,
  error: graphError,
  generation: graphGeneration,
  showIsolated: graphShowIsolated,
  viewGraph: graphViewGraph,
  neighborhoodHops: graphNeighborhoodHops,
  neighborhoodRootId: graphNeighborhoodRootId,
  nodeQuery: graphNodeQuery,
  nodeList: graphNodeList,
  selectedNodeId: graphSelectedNodeId,
  selectedNode: graphSelectedNode,
  focusSignal: graphFocusSignal,
  scopeNodes: graphScopeNodes,
  mergeSuggestions: graphMergeSuggestions,
  merging: graphMerging,
  lastMergeResult: graphLastMergeResult,
} = graph;

// ---------- M5：doc_type 组隐藏（图例点击切换，localStorage 持久化） ----------

/** 读取持久化的隐藏组（隐私模式/脏数据退化为空）。 */
function readGraphHiddenGroups(): string[] {
  try {
    const raw = localStorage.getItem('kg3d-hidden-groups');
    const v: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
  } catch {
    return [];
  }
}

const graphHiddenGroups = ref<string[]>(readGraphHiddenGroups());

watch(graphHiddenGroups, (v) => {
  try {
    localStorage.setItem('kg3d-hidden-groups', JSON.stringify(v));
  } catch {
    /* 隐私模式忽略 */
  }
});

/** 图例点击：切换 doc_type 组隐藏。 */
function onGraphToggleGroup(docType: string) {
  const i = graphHiddenGroups.value.indexOf(docType);
  if (i >= 0) graphHiddenGroups.value.splice(i, 1);
  else graphHiddenGroups.value.push(docType);
}

// 渲染视图派生（viewGraph = 孤立裁剪 + 邻域裁剪；computed 解构后模板自动解包）。
// M5 过滤管线：viewGraph → filterGraphByGroups（doc_type 组隐藏 + 边级联排除）。
const graphFiltered = computed(() => {
  const view = graphViewGraph.value;
  if (!graphHiddenGroups.value.length) return view;
  const out = filterGraphByGroups(
    view.nodes.map((n) => ({ docId: n.doc_id, name: n.name, relPath: n.rel_path, docType: n.doc_type })),
    view.edges,
    new Set(graphHiddenGroups.value),
  );
  const keptIds = new Set(out.nodes.map((n) => n.docId));
  return {
    nodes: view.nodes.filter((n) => keptIds.has(n.doc_id)),
    edges: out.edges,
    hiddenIsolated: view.hiddenIsolated,
  };
});
const graphRenderNodes = computed(() => graphFiltered.value.nodes);
const graphRenderEdges = computed(() => graphFiltered.value.edges);
const graphHiddenIsolated = computed(() => graphFiltered.value.hiddenIsolated);

// G5-E HUD 开关（纯 UI 状态，页面持有）。
const graphAutoRotate = ref(false);
const graphShowLabels = ref(true);

/** 局部图谱根节点名（统计行提示；根已被删时回退 id）。 */
const graphNeighborhoodRootName = computed(() => {
  const id = graphNeighborhoodRootId.value;
  return graphAllNodes.value.find((n) => n.doc_id === id)?.name ?? id;
});

/** 聚焦邻域：邻域模式下根锁定（改跳数不换根），否则以当前选中为根。 */
function onGraphFocusNeighborhood(hops: number) {
  const root =
    graphNeighborhoodHops.value > 0 && graphNeighborhoodRootId.value
      ? graphNeighborhoodRootId.value
      : graphSelectedNodeId.value;
  if (root) graph.focusNeighborhood(root, hops);
}

// ---------- SP2-8：图谱全屏覆盖 ----------

const graphFullscreen = ref(false);
/** 「展开全屏并定位」待聚焦节点：切库后 loadGraph 异步，待数据就绪再聚焦。 */
const pendingGraphFocusId = ref('');

/** 打开图谱全屏：集合对齐当前 vault；带 focusDocId 时数据就绪后聚焦该节点。 */
function onOpenGraph(focusDocId?: string) {
  if (selectedId.value && graphCollectionId.value !== selectedId.value) {
    graph.selectCollection(selectedId.value);
  }
  if (focusDocId) pendingGraphFocusId.value = focusDocId;
  graphFullscreen.value = true;
}

watch([graphFullscreen, graphGeneration], () => {
  const id = pendingGraphFocusId.value;
  if (!graphFullscreen.value || !id) return;
  if (graphAllNodes.value.some((n) => n.doc_id === id)) {
    graph.focusNode(id);
    pendingGraphFocusId.value = '';
  }
});

/** 图谱「在浏览中打开」→ 工作台打开该笔记（跨库时先切库），并退出全屏。 */
function onGraphOpenInWorkbench(payload: { docId: string; relPath: string }) {
  if (graphCollectionId.value && selectedId.value !== graphCollectionId.value) {
    selectedId.value = graphCollectionId.value;
  }
  const doc =
    documents.value.find((d) => d.id === payload.docId) ??
    ({
      id: payload.docId,
      source: payload.relPath,
      rel_path: payload.relPath,
      collection_id: graphCollectionId.value,
      mime_type: '',
    } as KnowledgeDocument);
  graphFullscreen.value = false;
  void workbench.openDoc(doc);
}

// ---------- SP2-8：设置浮层 ----------

const settingsOpen = ref(false);

// ---------- SP1-I（I-4）：knowledge.graph.delta WS 增量 ----------

/** 图谱增量到达时 +1：右栏五面板据此重拉反链/出链/悬空链/局部图谱（缓存已失效）。 */
const panelsRefreshNonce = ref(0);

useKnowledgeGraphDeltaWs((delta) => {
  const affected = applyGraphDelta(delta);
  panelsRefreshNonce.value++;
  if (graphCollectionId.value && affected.collectionIds.includes(graphCollectionId.value)) {
    void graph.loadGraph();
  }
});

// ---------- G3-F 拖拽移动（V12.5） ----------

function explorerDragStart(node: VaultTreeNode) {
  explorer.dragStartFile(node);
}

function explorerDragEnd() {
  explorer.dragEnd();
}

/** 目标目录展示名：末段目录名；库根 = 根目录。 */
function dropTargetLabel(prefix: string): string {
  const segs = prefix.split('/').filter(Boolean);
  return segs[segs.length - 1] ?? t('knowledgePage.vaultRoot');
}

/** drop 结果统一处理：moved 提示成功；conflict 弹策略选择（pendingMove 已由 composable 暂存）。 */
function handleDropResult(res: MoveDropResult, targetPrefix: string) {
  if (res === 'conflict') {
    moveConflictOpen.value = true;
  } else if (res === 'moved') {
    $q.notify({
      type: 'positive',
      message: t('knowledgePage.moveToDirSuccess', { dir: dropTargetLabel(targetPrefix) }),
    });
  }
}

async function onExplorerDropNode(node: VaultQTreeNode) {
  handleDropResult(await explorer.dropOnTarget({ vaultId: node.vaultId, prefix: node.prefix }), node.prefix);
}

/** 中栏空白处 drop = 移入当前目录（WorkbenchSidebar 的 drop-current-dir）。 */
async function onDropCurrentDir() {
  handleDropResult(await explorer.dropOnTarget({ vaultId: selectedId.value, prefix: explorerPrefix.value }), explorerPrefix.value);
}

// G3-F1 冲突弹窗（V12.5）：覆盖（旧版入回收站）/ 保留两份（自动改名）/ 取消。
const moveConflictOpen = ref(false);
const moveConflictLoading = ref(false);

const moveConflictTargetLabel = computed(() => dropTargetLabel(explorerPendingMove.value?.targetPrefix ?? ''));

async function onMoveConflictResolve(policy: 'overwrite' | 'rename') {
  // resolve 成功后 composable 会清空 pendingMove，先取目标目录供成功提示。
  const targetPrefix = explorerPendingMove.value?.targetPrefix ?? '';
  moveConflictLoading.value = true;
  try {
    const res = await explorer.resolveMoveConflict(policy);
    if (res !== 'conflict') {
      moveConflictOpen.value = false;
      handleDropResult(res, targetPrefix);
    }
  } finally {
    moveConflictLoading.value = false;
  }
}

// 冲突弹窗关闭（取消/成功）：丢弃 composable 暂存的待决（成功时本已清空，幂等）。
watch(moveConflictOpen, (open) => {
  if (!open) explorer.dismissMoveConflict();
});

// G1 树 hover「上传文件到此」：pendingUploadTarget 置位 → 触发隐藏 input 选文件。
const uploadFileInput = ref<HTMLInputElement | null>(null);
watch(pendingUploadTarget, (target) => {
  if (target) uploadFileInput.value?.click();
});

function onUploadInputChange(e: Event) {
  const el = e.target as HTMLInputElement;
  const files = Array.from(el.files ?? []);
  el.value = '';
  if (files.length) void onUploadFilesPicked(files);
}

onMounted(() => {
  void loadCollections();
  void loadEmbedderConfig();
});
</script>

<style lang="scss" scoped>
// SP2-8：沉浸工作台页——满高无滚动（内部栏自滚动），容器高度链见 app-global.sass
// `.app-page-container:has(.knowledge-page-wb)`（chat-page 同款模式）。
.knowledge-page-wb {
  &__main {
    flex: 1 1 0;
    min-height: 0;
  }

  &__settings {
    width: 640px;
    max-width: 92vw;
  }
}
</style>
