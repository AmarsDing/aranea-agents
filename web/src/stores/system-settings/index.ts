import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  getSystemSettings,
  updateSystemSettings,
  testWebResearch,
  getEcosystemPresetStatus,
  loadEcosystemPreset as loadEcosystemPresetApi,
  unloadEcosystemPreset as unloadEcosystemPresetApi,
} from '../../features/system-settings/api';
import type { UpdateSystemSettingsInput, EcosystemLoadedStatus } from '../../features/system-settings/types';
import type { SystemSettings } from '../../services/kratos/system_setting/v1/index';

export const useSystemSettingsStore = defineStore('systemSettings', () => {
  const settings = ref<SystemSettings | null>(null);
  const loading = ref(false);
  const ecosystemLoaded = ref<EcosystemLoadedStatus | null>(null);

  async function loadSettings() {
    loading.value = true;
    try {
      settings.value = await getSystemSettings();
    } finally {
      loading.value = false;
    }
  }

  async function saveAll(input: UpdateSystemSettingsInput) {
    settings.value = await updateSystemSettings(input);
    return settings.value;
  }

  async function testWebResearchConnection(input: Parameters<typeof testWebResearch>[0]) {
    return testWebResearch(input);
  }

  async function fetchEcosystemStatus() {
    ecosystemLoaded.value = await getEcosystemPresetStatus();
    return ecosystemLoaded.value;
  }

  async function loadEcosystemPreset(industries?: string[], force?: boolean) {
    const res = await loadEcosystemPresetApi(industries, force);
    await fetchEcosystemStatus();
    return res;
  }

  async function unloadEcosystemPreset(industries: string[]) {
    const res = await unloadEcosystemPresetApi(industries);
    await fetchEcosystemStatus();
    return res;
  }

  return {
    settings,
    loading,
    ecosystemLoaded,
    loadSettings,
    saveAll,
    testWebResearchConnection,
    fetchEcosystemStatus,
    loadEcosystemPreset,
    unloadEcosystemPreset,
  };
});
