<template>
  <q-card flat bordered class="monitor-card">
    <q-card-section class="row items-center q-col-gutter-md">
      <div class="col-12 col-md">
        <div class="text-h6 text-weight-bold">追踪</div>
        <div class="text-caption text-grey-7">来自真实模型调用事件的 Trace、Token、延迟与错误上下文</div>
      </div>
      <q-input v-model="keyword" dense outlined clearable debounce="200" class="col-12 col-md-4" label="搜索 Trace">
        <template #prepend><q-icon name="search" /></template>
      </q-input>
      <q-btn flat rounded icon="refresh" label="刷新" :loading="loading" @click="$emit('reload')" />
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
            <q-tooltip>查看详情</q-tooltip>
          </q-btn>
        </q-td>
      </template>
    </q-table>
  </q-card>

  <q-dialog v-model="detailOpen" maximized>
    <q-card class="monitor-trace-dialog">
      <q-card-section class="row items-start justify-between">
        <div>
          <div class="text-h6">追踪详情</div>
          <div class="text-caption text-grey-7">{{ detail?.agent_key || detail?.agent_id }} / {{ detail?.provider_code }} / {{ detail?.model_api_id }}</div>
        </div>
        <div class="row q-gutter-sm">
          <q-btn flat icon="content_copy" label="复制追踪" @click="copyDetail" />
          <q-btn flat round dense icon="close" v-close-popup />
        </div>
      </q-card-section>
      <q-separator />
      <q-card-section class="row q-col-gutter-md">
        <div class="col-12 col-md-4">
          <q-card flat bordered class="monitor-card">
            <q-card-section>
              <div class="text-subtitle1 text-weight-bold">摘要</div>
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
              <div class="text-subtitle1 text-weight-bold">Span 树</div>
              <q-tree :nodes="spanNodes" node-key="id" default-expand-all />
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
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { copyToClipboard, Notify, type QTableColumn } from "quasar";
import type { MonitorTraceEvent } from "./types";
import { compactJSON, formatCount, formatDate, formatLatency, formatMoney, parseJSON } from "./utils";

type TreeNode = {
  id: string;
  label: string;
  caption?: string;
  children?: TreeNode[];
};

const props = defineProps<{
  rows: MonitorTraceEvent[];
  loading: boolean;
}>();

defineEmits<{
  reload: [];
}>();

const keyword = ref("");
const detail = ref<MonitorTraceEvent | null>(null);
const detailOpen = ref(false);

const columns: QTableColumn<MonitorTraceEvent>[] = [
  { name: "name", label: "名称", field: "agent_key", align: "left" },
  { name: "tokens", label: "令牌 in / out", field: "total_tokens", align: "left" },
  { name: "latency", label: "延迟", field: "latency_ms", align: "left" },
  { name: "cost", label: "费用", field: "total_cost_micro_usd", align: "left" },
  { name: "error", label: "错误", field: "error_message", align: "left" },
  { name: "time", label: "时间", field: "occurred_at", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];

const filteredRows = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  if (!q) return props.rows;
  return props.rows.filter((row) =>
    [row.agent_key, row.agent_id, row.provider_code, row.provider_display_name, row.model_api_id, row.model_display_name, row.status, row.error_code, row.error_message]
      .some((value) => String(value || "").toLowerCase().includes(q))
  );
});

const detailJSON = computed(() => compactJSON(detail.value ?? {}));
const spanNodes = computed<TreeNode[]>(() => {
  if (!detail.value) return [];
  const metadata = parseJSON(detail.value.metadata_json || "");
  const metadataSpans = Array.isArray(metadata.spans) ? metadata.spans : [];
  if (metadataSpans.length) return metadataSpans.map((span, index) => spanToNode(span, index));
  return [
    {
      id: detail.value.id,
      label: `${detail.value.provider_code || "provider"} / ${detail.value.model_api_id || "model"}`,
      caption: `${detail.value.status} · ${formatLatency(detail.value.latency_ms)} · ${formatCount(detail.value.total_tokens)} tokens`,
      children: detail.value.error_message
        ? [{ id: `${detail.value.id}-error`, label: "error", caption: detail.value.error_message }]
        : undefined
    }
  ];
});

function openTrace(row: MonitorTraceEvent) {
  detail.value = row;
  detailOpen.value = true;
}

function spanToNode(span: unknown, index: number): TreeNode {
  const row = (span && typeof span === "object" ? span : {}) as Record<string, unknown>;
  const children = Array.isArray(row.children) ? row.children.map((child, childIndex) => spanToNode(child, childIndex)) : undefined;
  return {
    id: String(row.id || row.name || `span-${index}`),
    label: String(row.name || row.type || row.kind || `span #${index + 1}`),
    caption: [row.status, row.duration_ms ? `${row.duration_ms}ms` : "", row.model].filter(Boolean).join(" · "),
    children
  };
}

async function copyDetail() {
  await copyToClipboard(detailJSON.value);
  Notify.create({ message: "已复制", color: "positive", position: "top" });
}

function statusColor(status?: string) {
  if (status === "success") return "positive";
  if (status === "cancelled") return "grey";
  if (status === "timeout") return "orange";
  return "negative";
}
</script>
