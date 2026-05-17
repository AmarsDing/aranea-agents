import { defineStore } from "pinia";
import { ref } from "vue";
import {
  listChannels,
  listChannelCatalog,
  createChannel,
  updateChannel,
  deleteChannel,
  toggleChannel,
  type ChannelRow,
  type ChannelCatalogItem,
  type ChannelResourceInput
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

  return { channels, catalog, loading, loadChannels, loadCatalog, addChannel, editChannel, removeChannel, toggle };
});
