<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col-12 col-md">
        <div class="text-h6 text-weight-bold">Runs</div>
        <div class="text-caption text-grey-7">单次对话运行真相源（Token 用量 + Flow / Waterfall / Span）</div>
      </div>
      <q-input v-model="keyword" dense outlined clearable debounce="200" class="col-12 col-md-4" label="Search">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-btn flat rounded icon="refresh" label="Reload" :loading="loading" @click="$emit('reload')" />
    </q-card-section>
    <q-separator />
    <q-table flat :rows="filteredRows" :columns="columns" row-key="id" :loading="loading" :pagination="{ rowsPerPage: 12 }">
      <template #body-cell-name="props">
        <q-td :props="props">
          <div class="text-weight-bold">{{ props.row.agent_key || props.row.agent_id || "unknown agent" }}</div>
          <div class="row q-gutter-xs q-mt-xs">
            <q-chip dense outline>{{ props.row.provider_code || "provider" }}</q-chip>
            <q-chip dense outline>{{ props.row.model_api_id || "model" }}</q-chip>
            <q-chip dense :color="statusColor(props.row.status)" text-color="white">{{ props.row.status || "unknown" }}</q-chip>
          </div>
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
      <template #body-cell-error="props">
        <q-td :props="props">
          <div v-if="props.row.error_message" class="text-negative ellipsis">{{ props.row.error_message }}</div>
          <span v-else class="text-grey-7">-</span>
        </q-td>
      </template>
      <template #body-cell-time="props">
        <q-td :props="props">{{ formatDate(props.row.occurred_at) }}</q-td>
      </template>
      <template #body-cell-actions="props">
        <q-td :props="props">
          <q-btn flat dense round icon="account_tree" color="primary" @click="openTrace(props.row)">
            <q-tooltip>Details</q-tooltip>
          </q-btn>
        </q-td>
      </template>
    </q-table>
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
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useMonitorRunNavigation } from "../../features/monitor/useMonitorRunNavigation";
import { copyToClipboard, Notify, type QTableColumn } from "quasar";
import type { MonitorLogLine, MonitorTraceEvent } from "../../features/monitor/types";
import { compactJSON, formatCount, formatDate, formatLatency, formatMoney, parseJSON } from "../../features/monitor/utils";
import { flowLogMatchesTrace, sortFlowLogLines, traceCorrelationFromUsageRow } from "../../features/monitor/flow";
import { listFlowLogs, subscribeMonitorLogsWs } from "../../features/monitor/api";
import { GLOBAL_WS_SESSION_ID } from "../../config/runtime";
import TraceWaterfall from "./TraceWaterfall.vue";
import FlowTracePanel from "./FlowTracePanel.vue";
import FlowLogExportButton from "./FlowLogExportButton.vue";

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
const detail = ref<MonitorTraceEvent | null>(null);
const detailOpen = ref(false);
const detailTab = ref<"flow" | "waterfall" | "tree">("flow");
const flowLines = ref<MonitorLogLine[]>([]);

let flowWsSub: ReturnType<typeof subscribeMonitorLogsWs> | null = null;

const columns: QTableColumn<MonitorTraceEvent>[] = [
  { name: "name", label: "Agent", field: "agent_key", align: "left" },
  { name: "tokens", label: "Token in / out", field: "total_tokens", align: "left" },
  { name: "latency", label: "Latency", field: "latency_ms", align: "left" },
  { name: "cost", label: "Cost", field: "total_cost_micro_usd", align: "left" },
  { name: "error", label: "Error", field: "error_message", align: "left" },
  { name: "time", label: "Time", field: "occurred_at", align: "left" },
  { name: "actions", label: "Actions", field: "id", align: "right" }
];

const filteredRows = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  if (!q) return props.rows;
  return props.rows.filter((row) =>
    [row.agent_key, row.agent_id, row.provider_code, row.provider_display_name, row.model_api_id, row.model_display_name, row.status, row.error_code, row.error_message]
      .some((value) => String(value || "").toLowerCase().includes(q))
  );
});

const activeCorrelation = computed(() => {
  if (!detail.value) return { traceId: "", runId: "", sessionId: "" };
  return traceCorrelationFromUsageRow(detail.value);
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

async function openTrace(row: MonitorTraceEvent) {
  detail.value = row;
  detailTab.value = "flow";
  flowLines.value = [];
  detailOpen.value = true;
  await loadFlowHistory();
  startFlowStream();
}

async function loadFlowHistory() {
  const corr = activeCorrelation.value;
  if (!corr.traceId && !corr.runId && !corr.sessionId) return;
  try {
    const { items } = await listFlowLogs({
      traceId: corr.traceId || undefined,
      runId: corr.runId || undefined,
      sessionId: corr.sessionId || undefined,
      limit: 500
    });
    const seen = new Set<string>();
    const merged = [...items, ...flowLines.value].filter((line) => {
      const key = `${line.id}-${line.time}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return flowLogMatchesTrace(line, corr);
    });
    flowLines.value = sortFlowLogLines(merged);
  } catch {
    // HTTP history is best-effort; live WS still works
  }
}

function tryOpenHighlightedRun() {
  const hit = (props.highlightUsageEventId || "").trim();
  if (!hit || props.rows.length === 0) return;
  const row = props.rows.find((r) => String(r.id || "").trim() === hit);
  if (row) openTrace(row);
}

watch(() => props.highlightUsageEventId, tryOpenHighlightedRun);
watch(() => props.rows, tryOpenHighlightedRun, { deep: true });

function startFlowStream() {
  stopFlowStream();
  const corr = activeCorrelation.value;
  if (!corr.traceId && !corr.runId) return;
  const maxLines = 500;
  flowWsSub = subscribeMonitorLogsWs(GLOBAL_WS_SESSION_ID, (line) => {
    if (!flowLogMatchesTrace(line, corr)) return;
    flowLines.value = [...flowLines.value, line].slice(-maxLines);
  });
}

function stopFlowStream() {
  flowWsSub?.close();
  flowWsSub = null;
}

watch(detailOpen, (open) => {
  if (!open) stopFlowStream();
});

onBeforeUnmount(() => stopFlowStream());

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
