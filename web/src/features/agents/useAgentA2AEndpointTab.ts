import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import type { A2AAgentCard } from "../a2a/types";
import { useA2AStore } from "../../stores/a2a";

export function useAgentA2AEndpointTab(agentId: () => string) {
  const $q = useQuasar();
  const a2aStore = useA2AStore();
  const loading = ref(false);
  const saving = ref(false);
  const card = ref<A2AAgentCard | null>(null);

  const capabilityLines = computed({
    get: () => (card.value?.capabilities ?? []).map((c) => c.name).join("\n"),
    set: (text: string) => {
      if (!card.value) return;
      const names = text
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean);
      card.value.capabilities = names.map((name) => ({
        name,
        description: name,
        input_schema_json: "{}",
        output_schema_json: "{}"
      }));
    }
  });

  async function loadCard() {
    const id = agentId().trim();
    if (!id) return;
    loading.value = true;
    try {
      card.value = await a2aStore.refreshCard(id);
    } catch {
      card.value = {
        agent_id: id,
        display_name: "",
        workspace: "",
        enabled: false,
        capabilities: [],
        updated_at: ""
      };
    } finally {
      loading.value = false;
    }
  }

  async function saveEndpoint() {
    if (!card.value) return;
    if (card.value.enabled && card.value.capabilities.length === 0) {
      card.value.capabilities = [
        {
          name: "chat",
          description: "Default chat capability",
          input_schema_json: "{}",
          output_schema_json: "{}"
        }
      ];
    }
    saving.value = true;
    try {
      card.value = await a2aStore.updateCard(agentId(), {
        enabled: card.value.enabled,
        capabilities: card.value.capabilities
      });
      $q.notify({ type: "positive", message: "A2A AgentCard 已更新" });
    } catch (error) {
      $q.notify({ type: "negative", message: error instanceof Error ? error.message : "保存失败" });
    } finally {
      saving.value = false;
    }
  }

  onMounted(loadCard);

  return {
    loading,
    saving,
    card,
    capabilityLines,
    saveEndpoint
  };
}
