import { ref } from "vue";
import { usePlatformStore } from "../../../stores/platform";
import { useChatRuntimeStore } from "../../../stores/chat/runtimeStore";
import type { PlatformResource } from "../../platform/types";
import type { ChatOption } from "../types";
import {
  loadDialogModeFromStorage,
  loadModelFromStorage,
  saveDialogModeToStorage,
  saveModelToStorage,
  CHAT_MODE_OPTIONS,
} from "../../../config/chatOptions";
import { getProviderModelValue } from "./chatWorkspaceUtils";
import type { useAppStore } from "../../../stores/app";

type Store = ReturnType<typeof useAppStore>;

function chatModelOptionsToPlatform(rows: ChatOption[]): PlatformResource[] {
  return rows
    .filter((item) => item.enabled !== false)
    .map((item, index) => {
      let provider = "";
      let model = "";
      let capabilities: PlatformResource["capabilities"];
      try {
        const meta = JSON.parse(item.metadata_json || "{}") as {
          provider?: string;
          model?: string;
          capabilities?: PlatformResource["capabilities"];
        };
        provider = meta.provider ?? "";
        model = meta.model ?? "";
        capabilities = meta.capabilities;
      } catch {
        /* ignore */
      }
      return {
        id: item.key || `chat-opt-${index}`,
        resource: "llm-provider-models" as const,
        key: item.key,
        name: item.label || item.key,
        description: "",
        status: "active",
        enabled: item.enabled,
        sort_order: item.sort_order,
        parent_id: "",
        level: "",
        agent_id: "",
        provider,
        model,
        config_json: "{}",
        metadata_json: item.metadata_json,
        capabilities,
        created_at: "",
        updated_at: "",
        deleted_at: "",
      };
    });
}

export function useChatProviderOptions(store: Store) {
  const dialogMode = ref(loadDialogModeFromStorage("default"));
  const modelProvider = ref(loadModelFromStorage(""));
  const modeOpts = ref<Array<{ label: string; value: string }>>(
    CHAT_MODE_OPTIONS.map((o) => ({ label: o.label, value: o.value }))
  );
  const providerModels = ref<PlatformResource[]>([]);
  const provOpts = ref<Array<{ label: string; value: string; caption?: string }>>([]);

  function ensureSelectedModel() {
    if (!providerModels.value.length) return;
    const stored = providerModels.value.find((item) => getProviderModelValue(item) === modelProvider.value);
    if (stored) return;
    const agentModel = store.selectedAgent
      ? providerModels.value.find(
          (item) => item.provider === store.selectedAgent?.provider && item.model === store.selectedAgent?.model
        )
      : null;
    const nextModel = agentModel ?? providerModels.value[0];
    modelProvider.value = getProviderModelValue(nextModel);
    saveModelToStorage(modelProvider.value);
  }

  async function loadChatOptions() {
    const runtimeStore = useChatRuntimeStore();
    let modeRows: ChatOption[] = [];
    try {
      modeRows = await runtimeStore.listChatOptions("dialog_mode");
    } catch {
      /* keep fallback */
    }
    let modelRows: PlatformResource[] = [];
    try {
      const platformStore = usePlatformStore();
      modelRows = (await platformStore.loadResource("llm-provider-models")) as PlatformResource[];
    } catch {
      /* keep empty */
    }
    if (!modelRows.length) {
      try {
        const catalogModels = await runtimeStore.listChatOptions("model");
        if (catalogModels.length) {
          modelRows = chatModelOptionsToPlatform(catalogModels);
        }
      } catch {
        /* ignore */
      }
    }
    if (modeRows.length) {
      modeOpts.value = modeRows.map((item) => ({ label: item.label, value: item.key }));
    }
    providerModels.value = modelRows.filter((item) => item.enabled !== false);
    if (providerModels.value.length) {
      provOpts.value = providerModels.value.map((item) => ({
        label: item.name || item.model,
        value: getProviderModelValue(item),
        caption: `${item.provider} / ${item.model}`,
      }));
      ensureSelectedModel();
    }
  }

  function onModeChange(value: string) {
    dialogMode.value = value;
    saveDialogModeToStorage(value);
  }

  function onProviderChange(value: string) {
    modelProvider.value = value;
    saveModelToStorage(value);
  }

  return {
    dialogMode,
    modelProvider,
    modeOpts,
    providerModels,
    provOpts,
    loadChatOptions,
    onModeChange,
    onProviderChange,
  };
}
