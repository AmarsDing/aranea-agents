<template>
  <q-page class="app-standard-page app-registry-page knowledge-page">
    <AppPageHero
      kicker="RAG / Vault"
      title="知识库"
      subtitle="以本地文件夹为真相源的 Vault 知识库：文件夹树导航、摘要卡、双轨关联与语义检索。"
    >
      <template #actions>
        <q-btn
          outline
          rounded
          no-caps
          color="primary"
          icon="refresh"
          label="刷新"
          :loading="loading"
          @click="loadCollections"
        />
      </template>
    </AppPageHero>

    <q-banner v-if="unavailable" rounded class="app-banner-warning q-mb-md">
      知识库服务不可用：{{ unavailable }}。请确认 Postgres / pgvector 已配置。
    </q-banner>
    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadCollections" />
      </template>
    </q-banner>

    <div class="app-tab-shell q-mb-md">
      <q-tabs v-model="pageTab" dense align="left" class="text-primary">
        <q-tab name="explorer" :label="$t('knowledgePage.tabExplorer')" />
        <q-tab name="graph" :label="$t('knowledgePage.tabGraph')" />
        <q-tab name="settings" :label="$t('knowledgePage.tabSettings')" />
      </q-tabs>
    </div>

    <q-tab-panels v-model="pageTab" animated class="bg-transparent">
      <!-- P3 资源管理器：统一搜索框双区 + 三栏（Vault 树 / 文档列表 / 详情） -->
      <q-tab-panel name="explorer" class="q-pa-none">
        <knowledge-search-dual
          v-model:query="explorerQuery"
          class="q-mb-md"
          :intent="explorerIntent"
          :instant-results="explorerInstantResults"
          :semantic-results="explorerSemanticResults"
          :semantic-loading="explorerSemanticLoading"
          :semantic-ran="explorerSemanticRan"
          :semantic-error="explorerSemanticError"
          :show-instant="explorerShowInstant"
          :show-semantic="explorerShowSemantic"
          :doc-source-map="explorerDocSourceMap"
          :scope-prefix="explorerScopePrefix"
          :scope-nodes="explorerScopeNodes"
          @search="runExplorerSemantic"
          @select-instant="selectExplorerInstant"
          @select-semantic="selectExplorerSemantic"
          @update:scope-prefix="setExplorerScope"
          @scope-lazy-load="onExplorerLazyLoad"
        />

        <div class="knowledge-explorer-grid">
          <knowledge-vault-tree
            v-model:expanded-keys="explorerExpandedKeys"
            :nodes="explorerRootNodes"
            :selected-key="explorerSelectedKey"
            :loading="explorerTreeLoading"
            :error="explorerTreeError"
            :drag-file="explorerDragFile"
            @select-node="selectExplorerTreeNode"
            @lazy-load="onExplorerLazyLoad"
            @node-action="onTreeNodeAction"
            @create-vault="openCreateCollection"
            @drop-node="onExplorerDropNode"
            @retry="refreshExplorerTree"
          />

          <div class="knowledge-explorer-grid__mid">
            <knowledge-upload-queue
              v-if="uploadTasks.length"
              :tasks="uploadTasks"
              @clear-finished="clearFinishedUploadTasks"
              @remove-task="removeUploadTask"
            />
            <knowledge-doc-list
              :prefix="explorerPrefix"
              :entries="explorerEntries"
              :loading="explorerTreeLoading"
              :selected-doc-id="explorerDocId"
              :drag-file="explorerDragFile"
              :vault-id="selectedId"
              @select="onExplorerSelectDoc"
              @navigate="selectExplorerPrefix"
              @refresh="refreshExplorerTree"
              @ingest="ingestOpen = true"
              @drag-start="explorerDragStart"
              @drag-end="explorerDragEnd"
              @drop-prefix="onExplorerDropPrefix"
            />
          </div>

          <knowledge-doc-detail
            v-model:edit-draft="explorerEditDraft"
            :node="explorerSelectedNode"
            :preview-content="explorerPreviewContent"
            :preview-organized="explorerPreviewOrganized"
            :preview-loading="explorerPreviewLoading"
            :preview-error="explorerPreviewError"
            :links-error="explorerLinksError"
            :links="explorerLinks"
            :links-loading="explorerLinksLoading"
            :link-counts="explorerLinkCounts"
            :media-kind="explorerMediaKind"
            :editable="explorerEditable"
            :editing="explorerEditing"
            :edit-saving="explorerEditSaving"
            :asset-url="explorerAssetUrl"
            :asset-loading="explorerAssetLoading"
            @start-edit="startExplorerEdit"
            @cancel-edit="cancelExplorerEdit"
            @save-edit="onExplorerSaveEdit"
            @download-asset="downloadExplorerAsset"
            @delete="onExplorerDelete"
            @move="onExplorerMove"
            @navigate="onExplorerNavigate"
            @retry="explorer.reloadDetail()"
          />
        </div>
      </q-tab-panel>

      <!-- G4 3D 知识图谱（V12.7）：左 3D 力导向图 + 右操作台 -->
      <q-tab-panel name="graph" class="q-pa-none">
        <knowledge-graph-3-d
          :collections="collections"
          :collection-id="graphCollectionId"
          :link-types="graphLinkTypes"
          :path-prefix="graphPathPrefix"
          :nodes="graphRenderNodes"
          :edges="graphRenderEdges"
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
          @select-collection="graph.selectCollection"
          @toggle-link-type="graph.toggleLinkType"
          @set-path-prefix="graph.setPathPrefix"
          @update:show-isolated="(v: boolean) => (graphShowIsolated = v)"
          @update:node-query="(v: string) => (graphNodeQuery = v)"
          @select-node="graph.selectNode"
          @focus-node="graph.focusNode"
          @open-in-explorer="onGraphOpenInExplorer"
          @scope-lazy-load="graph.onScopeLazyLoad"
        />
      </q-tab-panel>

      <q-tab-panel name="settings" class="q-pa-none">
        <q-card flat class="app-pane-card">
          <q-card-section>
            <knowledge-embedder-panel :config="embedderConfig" :saving="embedderSaving" @save="saveEmbedderConfig" />
          </q-card-section>
        </q-card>
      </q-tab-panel>
    </q-tab-panels>

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
import AppPageHero from '../components/layout/AppPageHero.vue';
import { computed, onMounted, ref, watch } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import KnowledgeEmbedderPanel from '../components/knowledge/KnowledgeEmbedderPanel.vue';
import KnowledgeVaultTree from '../components/knowledge/KnowledgeVaultTree.vue';
import KnowledgeDocList from '../components/knowledge/KnowledgeDocList.vue';
import KnowledgeDocDetail from '../components/knowledge/KnowledgeDocDetail.vue';
import KnowledgeSearchDual from '../components/knowledge/KnowledgeSearchDual.vue';
import KnowledgeGraph3D from '../components/knowledge/KnowledgeGraph3D.vue';
import KnowledgeCreateDialog from '../components/knowledge/KnowledgeCreateDialog.vue';
import KnowledgeIngestDialog from '../components/knowledge/KnowledgeIngestDialog.vue';
import KnowledgeUploadQueue from '../components/knowledge/KnowledgeUploadQueue.vue';
import KnowledgeMoveDialog from '../components/knowledge/KnowledgeMoveDialog.vue';
import KnowledgeMoveConflictDialog from '../components/knowledge/KnowledgeMoveConflictDialog.vue';
import { useKnowledgePage } from '../features/knowledge/useKnowledgePage';
import { useKnowledgeGraph } from '../features/knowledge/useKnowledgeGraph';
import type { MoveDropResult, VaultQTreeNode } from '../features/knowledge/useVaultExplorer';
import type { KnowledgeDocument, VaultTreeNode } from '../features/knowledge/types';

const {
  collections,
  selectedId,
  documents,
  loading,
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
  loadEmbedderConfig,
  openCreateCollection,
  submitCreateCollection,
  submitIngest,
  confirmDeleteDocument,
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
  explorer,
} = useKnowledgePage();

// P3 资源管理器状态（composable 返回 refs，此处解构供模板自动解包）。
const {
  currentPrefix: explorerPrefix,
  rootNodes: explorerRootNodes,
  expandedKeys: explorerExpandedKeys,
  selectedKey: explorerSelectedKey,
  currentChildren: explorerEntries,
  treeLoading: explorerTreeLoading,
  treeError: explorerTreeError,
  onLazyLoad: onExplorerLazyLoad,
  selectPrefix: selectExplorerPrefix,
  selectTreeNode: selectExplorerTreeNode,
  refreshTree: refreshExplorerTree,
  selectedDocId: explorerDocId,
  selectedNode: explorerSelectedNode,
  previewContent: explorerPreviewContent,
  previewOrganized: explorerPreviewOrganized,
  previewLoading: explorerPreviewLoading,
  previewError: explorerPreviewError,
  linksError: explorerLinksError,
  links: explorerLinks,
  linksLoading: explorerLinksLoading,
  linkCounts: explorerLinkCounts,
  mediaKind: explorerMediaKind,
  editable: explorerEditable,
  editing: explorerEditing,
  editDraft: explorerEditDraft,
  editSaving: explorerEditSaving,
  assetUrl: explorerAssetUrl,
  assetLoading: explorerAssetLoading,
  selectDocument,
  navigateToDocument,
  startEdit: startExplorerEdit,
  cancelEdit: cancelExplorerEdit,
  saveEdit: saveExplorerEdit,
  downloadAsset: downloadExplorerAsset,
  searchQuery: explorerQuery,
  searchIntent: explorerIntent,
  instantResults: explorerInstantResults,
  showInstantZone: explorerShowInstant,
  showSemanticZone: explorerShowSemantic,
  semanticResults: explorerSemanticResults,
  semanticLoading: explorerSemanticLoading,
  semanticRan: explorerSemanticRan,
  semanticError: explorerSemanticError,
  docSourceMap: explorerDocSourceMap,
  runSemanticSearch: runExplorerSemantic,
  selectInstant: selectExplorerInstant,
  selectSemanticChunk: selectExplorerSemantic,
  // G3-F1 拖拽移动
  dragFile: explorerDragFile,
  pendingMove: explorerPendingMove,
  // G3-F2 搜索范围选择器
  searchScopePrefix: explorerScopePrefix,
  scopeRootNodes: explorerScopeNodes,
} = explorer;

function onExplorerSelectDoc(node: VaultTreeNode) {
  selectDocument(node.doc_id);
}

// G2-B5 编辑保存：'conflict' = CAS 检测到并发修改（后端已留双份），提示用户注意。
const $q = useQuasar();
const { t } = useI18n();

async function onExplorerSaveEdit() {
  const result = await saveExplorerEdit();
  if (result === 'saved') {
    $q.notify({ type: 'positive', message: t('knowledgePage.detailSaveSuccess') });
  } else if (result === 'conflict') {
    $q.notify({ type: 'warning', message: t('knowledgePage.detailConflictSaved'), timeout: 6000 });
  }
}

/** 详情区操作需要完整 KnowledgeDocument（删除确认/移动对话框复用旧逻辑）；列表未覆盖时按节点合成。 */
function resolveExplorerDoc(): KnowledgeDocument | null {
  const node = explorerSelectedNode.value;
  if (!node) return null;
  const found = documents.value.find((d) => d.id === node.doc_id);
  if (found) return found;
  return { id: node.doc_id, source: node.name, collection_id: selectedId.value } as KnowledgeDocument;
}

function onExplorerDelete() {
  const doc = resolveExplorerDoc();
  if (doc) confirmDeleteDocument(doc);
}

function onExplorerMove() {
  const doc = resolveExplorerDoc();
  if (doc) openMoveDialog(doc);
}

function onExplorerNavigate(payload: { docId: string; relPath: string }) {
  void navigateToDocument(payload.docId, payload.relPath);
}

// ---------- G4 3D 知识图谱（V12.7） ----------
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
  renderGraph: graphRenderGraph,
  nodeQuery: graphNodeQuery,
  nodeList: graphNodeList,
  selectedNodeId: graphSelectedNodeId,
  selectedNode: graphSelectedNode,
  focusSignal: graphFocusSignal,
  scopeNodes: graphScopeNodes,
} = graph;

// 渲染裁剪派生（renderGraph 为 computed，解构后模板自动解包）。
const graphRenderNodes = computed(() => graphRenderGraph.value.nodes);
const graphRenderEdges = computed(() => graphRenderGraph.value.edges);
const graphHiddenIsolated = computed(() => graphRenderGraph.value.hiddenIsolated);

/** 「在浏览中打开」：切浏览 tab 并定位选中文档（跨库时先切库，explorer 内部 watch flush:'sync' 立即复位 prefix）。 */
function onGraphOpenInExplorer(payload: { docId: string; relPath: string }) {
  if (graphCollectionId.value && selectedId.value !== graphCollectionId.value) {
    selectedId.value = graphCollectionId.value;
  }
  pageTab.value = 'explorer';
  void navigateToDocument(payload.docId, payload.relPath);
}

// ---------- G3-F 拖拽移动 + 搜索范围（V12.5/V12.6） ----------

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

async function onExplorerDropPrefix(prefix: string) {
  handleDropResult(await explorer.dropOnTarget({ vaultId: selectedId.value, prefix }), prefix);
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

function setExplorerScope(prefix: string) {
  explorer.setSearchScope(prefix);
}

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
.knowledge-explorer-grid {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr) 360px;
  gap: 16px;
  align-items: start;

  // UX-001：左右栏粘性 + 视口内限高，列内独立滚动——关联区展开不再把整页撑长。
  .knowledge-vault-tree,
  .knowledge-doc-detail {
    position: sticky;
    top: 76px;
    max-height: calc(100vh - 92px);
  }

  &__mid {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-width: 0;
  }
}

@media (max-width: 1200px) {
  .knowledge-explorer-grid {
    grid-template-columns: 220px minmax(0, 1fr);

    .knowledge-doc-detail {
      grid-column: 1 / -1;
      // 堆叠布局下页面滚动为自然行为，取消粘性与限高。
      position: static;
      max-height: none;
    }
  }
}
</style>
