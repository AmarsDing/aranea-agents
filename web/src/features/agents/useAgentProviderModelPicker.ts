import { computed, ref, type Ref } from "vue";
import { listPlatformResources, type PlatformResource } from "../platform/api";
import type { Agent } from "./types";

function providerContextWindowK(row: PlatformResource) {
  try {
    const parsed = JSON.parse(row.config_json || "{}") as { context_window_k?: number | string | null };
    const value = Number(parsed.context_window_k);
    return Number.isFinite(value) && value > 0 ? value : null;
  } catch {
    return null;
  }
}

/** Provider model dropdown for Agent settings. */
export function useAgentProviderModelPicker(form: Ref<Agent> | Agent) {
  const providerModels = ref<PlatformResource[]>([]);
  const providerModelSearch = ref("");
  const loadingProviderModels = ref(false);

  const providerModelOptions = computed(() =>
    providerModels.value
      .filter((row) => row.enabled && row.provider && row.model)
      .map((row) => {
        const contextWindowK = providerContextWindowK(row);
        return {
          label: row.name || row.model,
          value: row.id,
          caption: `${row.provider} / ${row.model}${contextWindowK ? ` · ${contextWindowK}K ctx` : ""}`,
          provider: row.provider,
          model: row.model,
        };
      }),
  );

  const filteredProviderModelOptions = computed(() => {
    const keyword = providerModelSearch.value.trim().toLowerCase();
    if (!keyword) return providerModelOptions.value;
    return providerModelOptions.value.filter((option) =>
      [option.label, option.caption, option.provider, option.model].some((value) =>
        value.toLowerCase().includes(keyword),
      ),
    );
  });

  const selectedProviderModelID = computed(
    () =>
      providerModelOptions.value.find((row) => row.provider === form.provider && row.model === form.model)?.value ??
      "",
  );

  async function loadProviderModels() {
    loadingProviderModels.value = true;
    try {
      providerModels.value = await listPlatformResources("llm-provider-models");
    } finally {
      loadingProviderModels.value = false;
    }
  }

  function selectProviderModel(value: string | null) {
    const selected = providerModels.value.find((row) => row.id === value);
    if (!selected) {
      form.provider = "";
      form.model = "";
      return;
    }
    form.provider = selected.provider;
    form.model = selected.model;
  }

  function filterProviderModels(value: string, update: (callback: () => void) => void) {
    update(() => {
      providerModelSearch.value = value;
    });
  }

  return {
    providerModels,
    providerModelSearch,
    loadingProviderModels,
    providerModelOptions,
    filteredProviderModelOptions,
    selectedProviderModelID,
    loadProviderModels,
    selectProviderModel,
    filterProviderModels,
  };
}
