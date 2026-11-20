<template>
  <AppRegistryTable
    table-class="mcp-servers-table"
    :rows="rows"
    :columns="MCP_SERVER_TABLE_COLUMNS"
    row-key="id"
    :loading="loading"
    hide-pagination
    :pagination="{ rowsPerPage: 0 }"
    :total="total"
  >
    <!-- 卡片视图：构件卡。整卡打开编辑（内置共享的只能测试）；主操作是测试连接 -->
    <template #item="props">
      <CapCard
        icon="hub"
        :code="props.row.key"
        :title="displayName(props.row)"
        :description="props.row.description || endpointLabel(props.row)"
        :status="cardStatus(props.row)"
        :facts="cardFacts(props.row)"
        :tags="cardTags(props.row)"
        @open="props.row.shared ? $emit('test', props.row) : $emit('edit', props.row)"
      >
        <template #switch>
          <q-toggle
            dense
            color="primary"
            :model-value="props.row.enabled"
            :disable="togglingId === props.row.id || props.row.shared"
            :aria-label="props.row.enabled ? t('cap.common.batchDisable') : t('cap.common.batchEnable')"
            @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
          >
            <q-tooltip v-if="props.row.shared">{{ t('cap.mcp.sharedLocked') }}</q-tooltip>
          </q-toggle>
        </template>
        <template #relation>
          <span class="cap-mono" :title="endpointLabel(props.row)">{{ endpointLabel(props.row) }}</span>
        </template>
        <template #actions>
          <q-btn
            flat
            dense
            no-caps
            size="sm"
            color="primary"
            icon="science"
            :label="t('cap.mcp.test')"
            :loading="testingId === props.row.id"
            @click="$emit('test', props.row)"
          />
          <q-btn flat dense round size="sm" icon="more_horiz" :aria-label="t('cap.common.more')">
            <q-menu anchor="bottom right" self="top right">
              <q-list dense class="cap-menu">
                <q-item
                  v-if="rowConfig(props.row).require_user_credentials"
                  v-close-popup
                  clickable
                  @click="$emit('credentials', props.row)"
                >
                  <q-item-section avatar><q-icon name="vpn_key" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.mcp.userCredentials') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable :disable="props.row.shared" @click="$emit('edit', props.row)">
                  <q-item-section avatar><q-icon name="edit" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.common.edit') }}</q-item-section>
                </q-item>
                <q-separator />
                <q-item
                  v-close-popup
                  clickable
                  class="text-negative"
                  :disable="props.row.shared"
                  @click="$emit('delete', props.row)"
                >
                  <q-item-section avatar><q-icon name="delete" size="18px" /></q-item-section>
                  <q-item-section>{{ t('cap.common.remove') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </template>
      </CapCard>
    </template>

    <template #body-cell-name="props">
      <q-td :props="props">
        <div class="row items-center no-wrap q-gutter-sm">
          <span class="health-dot" :class="`health-dot--${healthTone(props.row)}`">
            <q-tooltip>{{ healthTooltip(props.row) }}</q-tooltip>
          </span>
          <AppRegistryHoverTip :text="endpointLabel(props.row)" empty-label="暂无地址">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">
                {{ displayName(props.row) }}
                <q-badge v-if="props.row.shared" outline color="grey-6" class="q-ml-xs">内置</q-badge>
              </div>
              <div class="app-registry-cell-sub ellipsis">{{ props.row.key }}</div>
            </div>
          </AppRegistryHoverTip>
        </div>
      </q-td>
    </template>

    <template #body-cell-transport="props">
      <q-td :props="props">{{ transportLabel(props.row) }}</q-td>
    </template>

    <template #body-cell-toolPrefix="props">
      <q-td :props="props">
        <code v-if="rowConfig(props.row).tool_prefix" class="mcp-tool-prefix">
          mcp_{{ rowConfig(props.row).tool_prefix }}__
        </code>
        <span v-else class="text-grey-7">—</span>
      </q-td>
    </template>

    <template #body-cell-timeout="props">
      <q-td :props="props">{{ rowConfig(props.row).timeout_sec ?? 60 }}s</q-td>
    </template>

    <template #body-cell-health="props">
      <q-td :props="props">
        <div class="column items-start q-gutter-xs">
          <AppRegistryHoverTip :text="rowMetadata(props.row).last_error_message">
            <q-chip v-if="rowMetadata(props.row).health_status" dense outline :color="healthColor(props.row)">
              {{ healthLabel(props.row) }}
            </q-chip>
            <span v-else class="text-grey-7">—</span>
          </AppRegistryHoverTip>
          <span
            v-if="toolCountLabel(props.row)"
            class="mcp-tool-count"
            :class="{ 'mcp-tool-count--error': toolDiscoveryFailed(props.row) }"
          >
            {{ toolCountLabel(props.row) }}
            <q-tooltip>{{ toolCountTooltip(props.row) }}</q-tooltip>
          </span>
        </div>
      </q-td>
    </template>

    <template #body-cell-enabled="props">
      <q-td :props="props">
        <q-toggle
          dense
          color="primary"
          :model-value="props.row.enabled"
          :disable="togglingId === props.row.id || props.row.shared"
          @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
        >
          <q-tooltip v-if="props.row.shared">内置共享服务器，全租户共用，不可单独启停</q-tooltip>
        </q-toggle>
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
          <span v-if="props.row.shared" class="inline-block">
            <q-btn flat dense round icon="edit" color="primary" disable>
              <q-tooltip>内置共享服务器，不可编辑</q-tooltip>
            </q-btn>
          </span>
          <q-btn v-else flat dense round icon="edit" color="primary" @click="$emit('edit', props.row)">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <span v-if="props.row.shared" class="inline-block">
            <q-btn flat dense round icon="delete" color="negative" disable>
              <q-tooltip>内置共享服务器，不可删除</q-tooltip>
            </q-btn>
          </span>
          <q-btn v-else flat dense round icon="delete" color="negative" @click="$emit('delete', props.row)">
            <q-tooltip>{{ $t('common.delete') }}</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryHoverTip from '../layout/AppRegistryHoverTip.vue';
import CapCard from '../capability/CapCard.vue';
import type { CapFact, CapStatus, CapTag } from '../capability/capTypes';
import type { McpHealthTone, McpServerConfig, McpServerMetadata, McpServerRow } from '../../features/mcp/types';
import { parseJSON } from '../../features/mcp/utils';
import { MCP_SERVER_TABLE_COLUMNS } from './mcpServerTableUi';

const { t } = useI18n();

// 命名 tableProps：模板各 slot 的 props 形参会遮蔽外层绑定（vue/no-template-shadow）。
const tableProps = defineProps<{
  rows: McpServerRow[];
  loading: boolean;
  testingId: string;
  togglingId: string;
  healthTone: (row: McpServerRow) => McpHealthTone;
  healthTooltip: (row: McpServerRow) => string;
  /** 视图条上的计数（筛选后的总条数） */
  total?: number;
}>();

// ——— 构件卡取值 ———
function cardStatus(row: McpServerRow): CapStatus {
  if (!row.enabled) return { tone: 'off', label: t('cap.common.disabled') };
  const status = rowMetadata(row).health_status;
  const hint = rowMetadata(row).last_error_message || undefined;
  if (status === 'ok') return { tone: 'ok', label: t('cap.mcp.healthOk') };
  if (status === 'error') return { tone: 'alert', label: t('cap.mcp.healthError'), hint };
  if (status === 'degraded') return { tone: 'watch', label: t('cap.mcp.healthDegraded'), hint };
  return { tone: 'pending', label: t('cap.mcp.healthUnknown') };
}

function cardFacts(row: McpServerRow): CapFact[] {
  const m = rowMetadata(row);
  const tools: CapFact =
    typeof m.tool_count === 'number'
      ? { label: t('cap.mcp.tools'), value: m.tool_count, hint: toolCountTooltip(row) }
      : m.tools_error_message
        ? { label: t('cap.mcp.tools'), value: t('cap.mcp.discoverFailed'), tone: 'alert', hint: toolCountTooltip(row) }
        : { label: t('cap.mcp.tools'), value: '—', hint: t('cap.mcp.toolsUnknown') };
  const agents = row.agent_count;
  return [
    { label: t('cap.mcp.transport'), value: transportLabel(row) },
    tools,
    {
      label: t('cap.mcp.agents'),
      value: agents === null || agents === undefined ? '—' : agents,
      tone: agents === 0 && row.enabled ? 'watch' : undefined,
      hint: t('cap.mcp.agentsHint'),
    },
  ];
}

function cardTags(row: McpServerRow): CapTag[] {
  const config = rowConfig(row);
  const tags: CapTag[] = [];
  if (row.shared) tags.push({ label: t('cap.mcp.shared') });
  if (config.require_user_credentials) tags.push({ label: t('cap.mcp.needsUserCredential'), tone: 'watch' });
  if (config.auth?.type) tags.push({ label: config.auth.type, tone: 'pending' });
  if (config.tool_prefix) tags.push({ label: `mcp_${config.tool_prefix}__` });
  tags.push({ label: t('cap.mcp.timeoutTag', { s: config.timeout_sec ?? 60 }) });
  return tags;
}

defineEmits<{
  edit: [row: McpServerRow];
  delete: [row: McpServerRow];
  test: [row: McpServerRow];
  credentials: [row: McpServerRow];
  toggleEnabled: [row: McpServerRow, enabled: boolean];
}>();

// config_json/metadata_json 每行只解析一次：此前 name/transport/toolPrefix/
// timeout/health 各单元格每次渲染各自 JSON.parse（每行 6+ 次），现为按 rows
// 变化重建的查表缓存。
const parsedByRow = computed(() => {
  const map = new Map<string, { config: McpServerConfig; metadata: McpServerMetadata }>();
  for (const row of tableProps.rows) {
    map.set(row.id, {
      config: parseJSON<McpServerConfig>(row.config_json, {}),
      metadata: parseJSON<McpServerMetadata>(row.metadata_json, {}),
    });
  }
  return map;
});

function rowConfig(row: McpServerRow) {
  return parsedByRow.value.get(row.id)?.config ?? {};
}

function rowMetadata(row: McpServerRow) {
  return parsedByRow.value.get(row.id)?.metadata ?? {};
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

function healthLabel(row: McpServerRow) {
  const status = rowMetadata(row).health_status;
  if (status === 'ok') return t('mcpPage.statusOk');
  if (status === 'error') return t('mcpPage.statusError');
  if (status === 'degraded') return t('mcpPage.statusDegraded');
  return status || '';
}

// --- P2 工具发现展示（metadata.tool_*，与 health_* 正交） --------------------

/** 发现失败 = 无有效计数但有错误记录（保留 last-good 时不算失败）。 */
function toolDiscoveryFailed(row: McpServerRow) {
  const m = rowMetadata(row);
  return typeof m.tool_count !== 'number' && !!m.tools_error_message;
}

function toolCountLabel(row: McpServerRow) {
  const m = rowMetadata(row);
  if (typeof m.tool_count === 'number') return t('mcpPage.toolCount', { n: m.tool_count }, '{n} tools');
  if (m.tools_error_message) return t('mcpPage.toolDiscoveryFailed', 'Discovery failed');
  return '';
}

function toolCountTooltip(row: McpServerRow) {
  const m = rowMetadata(row);
  if (typeof m.tool_count === 'number') {
    const names = Array.isArray(m.tool_names) ? m.tool_names.filter((n): n is string => typeof n === 'string') : [];
    const shown = names.slice(0, 10).join('、');
    const suffix =
      m.tool_count > names.length || names.length > 10
        ? t('mcpPage.toolCountMore', { n: m.tool_count }, ' of {n}')
        : '';
    const list = shown ? `：${shown}${names.length > 10 ? '…' : ''}${suffix}` : '';
    const at = m.tools_discovered_at
      ? t('mcpPage.toolsDiscoveredAt', { at: formatDateTime(m.tools_discovered_at) }, ' (last discovered: {at})')
      : '';
    return t('mcpPage.toolsDiscovered', { n: m.tool_count, list, at }, 'Discovered {n} tools{list}{at}');
  }
  return m.tools_error_message
    ? t('mcpPage.toolDiscoveryError', { msg: m.tools_error_message }, 'Tool discovery failed: {msg}')
    : '';
}

function formatDateTime(value: string) {
  const d = new Date(value);
  return Number.isNaN(d.getTime()) ? value : d.toLocaleString();
}
</script>

<style scoped>
.mcp-tool-prefix {
  font-size: 12px;
  color: var(--color-text-secondary);
  background: color-mix(in srgb, var(--glass-surface) 80%, transparent);
  border: 1px solid var(--glass-border);
  border-radius: 6px;
  padding: 1px 6px;
}

.mcp-tool-count {
  font-size: 11px;
  line-height: 1.4;
  color: var(--color-text-secondary);
  cursor: default;
}

.mcp-tool-count--error {
  color: var(--q-warning);
}
</style>
