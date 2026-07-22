import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { ARTIFACT_TABLE_COLUMNS } from './artifactTableUi';
import type { ArtifactMeta } from './types';
import { useArtifactStore } from '../../stores/artifact';
import { validateArtifactFileSize, artifactMaxSizeHint } from './limits';
import { readFileAsBase64 } from './fileBase64';
import { formatBytes, formatDate } from '../../shared/format';

export type ArtifactsPageTab = 'session' | 'all';

export type ArtifactSessionGroup = {
  sessionId: string;
  items: ArtifactMeta[];
  totalSize: number;
};

export function useArtifactsPage() {
  const $q = useQuasar();
  const route = useRoute();
  const router = useRouter();
  const artifactStore = useArtifactStore();
  const { t } = useI18n();

  const rows = ref<ArtifactMeta[]>([]);
  const loading = ref(false);
  const error = ref('');
  const activeTab = ref<ArtifactsPageTab>('all');
  const sessionFilter = ref('');
  const search = ref('');
  const mimeFilter = ref('');
  const uploadOpen = ref(false);
  const uploadLoading = ref(false);
  const uploadFile = ref<File | null>(null);
  const uploadForm = ref({ session_id: '', name: '', mime_type: '' });
  const detailOpen = ref(false);
  const detailMeta = ref<ArtifactMeta | null>(null);
  const detailArtifactId = ref('');
  const detailVersions = ref<ArtifactMeta[]>([]);
  const detailVersion = ref<number | undefined>(undefined);
  const tableTotal = ref(0);
  const page = ref(1);
  const pageSize = ref(15);
  const pageMax = computed(() => Math.max(1, Math.ceil(tableTotal.value / pageSize.value)));

  const columns = ARTIFACT_TABLE_COLUMNS;

  /** 会话 Tab 下未填写 Session ID 时展示提示而非加载全部。 */
  const sessionFilterRequired = computed(() => activeTab.value === 'session' && !sessionFilter.value.trim());

  /** 「全部产物」Tab：当前页按 session 分组。 */
  const groupedRows = computed<ArtifactSessionGroup[]>(() => {
    if (activeTab.value !== 'all') return [];
    const map = new Map<string, ArtifactSessionGroup>();
    for (const row of rows.value) {
      const key = row.session_id || t('artifact.page.noSession');
      let g = map.get(key);
      if (!g) {
        g = { sessionId: key, items: [], totalSize: 0 };
        map.set(key, g);
      }
      g.items.push(row);
      g.totalSize += row.size;
    }
    return [...map.values()].sort((a, b) => b.totalSize - a.totalSize);
  });

  const mimeFilterOptions = computed(() => [
    { label: t('artifact.page.mimeAll'), value: '' },
    { label: t('artifact.page.mimeImage'), value: 'image/' },
    { label: t('artifact.page.mimeText'), value: 'text/' },
    { label: 'PDF', value: 'application/pdf' },
    { label: 'JSON', value: 'application/json' },
  ]);

  async function loadRows() {
    if (sessionFilterRequired.value) {
      rows.value = [];
      tableTotal.value = 0;
      error.value = '';
      return;
    }
    loading.value = true;
    error.value = '';
    try {
      const { page: currentPage, rowsPerPage } = { page: page.value, rowsPerPage: pageSize.value };
      const offset = (currentPage - 1) * rowsPerPage;
      const res = await artifactStore.loadArtifacts({
        session_id: sessionFilter.value.trim() || undefined,
        limit: rowsPerPage,
        offset,
        query: search.value.trim() || undefined,
        mime_type_prefix: mimeFilter.value || undefined,
      });
      rows.value = res.items;
      tableTotal.value = res.total;
    } catch (e) {
      error.value = e instanceof Error ? e.message : t('artifact.page.loadFailed');
    } finally {
      loading.value = false;
    }
  }

  function onTabChange(tab: ArtifactsPageTab) {
    if (activeTab.value === tab) return;
    activeTab.value = tab;
    page.value = 1;
    if (tab === 'all') {
      sessionFilter.value = '';
      if (route.query.session !== undefined) {
        void router.replace({ query: { ...route.query, session: undefined } });
      }
    } else {
      const q = route.query.session;
      if (typeof q === 'string' && q) sessionFilter.value = q;
    }
    void loadRows();
  }

  function onUploadFile(file: File | null) {
    if (!file) return;
    const sizeErr = validateArtifactFileSize(file.size);
    if (sizeErr) {
      $q.notify({ type: 'warning', message: sizeErr });
      uploadFile.value = null;
      return;
    }
    uploadForm.value.name = uploadForm.value.name || file.name;
    uploadForm.value.mime_type = uploadForm.value.mime_type || file.type || 'application/octet-stream';
  }

  watch(uploadFile, (file) => {
    onUploadFile(file);
  });

  async function submitUpload() {
    if (!uploadForm.value.session_id.trim()) {
      $q.notify({ type: 'warning', message: t('artifact.page.uploadNeedSession') });
      return;
    }
    if (!uploadFile.value) {
      $q.notify({ type: 'warning', message: t('artifact.page.uploadNeedFile') });
      return;
    }
    const sizeErr = validateArtifactFileSize(uploadFile.value.size);
    if (sizeErr) {
      $q.notify({ type: 'warning', message: sizeErr });
      return;
    }
    uploadLoading.value = true;
    try {
      const dataBase64 = await readFileAsBase64(uploadFile.value);
      await artifactStore.upload({
        session_id: uploadForm.value.session_id.trim(),
        name: uploadForm.value.name || uploadFile.value.name,
        mime_type: uploadForm.value.mime_type,
        data_base64: dataBase64,
      });
      uploadOpen.value = false;
      uploadFile.value = null;
      uploadForm.value = { session_id: '', name: '', mime_type: '' };
      await loadRows();
      $q.notify({ type: 'positive', message: t('artifact.page.uploadSuccess') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('artifact.page.uploadFailed') });
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
    void artifactStore
      .listVersions(row.id)
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
      window.open(artifactStore.artifactDownloadHref(signed.url), '_blank', 'noopener,noreferrer');
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('artifact.page.downloadLinkFailed') });
    }
  }

  async function downloadRow(row: ArtifactMeta) {
    try {
      const signed = await artifactStore.signDownload(row.id, row.version);
      window.open(artifactStore.artifactDownloadHref(signed.url), '_blank', 'noopener,noreferrer');
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('artifact.page.downloadLinkFailed') });
    }
  }

  function confirmDelete(row: ArtifactMeta) {
    $q.dialog({
      title: t('artifact.page.deleteTitle'),
      message: t('artifact.page.deleteConfirm', { name: row.name }),
      cancel: true,
    }).onOk(async () => {
      try {
        await artifactStore.remove(row.id);
        await loadRows();
        $q.notify({ type: 'positive', message: t('artifact.page.deleted') });
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('artifact.page.deleteFailed') });
      }
    });
  }

  function onSearchChange() {
    page.value = 1;
    void loadRows();
  }

  function onSessionFilterChange() {
    page.value = 1;
    void loadRows();
  }

  function resetFilters() {
    if (activeTab.value === 'all') sessionFilter.value = '';
    search.value = '';
    mimeFilter.value = '';
    page.value = 1;
    void loadRows();
  }

  onMounted(() => {
    const q = route.query.session;
    if (typeof q === 'string' && q) {
      sessionFilter.value = q;
      activeTab.value = 'session';
      uploadForm.value.session_id = q;
    }
    void loadRows();
  });

  watch(mimeFilter, () => {
    page.value = 1;
    void loadRows();
  });

  watch([page, pageSize], () => void loadRows());

  return {
    rows,
    loading,
    error,
    activeTab,
    sessionFilterRequired,
    groupedRows,
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
    page,
    pageSize,
    pageMax,
    columns,
    formatBytes,
    formatDate,
    loadRows,
    onTabChange,
    onSearchChange,
    onSessionFilterChange,
    resetFilters,
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
