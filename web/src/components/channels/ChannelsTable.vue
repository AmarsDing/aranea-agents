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
          <div class="row items-center no-wrap q-gutter-xs">
            <q-icon v-if="isChannelConnected(props.row)" name="circle" color="positive" size="10px">
              <q-tooltip>连接正常</q-tooltip>
            </q-icon>
            <div class="text-weight-bold">{{ props.row.name }}</div>
          </div>
          <div class="text-caption muted-caption">{{ props.row.key }}</div>
        </q-td>
      </template>

      <template #body-cell-type="props">
        <q-td :props="props">
          <q-chip dense square class="channel-type-chip">{{ catalogLabelForType(catalog, channelType(props.row)) }}</q-chip>
          <q-chip dense outline class="channel-mode-chip">{{ receiveMode(props.row) }}</q-chip>
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
            color="primary"
            class="channel-enable-toggle"
            :disable="togglingId === props.row.id"
            @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
          />
        </q-td>
      </template>

      <template #body-cell-updated="props">
        <q-td :props="props">{{ formatChannelDate(props.row.updated_at) }}</q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props" class="q-gutter-xs">
          <q-btn
            flat
            dense
            round
            class="channel-icon-btn"
            icon="science"
            color="primary"
            :loading="testingId === props.row.id"
            @click="$emit('testConnection', props.row)"
          >
            <q-tooltip>测试连接</q-tooltip>
          </q-btn>
          <q-btn flat dense round class="channel-icon-btn" icon="edit" color="primary" @click="$emit('edit', props.row)">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <q-btn flat dense round class="channel-icon-btn channel-icon-btn--danger" icon="delete" color="negative" @click="$emit('remove', props.row)">
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
import type { ChannelCatalogItem, ChannelRow } from "../../features/channels/types";
import {
  catalogLabelForType,
  channelMetadata,
  channelStatusBadgeText,
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
  edit: [row: ChannelRow];
  remove: [row: ChannelRow];
}>();

const tablePagination = { rowsPerPage: 12 };

const columns: QTableColumn<ChannelRow>[] = [
  { name: "name", label: "名称", field: "name", align: "left" },
  { name: "type", label: "平台", field: "config_json", align: "left" },
  { name: "status", label: "连接状态", field: "status", align: "left" },
  { name: "enabled", label: "启用", field: "enabled", align: "center" },
  { name: "updated", label: "最近更新", field: "updated_at", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];
</script>

<style scoped lang="sass">
.muted-caption
  color: var(--color-text-secondary)

.channels-data-table :deep(thead tr th)
  color: var(--color-text-secondary)

.channel-type-chip
  background: var(--color-accent)
  color: var(--color-on-accent)

body.body--dark .channel-type-chip
  background: rgba(0, 229, 255, 0.15)
  color: var(--color-neon-cyan)
  border: 1px solid rgba(0, 229, 255, 0.45)

.channel-mode-chip
  border-color: var(--glass-border)

.channel-icon-btn
  color: var(--color-icon-muted)

body:not(.body--dark) .channel-icon-btn:hover
  color: var(--color-accent)

body.body--dark .channel-icon-btn:hover
  color: var(--color-neon-cyan)

.channel-icon-btn--danger:hover
  color: var(--color-danger) !important

.channels-data-table :deep(.q-toggle__track)
  opacity: 1
</style>
