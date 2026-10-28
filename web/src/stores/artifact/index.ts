import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import {
  deleteArtifact,
  deleteArtifactVersion,
  getArtifact,
  listArtifacts,
  listArtifactVersions,
  uploadArtifact,
  signDownloadUrl,
  artifactDownloadHref,
  previewArtifact,
  revealArtifact,
  fetchLocalRevealEnabled,
} from '../../features/artifact/api';
import type {
  ArtifactData,
  ArtifactMeta,
  ArtifactPreview,
  ListArtifactsParams,
  ListArtifactsResult,
  UploadArtifactInput,
  ArtifactPreviewTarget,
  SessionArtifactList,
} from '../../features/artifact/types';

export const useArtifactStore = defineStore('artifact', () => {
  const artifacts = ref<ArtifactMeta[]>([]);
  const total = ref(0);
  const loading = ref(false);

  // 产物预览弹窗的全局唯一目标（P0/P1 会话产物点击查看，2026-09-01）。
  // 深层展示组件（ActionBlock 产物卡片）经 openArtifactPreview 触发，
  // 弹窗本体挂在 MainLayout；状态集中于此遵守数据流红线 #2/#4。
  const previewTarget = ref<ArtifactPreviewTarget | null>(null);
  const previewHost = ref<{ token: number; sessionId: string } | null>(null);
  const sessionLists = ref<Record<string, SessionArtifactList>>({});
  const previewVersions = ref<ArtifactMeta[]>([]);
  const previewVersionsLoading = ref(false);
  const previewVersionsError = ref('');
  let hostSequence = 0;
  const dialogPreviewTarget = computed(() => (previewTarget.value?.surface === 'panel' ? null : previewTarget.value));

  /** The task page owns one preview surface; old pages keep their dialog. */
  function claimPreviewHost(sessionId: string): () => void {
    const token = ++hostSequence;
    closeArtifactPreview();
    previewHost.value = { token, sessionId };
    return () => {
      if (previewHost.value?.token !== token) return;
      previewHost.value = null;
      closeArtifactPreview();
    };
  }

  function openArtifactPreview(id: string, version?: number): void {
    if (!id) return;
    const target: ArtifactPreviewTarget =
      version != null && (version > 0 || (version === 0 && previewHost.value)) ? { id, version } : { id };
    if (previewHost.value) {
      target.surface = 'panel';
      target.sessionId = previewHost.value.sessionId;
    }
    previewTarget.value = target;
    previewVersions.value = [];
    previewVersionsError.value = '';
    if (target.surface === 'panel') void loadPreviewVersions();
  }

  function closeArtifactPreview(): void {
    previewTarget.value = null;
    previewVersions.value = [];
    previewVersionsLoading.value = false;
  }

  async function loadPreviewVersions(): Promise<void> {
    const target = previewTarget.value;
    if (!target) return;
    previewVersionsLoading.value = true;
    previewVersionsError.value = '';
    try {
      const versions = await listArtifactVersions(target.id);
      if (previewTarget.value !== target) return;
      previewVersions.value = versions.filter((item) => !target.sessionId || item.session_id === target.sessionId);
    } catch (error) {
      if (previewTarget.value === target)
        previewVersionsError.value = error instanceof Error ? error.message : 'ARTIFACT_VERSIONS_FAILED';
    } finally {
      if (previewTarget.value === target) previewVersionsLoading.value = false;
    }
  }

  /** Independent from the artifact manager's filters and pagination. */
  async function loadSessionArtifacts(sessionId: string, append = false): Promise<void> {
    if (!sessionId) return;
    sessionLists.value[sessionId] ??= {
      items: [],
      total: null,
      offset: 0,
      loading: false,
      loadingMore: false,
      error: '',
      generation: 0,
      exhausted: false,
    };
    const state = sessionLists.value[sessionId]!;
    if (append && (state.loading || state.loadingMore || state.exhausted)) return;
    const generation = ++state.generation;
    const offset = append ? state.offset : 0;
    state.error = '';
    state.loading = !append;
    state.loadingMore = append;
    try {
      const result = await listArtifacts({ session_id: sessionId, limit: 20, offset });
      if (sessionLists.value[sessionId] !== state || state.generation !== generation) return;
      const rows = append ? [...state.items, ...result.items] : result.items;
      state.items = [...new Map(rows.map((item) => [`${item.id}:${item.version}`, item])).values()];
      state.total = Number.isFinite(result.total) && result.total >= 0 ? result.total : null;
      state.offset = offset + result.items.length;
      state.exhausted = result.items.length === 0 || (state.total != null && state.offset >= state.total);
    } catch (error) {
      if (sessionLists.value[sessionId] === state && state.generation === generation) {
        state.error = error instanceof Error ? error.message : 'ARTIFACT_LIST_FAILED';
      }
    } finally {
      if (sessionLists.value[sessionId] === state && state.generation === generation) {
        state.loading = false;
        state.loadingMore = false;
      }
    }
  }

  function releaseSessionArtifacts(sessionId: string): void {
    delete sessionLists.value[sessionId];
  }

  function removeSessionArtifact(sessionId: string, id: string): void {
    const state = sessionLists.value[sessionId];
    if (!state) return;
    state.items = state.items.filter((item) => item.id !== id);
    void loadSessionArtifacts(sessionId);
  }

  async function loadArtifacts(params: ListArtifactsParams = {}): Promise<ListArtifactsResult> {
    loading.value = true;
    try {
      const result = await listArtifacts(params);
      artifacts.value = result.items;
      total.value = result.total;
      return result;
    } finally {
      loading.value = false;
    }
  }

  async function upload(input: UploadArtifactInput): Promise<ArtifactMeta> {
    const meta = await uploadArtifact(input);
    // Do not optimistically insert into artifacts — the current list may have
    // active filters (session/mime/query) that the new item does not match.
    // The caller (useArtifactsPage) will refresh the list after upload.
    total.value += 1;
    if (sessionLists.value[input.session_id]) void loadSessionArtifacts(input.session_id);
    return meta;
  }

  async function get(id: string, version?: number): Promise<ArtifactData> {
    return getArtifact(id, version);
  }

  async function remove(id: string): Promise<void> {
    await deleteArtifact(id);
    artifacts.value = artifacts.value.filter((a) => a.id !== id);
    total.value = Math.max(0, total.value - 1);
  }

  async function removeVersion(id: string, version: number): Promise<void> {
    await deleteArtifactVersion(id, version);
  }

  async function listVersions(id: string): Promise<ArtifactMeta[]> {
    return listArtifactVersions(id);
  }

  async function signDownload(id: string, version?: number) {
    return signDownloadUrl(id, version);
  }

  async function loadPreview(id: string, version?: number): Promise<ArtifactPreview> {
    return previewArtifact(id, version);
  }

  /** 本地打开文件夹（M27 Phase 5，默认关闭，仅本地单机部署启用）。 */
  async function revealLocal(id: string): Promise<{ path: string }> {
    return revealArtifact(id);
  }

  /** 探测本地 reveal 功能开关（探知失败按未启用处理）。 */
  async function loadLocalRevealEnabled(): Promise<boolean> {
    return fetchLocalRevealEnabled();
  }

  /** 统一下载入口：签名 URL → 新窗口下载（attachment）。失败抛错由调用方提示。 */
  async function download(meta: { id: string; version?: number }): Promise<void> {
    const signed = await signDownloadUrl(meta.id, meta.version);
    if (signed.url) {
      window.open(artifactDownloadHref(signed.url), '_blank', 'noopener,noreferrer');
    }
  }

  return {
    artifacts,
    total,
    loading,
    previewTarget,
    dialogPreviewTarget,
    claimPreviewHost,
    previewVersions,
    previewVersionsLoading,
    previewVersionsError,
    loadPreviewVersions,
    sessionLists,
    loadSessionArtifacts,
    releaseSessionArtifacts,
    removeSessionArtifact,
    openArtifactPreview,
    closeArtifactPreview,
    loadArtifacts,
    upload,
    get,
    remove,
    removeVersion,
    listVersions,
    signDownload,
    loadPreview,
    download,
    revealLocal,
    loadLocalRevealEnabled,
    artifactDownloadHref,
  };
});
