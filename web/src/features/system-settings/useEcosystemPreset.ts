import { ref, computed, onMounted } from 'vue';
import { useQuasar } from 'quasar';
import { useSystemSettingsStore } from '../../stores/system-settings';
import type { IndustryLoadInfo } from './types';

export function useEcosystemPreset() {
  const $q = useQuasar();
  const settingsStore = useSystemSettingsStore();

  const ecosystemLoading = ref(false);
  const ecosystemActionLoading = ref<string | null>(null);
  const unloadDialogVisible = ref(false);
  const unloadTargetIndustry = ref('');
  const unloadTargetInfo = ref<IndustryLoadInfo | null>(null);

  const ecosystemEntries = computed(() => {
    const status = settingsStore.ecosystemLoaded;
    if (!status) return [] as [string, IndustryLoadInfo][];
    return Object.entries(status) as [string, IndustryLoadInfo][];
  });

  const unloadedIndustries = computed(() =>
    ecosystemEntries.value
      .filter(([, info]) => !info.loaded)
      .map(([industry]) => industry),
  );

  async function fetchEcosystemStatus() {
    ecosystemLoading.value = true;
    try {
      await settingsStore.fetchEcosystemStatus();
    } finally {
      ecosystemLoading.value = false;
    }
  }

  function notifyLoadResult(
    industry: string,
    result?: { agents_created: number; teams_created: number; taxonomy_nodes: number },
  ) {
    if (result) {
      $q.notify({
        type: 'positive',
        message: `已加载 ${industry}：Agent ${result.agents_created}，Team ${result.teams_created}，分类节点 ${result.taxonomy_nodes}`,
      });
    }
  }

  function notifyErrors(errors?: Record<string, string>) {
    if (errors && Object.keys(errors).length > 0) {
      for (const msg of Object.values(errors)) {
        $q.notify({ type: 'negative', message: msg });
      }
    }
  }

  async function handleLoadIndustry(industry: string) {
    ecosystemActionLoading.value = industry;
    try {
      const res = await settingsStore.loadEcosystemPreset([industry]);
      notifyLoadResult(industry, res.results?.[industry]);
      if (res.already_loaded?.length) {
        $q.notify({ type: 'info', message: `${res.already_loaded.join(', ')} 已加载过` });
      }
      notifyErrors(res.errors);
    } catch (e: unknown) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      ecosystemActionLoading.value = null;
    }
  }

  async function handleLoadAll() {
    ecosystemActionLoading.value = '__all__';
    try {
      const res = await settingsStore.loadEcosystemPreset(unloadedIndustries.value);
      for (const [industry, result] of Object.entries(res.results ?? {})) {
        notifyLoadResult(industry, result);
      }
      if (res.already_loaded?.length) {
        $q.notify({ type: 'info', message: `${res.already_loaded.join(', ')} 已加载过` });
      }
      notifyErrors(res.errors);
    } catch (e: unknown) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      ecosystemActionLoading.value = null;
    }
  }

  function confirmUnloadIndustry(industry: string, info: IndustryLoadInfo) {
    unloadTargetIndustry.value = industry;
    unloadTargetInfo.value = info;
    unloadDialogVisible.value = true;
  }

  async function handleUnloadConfirmed() {
    const industry = unloadTargetIndustry.value;
    ecosystemActionLoading.value = industry;
    try {
      const res = await settingsStore.unloadEcosystemPreset([industry]);
      const result = res.results?.[industry];
      if (result) {
        $q.notify({
          type: 'positive',
          message: `已卸载 ${industry}：删除 Agent ${result.agents_deleted}，Team ${result.teams_deleted}，分类节点 ${result.taxonomy_nodes_deleted}`,
        });
      }
      notifyErrors(res.errors);
      unloadDialogVisible.value = false;
    } catch (e: unknown) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      ecosystemActionLoading.value = null;
    }
  }

  onMounted(fetchEcosystemStatus);

  return {
    ecosystemLoading,
    ecosystemActionLoading,
    unloadDialogVisible,
    unloadTargetIndustry,
    unloadTargetInfo,
    ecosystemEntries,
    unloadedIndustries,
    fetchEcosystemStatus,
    handleLoadIndustry,
    handleLoadAll,
    confirmUnloadIndustry,
    handleUnloadConfirmed,
  };
}
