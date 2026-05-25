<template>
  <channel-glass-panel>
    <q-table
      flat
      class="channels-data-table"
      :rows="rows"
      :columns="columns"
      row-key="id"
      :loading="loading"
      :pagination="tablePagination"
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
                <div class="text-weight-bold">{{ props.row.name }}</div>
              </div>
              <div class="text-caption app-registry-muted-caption">{{ props.row.key }}</div>
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
          <div class="row items-center no-wrap q-gutter-xs">
            <q-icon v-if="isChannelConnected(props.row)" name="circle" color="positive" size="10px" />
            <q-badge rounded :color="props.row.enabled ? statusQuasarColor(props.row.status) : 'grey'">
              {{ channelStatusBadgeText(props.row) }}
            </q-badge>
          </div>
          <div v-if="channelMetadata(props.row).last_error_message" class="text-caption text-negative ellipsis">
            {{ channelMetadata(props.row).last_error_message }}
          </div>
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
          <span class="ellipsis">{{ channelExternalID(props.row) }}</span>
        </q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props" class="q-gutter-xs">
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
        </q-td>
      </template>
    </q-table>
  </channel-glass-panel>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import ChannelGlassPanel from "./ChannelGlassPanel.vue";
import ChannelPlatformAvatar from "./ChannelPlatformAvatar.vue";
import type { ChannelCatalogItem, ChannelRow } from "../../features/channels/types";
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

const tablePagination = { rowsPerPage: 12 };

const columns: QTableColumn<ChannelRow>[] = [
  { name: "name", label: "名称", field: "name", align: "left" },
  { name: "type", label: "平台", field: "config_json", align: "left" },
  { name: "external_id", label: "外部 ID", field: "metadata_json", align: "left" },
  { name: "status", label: "连接状态", field: "status", align: "left" },
  { name: "enabled", label: "启用", field: "enabled", align: "center" },
  { name: "updated", label: "最近更新", field: "updated_at", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];
</script>
