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

  return {
    artifacts,
    total,
    loading,
    loadArtifacts,
    upload,
    get,
    remove,
    removeVersion,
    listVersions,
    signDownload,
    loadPreview,
    artifactDownloadHref,
  };
});
