import { defineStore } from "pinia";
import { ref } from "vue";
import {
  deleteArtifact,
  getArtifact,
  listArtifacts,
  uploadArtifact
} from "../../features/artifact/api";
import type {
  ArtifactData,
  ArtifactMeta,
  ListArtifactsParams,
  ListArtifactsResult,
  UploadArtifactInput
} from "../../features/artifact/types";

export const useArtifactStore = defineStore("artifact", () => {
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
    artifacts.value.unshift(meta);
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

  return { artifacts, total, loading, loadArtifacts, upload, get, remove };
});
