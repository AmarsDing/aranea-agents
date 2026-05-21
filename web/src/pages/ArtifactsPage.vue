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
import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import { deleteArtifact, listArtifacts, signDownloadUrl, artifactDownloadHref, uploadArtifact } from "../features/artifact/api";
import ArtifactPreview from "../features/artifact/ArtifactPreview.vue";
import type { ArtifactMeta } from "../features/artifact/types";
import { registryCol } from "../features/ui/registryTableColumns";

const $q = useQuasar();
const rows = ref<ArtifactMeta[]>([]);
const loading = ref(false);
const error = ref("");
const sessionFilter = ref("");
const search = ref("");
const uploadOpen = ref(false);
const uploadLoading = ref(false);
const uploadFile = ref<File | null>(null);
const uploadForm = ref({ session_id: "", name: "", mime_type: "" });
const detailOpen = ref(false);
const detailMeta = ref<ArtifactMeta | null>(null);
const detailArtifactId = ref("");

const columns = [
  { name: "name", label: "名称", field: "name", align: "left" as const, sortable: true, ...registryCol.name },
  { name: "session_id", label: "Session", field: "session_id", align: "left" as const, ...registryCol.session },
  { name: "mime_type", label: "MIME", field: "mime_type", align: "left" as const, ...registryCol.mime },
  { name: "size", label: "大小", field: "size", align: "right" as const, ...registryCol.size },
  { name: "version", label: "版本", field: "version", align: "right" as const, ...registryCol.version },
  { name: "created_at", label: "创建时间", field: "created_at", align: "left" as const, ...registryCol.time },
  { name: "actions", label: "", field: "id", align: "right" as const, ...registryCol.actions }
];

const filteredRows = computed(() => {
  const kw = search.value.trim().toLowerCase();
  if (!kw) return rows.value;
  return rows.value.filter(
    (r) =>
      r.name.toLowerCase().includes(kw) ||
      r.mime_type.toLowerCase().includes(kw) ||
      r.session_id.toLowerCase().includes(kw)
  );
});

function formatBytes(n: number) {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = String(reader.result ?? "");
      const idx = result.indexOf(",");
      resolve(idx >= 0 ? result.slice(idx + 1) : result);
    };
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(file);
  });
}

async function loadRows() {
  loading.value = true;
  error.value = "";
  try {
    const res = await listArtifacts({
      session_id: sessionFilter.value.trim() || undefined,
      limit: 200
    });
    rows.value = res.items;
  } catch (e) {
    error.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    loading.value = false;
  }
}

function onUploadFile(file: File | null) {
  if (!file) return;
  uploadForm.value.name = uploadForm.value.name || file.name;
  uploadForm.value.mime_type = uploadForm.value.mime_type || file.type || "application/octet-stream";
}

async function submitUpload() {
  if (!uploadForm.value.session_id.trim()) {
    $q.notify({ type: "warning", message: "请填写 Session ID" });
    return;
  }
  if (!uploadFile.value) {
    $q.notify({ type: "warning", message: "请选择文件" });
    return;
  }
  uploadLoading.value = true;
  try {
    const dataBase64 = await fileToBase64(uploadFile.value);
    await uploadArtifact({
      session_id: uploadForm.value.session_id.trim(),
      name: uploadForm.value.name || uploadFile.value.name,
      mime_type: uploadForm.value.mime_type,
      data_base64: dataBase64
    });
    uploadOpen.value = false;
    uploadFile.value = null;
    uploadForm.value = { session_id: "", name: "", mime_type: "" };
    await loadRows();
    $q.notify({ type: "positive", message: "上传成功" });
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "上传失败" });
  } finally {
    uploadLoading.value = false;
  }
}

function openDetail(row: ArtifactMeta) {
  detailMeta.value = row;
  detailArtifactId.value = row.id;
  detailOpen.value = true;
}

async function onPreviewDownload(meta: ArtifactMeta) {
  try {
    const signed = await signDownloadUrl(meta.id, meta.version);
    window.open(artifactDownloadHref(signed.url), "_blank", "noopener,noreferrer");
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "获取下载链接失败" });
  }
}

async function downloadRow(row: ArtifactMeta) {
  try {
    const signed = await signDownloadUrl(row.id, row.version);
    window.open(artifactDownloadHref(signed.url), "_blank", "noopener,noreferrer");
  } catch (e) {
    $q.notify({ type: "negative", message: e instanceof Error ? e.message : "获取下载链接失败" });
  }
}

function confirmDelete(row: ArtifactMeta) {
  $q.dialog({
    title: "删除制品",
    message: `确定删除「${row.name}」？`,
    cancel: true
  }).onOk(async () => {
    try {
      await deleteArtifact(row.id);
      await loadRows();
      $q.notify({ type: "positive", message: "已删除" });
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "删除失败" });
    }
  });
}

onMounted(() => {
  void loadRows();
});
</script>
