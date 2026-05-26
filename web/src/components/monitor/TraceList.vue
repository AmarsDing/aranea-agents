<!--
  Container：Flow/WS 经 useMonitorTraceFlow + monitorStore；列表数据由 MonitorPage props 注入。
-->
<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="q-pb-none">
      <div class="text-h6 text-weight-bold">Runs</div>
      <div class="text-caption text-grey-7">单次对话运行真相源（Token 用量 + Flow / Waterfall / Span）</div>
    </q-card-section>

    <AppPageToolbar class="monitor-traces-toolbar">
      <q-input v-model="keyword" class="app-page-toolbar__search" dense outlined clearable debounce="200" label="搜索 Agent / 模型 / 状态">
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
        :columns="columns"
        row-key="id"
        :loading="loading"
        hide-pagination
        :pagination="{ rowsPerPage: 0 }"
      >
      <template #body-cell-name="props">
        <q-td :props="props">
          <AppRegistryHoverTip :text="props.row.error_message">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">{{ props.row.agent_key || props.row.agent_id || "unknown agent" }}</div>
              <div class="app-registry-cell-sub ellipsis">
                {{ props.row.provider_code || "provider" }} / {{ props.row.model_api_id || "model" }}
                ·
                <q-badge dense :color="statusColor(props.row.status)" text-color="white">{{ props.row.status || "unknown" }}</q-badge>
              </div>
            </div>
          </AppRegistryHoverTip>
        </q-td>
      </template>
      <template #body-cell-tokens="props">
        <q-td :props="props">
          {{ formatCount(props.row.input_tokens) }} / {{ formatCount(props.row.output_tokens) }}
        </q-td>
      </template>
      <template #body-cell-latency="props">
        <q-td :props="props">{{ formatLatency(props.row.latency_ms) }}</q-td>
      </template>
      <template #body-cell-cost="props">
        <q-td :props="props">{{ formatMoney(props.row.total_cost_micro_usd) }}</q-td>
      </template>
      <template #body-cell-time="props">
        <q-td :props="props">
          <span class="app-registry-cell-sub">{{ formatDate(props.row.occurred_at) }}</span>
        </q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <div class="app-registry-cell-actions">
            <q-btn flat dense round icon="account_tree" color="primary" aria-label="查看 Trace" @click="onOpenTrace(props.row)">
              <q-tooltip>Details</q-tooltip>
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

  <q-dialog v-model="detailOpen" maximized @hide="stopFlowStream">
    <q-card class="monitor-trace-dialog">
      <q-card-section class="row items-start justify-between">
        <div>
          <div class="text-h6">Trace detail</div>
          <div class="text-caption text-grey-7">
            {{ detail?.agent_key || detail?.agent_id }} / {{ detail?.provider_code }} / {{ detail?.model_api_id }}
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
            color="primary"
            @click="openChatSession(activeCorrelation.sessionId)"
          />
          <flow-log-export-button :trace-id="activeCorrelation.traceId" :lines="flowLines" />
          <q-btn flat icon="content_copy" label="Copy JSON" @click="copyDetail" />
          <q-btn flat round dense icon="close" v-close-popup />
        </div>
      </q-card-section>
      <q-separator />
      <q-card-section class="row q-col-gutter-md">
        <div class="col-12 col-md-4">
          <q-card flat bordered class="monitor-card">
            <q-card-section>
              <div class="text-subtitle1 text-weight-bold">Summary</div>
              <q-list dense>
                <q-item>
                  <q-item-section>Status</q-item-section>
                  <q-item-section side>
                    <q-badge :color="statusColor(detail?.status)">{{ detail?.status || "unknown" }}</q-badge>
                  </q-item-section>
                </q-item>
                <q-item>
                  <q-item-section>Agent</q-item-section>
                  <q-item-section side>{{ detail?.agent_key || detail?.agent_id || "-" }}</q-item-section>
                </q-item>
                <q-item>
                  <q-item-section>Provider / Model</q-item-section>
                  <q-item-section side>{{ detail?.provider_code || "-" }} / {{ detail?.model_api_id || "-" }}</q-item-section>
                </q-item>
                <q-item>
                  <q-item-section>Tokens</q-item-section>
                  <q-item-section side>{{ formatCount(detail?.input_tokens) }} / {{ formatCount(detail?.output_tokens) }}</q-item-section>
                </q-item>
                <q-item>
                  <q-item-section>Latency</q-item-section>
                  <q-item-section side>{{ formatLatency(detail?.latency_ms) }}</q-item-section>
                </q-item>
                <q-item>
                  <q-item-section>Cost</q-item-section>
                  <q-item-section side>{{ formatMoney(detail?.total_cost_micro_usd) }}</q-item-section>
                </q-item>
              </q-list>
            </q-card-section>
          </q-card>
        </div>
        <div class="col-12 col-md-8">
          <q-card flat bordered class="monitor-card">
            <q-card-section>
              <q-tabs v-model="detailTab" dense align="left" active-color="primary">
                <q-tab name="flow" label="Flow" icon="timeline" />
                <q-tab name="waterfall" label="Waterfall" icon="waterfall_chart" />
                <q-tab name="tree" label="Span tree" icon="account_tree" />
              </q-tabs>
              <q-separator class="q-mt-sm" />
              <q-tab-panels v-model="detailTab" animated class="q-mt-md">
                <q-tab-panel name="flow" class="q-pa-none">
                  <div class="row items-center q-mb-sm q-gutter-sm">
                    <q-badge outline color="teal">{{ flowLines.length }} flow logs</q-badge>
                    <span class="text-caption text-grey-7">Live capture while detail is open (filtered by trace_id)</span>
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
              <div class="text-subtitle1 text-weight-bold">Raw JSON</div>
              <pre class="monitor-json">{{ detailJSON }}</pre>
            </q-card-section>
          </q-card>
        </div>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useMonitorRunNavigation } from "../../features/monitor/useMonitorRunNavigation";
import { useMonitorTraceFlow } from "../../features/monitor/useMonitorTraceFlow";
import { copyToClipboard, Notify, type QTableColumn } from "quasar";
import type { MonitorTraceEvent } from "../../features/monitor/types";
import { compactJSON, formatCount, formatDate, formatLatency, formatMoney, parseJSON } from "../../features/monitor/utils";
import TraceWaterfall from "./TraceWaterfall.vue";
import FlowTracePanel from "./FlowTracePanel.vue";
import FlowLogExportButton from "./FlowLogExportButton.vue";
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../layout/AppRegistryHoverTip.vue";
import AppPageToolbar from "../layout/AppPageToolbar.vue";
import AppRegistryPagination from "../layout/AppRegistryPagination.vue";
import { registryColWidth } from "../../features/ui/registryTableColumns";

type TreeNode = {
  id: string;
  label: string;
  caption?: string;
  children?: TreeNode[];
};

const props = defineProps<{
  rows: MonitorTraceEvent[];
  loading: boolean;
  highlightUsageEventId?: string;
}>();

defineEmits<{
  reload: [];
}>();

const { openChatSession } = useMonitorRunNavigation();

const keyword = ref("");
const page = ref(1);
const pageSize = ref(12);
const detail = ref<MonitorTraceEvent | null>(null);
const detailOpen = ref(false);
const detailTab = ref<"flow" | "waterfall" | "tree">("flow");

const { flowLines, activeCorrelation, stopFlowStream, openTraceDetail } = useMonitorTraceFlow(detail, detailOpen);

const columns: QTableColumn<MonitorTraceEvent>[] = [
  { name: "name", label: "Agent", field: "agent_key", align: "left", ...registryColWidth("14%") },
  { name: "tokens", label: "Token in / out", field: "total_tokens", align: "left", ...registryColWidth("9%") },
  { name: "latency", label: "Latency", field: "latency_ms", align: "left", ...registryColWidth("72px") },
  { name: "cost", label: "Cost", field: "total_cost_micro_usd", align: "left", ...registryColWidth("72px") },
  { name: "time", label: "Time", field: "occurred_at", align: "left", ...registryColWidth("11%") },
  { name: "actions", label: "操作", field: "id", align: "right", ...registryColWidth("108px") }
];

const filteredRows = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  if (!q) return props.rows;
  return props.rows.filter((row) =>
    [row.agent_key, row.agent_id, row.provider_code, row.provider_display_name, row.model_api_id, row.model_display_name, row.status, row.error_code, row.error_message]
      .some((value) => String(value || "").toLowerCase().includes(q))
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

const detailJSON = computed(() => compactJSON(detail.value ?? {}));
const spanList = computed(() => {
  if (!detail.value) return [];
  const metadata = parseJSON(detail.value.metadata_json || "");
  return Array.isArray(metadata.spans) ? metadata.spans : [];
});
const spanNodes = computed<TreeNode[]>(() => {
  if (!detail.value) return [];
  const metadataSpans = spanList.value;
  if (metadataSpans.length) return metadataSpans.map((span, index) => spanToNode(span, index));
  return [
    {
      id: detail.value.id,
      label: `${detail.value.provider_code || "provider"} / ${detail.value.model_api_id || "model"}`,
      caption: `${detail.value.status} | ${formatLatency(detail.value.latency_ms)} | ${formatCount(detail.value.total_tokens)} tokens`,
      children: detail.value.error_message
        ? [{ id: `${detail.value.id}-error`, label: "error", caption: detail.value.error_message }]
        : undefined
    }
  ];
});

async function onOpenTrace(row: MonitorTraceEvent) {
  detailTab.value = "flow";
  await openTraceDetail(row);
}

function tryOpenHighlightedRun() {
  const hit = (props.highlightUsageEventId || "").trim();
  if (!hit || props.rows.length === 0) return;
  const row = props.rows.find((r) => String(r.id || "").trim() === hit);
  if (row) void onOpenTrace(row);
}

watch(() => props.highlightUsageEventId, tryOpenHighlightedRun);
watch(() => props.rows, tryOpenHighlightedRun, { deep: true });

function spanToNode(span: unknown, index: number): TreeNode {
  const row = (span && typeof span === "object" ? span : {}) as Record<string, unknown>;
  const children = Array.isArray(row.children) ? row.children.map((child, childIndex) => spanToNode(child, childIndex)) : undefined;
  return {
    id: String(row.id || row.name || `span-${index}`),
    label: String(row.name || row.type || row.kind || `span #${index + 1}`),
    caption: [row.status, row.duration_ms ? `${row.duration_ms}ms` : "", row.model, row.tool_name].filter(Boolean).join(" | "),
    children
  };
}

async function copyDetail() {
  await copyToClipboard(detailJSON.value);
  Notify.create({ message: "Copied", color: "positive", position: "top" });
}

function statusColor(status?: string) {
  if (status === "ok" || status === "success") return "positive";
  if (status === "cancelled") return "grey";
  if (status === "timeout") return "orange";
  return "negative";
}
</script>

<style scoped>
.monitor-traces-toolbar {
  padding: 0 16px 8px;
  border-bottom: none;
}
</style>
