import { computed, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { createArtifactColumns } from './artifactTableUi';
import type { ArtifactMeta } from './types';
import { useArtifactStore } from '../../stores/artifact';
import { searchSessions } from '../session/api';
import type { Session } from '../session/types';
import { validateArtifactFileSize, artifactMaxSizeHint } from './limits';
import { readFileAsBase64 } from './fileBase64';
import { formatBytes, formatDate } from '../../shared/format';

export type ArtifactsPageTab = 'session' | 'all';

export type ArtifactSessionGroup = {
  sessionId: string;
  items: ArtifactMeta[];
  totalSize: number;
};

/** 会话下拉选项：label 为标题（空标题回退「未命名会话」），caption 为短 UUID。 */
export type SessionSelectOption = {
  label: string;
  value: string;
  caption: string;
};

/** 组头会话标识：UUID 过长时缩短为前 8 位（完整值经 tooltip 展示）。 */
export function shortSessionId(sessionId: string) {
  return sessionId.length > 12 ? sessionId.slice(0, 8) + '…' : sessionId;
}

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
  const revealEnabled = ref(false);
  const tableTotal = ref(0);
  const page = ref(1);
  const pageSize = ref(15);
  const pageMax = computed(() => Math.max(1, Math.ceil(tableTotal.value / pageSize.value)));

  const columns = computed(() => createArtifactColumns(t));

  /** 工作区会话列表（供组头标题映射 + 会话筛选下拉）。加载失败静默回退为短 UUID。 */
  const sessionList = ref<Session[]>([]);
  const sessionTitleMap = computed(() => {
    const map = new Map<string, string>();
    for (const s of sessionList.value) {
      if (s.id && s.title) map.set(s.id, s.title);
    }
    return map;
  });

  /** 组头主标题：会话标题；无标题/未加载到会话时回退短 UUID。 */
  function groupHeaderTitle(sessionId: string): string {
    return sessionTitleMap.value.get(sessionId) || shortSessionId(sessionId);
  }

  /** 组头副标题：有标题时显示短 UUID 便于区分同名会话；无标题时不重复显示。 */
  function groupHeaderCaption(sessionId: string): string {
    return sessionTitleMap.value.has(sessionId) ? shortSessionId(sessionId) : '';
  }

  /** 下拉选项全集；当前筛选值不在最近会话中时补一项，保证选中态可显示。 */
  const baseSessionOptions = computed<SessionSelectOption[]>(() =>
    sessionList.value
      .filter((s) => s.id)
      .map((s) => ({
        label: s.title || t('artifact.page.untitledSession'),
        value: s.id,
        caption: shortSessionId(s.id),
      })),
  );

  /** 当前值不在选项中时补临时项，避免 q-select 选中态显示原始值。 */
  function withCurrentPatched(opts: SessionSelectOption[], current: string): SessionSelectOption[] {
    const id = current.trim();
    if (id && !opts.some((o) => o.value === id)) {
      return [{ label: shortSessionId(id), value: id, caption: id }, ...opts];
    }
    return opts;
  }

  const sessionSelectOptions = computed<SessionSelectOption[]>(() =>
    withCurrentPatched(baseSessionOptions.value, sessionFilter.value),
  );

  /** 上传对话框选项：补丁项跟随 uploadForm.session_id。 */
  const uploadSessionOptions = computed<SessionSelectOption[]>(() =>
    withCurrentPatched(baseSessionOptions.value, uploadForm.value.session_id),
  );

  const sessionFilteredOptions = ref<SessionSelectOption[]>([]);

  /** q-select @filter：按标题或 UUID 子串过滤；update 回调填充过滤结果。 */
  function filterSessionOptions(val: string, update: (fn: () => void) => void) {
    update(() => {
      const needle = val.trim().toLowerCase();
      sessionFilteredOptions.value = needle
        ? sessionSelectOptions.value.filter(
            (o) => o.label.toLowerCase().includes(needle) || o.value.toLowerCase().includes(needle),
          )
        : sessionSelectOptions.value;
    });
  }

  async function loadSessionOptions() {
    try {
      const res = await searchSessions({ limit: 200 });
      sessionList.value = res.items ?? [];
    } catch {
      sessionList.value = [];
    }
  }


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
    { label: t('artifact.page.mimeAudio'), value: 'audio/' },
    { label: t('artifact.page.mimeVideo'), value: 'video/' },
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
    // M27 Phase 5：对话框打开时探测本地 reveal 开关（默认关闭 → 隐藏按钮）。
    void artifactStore.loadLocalRevealEnabled().then((enabled) => {
      revealEnabled.value = enabled;
    });
  }

  /** 本地文件管理器打开制品所在目录（emit 自详情对话框）。 */
  async function revealDetail(meta: ArtifactMeta) {
    try {
      await artifactStore.revealLocal(meta.id);
      $q.notify({ type: 'positive', message: t('artifact.detail.revealed') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : t('artifact.detail.revealFailed') });
    }
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

  /** 删除当前查看的单个版本（仅多版本时入口可见）；删后切到剩余最高版本并刷新列表。 */
  function confirmDeleteVersion(meta: ArtifactMeta) {
    if (detailVersions.value.length <= 1) return;
    $q.dialog({
      title: t('artifact.detail.deleteVersionTitle'),
      message: t('artifact.detail.deleteVersionConfirm', { name: meta.name, version: meta.version }),
      cancel: true,
    }).onOk(async () => {
      try {
        await artifactStore.removeVersion(meta.id, meta.version);
        const items = await artifactStore.listVersions(meta.id).catch(() => [] as ArtifactMeta[]);
        detailVersions.value = items;
        if (items.length === 0) {
          detailOpen.value = false;
          detailMeta.value = null;
        } else {
          const latest = items.reduce((a, b) => (b.version > a.version ? b : a));
          detailMeta.value = latest;
          detailArtifactId.value = latest.id;
          detailVersion.value = latest.version;
        }
        await loadRows();
        $q.notify({ type: 'positive', message: t('artifact.detail.versionDeleted') });
      } catch (e) {
        $q.notify({
          type: 'negative',
          message: e instanceof Error ? e.message : t('artifact.detail.deleteVersionFailed'),
        });
      }
    });
  }

  function onSearchChange() {
    page.value = 1;
    void loadRows();
  }

  function onSessionFilterChange() {
    // clearable q-select 清空时写入 null，归一化为空串避免后续 trim() 抛错。
    if (sessionFilter.value == null) sessionFilter.value = '';
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
    void loadSessionOptions();
  });

  /** 选项全集变化时同步过滤结果（初次加载 / 当前值补丁项出现）。 */
  watch(sessionSelectOptions, (opts) => {
    sessionFilteredOptions.value = opts;
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
    sessionFilteredOptions,
    filterSessionOptions,
    uploadSessionOptions,
    groupHeaderTitle,
    groupHeaderCaption,
    shortSessionId,
    uploadOpen,
    uploadLoading,
    uploadFile,
    uploadForm,
    detailOpen,
    detailMeta,
    detailArtifactId,
    detailVersions,
    detailVersion,
    revealEnabled,
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
    revealDetail,
    selectDetailVersion,
    onPreviewDownload,
    downloadRow,
    confirmDelete,
    confirmDeleteVersion,
    artifactMaxSizeHint,
  };
}
