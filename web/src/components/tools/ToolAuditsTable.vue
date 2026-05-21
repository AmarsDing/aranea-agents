<template>
  <tool-glass-panel v-if="!loading && rows.length === 0">
    <q-card-section class="column items-center text-center q-pa-xl">
      <q-avatar size="72px" color="primary" text-color="white" icon="policy" />
      <div class="text-h6 q-mt-md">暂无工具审计记录</div>
      <div class="text-body2 muted-caption q-mt-sm">Agent 运行时的工具调用将写入结构化审计日志（默认保留 90 天）。</div>
    </q-card-section>
  </tool-glass-panel>

  <tool-glass-panel v-else>
    <q-table
      flat
      class="tool-runs-data-table"
      row-key="id"
      :rows="rows"
      :columns="columns"
      :loading="loading"
      :pagination="tablePagination"
      hide-pagination
    >
      <template #body-cell-tool="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.tool_key }}</div>
          <div class="text-caption muted-caption">{{ props.row.action }}</div>
        </q-td>
      </template>

      <template #body-cell-actor="props">
        <q-td :props="props">
          <div>{{ props.row.agent_id || "—" }}</div>
          <div class="text-caption muted-caption">{{ props.row.user_id || props.row.session_id || "—" }}</div>
        </q-td>
      </template>

      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge rounded :color="toolInvocationStatusColor(props.row.status)">{{ toolInvocationStatusLabel(props.row.status) }}</q-badge>
        </q-td>
      </template>

      <template #body-cell-summary="props">
        <q-td :props="props">
          <div class="text-caption ellipsis">{{ clipPreview(props.row.result_summary) || "无摘要" }}</div>
        </q-td>
      </template>

      <template #body-cell-time="props">
        <q-td :props="props">
          <div>{{ formatInvocationWhen(props.row.created_at) }}</div>
          <div class="text-caption muted-caption">{{ props.row.source }}</div>
        </q-td>
      </template>
    </q-table>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import ToolGlassPanel from "./ToolGlassPanel.vue";
import type { ToolInvocationAudit } from "../../features/tools/types";
import {
  clipPreview,
  formatInvocationWhen,
  toolInvocationStatusColor,
  toolInvocationStatusLabel
} from "./toolUi";

defineProps<{
  rows: ToolInvocationAudit[];
  loading: boolean;
}>();

const tablePagination = { rowsPerPage: 0 };

const columns: QTableColumn<ToolInvocationAudit>[] = [
  { name: "tool", label: "Tool / Action", field: "tool_key", align: "left" },
  { name: "actor", label: "Agent / User", field: "agent_id", align: "left" },
  { name: "status", label: "状态", field: "status", align: "left" },
  { name: "summary", label: "结果摘要", field: "result_summary", align: "left" },
  { name: "time", label: "时间", field: "created_at", align: "left" }
];
</script>
