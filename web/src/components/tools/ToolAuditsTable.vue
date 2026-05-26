<template>
  <tool-glass-panel v-if="!loading && rows.length === 0">
    <q-card-section class="column items-center text-center q-pa-xl">
      <q-avatar size="72px" color="primary" text-color="white" icon="policy" />
      <div class="text-h6 q-mt-md">暂无工具审计记录</div>
      <div class="text-body2 muted-caption q-mt-sm">Agent 运行时的工具调用将写入结构化审计日志（默认保留 90 天）。</div>
    </q-card-section>
  </tool-glass-panel>

  <tool-glass-panel v-else>
    <AppRegistryTable
      :shell="false"
      table-class="tool-runs-data-table"
      row-key="id"
      :rows="rows"
      :columns="TOOL_AUDITS_TABLE_COLUMNS"
      :loading="loading"
      hide-pagination
      :pagination="{ rowsPerPage: 0 }"
    >
      <template #body-cell-tool="props">
        <q-td :props="props">
          <AppRegistryHoverTip :text="clipPreview(props.row.result_summary)" empty-label="无摘要">
            <div class="min-width-0">
              <div class="app-registry-cell-primary ellipsis">{{ props.row.tool_key }}</div>
              <div class="app-registry-cell-sub ellipsis">{{ props.row.action }}</div>
            </div>
          </AppRegistryHoverTip>
        </q-td>
      </template>

      <template #body-cell-actor="props">
        <q-td :props="props">
          <div class="app-registry-cell-primary ellipsis">{{ props.row.agent_id || "—" }}</div>
          <div class="app-registry-cell-sub ellipsis">{{ props.row.user_id || props.row.session_id || "—" }}</div>
        </q-td>
      </template>

      <template #body-cell-status="props">
        <q-td :props="props">
          <q-badge rounded :color="toolInvocationStatusColor(props.row.status)">{{ toolInvocationStatusLabel(props.row.status) }}</q-badge>
        </q-td>
      </template>

      <template #body-cell-time="props">
        <q-td :props="props">
          <div>{{ formatInvocationWhen(props.row.created_at) }}</div>
          <div class="text-caption muted-caption ellipsis">{{ props.row.source }}</div>
        </q-td>
      </template>
    </AppRegistryTable>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../layout/AppRegistryHoverTip.vue";
import ToolGlassPanel from "./ToolGlassPanel.vue";
import type { ToolInvocationAudit } from "../../features/tools/types";
import {
  TOOL_AUDITS_TABLE_COLUMNS,
  clipPreview,
  formatInvocationWhen,
  toolInvocationStatusColor,
  toolInvocationStatusLabel
} from "./toolUi";

defineProps<{
  rows: ToolInvocationAudit[];
  loading: boolean;
}>();
</script>
