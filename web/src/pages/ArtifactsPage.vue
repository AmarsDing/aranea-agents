<template>
  <q-page class="app-page-cream artifacts-page q-pa-sm q-pa-md-md">
    <section class="artifacts-hero">
      <div>
        <div class="artifacts-kicker">Session artifacts</div>
        <h1 class="artifacts-title">制品管理</h1>
        <p class="artifacts-subtitle">上传、浏览与删除会话关联的文件制品。存储后端依部署配置（本地或对象存储）。</p>
      </div>
      <div class="row q-gutter-sm">
        <q-btn color="primary" rounded unelevated icon="upload" label="上传" @click="uploadOpen = true" />
        <q-btn outline rounded color="primary" icon="refresh" label="刷新" :loading="loading" @click="loadRows" />
      </div>
    </section>

    <q-card flat bordered class="q-mb-md">
      <q-card-section class="row q-col-gutter-md items-center">
        <q-input v-model="sessionFilter" class="col-12 col-md-5" dense outlined clearable label="按 Session ID 筛选" />
        <q-input v-model="search" class="col-12 col-md-5" dense outlined clearable debounce="200" label="搜索名称 / MIME" />
      </q-card-section>
    </q-card>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" label="重试" @click="loadRows" />
      </template>
    </q-banner>

    <q-card flat bordered>
      <q-table flat :rows="filteredRows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 15 }">
        <template #body-cell-size="props">
          <q-td :props="props">{{ formatBytes(props.row.size) }}</q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props">
            <q-btn flat dense round color="primary" icon="visibility" @click="openDetail(props.row)">
              <q-tooltip>查看</q-tooltip>
            </q-btn>
            <q-btn flat dense round color="primary" icon="download" @click="downloadRow(props.row)">
              <q-tooltip>签名下载</q-tooltip>
            </q-btn>
            <q-btn flat dense round color="negative" icon="delete" @click="confirmDelete(props.row)">
              <q-tooltip>删除</q-tooltip>
            </q-btn>
          </q-td>
        </template>
      </q-table>
    </q-card>

    <q-dialog v-model="uploadOpen" persistent>
      <q-card style="min-width: 420px; max-width: 94vw">
        <q-card-section class="text-h6">上传制品</q-card-section>
        <q-card-section class="q-gutter-md">
          <q-input v-model="uploadForm.session_id" dense outlined label="Session ID" />
          <q-input v-model="uploadForm.name" dense outlined label="文件名" />
          <q-input v-model="uploadForm.mime_type" dense outlined label="MIME" placeholder="application/octet-stream" />
          <q-file v-model="uploadFile" label="选择文件" outlined dense @update:model-value="onUploadFile" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="取消" v-close-popup />
          <q-btn color="primary" unelevated label="上传" :loading="uploadLoading" @click="submitUpload" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="detailOpen">
      <q-card style="min-width: 520px; max-width: 96vw">
        <q-card-section class="text-h6">{{ detailMeta?.name }}</q-card-section>
        <q-card-section v-if="detailMeta" class="q-gutter-sm text-body2">
          <div><b>ID：</b>{{ detailMeta.id }}</div>
          <div><b>Session：</b>{{ detailMeta.session_id }}</div>
          <div><b>SHA256：</b><span class="text-caption">{{ detailMeta.sha256 }}</span></div>
          <div><b>存储：</b>{{ detailMeta.storage_kind }} — {{ detailMeta.storage_uri }}</div>
          <div><b>大小：</b>{{ formatBytes(detailMeta.size) }} · v{{ detailMeta.version }}</div>
        </q-card-section>
        <q-card-section v-if="detailPreview">
          <pre class="artifact-preview">{{ detailPreview }}</pre>
        </q-card-section>
        <q-card-section v-else-if="detailLoading" class="text-center">
          <q-spinner color="primary" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat label="关闭" v-close-popup />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import { deleteArtifact, listArtifacts, previewArtifact, signDownloadUrl, artifactDownloadHref, uploadArtifact } from "../features/artifact/api";
import type { ArtifactMeta } from "../features/artifact/types";

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
const detailPreview = ref("");
const detailLoading = ref(false);

const columns = [
  { name: "name", label: "名称", field: "name", align: "left" as const, sortable: true },
  { name: "session_id", label: "Session", field: "session_id", align: "left" as const },
  { name: "mime_type", label: "MIME", field: "mime_type", align: "left" as const },
  { name: "size", label: "大小", field: "size", align: "right" as const },
  { name: "version", label: "版本", field: "version", align: "right" as const },
  { name: "created_at", label: "创建时间", field: "created_at", align: "left" as const },
  { name: "actions", label: "", field: "id", align: "right" as const }
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

async function openDetail(row: ArtifactMeta) {
  detailMeta.value = row;
  detailPreview.value = "";
  detailOpen.value = true;
  detailLoading.value = true;
  try {
    const preview = await previewArtifact(row.id);
    detailMeta.value = preview.meta;
    if (preview.preview_kind === "text") {
      detailPreview.value = preview.text_content;
    } else if (preview.preview_kind === "image" && preview.data_base64) {
      detailPreview.value = `[图片预览] data:${preview.meta.mime_type};base64,${preview.data_base64.slice(0, 80)}…`;
    } else {
      detailPreview.value = `[${preview.preview_kind} 内容 ${formatBytes(preview.meta.size)}]`;
    }
  } catch (e) {
    detailPreview.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    detailLoading.value = false;
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

<style scoped>
.artifacts-hero {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 1.25rem;
}
.artifacts-kicker {
  font-size: 0.75rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--q-primary);
  font-weight: 600;
}
.artifacts-title {
  margin: 0.25rem 0;
  font-size: 1.75rem;
  font-weight: 700;
}
.artifacts-subtitle {
  margin: 0;
  color: #666;
  max-width: 36rem;
}
.artifact-preview {
  max-height: 320px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  background: #f5f5f5;
  padding: 12px;
  border-radius: 8px;
}
</style>
