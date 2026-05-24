<template>
  <q-page class="app-page-cream app-registry-page knowledge-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">RAG / pgvector</div>
        <h1 class="app-page-title">知识库</h1>
        <p class="app-page-subtitle">管理向量集合、文档入库与语义检索。需后端配置 Postgres + pgvector。</p>
      </div>
      <div class="app-actions-bar">
        <q-btn color="primary" unelevated rounded no-caps icon="add" label="新建集合" @click="openCreateCollection" />
        <q-btn outline rounded no-caps color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadCollections" />
      </div>
    </section>

    <q-banner v-if="unavailable" rounded class="app-banner-warning q-mb-md">
      知识库服务不可用：{{ unavailable }}。请确认 Postgres / pgvector 已配置。
    </q-banner>
    <knowledge-embedder-panel
      :config="embedderConfig"
      :saving="embedderSaving"
      @save="saveEmbedderConfig"
    />
    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadCollections" />
      </template>
    </q-banner>

    <div class="app-split-layout">
      <knowledge-collection-list
        :collections="collections"
        :selected-id="selectedId"
        :loading="loading"
        @select="selectCollection"
      />

      <div>
        <q-card v-if="selectedCollection" flat class="app-pane-card">
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="app-registry-cell-primary text-h6">{{ selectedCollection.name }}</div>
              <div class="app-registry-cell-sub">{{ selectedCollection.description || "无描述" }}</div>
            </div>
            <q-btn flat no-caps color="negative" icon="delete" label="删除集合" @click="confirmDeleteCollection" />
          </q-card-section>
          <div class="app-tab-shell app-tab-shell--inset">
            <q-tabs v-model="tab" dense align="left" class="text-primary">
              <q-tab name="documents" label="文档" />
              <q-tab name="search" label="检索" />
            </q-tabs>
          </div>
          <q-tab-panels v-model="tab" animated class="app-pane-card__body">
            <q-tab-panel name="documents" class="q-pa-md">
              <knowledge-documents-panel
                :documents="documents"
                :loading="docsLoading"
                @open-ingest="ingestOpen = true"
                @refresh="loadDocuments"
                @delete-document="confirmDeleteDocument"
              />
            </q-tab-panel>
            <q-tab-panel name="search" class="q-pa-md">
              <knowledge-search-panel
                v-model:query="searchQuery"
                v-model:top-k="searchTopK"
                :results="searchResults"
                :loading="searchLoading"
                :searched="searchRan"
                @search="runSearch"
              />
            </q-tab-panel>
          </q-tab-panels>
        </q-card>
        <div v-else class="app-registry-empty app-pane-card">
          <q-icon name="library_books" size="48px" color="grey-6" />
          <div class="text-h6">请选择一个集合</div>
          <div class="text-body2">从左侧列表选择集合，或点击「新建集合」创建。</div>
        </div>
      </div>
    </div>

    <knowledge-create-dialog
      v-model:open="createOpen"
      v-model:name="createForm.name"
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
      v-model:file="ingestFile"
      :loading="ingestLoading"
      @update:file="onIngestFile"
      @submit="submitIngest"
    />
  </q-page>
</template>

<script setup lang="ts">
import { onMounted } from "vue";
import KnowledgeEmbedderPanel from "../components/knowledge/KnowledgeEmbedderPanel.vue";
import KnowledgeCollectionList from "../components/knowledge/KnowledgeCollectionList.vue";
import KnowledgeDocumentsPanel from "../components/knowledge/KnowledgeDocumentsPanel.vue";
import KnowledgeSearchPanel from "../components/knowledge/KnowledgeSearchPanel.vue";
import KnowledgeCreateDialog from "../components/knowledge/KnowledgeCreateDialog.vue";
import KnowledgeIngestDialog from "../components/knowledge/KnowledgeIngestDialog.vue";
import { useKnowledgePage } from "../features/knowledge/useKnowledgePage";
import { useKnowledgeStore } from "../stores/knowledge";

const knowledgeStore = useKnowledgeStore();
const {
  collections,
  selectedId,
  selectedCollection,
  documents,
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
  selectCollection,
  openCreateCollection,
  submitCreateCollection,
  confirmDeleteCollection,
  onIngestFile,
  submitIngest,
  confirmDeleteDocument,
  runSearch
} = useKnowledgePage();

onMounted(() => {
  void loadCollections();
  void knowledgeStore.loadEmbedderConfig().catch((e) => {
    console.warn("[knowledge] embedder config load failed", e);
  });
});
</script>
