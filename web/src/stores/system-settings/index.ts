import { defineStore } from "pinia";
import { ref } from "vue";
import {
  getSystemSettings,
  updateSystemSettings,
  testWebResearch,
  type UpdateSystemSettingsInput
} from "../../features/system-settings/api";
import type { SystemSettings } from "../../services/kratos/system_setting/v1/index";

export const useSystemSettingsStore = defineStore("systemSettings", () => {
  const settings = ref<SystemSettings | null>(null);
  const loading = ref(false);

  async function loadSettings() {
    loading.value = true;
    try {
      settings.value = await getSystemSettings();
    } finally {
      loading.value = false;
    }
  }

  async function saveSettings(rootDirectory: string, workDirectory: string) {
    settings.value = await updateSystemSettings({
      rootDirectory,
      workDirectory,
      globalMonthlyMicroUsd: settings.value?.globalMonthlyMicroUsd ?? 0,
      a2aPublicBaseUrl: settings.value?.a2aPublicBaseUrl ?? ""
    });
    return settings.value;
  }

  async function saveAll(input: UpdateSystemSettingsInput) {
    settings.value = await updateSystemSettings(input);
    return settings.value;
  }

  async function testWebResearchConnection(input: Parameters<typeof testWebResearch>[0]) {
    return testWebResearch(input);
  }

  return { settings, loading, loadSettings, saveSettings, saveAll, testWebResearchConnection };
});
