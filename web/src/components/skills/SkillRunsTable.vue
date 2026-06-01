<template>
  <AppRegistryTable
    table-class="skill-runs-table"
    row-key="id"
    :rows="rows"
    :columns="SKILL_RUNS_TABLE_COLUMNS"
    :loading="loading"
    hide-pagination
  >
    <template #body-cell-time="props">
      <q-td :props="props">
        <div>{{ formatDate(props.row.started_at) }}</div>
        <div class="text-caption text-grey-7">{{ formatDuration(props.row.duration_ms) }}</div>
      </q-td>
    </template>
    <template #body-cell-skill="props">
      <q-td :props="props">
        <AppRegistryHoverTip :text="props.row.input_preview" empty-label="无输入摘要">
          <div class="min-width-0">
            <div class="app-registry-cell-primary">{{ props.row.skill_name || props.row.skill_id }}</div>
            <q-chip dense size="sm" color="primary" text-color="white">{{ props.row.skill_version || "unknown" }}</q-chip>
          </div>
        </AppRegistryHoverTip>
      </q-td>
    </template>
    <template #body-cell-agent="props">
      <q-td :props="props">{{ props.row.agent_display_name || props.row.agent_id || "-" }}</q-td>
    </template>
    <template #body-cell-status="props">
      <q-td :props="props">
        <AppRegistryHoverTip
          :text="props.row.status === 'success' ? props.row.output_preview : props.row.error_message"
          empty-label="无输出摘要"
        >
          <q-badge rounded :color="props.row.status === 'success' ? 'positive' : 'negative'">
            {{ props.row.status === "success" ? "成功" : "失败" }}
          </q-badge>
        </AppRegistryHoverTip>
      </q-td>
    </template>
  </AppRegistryTable>
</template>

<script setup lang="ts">
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import AppRegistryHoverTip from "../layout/AppRegistryHoverTip.vue";
import type { SkillInvocation } from "../../features/skills/types";
import { SKILL_RUNS_TABLE_COLUMNS } from "./skillTableUi";

defineProps<{
  rows: SkillInvocation[];
  loading: boolean;
}>();

function formatDate(value?: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function formatDuration(value: number) {
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}
</script>
