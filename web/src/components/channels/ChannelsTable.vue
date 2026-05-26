<template>
  <AppRegistryTable
    table-class="channels-data-table"
    :rows="rows"
    :columns="columns"
    row-key="id"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <div class="row items-center no-wrap q-gutter-sm">
          <channel-platform-avatar
            :type="channelType(props.row)"
            :label="props.row.name"
            :metadata="channelMetadata(props.row)"
            size="32px"
          />
          <div class="min-width-0">
            <div class="row items-center no-wrap q-gutter-xs">
              <q-icon v-if="isChannelConnected(props.row)" name="circle" color="positive" size="10px">
                <q-tooltip>连接正常</q-tooltip>
              </q-icon>
              <div class="app-registry-cell-primary ellipsis">{{ props.row.name }}</div>
            </div>
            <div class="app-registry-cell-sub ellipsis">{{ props.row.key }}</div>
          </div>
        </div>
      </q-td>
    </template>

    <template #body-cell-type="props">
      <q-td :props="props">
        <span class="channel-tag channel-tag--type">{{ catalogLabelForType(catalog, channelType(props.row)) }}</span>
        <span class="channel-tag channel-tag--mode">{{ receiveMode(props.row) }}</span>
      </q-td>
    </template>

    <template #body-cell-status="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="channelMetadata(props.row).last_error_message">
          <div class="row items-center no-wrap q-gutter-xs">
            <q-icon v-if="isChannelConnected(props.row)" name="circle" color="positive" size="10px" />
            <q-badge rounded :color="props.row.enabled ? statusQuasarColor(props.row.status) : 'grey'">
              {{ channelStatusBadgeText(props.row) }}
            </q-badge>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props">
        <q-toggle
          :model-value="props.row.enabled"
          class="channel-enable-toggle"
          :disable="togglingId === props.row.id"
          @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
        />
      </q-td>
    </template>

    <template #body-cell-updated="props">
      <q-td :props="props">{{ formatChannelDate(props.row.updated_at) }}</q-td>
    </template>

    <template #body-cell-external_id="props">
      <q-td :props="props">
        <span class="app-registry-cell-sub ellipsis" :title="channelExternalID(props.row)">{{ channelExternalID(props.row) }}</span>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <div class="app-registry-cell-actions">
          <q-btn
            v-if="channelSupportsWebhook(props.row, catalog)"
            flat
            dense
            round
            class="channel-icon-btn"
            icon="link"
            @click="$emit('copyWebhook', props.row)"
          >
            <q-tooltip>复制 Webhook URL</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="channel-icon-btn"
            icon="science"
            :loading="testingId === props.row.id"
            @click="$emit('testConnection', props.row)"
          >
            <q-tooltip>测试连接</q-tooltip>
          </q-btn>
          <q-btn flat dense round class="channel-icon-btn" icon="edit" @click="$emit('edit', props.row)">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <q-btn flat dense round class="channel-icon-btn channel-icon-btn--danger" icon="delete" @click="$emit('remove', props.row)">
            <q-tooltip>删除</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../layout/AppRegistryHoverTip.vue";
import ChannelPlatformAvatar from "./ChannelPlatformAvatar.vue";
import type { ChannelCatalogItem, ChannelRow } from "../../features/channels/types";
import { registryColWidth } from "../../features/ui/registryTableColumns";
import {
  catalogLabelForType,
  channelExternalID,
  channelMetadata,
  channelStatusBadgeText,
  channelSupportsWebhook,
  channelType,
  formatChannelDate,
  isChannelConnected,
  receiveMode,
  statusQuasarColor
} from "./channelUi";

defineProps<{
  rows: ChannelRow[];
  catalog: ChannelCatalogItem[];
  loading: boolean;
  togglingId: string;
  testingId: string;
}>();

defineEmits<{
  toggleEnabled: [row: ChannelRow, value: boolean];
  testConnection: [row: ChannelRow];
  copyWebhook: [row: ChannelRow];
  edit: [row: ChannelRow];
  remove: [row: ChannelRow];
}>();

const columns: QTableColumn<ChannelRow>[] = [
  { name: "name", label: "名称", field: "name", align: "left", ...registryColWidth("14%") },
  { name: "type", label: "平台", field: "config_json", align: "left", ...registryColWidth("11%") },
  { name: "external_id", label: "外部 ID", field: "metadata_json", align: "left", ...registryColWidth("10%") },
  { name: "status", label: "连接状态", field: "status", align: "left", ...registryColWidth("9%") },
  { name: "enabled", label: "启用", field: "enabled", align: "center", ...registryColWidth("64px") },
  { name: "updated", label: "最近更新", field: "updated_at", align: "left", ...registryColWidth("11%") },
  { name: "actions", label: "操作", field: "id", align: "right", ...registryColWidth("148px") }
];
</script>
