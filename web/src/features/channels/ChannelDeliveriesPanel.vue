// Container: approved — feature-local ops panel; manages delivery/turn-job data // through channels store.
Self-contained CRUD lifecycle within this panel.
<template>
  <div class="channel-deliveries-panel">
    <div class="row items-center justify-between q-mb-sm">
      <div class="text-subtitle2">{{ t('channelEditor.deliveriesTitle') }}</div>
      <q-btn
        flat
        dense
        no-caps
        icon="refresh"
        :label="t('channelEditor.deliveriesRefresh')"
        :loading="loading"
        @click="load"
      />
    </div>
    <p class="text-caption text-grey-7 q-mb-sm">{{ t('channelEditor.deliveriesHint') }}</p>
    <q-banner v-if="error" dense rounded class="bg-negative text-white q-mb-sm">{{ error }}</q-banner>
    <AppRegistryTable
      :rows="rows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 10 }"
      table-class="channel-deliveries-table"
    >
      <template #body-cell-status="slotProps">
        <q-td :props="slotProps">
          <q-badge
            :color="statusColor(String(slotProps.row.status))"
            :label="statusLabel(String(slotProps.row.status))"
          />
        </q-td>
      </template>
      <template #body-cell-agent_id="slotProps">
        <q-td :props="slotProps">
          <span class="app-registry-cell-primary ellipsis">{{ agentNameById(slotProps.row.agent_id) }}</span>
        </q-td>
      </template>
      <template #body-cell-payload="slotProps">
        <q-td :props="slotProps">
          <AppRegistryHoverTip
            :text="slotProps.row.error_message || slotProps.row.payload_json"
            :empty-label="t('channelEditor.noContent')"
          >
            <span class="app-registry-cell-sub ellipsis">{{ payloadPreview(slotProps.row.payload_json) }}</span>
          </AppRegistryHoverTip>
        </q-td>
      </template>
      <template #body-cell-updated_at="slotProps">
        <q-td :props="slotProps">{{ slotProps.row.updated_at || slotProps.row.created_at || '—' }}</q-td>
      </template>
      <template #no-data>
        <div class="full-width text-center text-grey-6 q-pa-md">{{ t('channelEditor.deliveriesEmpty') }}</div>
      </template>
    </AppRegistryTable>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../../components/layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../../components/layout/AppRegistryHoverTip.vue';
import { useChannelsStore } from '../../stores/channels';
import {
  channelDeliveriesColumns,
  deliveryStatusColor as statusColor,
  deliveryStatusLabel as statusLabel,
} from '../../components/channels/channelUi';
import type { ChannelDeliveryRow } from './types';
import type { Agent } from '../agents/types';

const props = defineProps<{ channelId: string }>();

const { t } = useI18n();
const channelsStore = useChannelsStore();
const loading = ref(false);
const error = ref('');
const rows = ref<ChannelDeliveryRow[]>([]);
const columns = channelDeliveriesColumns(t);
const agentCache = ref<Agent[]>([]);

function agentNameById(id: string): string {
  if (!id) return '—';
  const agent = agentCache.value.find((a) => a.id === id);
  return agent ? agent.display_name || agent.agent_key || id : id;
}

function payloadPreview(raw: string) {
  const compact = raw.trim().replace(/\s+/g, ' ');
  return compact ? (compact.length > 80 ? `${compact.slice(0, 80)}…` : compact) : '—';
}

async function load() {
  if (!props.channelId) return;
  loading.value = true;
  error.value = '';
  try {
    const [deliveries, { agents }] = await Promise.all([
      channelsStore.loadDeliveries(props.channelId, 30),
      channelsStore.routingAgents.length > 0
        ? Promise.resolve({ agents: channelsStore.routingAgents })
        : channelsStore.loadRoutingOptions(),
    ]);
    rows.value = deliveries;
    agentCache.value = agents;
  } catch (err) {
    error.value = err instanceof Error ? err.message : t('channelEditor.loadFailed');
  } finally {
    loading.value = false;
  }
}

watch(
  () => props.channelId,
  () => void load(),
  { immediate: false },
);
onMounted(() => void load());
</script>
