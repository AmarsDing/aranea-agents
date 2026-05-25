<template>
  <q-page class="app-page-cream app-registry-page artifacts-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Session artifacts</div>
        <h1 class="app-page-title">制品管理</h1>
        <p class="app-page-subtitle">上传、浏览与删除会话关联的文件制品。当前为本地 FS 存储；S3/COS 后续支持。</p>
      </div>
      <div class="app-actions-bar">
        <q-btn color="primary" unelevated rounded no-caps icon="upload" label="上传" @click="uploadOpen = true" />
        <q-btn outline rounded no-caps color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </div>
    </section>

    <q-card flat class="app-registry-panel">
      <q-card-section class="app-form-field-grid app-registry-toolbar app-form-field-grid--2col items-center">
        <q-input v-model="sessionFilter" class="app-field-md" dense outlined clearable label="按 Session ID 筛选" />
        <div class="row q-gutter-sm items-center">
          <q-input v-model="search" class="app-field-md col" dense outlined clearable debounce="300" label="搜索名称 / MIME / Session" @update:model-value="onSearchChange" />
          <q-select v-model="mimeFilter" class="app-field-sm" dense outlined emit-value map-options :options="mimeFilterOptions" label="MIME" style="min-width: 140px" />
        </div>
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <div class="app-registry-table-shell">
      <q-table
        flat
        dense
        class="app-registry-table"
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        v-model:pagination="pagination"
        :rows-per-page-options="[15, 30, 50]"
        :rows-number="tableTotal"
        @request="onTableRequest"
      >
        <template #body-cell-name="props">
          <q-td :props="props">
            <div class="app-registry-cell-primary">{{ props.row.name }}</div>
          </q-td>
        </template>
        <template #body-cell-size="props">
          <q-td :props="props">{{ formatBytes(props.row.size) }}</q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <div class="app-registry-cell-actions">
              <q-btn flat dense round color="primary" icon="visibility" @click="openDetail(props.row)">
                <q-tooltip>查看</q-tooltip>
              </q-btn>
              <q-btn flat dense round color="primary" icon="download" @click="downloadRow(props.row)">
                <q-tooltip>签名下载</q-tooltip>
              </q-btn>
              <q-btn flat dense round color="negative" icon="delete" @click="confirmDelete(props.row)">
                <q-tooltip>删除</q-tooltip>
              </q-btn>
            </div>
          </q-td>
        </template>
      </q-table>
    </div>

    <ArtifactsUploadDialog
      v-model:open="uploadOpen"
      v-model:file="uploadFile"
      v-model:session-id="uploadForm.session_id"
      v-model:name="uploadForm.name"
      v-model:mime-type="uploadForm.mime_type"
      :loading="uploadLoading"
      :max-size-hint="artifactMaxSizeHint()"
      @submit="submitUpload"
    />

    <ArtifactsDetailDialog
      v-model:open="detailOpen"
      :meta="detailMeta"
      :artifact-id="detailArtifactId"
      :selected-version="detailVersion"
      :versions="detailVersions"
      :format-bytes="formatBytes"
      @select-version="selectDetailVersion"
      @download="onPreviewDownload"
    />
  </q-page>
</template>

<script setup lang="ts">
import ArtifactsDetailDialog from "../features/artifact/components/ArtifactsDetailDialog.vue";
import ArtifactsUploadDialog from "../features/artifact/components/ArtifactsUploadDialog.vue";
import { useArtifactsPage } from "../features/artifact/useArtifactsPage";

const {
  loading,
  error,
  sessionFilter,
  search,
  uploadOpen,
  uploadLoading,
  uploadFile,
  uploadForm,
  detailOpen,
  detailMeta,
  detailArtifactId,
  detailVersions,
  detailVersion,
  mimeFilter,
  mimeFilterOptions,
  columns,
  rows,
  tableTotal,
  pagination,
  formatBytes,
  loadRows,
  onTableRequest,
  onSearchChange,
  submitUpload,
  openDetail,
  selectDetailVersion,
  onPreviewDownload,
  downloadRow,
  confirmDelete,
  artifactMaxSizeHint,
} = useArtifactsPage();
</script>
