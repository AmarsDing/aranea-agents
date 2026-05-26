<template>
  <tool-glass-panel v-if="!loading && rows.length === 0">
    <q-card-section class="column items-center text-center q-pa-xl">
      <q-avatar size="72px" color="primary" text-color="white" icon="history" />
      <div class="text-h6 q-mt-md">暂无 Tool 调用记录</div>
      <div class="text-body2 muted-caption q-mt-sm">接入 ADK Tool 执行审计后，这里会显示参数摘要、结果摘要和耗时。</div>
    </q-card-section>
  </tool-glass-panel>

  <tool-glass-panel v-else>
    <AppRegistryTable
      :shell="false"
      table-class="tool-runs-data-table"
      row-key="id"
      :rows="rows"
      :columns="columns"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-tool="props">
        <q-td :props="props">
          <AppRegistryHoverTip :text="toolRunHoverText(props.row)" empty-label="无参数 / 结果摘要">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">{{ props.row.tool_display_name || props.row.tool_key }}</div>
              <div class="app-registry-cell-sub ellipsis">{{ props.row.tool_key }}</div>
            </div>
          </AppRegistryHoverTip>
        </q-td>
      </template>

      <template #body-cell-agent="props">
        <q-td :props="props">
          <div class="app-registry-cell-primary ellipsis">{{ invocationAgentLine(props.row) }}</div>
          <div class="app-registry-cell-sub ellipsis">{{ props.row.agent_id }}</div>
        </q-td>
      </template>

      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge rounded :color="toolInvocationStatusColor(props.row.status)">{{ toolInvocationStatusLabel(props.row.status) }}</q-badge>
        </q-td>
      </template>

      <template #body-cell-session_id="props">
        <q-td :props="props">
          <span class="app-registry-cell-sub ellipsis" :title="props.row.session_id">{{ props.row.session_id || "—" }}</span>
        </q-td>
      </template>

      <template #body-cell-time="props">
        <q-td :props="props">
          <div>{{ formatInvocationWhen(props.row.started_at) }}</div>
          <div class="text-caption muted-caption">{{ formatInvocationDuration(props.row.duration_ms) }}</div>
        </q-td>
      </template>
    </AppRegistryTable>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../layout/AppRegistryHoverTip.vue";
import ToolGlassPanel from "./ToolGlassPanel.vue";
import type { ToolInvocation } from "../../features/tools/types";
import { registryColWidth } from "../../features/ui/registryTableColumns";
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

const columns: QTableColumn<ToolInvocation>[] = [
  { name: "tool", label: "Tool", field: "tool_key", align: "left", ...registryColWidth("18%") },
  { name: "agent", label: "Agent", field: "agent_id", align: "left", ...registryColWidth("10%") },
  { name: "status", label: "状态", field: "status", align: "left", ...registryColWidth("9%") },
  { name: "session_id", label: "Session", field: "session_id", align: "left", ...registryColWidth("10%") },
  { name: "time", label: "时间 / 耗时", field: "started_at", align: "left", ...registryColWidth("11%") }
];

function toolRunHoverText(row: ToolInvocation) {
  const input = clipPreview(row.input_preview);
  const output = clipPreview(row.output_preview || row.error_message);
  const parts = [];
  if (input) parts.push(`参数：${input}`);
  if (output) parts.push(`结果：${output}`);
  return parts.join("\n\n");
}
</script>
