<template>
  <q-page class="app-standard-page app-registry-page artifacts-page">
    <AppPageHero
      kicker="Session artifacts"
      title="制品管理"
      subtitle="上传、浏览与删除会话关联的文件制品。当前为本地 FS 存储；S3/COS 后续支持。"
    >
      <template #actions>
        <q-btn color="primary" unelevated rounded no-caps icon="upload" label="上传" @click="uploadOpen = true" />
      </template>
    </AppPageHero>

    <AppPageToolbar>
      <q-input v-model="sessionFilter" class="app-page-toolbar__field" dense outlined clearable debounce="300" label="按 Session ID 筛选" @update:model-value="onSessionFilterChange" />
      <q-input v-model="search" class="app-page-toolbar__search" dense outlined clearable debounce="300" label="搜索名称 / MIME / Session" @update:model-value="onSearchChange" />
      <q-select v-model="mimeFilter" class="app-page-toolbar__field" dense outlined emit-value map-options :options="mimeFilterOptions" label="MIME" />
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="resetFilters" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </template>
    </AppPageToolbar>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="inventory_2" />
        <div class="text-h6 q-mt-md">暂无制品</div>
        <div class="text-body2 text-grey-7 q-mt-sm">上传文件或调整筛选条件。</div>
      </q-card-section>
    </q-card>

    <template v-else>
      <AppRegistryTable
        :rows="rows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-name="props">
          <q-td :props="props">
            <div class="app-registry-cell-primary">{{ props.row.name }}</div>
          </q-td>
        </template>
        <template #body-cell-size="props">
          <q-td :props="props">{{ formatBytes(props.row.size) }}</q-td>
        </template>
        <template #body-cell-version="props">
          <q-td :props="props">v{{ props.row.version }}</q-td>
        </template>
        <template #body-cell-created_at="props">
          <q-td :props="props">{{ formatDate(props.row.created_at) }}</q-td>
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
      </AppRegistryTable>

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="tableTotal"
        :loading="loading"
        label="个制品"
        :page-size-options="[15, 30, 50]"
      />
    </template>

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
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppPageToolbar from "../components/layout/AppPageToolbar.vue";
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import ArtifactsDetailDialog from "../components/artifact/ArtifactsDetailDialog.vue";
import ArtifactsUploadDialog from "../components/artifact/ArtifactsUploadDialog.vue";
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
  page,
  pageSize,
  pageMax,
  formatBytes,
  formatDate,
  loadRows,
  onSearchChange,
  onSessionFilterChange,
  resetFilters,
  submitUpload,
  openDetail,
  selectDetailVersion,
  onPreviewDownload,
  downloadRow,
  confirmDelete,
  artifactMaxSizeHint,
} = useArtifactsPage();
</script>
