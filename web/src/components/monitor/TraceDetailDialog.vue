<!--
  Pure presentation: Trace 详情弹窗（APM 风）。
  头部（状态点+标题+徽章+关联 ID 复制+操作组）→ 指标 stat 卡 → 错误面板 → 元信息 definition grid
  → 胶囊分段 Tab（流程/瀑布图/Span 树）→ 原始 JSON。树与瀑布共享 selectedSpanId 联动高亮。
-->
<template>
  <q-dialog :model-value="open" maximized @update:model-value="emit('update:open', $event)">
    <q-card class="app-dialog-card app-glass-dialog monitor-trace-dialog">
      <q-card-section class="app-glass-dialog__head trace-head row items-start justify-between no-wrap">
        <div class="trace-head__main">
          <span class="trace-head__status" :class="`trace-head__status--${headTone}`" />
          <div class="trace-head__text">
            <div class="trace-head__title-row row items-center q-gutter-sm no-wrap">
              <span class="trace-head__title ellipsis">{{
                detail?.name || detail?.key || t('monitorPage.traces.detailFallbackTitle')
              }}</span>
              <q-badge v-if="detail?.domain" outline :color="domainColor(detail)">{{ domainLabel(detail) }}</q-badge>
              <q-badge :color="traceStatusColor(detail?.status)" text-color="white">{{
                statusLabel(detail?.status)
              }}</q-badge>
            </div>
            <div class="trace-head__ids row items-center q-gutter-md">
              <span class="trace-head__id text-mono">
                trace_id: {{ activeCorrelation.traceId || detail?.key || '-' }}
                <q-btn
                  v-if="activeCorrelation.traceId || detail?.key"
                  flat
                  dense
                  round
                  size="xs"
                  icon="content_copy"
                  @click="copyText(activeCorrelation.traceId || detail?.key || '')"
                />
              </span>
              <span v-if="activeCorrelation.runId" class="trace-head__id text-mono">
                run_id: {{ activeCorrelation.runId }}
                <q-btn flat dense round size="xs" icon="content_copy" @click="copyText(activeCorrelation.runId)" />
              </span>
            </div>
          </div>
        </div>
        <div class="row q-gutter-sm items-center no-wrap">
          <q-btn
            v-if="activeCorrelation.sessionId"
            flat
            no-caps
            icon="chat"
            :label="t('monitorPage.traces.openSession')"
            color="accent"
            @click="emit('openChatSession', activeCorrelation.sessionId)"
          />
          <flow-log-export-button :trace-id="activeCorrelation.traceId" :lines="flowLines" @export="onExportFlow" />
          <q-btn flat no-caps icon="content_copy" :label="t('monitorPage.traces.copyJson')" @click="copyDetail" />
          <q-btn v-close-popup flat round dense icon="close" />
        </div>
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-glass-dialog__body">
          <!-- 指标 stat 卡：图标 + label + 等宽大数值 -->
          <div class="trace-metrics">
            <div
              v-for="card in metricCards"
              :key="card.label"
              class="trace-metric-card"
              :class="{ 'trace-metric-card--danger': card.danger }"
            >
              <q-icon :name="card.icon" size="18px" class="trace-metric-card__icon" />
              <div class="trace-metric-card__text">
                <div class="trace-metric-card__label">{{ card.label }}</div>
                <div class="trace-metric-card__value text-mono">{{ card.value }}</div>
              </div>
            </div>
          </div>

          <!-- 错误面板：失败/超时/中断时置顶可见 -->
          <q-banner v-if="errorPanel.visible" dense rounded class="bg-negative text-white q-mb-md">
            <template #avatar><q-icon name="error_outline" /></template>
            <div class="text-weight-bold">{{ errorPanel.title }}</div>
            <div v-for="(line, idx) in errorPanel.lines" :key="idx" class="text-caption">{{ line }}</div>
          </q-banner>

          <!-- 元信息 definition grid -->
          <div class="trace-meta-grid">
            <div v-if="detail?.team_name" class="trace-meta-item">
              <div class="trace-meta-item__label">{{ t('monitorPage.traces.summaryTeam') }}</div>
              <div class="trace-meta-item__value ellipsis">{{ detail.team_name }}</div>
            </div>
            <div class="trace-meta-item">
              <div class="trace-meta-item__label">Agent</div>
              <div class="trace-meta-item__value ellipsis">
                {{ detail?.agent_name || '-' }}
                <span v-if="detail?.agent_name && detail?.agent_id" class="trace-meta-item__sub text-mono">{{
                  detail.agent_id
                }}</span>
              </div>
            </div>
            <div class="trace-meta-item">
              <div class="trace-meta-item__label">{{ t('monitorPage.traces.summaryProviderModel') }}</div>
              <div class="trace-meta-item__value ellipsis">
                {{ detail?.provider || '-' }} / {{ detail?.model || '-' }}
              </div>
            </div>
            <div class="trace-meta-item">
              <div class="trace-meta-item__label">{{ t('monitorPage.traces.summaryCreatedAt') }}</div>
              <div class="trace-meta-item__value text-mono">{{ formatDate(detail?.created_at || '') }}</div>
            </div>
            <div class="trace-meta-item">
              <div class="trace-meta-item__label">{{ t('monitorPage.traces.summaryUpdatedAt') }}</div>
              <div class="trace-meta-item__value text-mono">{{ formatDate(detail?.updated_at || '') }}</div>
            </div>
          </div>

          <!-- 胶囊分段 Tab + 内容面板 -->
          <div class="trace-tabs">
            <q-btn-toggle
              v-model="detailTab"
              no-caps
              unelevated
              rounded
              spread
              class="trace-tabs__toggle"
              toggle-color="accent"
              :options="tabOptions"
            />
          </div>

          <div class="trace-panel">
            <div v-show="detailTab === 'flow'">
              <div class="row items-center q-mb-sm q-gutter-sm">
                <q-badge outline color="accent">{{
                  t('monitorPage.traces.flowLinesBadge', { count: flowLines.length })
                }}</q-badge>
                <span class="text-caption trace-panel__hint">{{ t('monitorPage.traces.flowLiveHint') }}</span>
              </div>
              <flow-trace-panel :lines="flowLines" />
            </div>
            <div v-show="detailTab === 'waterfall'">
              <trace-waterfall :spans="spanList" :selected-id="selectedSpanId" @select="onSpanSelect" />
            </div>
            <div v-show="detailTab === 'tree'">
              <trace-span-tree :spans="spanList" :selected-id="selectedSpanId" @select="onSpanSelect" />
            </div>
          </div>

          <q-card flat bordered class="monitor-card q-mt-md">
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
        </q-card-section>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { copyToClipboard } from 'quasar';
import { useI18n } from 'vue-i18n';
import type { MonitorLogLine, MonitorTrace } from '../../features/monitor/types';
import { compactJSON, formatDate, parseJSON } from '../../features/monitor/utils';
import { downloadFlowDiagnosticJsonl } from '../../features/monitor/flow';
import { formatCompactInt, formatCostUsd } from '../../features/monitor/runFormat';
import JsonCodeViewer from '../common/JsonCodeViewer.vue';
import TraceWaterfall from './TraceWaterfall.vue';
import TraceSpanTree from './TraceSpanTree.vue';
import FlowTracePanel from './FlowTracePanel.vue';
import FlowLogExportButton from './FlowLogExportButton.vue';
import {
  traceDomainColor,
  traceDomainLabel,
  traceRunMetrics,
  traceStatusColor,
  traceStatusLabel,
} from './monitorTableUi';

const { t } = useI18n();

const props = defineProps<{
  open: boolean;
  detail: MonitorTrace | null;
  /** Persisted spans from the GetMonitorTrace detail API (monitor_trace_spans). */
  detailSpans?: unknown[];
  flowLines: MonitorLogLine[];
  activeCorrelation: { traceId: string; runId: string; sessionId: string };
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  openChatSession: [sessionId: string];
  notify: [payload: { message: string; type: 'positive' | 'negative' | 'warning' }];
}>();

type DetailTab = 'flow' | 'waterfall' | 'tree';

const detailTab = ref<DetailTab>('flow');
/** 树 ↔ 瀑布 联动选中 */
const selectedSpanId = ref('');

const tabOptions = computed(() => [
  { value: 'flow', label: t('monitorPage.traces.tabFlow'), icon: 'timeline' },
  { value: 'waterfall', label: t('monitorPage.traces.tabWaterfall'), icon: 'waterfall_chart' },
  { value: 'tree', label: t('monitorPage.traces.tabTree'), icon: 'account_tree' },
]);

// 每次打开新 trace 复位 Tab 与选中态
watch(
  () => props.detail?.id,
  () => {
    detailTab.value = 'flow';
    selectedSpanId.value = '';
  },
);

function statusLabel(status?: string): string {
  return traceStatusLabel(t, status);
}

function domainLabel(row: MonitorTrace): string {
  return traceDomainLabel(t, row.domain);
}

function domainColor(row: MonitorTrace): string {
  return traceDomainColor(row.domain);
}

function formatLatency(ms: number): string {
  if (ms <= 0) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

/** 头部状态点色调（running 脉冲 / ok 绿 / error 红 / warn 橙 / 默认灰） */
const headTone = computed(() => {
  const s = String(props.detail?.status ?? '');
  if (s === 'running') return 'running';
  if (s === 'ok' || s === 'success') return 'ok';
  if (s === 'error') return 'error';
  if (s === 'timeout' || s === 'interrupted') return 'warn';
  return 'idle';
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

/** 指标 stat 卡：Tokens / 延迟 / 成本 / Span / 错误 */
const metricCards = computed(() => {
  const d = props.detail;
  if (!d) return [];
  const cfg = detailConfig.value;
  const spanCount = Number(cfg.span_count ?? 0);
  const errorCount = Number(cfg.error_count ?? 0);
  const metrics = traceRunMetrics(d);
  return [
    {
      label: 'Tokens',
      icon: 'data_usage',
      value: metrics.total_tokens > 0 ? formatCompactInt(metrics.total_tokens) : '-',
      danger: false,
    },
    {
      label: t('monitorPage.traces.metricLatency'),
      icon: 'schedule',
      value: formatLatency(metrics.duration_ms),
      danger: false,
    },
    {
      label: t('monitorPage.traces.metricCost'),
      icon: 'payments',
      value: metrics.total_cost_usd > 0 ? formatCostUsd(metrics.total_cost_usd) : '-',
      danger: false,
    },
    {
      label: t('monitorPage.traces.metricSpans'),
      icon: 'account_tree',
      value: spanCount > 0 ? String(spanCount) : '-',
      danger: false,
    },
    {
      label: t('monitorPage.traces.metricErrors'),
      icon: 'error_outline',
      value: String(errorCount),
      danger: errorCount > 0,
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
      const err = r.error && typeof r.error === 'object' ? (r.error as Record<string, unknown>).message : r.error;
      return `${String(r.name || r.kind || 'span')} — ${String(err || r.status || 'error')}`;
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

function onSpanSelect(id: string) {
  selectedSpanId.value = id;
}

async function copyText(text: string) {
  if (!text) return;
  await copyToClipboard(text);
  emit('notify', { message: t('monitorPage.traces.copied'), type: 'positive' });
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
.trace-head {
  gap: 16px;
}

.trace-head__main {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  min-width: 0;
  flex: 1;
}

.trace-head__status {
  flex-shrink: 0;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  margin-top: 6px;
  background: var(--color-text-icon-muted);
}

.trace-head__status--ok {
  background: var(--color-success);
  box-shadow: 0 0 8px color-mix(in srgb, var(--color-success) 55%, transparent);
}

.trace-head__status--error {
  background: var(--color-danger);
  box-shadow: 0 0 8px color-mix(in srgb, var(--color-danger) 55%, transparent);
}

.trace-head__status--warn {
  background: var(--color-warning);
}

.trace-head__status--running {
  background: var(--color-accent);
  animation: trace-head-pulse 1.4s ease-in-out infinite;
}

@keyframes trace-head-pulse {
  0%,
  100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-accent) 45%, transparent);
  }

  50% {
    box-shadow: 0 0 0 6px transparent;
  }
}

.trace-head__text {
  min-width: 0;
  flex: 1;
}

.trace-head__title {
  font-size: 17px;
  font-weight: 700;
  max-width: 60ch;
}

.trace-head__ids {
  margin-top: 2px;
  color: var(--color-text-secondary);
}

.trace-head__id {
  font-size: 11px;
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.text-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}

/* ── 指标 stat 卡 ── */
.trace-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 10px;
  margin-bottom: 16px;
}

.trace-metric-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--glass-surface) 80%, transparent);
}

.trace-metric-card__icon {
  color: var(--color-accent);
  flex-shrink: 0;
}

.trace-metric-card__label {
  font-size: 11px;
  color: var(--color-text-secondary);
}

.trace-metric-card__value {
  font-size: 18px;
  font-weight: 700;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}

.trace-metric-card--danger {
  border-color: color-mix(in srgb, var(--color-danger) 45%, transparent);
}

.trace-metric-card--danger .trace-metric-card__icon,
.trace-metric-card--danger .trace-metric-card__value {
  color: var(--color-danger);
}

/* ── 元信息 definition grid ── */
.trace-meta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px 24px;
  padding: 12px 16px;
  margin-bottom: 16px;
  border-radius: 14px;
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent);
}

.trace-meta-item__label {
  font-size: 11px;
  color: var(--color-text-secondary);
  margin-bottom: 2px;
}

.trace-meta-item__value {
  font-size: 13px;
}

.trace-meta-item__sub {
  margin-left: 6px;
  font-size: 11px;
  color: var(--color-text-secondary);
}

/* ── 胶囊分段 Tab ── */
.trace-tabs {
  margin-bottom: 12px;
}

.trace-tabs__toggle {
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--glass-surface) 60%, transparent);
  max-width: 480px;
}

.trace-panel {
  min-height: 240px;
}

.trace-panel__hint {
  color: var(--color-text-secondary);
}
</style>
