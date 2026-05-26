import { computed, ref, toValue, type MaybeRefOrGetter } from "vue";
import { listPlatformResources } from "../platform/api";
import type { PlatformResource } from "../platform/types";
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
export function useAgentProviderModelPicker(form: MaybeRefOrGetter<Agent>) {
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

  const selectedProviderModelID = computed(() => {
    const f = toValue(form);
    return (
      providerModelOptions.value.find((row) => row.provider === f.provider && row.model === f.model)?.value ?? ""
    );
  });

  /** Agent 有 provider/model 但目录无启用匹配行（含被禁用、误填、已删除）。 */
  const orphanProviderModel = computed(() => {
    const f = toValue(form);
    const p = String(f.provider ?? "").trim();
    const m = String(f.model ?? "").trim();
    if (!p || !m) return false;
    return !providerModelOptions.value.some((row) => row.provider === p && row.model === m);
  });

  const disabledCatalogMatch = computed(() => {
    const f = toValue(form);
    const p = String(f.provider ?? "").trim();
    const m = String(f.model ?? "").trim();
    if (!p || !m) return false;
    return providerModels.value.some((row) => row.provider === p && row.model === m && !row.enabled);
  });

  async function loadProviderModels() {
    loadingProviderModels.value = true;
    try {
      providerModels.value = await listPlatformResources("llm-provider-models");
    } finally {
      loadingProviderModels.value = false;
    }
  }

  function selectProviderModel(value: string | null) {
    const f = toValue(form);
    const selected = providerModels.value.find((row) => row.id === value);
    providerModelSearch.value = "";
    if (!selected) {
      f.provider = "";
      f.model = "";
      return;
    }
    f.provider = selected.provider;
    f.model = selected.model;
  }

  function resetProviderModelFilter() {
    providerModelSearch.value = "";
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
    orphanProviderModel,
    disabledCatalogMatch,
    loadProviderModels,
    selectProviderModel,
    filterProviderModels,
    resetProviderModelFilter,
  };
}
