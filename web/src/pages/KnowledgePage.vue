<template>
  <q-page class="app-standard-page app-registry-page knowledge-page">
    <AppPageHero
      kicker="RAG / Vault"
      title="知识库"
      subtitle="以本地文件夹为真相源的 Vault 知识库：文件夹树导航、摘要卡、双轨关联与语义检索。"
    >
      <template #actions>
        <q-btn color="primary" unelevated rounded no-caps icon="add" label="新建集合" @click="openCreateCollection" />
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
        <q-tab name="search" :label="$t('knowledgePage.tabSearch')" />
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
          :show-instant="explorerShowInstant"
          :show-semantic="explorerShowSemantic"
          :doc-source-map="explorerDocSourceMap"
          @search="runExplorerSemantic"
          @select-instant="selectExplorerInstant"
          @select-semantic="selectExplorerSemantic"
        />

        <div class="knowledge-explorer-grid">
          <knowledge-vault-tree
            :collections="collections"
            :selected-vault-id="selectedId"
            :nodes="explorerRootNodes"
            :selected-prefix="explorerPrefix"
            :loading="explorerTreeLoading"
            :error="explorerTreeError"
            @select-vault="selectCollection"
            @select-prefix="selectExplorerPrefix"
            @lazy-load="onExplorerLazyLoad"
            @delete-vault="confirmDeleteCollection"
          />

          <div class="knowledge-explorer-grid__mid">
            <!-- US-14 上传免预选：拖拽区常驻中栏顶部，未选中集合时自动落入「默认知识库」 -->
            <knowledge-drop-zone @files-selected="enqueueUploadFiles" />
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
              @select="onExplorerSelectDoc"
              @navigate="selectExplorerPrefix"
              @refresh="refreshExplorerTree"
              @ingest="ingestOpen = true"
            />
          </div>

          <knowledge-doc-detail
            :node="explorerSelectedNode"
            :expanded="explorerExpanded"
            :preview-content="explorerPreviewContent"
            :preview-organized="explorerPreviewOrganized"
            :preview-loading="explorerPreviewLoading"
            :links="explorerLinks"
            :links-loading="explorerLinksLoading"
            @toggle-expand="toggleExplorerDetail"
            @delete="onExplorerDelete"
            @move="onExplorerMove"
            @navigate="onExplorerNavigate"
          />
        </div>
      </q-tab-panel>

      <q-tab-panel name="search" class="q-pa-none">
        <q-card flat class="app-pane-card">
          <q-card-section>
            <knowledge-search-panel
              v-model:query="searchQuery"
              v-model:top-k="searchTopK"
              v-model:min-score="searchMinScore"
              v-model:hybrid-mode="searchHybridMode"
              v-model:rewrite-strategy="searchRewriteStrategy"
              v-model:use-rerank="searchUseRerank"
              v-model:scope-id="searchScopeId"
              :scope-options="searchScopeOptions"
              :results="searchResults"
              :doc-source-map="docSourceMap"
              :loading="searchLoading"
              :searched="searchRan"
              @search="runSearch"
            />
          </q-card-section>
        </q-card>
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
  </q-page>
</template>

<script setup lang="ts">
import AppPageHero from '../components/layout/AppPageHero.vue';
import { onMounted } from 'vue';
import KnowledgeEmbedderPanel from '../components/knowledge/KnowledgeEmbedderPanel.vue';
import KnowledgeVaultTree from '../components/knowledge/KnowledgeVaultTree.vue';
import KnowledgeDocList from '../components/knowledge/KnowledgeDocList.vue';
import KnowledgeDocDetail from '../components/knowledge/KnowledgeDocDetail.vue';
import KnowledgeSearchDual from '../components/knowledge/KnowledgeSearchDual.vue';
import KnowledgeSearchPanel from '../components/knowledge/KnowledgeSearchPanel.vue';
import KnowledgeCreateDialog from '../components/knowledge/KnowledgeCreateDialog.vue';
import KnowledgeIngestDialog from '../components/knowledge/KnowledgeIngestDialog.vue';
import KnowledgeDropZone from '../components/knowledge/KnowledgeDropZone.vue';
import KnowledgeUploadQueue from '../components/knowledge/KnowledgeUploadQueue.vue';
import KnowledgeMoveDialog from '../components/knowledge/KnowledgeMoveDialog.vue';
import { useKnowledgePage } from '../features/knowledge/useKnowledgePage';
import type { KnowledgeDocument, VaultTreeNode } from '../features/knowledge/types';

const {
  collections,
  selectedId,
  documents,
  docSourceMap,
  loading,
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
  loadEmbedderConfig,
  selectCollection,
  openCreateCollection,
  submitCreateCollection,
  confirmDeleteCollection,
  submitIngest,
  confirmDeleteDocument,
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
} = useKnowledgePage();

// P3 资源管理器状态（composable 返回 refs，此处解构供模板自动解包）。
const {
  currentPrefix: explorerPrefix,
  rootNodes: explorerRootNodes,
  currentChildren: explorerEntries,
  treeLoading: explorerTreeLoading,
  treeError: explorerTreeError,
  onLazyLoad: onExplorerLazyLoad,
  selectPrefix: selectExplorerPrefix,
  refreshTree: refreshExplorerTree,
  selectedDocId: explorerDocId,
  selectedNode: explorerSelectedNode,
  detailExpanded: explorerExpanded,
  previewContent: explorerPreviewContent,
  previewOrganized: explorerPreviewOrganized,
  previewLoading: explorerPreviewLoading,
  links: explorerLinks,
  linksLoading: explorerLinksLoading,
  selectDocument,
  navigateToDocument,
  toggleDetail: toggleExplorerDetail,
  searchQuery: explorerQuery,
  searchIntent: explorerIntent,
  instantResults: explorerInstantResults,
  showInstantZone: explorerShowInstant,
  showSemanticZone: explorerShowSemantic,
  semanticResults: explorerSemanticResults,
  semanticLoading: explorerSemanticLoading,
  semanticRan: explorerSemanticRan,
  docSourceMap: explorerDocSourceMap,
  runSemanticSearch: runExplorerSemantic,
  selectInstant: selectExplorerInstant,
  selectSemanticChunk: selectExplorerSemantic,
} = explorer;

function onExplorerSelectDoc(node: VaultTreeNode) {
  selectDocument(node.doc_id);
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
    }
  }
}
</style>
