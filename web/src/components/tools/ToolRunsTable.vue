<template>
  <tool-glass-panel v-if="!loading && rows.length === 0">
    <q-card-section class="column items-center text-center q-pa-xl">
      <q-avatar size="72px" color="primary" text-color="white" icon="history" />
      <div class="text-h6 q-mt-md">暂无 Tool 调用记录</div>
      <div class="text-body2 muted-caption q-mt-sm">接入 ADK Tool 执行审计后，这里会显示参数摘要、结果摘要和耗时。</div>
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
          <div class="text-weight-medium">{{ props.row.tool_display_name || props.row.tool_key }}</div>
          <div class="text-caption muted-caption">{{ props.row.tool_key }}</div>
        </q-td>
      </template>

      <template #body-cell-agent="props">
        <q-td :props="props">
          <div>{{ invocationAgentLine(props.row) }}</div>
          <div class="text-caption muted-caption">{{ props.row.agent_id }}</div>
        </q-td>
      </template>

      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge rounded :color="toolInvocationStatusColor(props.row.status)">{{ toolInvocationStatusLabel(props.row.status) }}</q-badge>
        </q-td>
      </template>

      <template #body-cell-preview="props">
        <q-td :props="props">
          <div class="text-caption ellipsis">{{ clipPreview(props.row.input_preview) || "无参数摘要" }}</div>
          <div class="text-caption muted-caption ellipsis">{{ clipPreview(props.row.output_preview || props.row.error_message) || "无结果摘要" }}</div>
        </q-td>
      </template>

      <template #body-cell-time="props">
        <q-td :props="props">
          <div>{{ formatInvocationWhen(props.row.started_at) }}</div>
          <div class="text-caption muted-caption">{{ formatInvocationDuration(props.row.duration_ms) }}</div>
        </q-td>
      </template>
    </q-table>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import ToolGlassPanel from "./ToolGlassPanel.vue";
import type { ToolInvocation } from "../../features/tools/types";
import {
  clipPreview,
  formatInvocationDuration,
  formatInvocationWhen,
  invocationAgentLine,
  toolInvocationStatusColor,
  toolInvocationStatusLabel
} from "./toolUi";

defineProps<{
  rows: ToolInvocation[];
  loading: boolean;
}>();

const tablePagination = { rowsPerPage: 0 };

const columns: QTableColumn<ToolInvocation>[] = [
  { name: "tool", label: "Tool", field: "tool_key", align: "left" },
  { name: "agent", label: "Agent", field: "agent_id", align: "left" },
  { name: "status", label: "状态", field: "status", align: "left" },
  { name: "preview", label: "参数 / 结果摘要", field: "input_preview", align: "left", style: "max-width: 420px;" },
  { name: "session_id", label: "Session", field: "session_id", align: "left" },
  { name: "time", label: "时间 / 耗时", field: "started_at", align: "left" }
];
</script>

<style scoped lang="sass">
.muted-caption
  color: var(--color-text-secondary)

.tool-runs-data-table :deep(thead tr th)
  color: var(--color-text-secondary)
</style>
