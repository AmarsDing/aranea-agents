import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listChannels,
  listChannelCatalog,
  listChannelCredentials,
  createChannel,
  updateChannel,
  deleteChannel,
  toggleChannel,
  testChannel,
  listChannelDeliveries,
  listChannelTurnJobs,
  type ChannelRow,
  type ChannelCatalogItem,
  type ChannelCredential,
  type ChannelResourceInput,
  type ChannelDeliveryRow,
  type ChannelTurnJobRow
} from "../../features/channels/api";

export const useChannelsStore = defineStore("channels", () => {
  const channels = ref<ChannelRow[]>([]);
  const catalog = ref<ChannelCatalogItem[]>([]);
  const loading = ref(false);

  async function loadChannels() {
    loading.value = true;
    try {
      channels.value = await listChannels();
    } finally {
      loading.value = false;
    }
  }

  async function loadCatalog() {
    catalog.value = await listChannelCatalog();
  }

  async function addChannel(payload: ChannelResourceInput) {
    const created = await createChannel(payload);
    channels.value.push(created);
    return created;
  }

  async function editChannel(id: string, payload: Partial<ChannelResourceInput>) {
    const updated = await updateChannel(id, payload);
    channels.value = channels.value.map((c) => (c.id === id ? updated : c));
    return updated;
  }

  async function removeChannel(id: string) {
    await deleteChannel(id);
    channels.value = channels.value.filter((c) => c.id !== id);
  }

  async function toggle(id: string, enabled: boolean) {
    const updated = await toggleChannel(id, enabled);
    channels.value = channels.value.map((c) => (c.id === id ? updated : c));
    return updated;
  }

  async function fetchCredentials(channelId: string): Promise<ChannelCredential[]> {
    return listChannelCredentials(channelId);
  }

  async function testConnection(channelId: string) {
    return testChannel(channelId);
  }

  async function loadTurnJobs(channelId: string, limit = 30): Promise<ChannelTurnJobRow[]> {
    const data = await listChannelTurnJobs(channelId, limit);
    return data.items;
  }

  async function loadDeliveries(channelId: string, limit = 30): Promise<ChannelDeliveryRow[]> {
    const data = await listChannelDeliveries(channelId, limit);
    return data.items;
  }

  return {
    channels,
    catalog,
    loading,
    loadChannels,
    loadCatalog,
    addChannel,
    editChannel,
    removeChannel,
    toggle,
    fetchCredentials,
    testConnection,
    loadTurnJobs,
    loadDeliveries
  };
});
