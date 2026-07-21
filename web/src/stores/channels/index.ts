import { defineStore } from 'pinia';
import { ref } from 'vue';
import {
  listChannels,
  listChannelsPaged,
  listChannelCatalog,
  listChannelCredentials,
  createChannel,
  updateChannel,
  deleteChannel,
  toggleChannel,
  testChannel,
  listChannelDeliveries,
  listChannelTurnJobs,
  type ChannelListQuery,
} from '../../features/channels/api';
import type {
  ChannelRow,
  ChannelTypeItem,
  ChannelCredential,
  ChannelResourceInput,
  ChannelDeliveryRow,
  ChannelTurnJobRow,
} from '../../features/channels/types';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';
import { listAgents } from '../../features/agents/api';
import { listTeams } from '../../features/teams/api';
// TECH-DEBT(#channel-store-catalog): channels Store 直接调用 agents/teams api 而非通过对应 Store。
// 根因：agents/teams Store 是页面级 Store（agentsPage/teams），不适合在此注入。
// 修复路径：创建轻量级 catalog Store（useAgentCatalogStore / useTeamCatalogStore），
// 仅提供 id/name/key 只读目录数据，channels Store 和其他需要目录数据的 Store 均通过 catalog Store 获取。

export const useChannelsStore = defineStore('channels', () => {
  const channels = ref<ChannelRow[]>([]);
  const total = ref(0);
  const catalog = ref<ChannelTypeItem[]>([]);
  const loading = ref(false);
  const routingAgents = ref<Agent[]>([]);
  const routingTeams = ref<Team[]>([]);
  const routingOptionsLoading = ref(false);

  async function loadChannels(query?: ChannelListQuery) {
    loading.value = true;
    try {
      if (query) {
        const result = await listChannelsPaged(query);
        channels.value = result.items;
        total.value = result.total;
        return result;
      }
      channels.value = await listChannels();
      total.value = channels.value.length;
      return { items: channels.value, total: total.value, page: 1, page_size: total.value };
    } finally {
      loading.value = false;
    }
  }

  async function loadCatalog() {
    catalog.value = await listChannelCatalog();
  }

  async function loadAll(query?: ChannelListQuery) {
    loading.value = true;
    try {
      const [catalogData, channelsResult] = await Promise.all([
        listChannelCatalog(),
        query
          ? listChannelsPaged(query)
          : listChannels().then((items) => ({
              items,
              total: items.length,
              page: 1,
              page_size: items.length,
            })),
      ]);
      catalog.value = catalogData;
      channels.value = channelsResult.items;
      total.value = channelsResult.total;
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
    total,
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
