import { defineStore } from 'pinia';
import { ref } from 'vue';
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
} from '../../features/artifact/types';

export const useArtifactStore = defineStore('artifact', () => {
  const artifacts = ref<ArtifactMeta[]>([]);
  const total = ref(0);
  const loading = ref(false);

  // 产物预览弹窗的全局唯一目标（P0/P1 会话产物点击查看，2026-09-01）。
  // 深层展示组件（ActionBlock 产物卡片）经 openArtifactPreview 触发，
  // 弹窗本体挂在 MainLayout；状态集中于此遵守数据流红线 #2/#4。
  const previewTarget = ref<{ id: string; version?: number } | null>(null);

  function openArtifactPreview(id: string, version?: number): void {
    if (!id) return;
    previewTarget.value = version && version > 0 ? { id, version } : { id };
  }

  function closeArtifactPreview(): void {
    previewTarget.value = null;
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
