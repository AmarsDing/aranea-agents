import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { useChannelsStore } from "../../stores/channels";
import { channelsReferencingAgent } from "../channels/channelAgentRefs";
import type { ChannelRow } from "../channels/types";

export function useAgentChannelRefs(agentId: () => string, agentKey: () => string) {
  const router = useRouter();
  const channelsStore = useChannelsStore();
  const loading = ref(false);
  const loadError = ref("");

  const refs = computed(() =>
    channelsReferencingAgent(channelsStore.channels, agentId(), agentKey())
  );

  async function loadChannels() {
    loading.value = true;
    loadError.value = "";
    try {
      await channelsStore.loadChannels();
    } catch (e) {
      loadError.value = e instanceof Error ? e.message : "加载通道失败";
    } finally {
      loading.value = false;
    }
  }

  function channelTypeLabel(ch: ChannelRow): string {
    try {
      const cfg = JSON.parse(ch.config_json || "{}") as { type?: string };
      return cfg.type || "channel";
    } catch {
      return "channel";
    }
  }

  function openChannels() {
    void router.push({ name: "channels" });
  }

  onMounted(() => void loadChannels());

  return { refs, loading, loadError, channelTypeLabel, openChannels, reload: loadChannels };
}
