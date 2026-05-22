<template>
  <q-page class="app-page-cream app-registry-page artifacts-page">
    <section class="app-page-hero">
      <div>
        <div class="app-page-kicker">Session artifacts</div>
        <h1 class="app-page-title">制品管理</h1>
        <p class="app-page-subtitle">上传、浏览与删除会话关联的文件制品。存储后端依部署配置（本地或对象存储）。</p>
      </div>
      <div class="app-actions-bar">
        <q-btn color="primary" unelevated rounded no-caps icon="upload" label="上传" @click="uploadOpen = true" />
        <q-btn outline rounded no-caps color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </div>
    </section>

    <q-card flat class="app-registry-panel">
      <q-card-section class="app-form-field-grid app-registry-toolbar app-form-field-grid--2col items-center">
        <q-input v-model="sessionFilter" class="app-field-md" dense outlined clearable label="按 Session ID 筛选" />
        <q-input v-model="search" class="app-field-md" dense outlined clearable debounce="200" label="搜索名称 / MIME" />
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <div class="app-registry-table-shell">
      <q-table flat dense class="app-registry-table" :rows="filteredRows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 15 }">
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

    <q-dialog v-model="uploadOpen" persistent>
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section class="text-h6">上传制品</q-card-section>
        <q-card-section class="app-dialog-body q-gutter-md q-pt-none">
          <q-input v-model="uploadForm.session_id" class="app-field-md" dense outlined label="Session ID" />
          <q-input v-model="uploadForm.name" class="app-field-md" dense outlined label="文件名" />
          <q-input v-model="uploadForm.mime_type" class="app-field-sm" dense outlined label="MIME" placeholder="application/octet-stream" />
          <q-file v-model="uploadFile" label="选择文件" outlined dense @update:model-value="onUploadFile" />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="取消" v-close-popup />
          <q-btn color="primary" unelevated no-caps label="上传" :loading="uploadLoading" @click="submitUpload" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="detailOpen">
      <q-card class="app-dialog-card app-dialog-card--md">
        <q-card-section class="text-h6">{{ detailMeta?.name }}</q-card-section>
        <q-card-section v-if="detailMeta" class="app-dialog-body q-gutter-sm q-pt-none text-body2">
          <div><b>ID：</b>{{ detailMeta.id }}</div>
          <div><b>Session：</b>{{ detailMeta.session_id }}</div>
          <div><b>SHA256：</b><span class="text-caption">{{ detailMeta.sha256 }}</span></div>
          <div><b>存储：</b>{{ detailMeta.storage_kind }} — {{ detailMeta.storage_uri }}</div>
          <div><b>大小：</b>{{ formatBytes(detailMeta.size) }} · v{{ detailMeta.version }}</div>
        </q-card-section>
        <q-card-section v-if="detailArtifactId">
          <ArtifactPreview :artifact-id="detailArtifactId" :show-download="true" @download="onPreviewDownload" />
        </q-card-section>
        <q-card-actions align="right" class="app-actions-bar">
          <q-btn flat no-caps label="关闭" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import ArtifactPreview from "../features/artifact/ArtifactPreview.vue";
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
  columns,
  filteredRows,
  formatBytes,
  loadRows,
  onUploadFile,
  submitUpload,
  openDetail,
  onPreviewDownload,
  downloadRow,
  confirmDelete
} = useArtifactsPage();
</script>
