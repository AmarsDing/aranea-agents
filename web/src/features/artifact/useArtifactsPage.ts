import { computed, onMounted, ref, watch } from "vue";
import { useQuasar } from "quasar";
import { registryCol } from "../ui/registryTableColumns";
import type { ArtifactMeta } from "./types";
import { useArtifactStore } from "../../stores/artifact";

export function useArtifactsPage() {
  const $q = useQuasar();
  const artifactStore = useArtifactStore();

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
      const res = await artifactStore.loadArtifacts({
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
    detailOpen.value = true;
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

  onMounted(() => {
    void loadRows();
  });

  watch(sessionFilter, () => {
    void loadRows();
  });

  return {
    rows,
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
  };
}
