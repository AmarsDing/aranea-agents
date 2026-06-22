import { defineStore } from 'pinia';
import { computed, ref } from 'vue';
import {
  listRuntimeProfiles,
  getRuntimeProfile,
  createRuntimeProfile,
  updateRuntimeProfile,
  deleteRuntimeProfile,
  setRuntimeProfileActive,
} from '../../features/agents/api.runtime-profile';
import type {
  RuntimeProfile,
  CreateRuntimeProfileRequest,
  UpdateRuntimeProfileRequest,
} from '../../features/agents/api.runtime-profile';

export const useRuntimeProfileStore = defineStore('runtimeProfile', () => {
  const profiles = ref<RuntimeProfile[]>([]);
  const current = ref<RuntimeProfile | null>(null);
  const pendingCount = ref(0);
  const loading = computed(() => pendingCount.value > 0);
  const error = ref<string | null>(null);

  async function fetchProfiles(agentId: string, activeOnly = false): Promise<RuntimeProfile[]> {
    pendingCount.value++;
    error.value = null;
    try {
      const result = await listRuntimeProfiles(agentId, activeOnly);
      profiles.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  async function fetchOne(id: string): Promise<RuntimeProfile> {
    pendingCount.value++;
    error.value = null;
    try {
      const result = await getRuntimeProfile(id);
      current.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  async function create(req: CreateRuntimeProfileRequest): Promise<RuntimeProfile> {
    pendingCount.value++;
    error.value = null;
    try {
      const result = await createRuntimeProfile(req);
      profiles.value.push(result);
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  async function update(req: UpdateRuntimeProfileRequest): Promise<RuntimeProfile> {
    pendingCount.value++;
    error.value = null;
    try {
      const result = await updateRuntimeProfile(req);
      const idx = profiles.value.findIndex((p) => p.id === result.id);
      if (idx !== -1) profiles.value[idx] = result;
      if (current.value?.id === result.id) current.value = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  async function remove(id: string): Promise<void> {
    pendingCount.value++;
    error.value = null;
    try {
      await deleteRuntimeProfile(id);
      profiles.value = profiles.value.filter((p) => p.id !== id);
      if (current.value?.id === id) current.value = null;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  async function setActive(id: string, active: boolean): Promise<RuntimeProfile> {
    pendingCount.value++;
    error.value = null;
    try {
      const result = await setRuntimeProfileActive(id, active);
      // Enforce single-active invariant locally: deactivate others when activating
      if (active) {
        for (const p of profiles.value) {
          if (p.id !== id && p.isActive) p.isActive = false;
        }
      }
      const idx = profiles.value.findIndex((p) => p.id === id);
      if (idx !== -1) profiles.value[idx] = result;
      return result;
    } catch (e) {
      error.value = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      pendingCount.value--;
    }
  }

  function reset(): void {
    profiles.value = [];
    current.value = null;
    error.value = null;
  }

  return {
    profiles,
    current,
    loading,
    error,
    fetchProfiles,
    fetchOne,
    create,
    update,
    remove,
    setActive,
    reset,
  };
});
