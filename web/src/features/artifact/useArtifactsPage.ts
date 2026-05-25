import { onMounted, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { registryCol } from "../ui/registryTableColumns";
import type { ArtifactMeta } from "./types";
import { listArtifactVersions } from "./api";
import { useArtifactStore } from "../../stores/artifact";
import { validateArtifactFileSize, artifactMaxSizeHint } from "./limits";
import { readFileAsBase64 } from "./fileBase64";

export function useArtifactsPage() {
  const $q = useQuasar();
  const artifactStore = useArtifactStore();

  const rows = ref<ArtifactMeta[]>([]);
  const loading = ref(false);
  const error = ref("");
  const sessionFilter = ref("");
  const search = ref("");
  const mimeFilter = ref("");
  const uploadOpen = ref(false);
  const uploadLoading = ref(false);
  const uploadFile = ref<File | null>(null);
  const uploadForm = ref({ session_id: "", name: "", mime_type: "" });
  const detailOpen = ref(false);
  const detailMeta = ref<ArtifactMeta | null>(null);
  const detailArtifactId = ref("");
  const detailVersions = ref<ArtifactMeta[]>([]);
  const detailVersion = ref<number | undefined>(undefined);
  const tableTotal = ref(0);
  const pagination = ref({ page: 1, rowsPerPage: 15 });

  const columns = [
    { name: "name", label: "名称", field: "name", align: "left" as const, sortable: true, ...registryCol.name },
    { name: "session_id", label: "Session", field: "session_id", align: "left" as const, ...registryCol.session },
    { name: "mime_type", label: "MIME", field: "mime_type", align: "left" as const, ...registryCol.mime },
    { name: "size", label: "大小", field: "size", align: "right" as const, ...registryCol.size },
    { name: "version", label: "版本", field: "version", align: "right" as const, ...registryCol.version },
    { name: "created_at", label: "创建时间", field: "created_at", align: "left" as const, ...registryCol.time },
    { name: "actions", label: "", field: "id", align: "right" as const, ...registryCol.actions }
  ];

  const mimeFilterOptions = [
    { label: "全部类型", value: "" },
    { label: "图片", value: "image/" },
    { label: "文本/代码", value: "text/" },
    { label: "PDF", value: "application/pdf" },
    { label: "JSON", value: "application/json" }
  ];

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

  async function loadRows() {
    loading.value = true;
    error.value = "";
    try {
      const { page, rowsPerPage } = pagination.value;
      const offset = (page - 1) * rowsPerPage;
      const res = await artifactStore.loadArtifacts({
        session_id: sessionFilter.value.trim() || undefined,
        limit: rowsPerPage,
        offset,
        query: search.value.trim() || undefined,
        mime_type_prefix: mimeFilter.value || undefined
      });
      rows.value = res.items;
      tableTotal.value = res.total;
    } catch (e) {
      error.value = e instanceof Error ? e.message : "加载失败";
    } finally {
      loading.value = false;
    }
  }

  function onTableRequest(props: { pagination: { page: number; rowsPerPage: number } }) {
    pagination.value = { ...props.pagination };
    void loadRows();
  }

  function onUploadFile(file: File | null) {
    if (!file) return;
    const sizeErr = validateArtifactFileSize(file.size);
    if (sizeErr) {
      $q.notify({ type: "warning", message: sizeErr });
      uploadFile.value = null;
      return;
    }
    uploadForm.value.name = uploadForm.value.name || file.name;
    uploadForm.value.mime_type = uploadForm.value.mime_type || file.type || "application/octet-stream";
  }

  watch(uploadFile, (file) => {
    onUploadFile(file);
  });

  async function submitUpload() {
    if (!uploadForm.value.session_id.trim()) {
      $q.notify({ type: "warning", message: "请填写 Session ID" });
      return;
    }
    if (!uploadFile.value) {
      $q.notify({ type: "warning", message: "请选择文件" });
      return;
    }
    const sizeErr = validateArtifactFileSize(uploadFile.value.size);
    if (sizeErr) {
      $q.notify({ type: "warning", message: sizeErr });
      return;
    }
    uploadLoading.value = true;
    try {
      const dataBase64 = await readFileAsBase64(uploadFile.value);
      await artifactStore.upload({
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
    detailVersion.value = row.version;
    detailVersions.value = [];
    detailOpen.value = true;
    void listArtifactVersions(row.id)
      .then((items) => {
        detailVersions.value = items;
      })
      .catch(() => {
        detailVersions.value = [];
      });
  }

  function selectDetailVersion(v: ArtifactMeta) {
    detailMeta.value = v;
    detailArtifactId.value = v.id;
    detailVersion.value = v.version;
  }

  async function onPreviewDownload(meta: ArtifactMeta) {
    try {
      const signed = await artifactStore.signDownload(meta.id, meta.version);
      window.open(artifactStore.artifactDownloadHref(signed.url), "_blank", "noopener,noreferrer");
    } catch (e) {
      $q.notify({ type: "negative", message: e instanceof Error ? e.message : "获取下载链接失败" });
    }
  }

  async function downloadRow(row: ArtifactMeta) {
    try {
      const signed = await artifactStore.signDownload(row.id, row.version);
      window.open(artifactStore.artifactDownloadHref(signed.url), "_blank", "noopener,noreferrer");
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
        await artifactStore.remove(row.id);
        await loadRows();
        $q.notify({ type: "positive", message: "已删除" });
      } catch (e) {
        $q.notify({ type: "negative", message: e instanceof Error ? e.message : "删除失败" });
      }
    });
  }

  function onSearchChange() {
    pagination.value.page = 1;
    void loadRows();
  }

  onMounted(() => {
    void loadRows();
  });

  watch(sessionFilter, () => {
    pagination.value.page = 1;
    void loadRows();
  });

  watch([mimeFilter], () => {
    pagination.value.page = 1;
    void loadRows();
  });

  return {
    rows,
    loading,
    error,
    sessionFilter,
    search,
    mimeFilter,
    mimeFilterOptions,
    uploadOpen,
    uploadLoading,
    uploadFile,
    uploadForm,
    detailOpen,
    detailMeta,
    detailArtifactId,
    detailVersions,
    detailVersion,
    tableTotal,
    pagination,
    columns,
    formatBytes,
    loadRows,
    onTableRequest,
    onSearchChange,
    onUploadFile,
    submitUpload,
    openDetail,
    selectDetailVersion,
    onPreviewDownload,
    downloadRow,
    confirmDelete,
    artifactMaxSizeHint,
  };
}
