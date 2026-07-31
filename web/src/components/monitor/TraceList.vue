<!--
  Pure presentation: all data via props, all actions via emits.
  No composable / Store access.
-->
<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="q-pb-none">
      <div class="text-h6 text-weight-bold">{{ t('monitorPage.traces.title') }}</div>
      <div class="text-caption text-grey-7">{{ t('monitorPage.traces.subtitle') }}</div>
    </q-card-section>

    <AppPageToolbar class="monitor-traces-toolbar">
      <q-input
        v-model="keyword"
        class="app-page-toolbar__search"
        dense
        outlined
        clearable
        debounce="200"
        :label="t('monitorPage.traces.searchPlaceholder')"
      >
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <template #actions>
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('monitorPage.traces.reset')" @click="keyword = ''" />
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('monitorPage.traces.refresh')"
          :loading="loading"
          @click="$emit('reload')"
        />
      </template>
    </AppPageToolbar>

    <div class="app-registry-table-shell">
      <AppRegistryTable
        :shell="false"
        table-class="monitor-traces-table"
        :rows="pagedRows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
        <template #body-cell-name="slotProps">
          <q-td :props="slotProps">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis trace-name-cell">
                {{ slotProps.row.name || slotProps.row.key || 'unknown' }}
                <q-tooltip max-width="400px" class="text-body2">{{
                  slotProps.row.name || slotProps.row.key || 'unknown'
                }}</q-tooltip>
              </div>
              <div class="app-registry-cell-sub ellipsis row items-center q-gutter-xs">
                <q-badge v-if="domainLabel(slotProps.row)" dense outline :color="domainColor(slotProps.row)">
                  {{ domainLabel(slotProps.row) }}
                </q-badge>
                <span class="text-mono">{{ shortId(slotProps.row.key || slotProps.row.id) }}</span>
              </div>
            </div>
          </q-td>
        </template>
        <template #body-cell-agent="slotProps">
          <q-td :props="slotProps">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">
                {{ slotProps.row.agent_name || '-' }}
                <q-tooltip v-if="slotProps.row.agent_name" max-width="300px" class="text-body2">{{
                  slotProps.row.agent_name
                }}</q-tooltip>
              </div>
              <div
                v-if="slotProps.row.agent_name && slotProps.row.agent_id"
                class="app-registry-cell-sub ellipsis text-mono"
              >
                {{ slotProps.row.agent_id }}
              </div>
            </div>
          </q-td>
        </template>
        <template #body-cell-model="slotProps">
          <q-td :props="slotProps">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">{{ slotProps.row.model || '-' }}</div>
              <div class="app-registry-cell-sub ellipsis">{{ slotProps.row.provider || '-' }}</div>
            </div>
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
        <template #body-cell-status="slotProps">
          <q-td :props="slotProps">
            <q-badge dense :color="traceStatusColor(slotProps.row.status)" text-color="white">
              {{ statusLabel(slotProps.row.status) }}
            </q-badge>
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
                :aria-label="t('monitorPage.traces.viewTraceAria')"
                @click="$emit('openTrace', slotProps.row)"
              >
                <q-tooltip>{{ t('monitorPage.traces.detailTooltip') }}</q-tooltip>
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
        :label="t('monitorPage.traces.paginationLabel')"
        :page-size-options="[12, 24, 48]"
      />
    </div>
  </q-card>

  <q-dialog :model-value="detailOpen" maximized @update:model-value="$emit('update:detailOpen', $event)">
    <q-card class="app-dialog-card app-glass-dialog monitor-trace-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between">
        <div>
          <div class="app-glass-dialog__title row items-center q-gutter-sm">
            <span>{{ detail?.name || detail?.key || t('monitorPage.traces.detailFallbackTitle') }}</span>
            <q-badge v-if="detail?.domain" dense outline :color="domainColor(detail)">
              {{ domainLabel(detail) }}
            </q-badge>
            <q-badge dense :color="traceStatusColor(detail?.status)" text-color="white">
              {{ statusLabel(detail?.status) }}
            </q-badge>
          </div>
          <div class="app-glass-dialog__subtitle text-mono">
            trace_id: {{ activeCorrelation.traceId || detail?.key || '-' }}
            <span v-if="activeCorrelation.runId"> / run_id: {{ activeCorrelation.runId }}</span>
          </div>
        </div>
        <div class="row q-gutter-sm items-center">
          <q-btn
            v-if="activeCorrelation.sessionId"
            flat
            no-caps
            icon="chat"
            :label="t('monitorPage.traces.openSession')"
            color="accent"
            @click="$emit('openChatSession', activeCorrelation.sessionId)"
          />
          <flow-log-export-button :trace-id="activeCorrelation.traceId" :lines="flowLines" @export="onExportFlow" />
          <q-btn flat icon="content_copy" :label="t('monitorPage.traces.copyJson')" @click="copyDetail" />
          <q-btn v-close-popup flat round dense icon="close" />
        </div>
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-glass-dialog__body">
          <!-- 指标条：运维一眼看到规模与成本 -->
          <div class="row q-col-gutter-sm q-mb-md">
            <div v-for="chip in metricChips" :key="chip.label" class="col-auto">
              <q-card flat bordered class="monitor-card q-px-md q-py-sm">
                <div class="text-caption text-grey-7">{{ chip.label }}</div>
                <div class="text-subtitle1 text-weight-bold" :class="chip.tone">{{ chip.value }}</div>
              </q-card>
            </div>
          </div>

          <!-- 错误面板：失败/超时/中断时置顶可见 -->
          <q-banner v-if="errorPanel.visible" dense rounded class="bg-negative text-white q-mb-md">
            <template #avatar><q-icon name="error_outline" /></template>
            <div class="text-weight-bold">{{ errorPanel.title }}</div>
            <div v-for="(line, idx) in errorPanel.lines" :key="idx" class="text-caption">{{ line }}</div>
          </q-banner>

          <div class="row q-col-gutter-md">
            <div class="col-12 col-md-4">
              <q-card flat bordered class="monitor-card">
                <q-card-section>
                  <div class="text-subtitle1 text-weight-bold">{{ t('monitorPage.traces.summaryTitle') }}</div>
                  <q-list dense>
                    <q-item>
                      <q-item-section>{{ t('monitorPage.traces.summaryStatus') }}</q-item-section>
                      <q-item-section side>
                        <q-badge :color="traceStatusColor(detail?.status)" text-color="white">
                          {{ statusLabel(detail?.status) }}
                        </q-badge>
                      </q-item-section>
                    </q-item>
                    <q-item v-if="detail?.domain">
                      <q-item-section>{{ t('monitorPage.traces.summaryDomain') }}</q-item-section>
                      <q-item-section side>{{ domainLabel(detail) }}</q-item-section>
                    </q-item>
                    <q-item>
                      <q-item-section>{{ t('monitorPage.traces.summaryName') }}</q-item-section>
                      <q-item-section side>{{ detail?.name || '-' }}</q-item-section>
                    </q-item>
                    <q-item v-if="detail?.team_name">
                      <q-item-section>{{ t('monitorPage.traces.summaryTeam') }}</q-item-section>
                      <q-item-section side>{{ detail.team_name }}</q-item-section>
                    </q-item>
                    <q-item>
                      <q-item-section>Agent</q-item-section>
                      <q-item-section side>
                        <div class="text-right">
                          <div>{{ detail?.agent_name || '-' }}</div>
                          <div v-if="detail?.agent_name && detail?.agent_id" class="text-caption text-grey-7 text-mono">
                            {{ detail.agent_id }}
                          </div>
                        </div>
                      </q-item-section>
                    </q-item>
                    <q-item>
                      <q-item-section>{{ t('monitorPage.traces.summaryProviderModel') }}</q-item-section>
                      <q-item-section side>{{ detail?.provider || '-' }} / {{ detail?.model || '-' }}</q-item-section>
                    </q-item>
                    <q-item>
                      <q-item-section>{{ t('monitorPage.traces.summaryCreatedAt') }}</q-item-section>
                      <q-item-section side>{{ formatDate(detail?.created_at || '') }}</q-item-section>
                    </q-item>
                    <q-item>
                      <q-item-section>{{ t('monitorPage.traces.summaryUpdatedAt') }}</q-item-section>
                      <q-item-section side>{{ formatDate(detail?.updated_at || '') }}</q-item-section>
                    </q-item>
                  </q-list>
                </q-card-section>
              </q-card>
            </div>
            <div class="col-12 col-md-8">
              <q-card flat bordered class="monitor-card">
                <q-card-section>
                  <q-tabs v-model="detailTab" dense align="left" active-color="accent">
                    <q-tab name="flow" :label="t('monitorPage.traces.tabFlow')" icon="timeline" />
                    <q-tab name="waterfall" :label="t('monitorPage.traces.tabWaterfall')" icon="waterfall_chart" />
                    <q-tab name="tree" :label="t('monitorPage.traces.tabTree')" icon="account_tree" />
                  </q-tabs>
                  <q-separator class="q-mt-sm" />
                  <q-tab-panels v-model="detailTab" animated class="q-mt-md">
                    <q-tab-panel name="flow" class="q-pa-none">
                      <div class="row items-center q-mb-sm q-gutter-sm">
                        <q-badge outline color="teal">
                          {{ t('monitorPage.traces.flowLinesBadge', { count: flowLines.length }) }}
                        </q-badge>
                        <span class="text-caption text-grey-7">{{ t('monitorPage.traces.flowLiveHint') }}</span>
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
                <q-expansion-item
                  dense-toggle
                  icon="data_object"
                  :label="t('monitorPage.traces.rawJson')"
                  header-class="text-subtitle2"
                >
                  <q-card-section>
                    <pre class="monitor-json">{{ detailJSON }}</pre>
                  </q-card-section>
                </q-expansion-item>
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
import { useI18n } from 'vue-i18n';
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
import {
  createMonitorTraceColumns,
  traceDomainColor,
  traceDomainLabel,
  traceRunMetrics,
  traceStatusColor,
  traceStatusLabel,
} from './monitorTableUi';

type TreeNode = {
  id: string;
  label: string;
  caption?: string;
  children?: TreeNode[];
};

const { t } = useI18n();

const columns = computed(() => createMonitorTraceColumns(t));

function statusLabel(status?: string): string {
  return traceStatusLabel(t, status);
}

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

function formatLatencyMs(ms: number): string {
  if (ms <= 0) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function shortId(id: string): string {
  const s = String(id || '');
  return s.length > 13 ? `${s.slice(0, 8)}…${s.slice(-4)}` : s;
}

function domainLabel(row: MonitorTrace): string {
  return traceDomainLabel(t, row.domain);
}

function domainColor(row: MonitorTrace): string {
  return traceDomainColor(row.domain);
}

const props = defineProps<{
  rows: MonitorTrace[];
  loading: boolean;
  highlightUsageEventId?: string;
  flowLines: MonitorLogLine[];
  activeCorrelation: { traceId: string; runId: string; sessionId: string };
  detail: MonitorTrace | null;
  /** Persisted spans from the GetMonitorTrace detail API (monitor_trace_spans). */
  detailSpans?: unknown[];
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
    [row.name, row.key, row.agent_id, row.agent_name, row.team_name, row.provider, row.model, row.status].some(
      (value) =>
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
const detailConfig = computed<Record<string, unknown>>(() => parseJSON(props.detail?.config_json || ''));
const spanList = computed(() => {
  if (!props.detail) return [];
  // Prefer persisted spans from the detail API; fall back to legacy metadata.spans.
  if (props.detailSpans?.length) return props.detailSpans;
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
      caption: statusLabel(props.detail.status),
    },
  ];
});

/** 指标条：Tokens / 延迟 / 成本 / Span / 错误 */
const metricChips = computed(() => {
  const d = props.detail;
  if (!d) return [];
  const cfg = detailConfig.value;
  const spanCount = Number(cfg.span_count ?? 0);
  const errorCount = Number(cfg.error_count ?? 0);
  const metrics = traceRunMetrics(d);
  return [
    { label: 'Tokens', value: metrics.total_tokens > 0 ? String(metrics.total_tokens) : '-', tone: '' },
    { label: t('monitorPage.traces.metricLatency'), value: formatLatencyMs(metrics.duration_ms), tone: '' },
    {
      label: t('monitorPage.traces.metricCost'),
      value: metrics.total_cost_usd > 0 ? `$${metrics.total_cost_usd.toFixed(4)}` : '-',
      tone: '',
    },
    { label: t('monitorPage.traces.metricSpans'), value: spanCount > 0 ? String(spanCount) : '-', tone: '' },
    {
      label: t('monitorPage.traces.metricErrors'),
      value: String(errorCount),
      tone: errorCount > 0 ? 'text-negative' : 'text-grey-7',
    },
  ];
});

/** 错误面板：失败/超时/中断状态或存在错误 span 时展示 */
const errorPanel = computed(() => {
  const d = props.detail;
  if (!d) return { visible: false, title: '', lines: [] as string[] };
  const status = String(d.status || '');
  const errorCount = Number(detailConfig.value.error_count ?? 0);
  const errorSpans = spanList.value
    .filter((s: unknown) => String((s as Record<string, unknown>)?.status ?? '') === 'error')
    .slice(0, 5)
    .map((s: unknown) => {
      const r = s as Record<string, unknown>;
      return `${String(r.name || r.kind || 'span')} — ${String(r.error || r.message || r.status || 'error')}`;
    });
  const statusInError = status === 'error' || status === 'timeout' || status === 'interrupted';
  if (!statusInError && errorCount <= 0 && errorSpans.length === 0) {
    return { visible: false, title: '', lines: [] as string[] };
  }
  const title =
    status === 'timeout'
      ? t('monitorPage.traces.errorTimeout')
      : status === 'interrupted'
        ? t('monitorPage.traces.errorInterrupted')
        : t('monitorPage.traces.errorFailed', { count: errorCount });
  return { visible: true, title, lines: errorSpans };
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
  emit('notify', { message: t('monitorPage.traces.copied'), type: 'positive' });
}

function onExportFlow() {
  if (!props.flowLines.length) {
    emit('notify', { message: t('monitorPage.traces.exportEmpty'), type: 'warning' });
    return;
  }
  downloadFlowDiagnosticJsonl(props.activeCorrelation.traceId, props.flowLines);
  emit('notify', { message: t('monitorPage.traces.exportDone'), type: 'positive' });
}
</script>

<style scoped>
.monitor-traces-toolbar {
  padding: 0 16px 8px;
  border-bottom: none;
}
.trace-name-cell {
  max-width: 20ch;
}
.text-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
}
</style>
