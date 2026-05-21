<template>
  <q-page class="app-page-cream knowledge-page q-pa-sm q-pa-md-md">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">RAG / pgvector</div>
        <h1 class="app-page-title">知识库</h1>
        <p class="app-page-subtitle">管理向量集合、文档入库与语义检索。需后端配置 Postgres + pgvector。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn color="primary" rounded unelevated icon="add" label="新建集合" @click="openCreateCollection" />
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadCollections" />
      </div>
    </section>

    <q-banner v-if="unavailable" rounded class="bg-warning text-dark q-mb-md">
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

    <div class="row q-col-gutter-md">
      <div class="col-12 col-lg-4">
        <knowledge-collection-list
          :collections="collections"
          :selected-id="selectedId"
          :loading="loading"
          @select="selectCollection"
        />
      </div>

      <div class="col-12 col-lg-8">
        <q-card v-if="selectedCollection" flat bordered>
          <q-card-section class="row items-center justify-between">
            <div>
              <div class="text-h6">{{ selectedCollection.name }}</div>
              <div class="text-caption text-grey-7">{{ selectedCollection.description || "无描述" }}</div>
            </div>
            <q-btn flat color="negative" icon="delete" label="删除集合" @click="confirmDeleteCollection" />
          </q-card-section>
          <q-tabs v-model="tab" dense align="left" class="text-primary">
            <q-tab name="documents" label="文档" />
            <q-tab name="search" label="检索" />
          </q-tabs>
          <q-separator />
          <q-tab-panels v-model="tab" animated>
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
        <q-card v-else flat bordered class="q-pa-lg text-center text-grey-7">
          请从左侧选择一个集合，或新建集合。
        </q-card>
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
  void knowledgeStore.loadEmbedderConfig();
});
</script>
