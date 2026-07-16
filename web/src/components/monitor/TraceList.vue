<!--
  Pure presentation: all data via props, all actions via emits.
  No composable / Store access.
-->
<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="q-pb-none">
      <div class="text-h6 text-weight-bold">运行</div>
      <div class="text-caption text-grey-7">单次对话运行真相源（Trace + Flow / Waterfall / Span）</div>
    </q-card-section>

    <AppPageToolbar class="monitor-traces-toolbar">
      <q-input
        v-model="keyword"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="200"
        label="搜索名称 / Agent / 模型 / 状态"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" label="重置" @click="keyword = ''" />
        <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('reload')" />
      </template>
    </AppPageToolbar>

    <div class="app-registry-table-shell">
      <AppRegistryTable
        :shell="false"
        table-class="monitor-traces-table"
        :rows="pagedRows"
        :columns="MONITOR_TRACES_TABLE_COLUMNS"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-name="slotProps">
          <q-td :props="slotProps">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">
                {{ slotProps.row.name || slotProps.row.key || 'unknown' }}
              </div>
              <div class="app-registry-cell-sub ellipsis">
                {{ slotProps.row.provider || 'provider' }} / {{ slotProps.row.model || 'model' }}
                ·
                <q-badge dense :color="statusColor(slotProps.row.status)" text-color="white">{{
                  slotProps.row.status || 'unknown'
                }}</q-badge>
              </div>
            </div>
          </q-td>
        </template>
        <template #body-cell-agent="slotProps">
          <q-td :props="slotProps">
            <span class="app-registry-cell-sub">{{ slotProps.row.agent_id || '-' }}</span>
          </q-td>
        </template>
        <template #body-cell-provider="slotProps">
          <q-td :props="slotProps">
            <span class="app-registry-cell-sub">{{ slotProps.row.provider || '-' }}</span>
          </q-td>
        </template>
        <template #body-cell-model="slotProps">
          <q-td :props="slotProps">
            <span class="app-registry-cell-sub">{{ slotProps.row.model || '-' }}</span>
          </q-td>
        </template>
        <template #body-cell-tokens="slotProps">
          <q-td :props="slotProps">
            <span class="app-registry-cell-sub">{{ formatTokens(slotProps.row) }}</span>
          </q-td>
        </template>
        <template #body-cell-latency="slotProps">
          <q-td :props="slotProps">
            <span class="app-registry-cell-sub">{{ formatLatency(slotProps.row) }}</span>
          </q-td>
        </template>
        <template #body-cell-cost="slotProps">
          <q-td :props="slotProps">
            <span class="app-registry-cell-sub">{{ formatCost(slotProps.row) }}</span>
          </q-td>
        </template>
        <template #body-cell-time="slotProps">
          <q-td :props="slotProps">
            <span class="app-registry-cell-sub">{{ formatDate(slotProps.row.created_at) }}</span>
          </q-td>
        </template>
        <template #body-cell-actions="slotProps">
          <q-td :props="slotProps">
            <div class="app-registry-cell-actions">
              <q-btn
                flat
                dense
                round
                icon="account_tree"
                color="accent"
                aria-label="查看 Trace"
                @click="$emit('openTrace', slotProps.row)"
              >
                <q-tooltip>详情</q-tooltip>
              </q-btn>
            </div>
          </q-td>
        </template>
      </AppRegistryTable>

      <AppRegistryPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :page-max="pageMax"
        :total="filteredRows.length"
        :loading="loading"
        label="条 Run"
        :page-size-options="[12, 24, 48]"
      />
    </div>
  </q-card>

  <q-dialog :model-value="detailOpen" maximized @update:model-value="$emit('update:detailOpen', $event)">
    <q-card class="app-dialog-card app-glass-dialog monitor-trace-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between">
        <div>
          <div class="app-glass-dialog__title">Trace 详情</div>
          <div class="app-glass-dialog__subtitle">
            {{ detail?.name || detail?.key }} / {{ detail?.provider }} / {{ detail?.model }}
          </div>
          <div v-if="activeCorrelation.traceId" class="text-caption text-grey-6 q-mt-xs">
            trace_id: {{ activeCorrelation.traceId }}
            <span v-if="activeCorrelation.runId"> / run_id: {{ activeCorrelation.runId }}</span>
          </div>
        </div>
        <div class="row q-gutter-sm items-center">
          <q-btn
            v-if="activeCorrelation.sessionId"
            flat
            no-caps
            icon="chat"
            label="打开会话"
            color="accent"
            @click="$emit('openChatSession', activeCorrelation.sessionId)"
          />
          <flow-log-export-button :trace-id="activeCorrelation.traceId" :lines="flowLines" @export="onExportFlow" />
          <q-btn flat icon="content_copy" label="复制 JSON" @click="copyDetail" />
          <q-btn v-close-popup flat round dense icon="close" />
        </div>
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-glass-dialog__body">
          <div class="row q-col-gutter-md">
            <div class="col-12 col-md-4">
              <q-card flat bordered class="monitor-card">
                <q-card-section>
                  <div class="text-subtitle1 text-weight-bold">概要</div>
                  <q-list dense>
                    <q-item>
                      <q-item-section>状态</q-item-section>
                      <q-item-section side>
                        <q-badge :color="statusColor(detail?.status)">{{ detail?.status || 'unknown' }}</q-badge>
                      </q-item-section>
                    </q-item>
                    <q-item>
                      <q-item-section>名称</q-item-section>
                      <q-item-section side>{{ detail?.name || '-' }}</q-item-section>
                    </q-item>
                    <q-item>
                      <q-item-section>Agent</q-item-section>
                      <q-item-section side>{{ detail?.agent_id || '-' }}</q-item-section>
                    </q-item>
                    <q-item>
                      <q-item-section>Provider / 模型</q-item-section>
                      <q-item-section side>{{ detail?.provider || '-' }} / {{ detail?.model || '-' }}</q-item-section>
                    </q-item>
                  </q-list>
                </q-card-section>
              </q-card>
            </div>
            <div class="col-12 col-md-8">
              <q-card flat bordered class="monitor-card">
                <q-card-section>
                  <q-tabs v-model="detailTab" dense align="left" active-color="accent">
                    <q-tab name="flow" label="流程" icon="timeline" />
                    <q-tab name="waterfall" label="瀑布图" icon="waterfall_chart" />
                    <q-tab name="tree" label="Span 树" icon="account_tree" />
                  </q-tabs>
                  <q-separator class="q-mt-sm" />
                  <q-tab-panels v-model="detailTab" animated class="q-mt-md">
                    <q-tab-panel name="flow" class="q-pa-none">
                      <div class="row items-center q-mb-sm q-gutter-sm">
                        <q-badge outline color="teal">{{ flowLines.length }} 流程日志</q-badge>
                        <span class="text-caption text-grey-7">实时捕获（按 trace_id 过滤，详情打开期间持续接收）</span>
                      </div>
                      <flow-trace-panel :lines="flowLines" />
                    </q-tab-panel>
                    <q-tab-panel name="waterfall" class="q-pa-none">
                      <trace-waterfall :spans="spanList" />
                    </q-tab-panel>
                    <q-tab-panel name="tree" class="q-pa-none">
                      <q-tree :nodes="spanNodes" node-key="id" default-expand-all />
                    </q-tab-panel>
                  </q-tab-panels>
                </q-card-section>
              </q-card>
            </div>
            <div class="col-12">
              <q-card flat bordered class="monitor-card">
                <q-card-section>
                  <div class="text-subtitle1 text-weight-bold">原始 JSON</div>
                  <pre class="monitor-json">{{ detailJSON }}</pre>
                </q-card-section>
              </q-card>
            </div>
          </div>
        </q-card-section>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { copyToClipboard } from 'quasar';
import type { MonitorTrace } from '../../features/monitor/types';
import type { MonitorLogLine } from '../../features/monitor/types';
import { compactJSON, formatDate, parseJSON } from '../../features/monitor/utils';
import { downloadFlowDiagnosticJsonl } from '../../features/monitor/flow';
import TraceWaterfall from './TraceWaterfall.vue';
import FlowTracePanel from './FlowTracePanel.vue';
import FlowLogExportButton from './FlowLogExportButton.vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppPageToolbar from '../layout/AppPageToolbar.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import { MONITOR_TRACES_TABLE_COLUMNS, traceRunMetrics, traceStatusColor as statusColor } from './monitorTableUi';

type TreeNode = {
  id: string;
  label: string;
  caption?: string;
  children?: TreeNode[];
};

function formatTokens(row: MonitorTrace): string {
  const n = traceRunMetrics(row).total_tokens;
  return n > 0 ? String(n) : '-';
}

function formatLatency(row: MonitorTrace): string {
  const ms = traceRunMetrics(row).duration_ms;
  if (ms <= 0) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function formatCost(row: MonitorTrace): string {
  const usd = traceRunMetrics(row).total_cost_usd;
  if (usd <= 0) return '-';
  return `$${usd.toFixed(4)}`;
}

const props = defineProps<{
  rows: MonitorTrace[];
  loading: boolean;
  highlightUsageEventId?: string;
  flowLines: MonitorLogLine[];
  activeCorrelation: { traceId: string; runId: string; sessionId: string };
  detail: MonitorTrace | null;
  detailOpen: boolean;
}>();

const emit = defineEmits<{
  reload: [];
  notify: [payload: { message: string; type: 'positive' | 'negative' | 'warning' }];
  openTrace: [row: MonitorTrace];
  closeDetail: [];
  'update:detailOpen': [value: boolean];
  openChatSession: [sessionId: string];
}>();

const keyword = ref('');
const page = ref(1);
const pageSize = ref(12);
const detailTab = ref<'flow' | 'waterfall' | 'tree'>('flow');

const filteredRows = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  if (!q) return props.rows;
  return props.rows.filter((row) =>
    [row.name, row.key, row.agent_id, row.provider, row.model, row.status, row.description].some((value) =>
      String(value || '')
        .toLowerCase()
        .includes(q),
    ),
  );
});

const pageMax = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / pageSize.value)));
const pagedRows = computed(() => {
  const start = (page.value - 1) * pageSize.value;
  return filteredRows.value.slice(start, start + pageSize.value);
});

watch(keyword, () => {
  page.value = 1;
});

const detailJSON = computed(() => compactJSON(props.detail ?? {}));
const spanList = computed(() => {
  if (!props.detail) return [];
  const metadata = parseJSON(props.detail.metadata_json || '');
  return Array.isArray(metadata.spans) ? metadata.spans : [];
});
const spanNodes = computed<TreeNode[]>(() => {
  if (!props.detail) return [];
  const metadataSpans = spanList.value;
  if (metadataSpans.length) return metadataSpans.map((span: unknown, index: number) => spanToNode(span, index));
  return [
    {
      id: props.detail.id,
      label: `${props.detail.provider || 'provider'} / ${props.detail.model || 'model'}`,
      caption: props.detail.status,
    },
  ];
});

function tryOpenHighlightedRun() {
  const hit = (props.highlightUsageEventId || '').trim();
  if (!hit || props.rows.length === 0) return;
  const row = props.rows.find((r) => String(r.id || '').trim() === hit);
  if (row) emit('openTrace', row);
}

watch(() => props.highlightUsageEventId, tryOpenHighlightedRun);
watch(() => props.rows.length, tryOpenHighlightedRun);

function spanToNode(span: unknown, index: number): TreeNode {
  const row = (span && typeof span === 'object' ? span : {}) as Record<string, unknown>;
  const children = Array.isArray(row.children)
    ? row.children.map((child: unknown, childIndex: number) => spanToNode(child, childIndex))
    : undefined;
  return {
    id: String(row.id || row.name || `span-${index}`),
    label: String(row.name || row.type || row.kind || `span #${index + 1}`),
    caption: [row.status, row.duration_ms ? `${row.duration_ms}ms` : '', row.model, row.tool_name]
      .filter(Boolean)
      .join(' | '),
    children,
  };
}

async function copyDetail() {
  await copyToClipboard(detailJSON.value);
  emit('notify', { message: '已复制', type: 'positive' });
}

function onExportFlow() {
  if (!props.flowLines.length) {
    emit('notify', { message: '暂无流程日志可导出', type: 'warning' });
    return;
  }
  downloadFlowDiagnosticJsonl(props.activeCorrelation.traceId, props.flowLines);
  emit('notify', { message: '已下载流程诊断 JSONL', type: 'positive' });
}
</script>

<style scoped>
.monitor-traces-toolbar {
  padding: 0 16px 8px;
  border-bottom: none;
}
</style>
