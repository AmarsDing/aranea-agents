import { defineStore } from "pinia";
import { ref } from "vue";
import { getAgent, getAgentPromptPreview, updateAgent, type Agent } from "../../features/agents/api";

/** Agent 详情 / 设置页：HTTP 仅在此 Store actions（vue-design §0.1）。 */
export const useAgentDetailStore = defineStore("agentDetail", () => {
  const loading = ref(false);
  const saving = ref(false);
  const previewLoading = ref(false);

  async function fetchById(id: string): Promise<Agent> {
    loading.value = true;
    try {
      return await getAgent(id);
    } finally {
      loading.value = false;
    }
  }

  async function patch(id: string, payload: Partial<Agent>): Promise<Agent> {
    saving.value = true;
    try {
      return await updateAgent(id, payload);
    } finally {
      saving.value = false;
    }
  }

  async function fetchPromptPreview(id: string, mode?: string): Promise<string> {
    previewLoading.value = true;
    try {
      return await getAgentPromptPreview(id, mode);
    } finally {
      previewLoading.value = false;
    }
  }

  return {
    loading,
    saving,
    previewLoading,
    fetchById,
    patch,
    fetchPromptPreview
  };
});
