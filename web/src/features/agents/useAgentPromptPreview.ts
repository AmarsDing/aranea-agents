import { ref, watch, type Ref } from "vue";
import { promptModes, type PromptMode } from "../../components/agents/agentUi";
import { useAgentDetailStore } from "../../stores/agents";

/** System prompt preview dialog: mode tabs + fetched preview text. */
export function useAgentPromptPreview(
  agentId: Ref<string>,
  systemPromptMode: Ref<string>,
) {
  const detailStore = useAgentDetailStore();
  const promptDialog = ref(false);
  const previewMode = ref<PromptMode>("complete");
  const promptPreview = ref("");

  async function loadPromptPreview() {
    const id = agentId.value.trim();
    if (!id) return;
    promptPreview.value = await detailStore.fetchPromptPreview(id, previewMode.value);
  }

  function syncPreviewModeFromAgent(mode?: string) {
    previewMode.value = (String(mode ?? "").trim() as PromptMode) || "complete";
  }

  watch(previewMode, () => void loadPromptPreview());

  watch(systemPromptMode, (value) => {
    previewMode.value = (value as PromptMode) || "complete";
  });

  return {
    promptDialog,
    previewMode,
    promptPreview,
    promptModes,
    loadPromptPreview,
    syncPreviewModeFromAgent,
  };
}
