import { ref, watch, type Ref } from 'vue';
import { promptModes, type PromptMode, type PromptModeOption } from '../../components/agents/agentUi';

export type { PromptModeOption };
import { useAgentDetailStore } from '../../stores/agents';
import type { AgentPromptPreview } from './types';

function emptyPromptPreview(): AgentPromptPreview {
  return {
    summary: '',
    instruction: '',
    sections: [],
    static_total_tokens: 0,
    runtime_overlay_est_tokens: 0,
    runtime_note: '',
  };
}

/** System prompt preview dialog: mode tabs + fetched preview report. */
export function useAgentPromptPreview(agentId: Ref<string>, systemPromptMode: Ref<string>) {
  const detailStore = useAgentDetailStore();
  const promptDialog = ref(false);
  const previewMode = ref<PromptMode>('complete');
  const promptPreview = ref<AgentPromptPreview>(emptyPromptPreview());

  async function loadPromptPreview() {
    const id = agentId.value.trim();
    if (!id) return;
    promptPreview.value = await detailStore.fetchPromptPreview(id, previewMode.value);
  }

  function syncPreviewModeFromAgent(mode?: string) {
    previewMode.value = (String(mode ?? '').trim() as PromptMode) || 'complete';
  }

  watch(previewMode, () => void loadPromptPreview());

  watch(systemPromptMode, (value) => {
    previewMode.value = (value as PromptMode) || 'complete';
  });

  watch(promptDialog, (open) => {
    if (open) void loadPromptPreview();
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
