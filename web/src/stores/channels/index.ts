import { defineStore } from 'pinia';
import { ref } from 'vue';
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
} from '../../features/channels/api';
import type {
  ChannelRow,
  ChannelCatalogItem,
  ChannelCredential,
  ChannelResourceInput,
  ChannelDeliveryRow,
  ChannelTurnJobRow,
} from '../../features/channels/types';
import { listAgents } from '../../features/agents/api';
import type { Agent } from '../../features/agents/types';
import { listTeams } from '../../features/teams/api';
import type { Team } from '../../features/teams/types';

export const useChannelsStore = defineStore('channels', () => {
  const channels = ref<ChannelRow[]>([]);
  const catalog = ref<ChannelCatalogItem[]>([]);
  const loading = ref(false);
  const routingAgents = ref<Agent[]>([]);
  const routingTeams = ref<Team[]>([]);
  const routingOptionsLoading = ref(false);

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

  async function loadAll() {
    loading.value = true;
    try {
      const [catalogData, channelsData] = await Promise.all([listChannelCatalog(), listChannels()]);
      catalog.value = catalogData;
      channels.value = channelsData;
    } finally {
      loading.value = false;
    }
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

  function upsertChannel(row: ChannelRow) {
    const index = channels.value.findIndex((c) => c.id === row.id);
    if (index >= 0) channels.value[index] = row;
    else channels.value.unshift(row);
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

  async function loadRoutingOptions() {
    routingOptionsLoading.value = true;
    try {
      const [agents, teams] = await Promise.all([listAgents({ limit: 200 }), listTeams()]);
      routingAgents.value = agents;
      routingTeams.value = teams;
      return { agents, teams };
    } finally {
      routingOptionsLoading.value = false;
    }
  }

  return {
    channels,
    catalog,
    loading,
    routingAgents,
    routingTeams,
    routingOptionsLoading,
    loadChannels,
    loadCatalog,
    loadAll,
    addChannel,
    editChannel,
    removeChannel,
    upsertChannel,
    toggle,
    fetchCredentials,
    testConnection,
    loadTurnJobs,
    loadDeliveries,
    loadRoutingOptions,
  };
});
