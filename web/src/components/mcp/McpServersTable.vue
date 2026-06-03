<template>
  <AppRegistryTable
    table-class="mcp-servers-table"
    :rows="rows"
    :columns="MCP_SERVER_TABLE_COLUMNS"
    row-key="id"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
  >
    <template #body-cell-name="props">
      <q-td :props="props">
        <div class="row items-center no-wrap q-gutter-sm">
          <span class="health-dot" :class="`health-dot--${healthTone(props.row)}`">
            <q-tooltip>{{ healthTooltip(props.row) }}</q-tooltip>
          </span>
          <AppRegistryHoverTip :text="endpointLabel(props.row)" empty-label="暂无地址">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">{{ displayName(props.row) }}</div>
              <div class="app-registry-cell-sub ellipsis">{{ props.row.key }}</div>
            </div>
          </AppRegistryHoverTip>
        </div>
      </q-td>
    </template>

    <template #body-cell-transport="props">
      <q-td :props="props">{{ transportLabel(props.row) }}</q-td>
    </template>

    <template #body-cell-health="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="rowMetadata(props.row).last_error_message">
          <q-chip v-if="rowMetadata(props.row).health_status" dense outline :color="healthColor(props.row)">
            {{ rowMetadata(props.row).health_status }}
          </q-chip>
          <span v-else class="text-grey-7">—</span>
        </AppRegistryHoverTip>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props">
        <q-chip dense square :color="props.row.enabled ? 'primary' : 'grey'" text-color="white">
          {{ props.row.enabled ? 'enabled' : 'disabled' }}
        </q-chip>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <div class="app-registry-cell-actions">
          <q-btn
            v-if="rowConfig(props.row).require_user_credentials"
            flat
            dense
            round
            icon="vpn_key"
            color="secondary"
            @click="$emit('credentials', props.row)"
          >
            <q-tooltip>用户凭据</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="science"
            color="primary"
            :loading="testingId === props.row.id"
            @click="$emit('test', props.row)"
          >
            <q-tooltip>测试连接</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="edit" color="primary" @click="$emit('edit', props.row)">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <q-btn flat dense round icon="delete" color="negative" @click="$emit('delete', props.row)">
            <q-tooltip>删除</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import type { McpServerConfig, McpServerMetadata, McpServerRow } from '../../features/mcp/types';
import { parseJSON } from '../../features/mcp/utils';
import { MCP_SERVER_TABLE_COLUMNS } from './mcpServerTableUi';

defineProps<{
  rows: McpServerRow[];
  loading: boolean;
  testingId: string;
  healthTone: (row: McpServerRow) => string;
  healthTooltip: (row: McpServerRow) => string;
}>();

defineEmits<{
  edit: [row: McpServerRow];
  delete: [row: McpServerRow];
  test: [row: McpServerRow];
  credentials: [row: McpServerRow];
}>();

function rowConfig(row: McpServerRow) {
  return parseJSON<McpServerConfig>(row.config_json, {});
}

function rowMetadata(row: McpServerRow) {
  return parseJSON<McpServerMetadata>(row.metadata_json, {});
}

function displayName(row: McpServerRow) {
  return row.name || row.key;
}

function transportLabel(row: McpServerRow) {
  const config = rowConfig(row);
  if (config.transport === 'streamable_http') return 'Streamable HTTP';
  if (config.transport === 'sse') return 'SSE';
  return 'stdio';
}

function endpointLabel(row: McpServerRow) {
  const config = rowConfig(row);
  if (config.transport === 'stdio') {
    return [config.command, ...(config.args ?? [])].filter(Boolean).join(' ') || '—';
  }
  return config.url || '—';
}

function healthColor(row: McpServerRow) {
  const status = rowMetadata(row).health_status;
  if (status === 'ok') return 'positive';
  if (status === 'error') return 'negative';
  if (status === 'degraded') return 'warning';
  return 'grey';
}
</script>
