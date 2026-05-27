// Container: approved — feature-local ops panel; data is loaded through channels store.
<template>
  <div class="channel-deliveries-panel">
    <div class="row items-center justify-between q-mb-sm">
      <div class="text-subtitle2">{{ t("channelEditor.deliveriesTitle") }}</div>
      <q-btn
        flat
        dense
        no-caps
        icon="refresh"
        :label="t('channelEditor.turnJobsRefresh')"
        :loading="loading"
        @click="load"
      />
    </div>
    <p class="text-caption text-grey-7 q-mb-sm">{{ t("channelEditor.deliveriesHint") }}</p>
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
      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge :color="statusColor(String(props.row.status))" :label="statusLabel(String(props.row.status))" />
        </q-td>
      </template>
      <template #body-cell-agent_id="props">
        <q-td :props="props">
          <span class="app-registry-cell-primary ellipsis">{{ props.row.agent_id || "—" }}</span>
        </q-td>
      </template>
      <template #body-cell-payload="props">
        <q-td :props="props">
          <AppRegistryHoverTip :text="props.row.error_message || props.row.payload_json" empty-label="暂无内容">
            <span class="app-registry-cell-sub ellipsis">{{ payloadPreview(props.row.payload_json) }}</span>
          </AppRegistryHoverTip>
        </q-td>
      </template>
      <template #body-cell-updated_at="props">
        <q-td :props="props">{{ props.row.updated_at || props.row.created_at || "—" }}</q-td>
      </template>
      <template #no-data>
        <div class="full-width text-center text-grey-6 q-pa-md">{{ t("channelEditor.deliveriesEmpty") }}</div>
      </template>
    </AppRegistryTable>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import AppRegistryTable from "../../components/layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../../components/layout/AppRegistryHoverTip.vue";
import { deliveryStatusFromChannelStatus } from "../../domain/conversation";
import { presentDeliveryStatus, toneToQuasarColor } from "../../domain/conversationPresentation";
import { useChannelsStore } from "../../stores/channels";
import { CHANNEL_DELIVERIES_TABLE_COLUMNS } from "../../components/channels/channelUi";
import type { ChannelDeliveryRow } from "./types";

const props = defineProps<{ channelId: string }>();

const { t } = useI18n();
const channelsStore = useChannelsStore();
const loading = ref(false);
const error = ref("");
const rows = ref<ChannelDeliveryRow[]>([]);
const columns = CHANNEL_DELIVERIES_TABLE_COLUMNS;

function presentation(status: string) {
  return presentDeliveryStatus(deliveryStatusFromChannelStatus(status));
}

function statusColor(status: string) {
  return toneToQuasarColor(presentation(status).tone);
}

function statusLabel(status: string) {
  return deliveryStatusFromChannelStatus(status) ? presentation(status).label : (status || "—");
}

function payloadPreview(raw: string) {
  const compact = raw.trim().replace(/\s+/g, " ");
  return compact ? (compact.length > 80 ? `${compact.slice(0, 80)}…` : compact) : "—";
}

async function load() {
  if (!props.channelId) return;
  loading.value = true;
  error.value = "";
  try {
    rows.value = await channelsStore.loadDeliveries(props.channelId, 30);
  } catch (err) {
    error.value = err instanceof Error ? err.message : "load failed";
  } finally {
    loading.value = false;
  }
}

watch(() => props.channelId, () => void load(), { immediate: false });
onMounted(() => void load());
</script>
