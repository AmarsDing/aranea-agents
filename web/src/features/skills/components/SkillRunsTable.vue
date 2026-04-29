<template>
  <q-table flat bordered class="skill-runs-table" row-key="id" :rows="rows" :columns="columns" :loading="loading" hide-pagination>
    <template #body-cell-time="props">
      <q-td :props="props">
        <div>{{ formatDate(props.row.started_at) }}</div>
        <div class="text-caption text-grey-7">{{ formatDuration(props.row.duration_ms) }}</div>
      </q-td>
    </template>
    <template #body-cell-skill="props">
      <q-td :props="props">
        <div class="text-weight-medium">{{ props.row.skill_name || props.row.skill_id }}</div>
        <q-chip dense size="sm" color="primary" text-color="white">{{ props.row.skill_version || "unknown" }}</q-chip>
      </q-td>
    </template>
    <template #body-cell-agent="props">
      <q-td :props="props">{{ props.row.agent_display_name || props.row.agent_id || "-" }}</q-td>
    </template>
    <template #body-cell-status="props">
      <q-td :props="props">
        <q-badge rounded :color="props.row.status === 'success' ? 'positive' : 'negative'">
          {{ props.row.status === "success" ? "成功" : "失败" }}
        </q-badge>
      </q-td>
    </template>
    <template #body-cell-output="props">
      <q-td :props="props">
        <div :class="props.row.status === 'success' ? 'text-positive' : 'text-negative'">
          {{ props.row.status === "success" ? props.row.output_preview || "-" : props.row.error_message || "-" }}
        </div>
      </q-td>
    </template>
  </q-table>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import type { SkillInvocation } from "../types";

defineProps<{
  rows: SkillInvocation[];
  loading: boolean;
}>();

const columns: QTableColumn<SkillInvocation>[] = [
  { name: "time", label: "时间 / 耗时", field: "started_at", align: "left" },
  { name: "skill", label: "Skill", field: "skill_name", align: "left" },
  { name: "agent", label: "Agent", field: "agent_display_name", align: "left" },
  { name: "status", label: "结果", field: "status", align: "left" },
  { name: "input", label: "输入", field: "input_preview", align: "left", style: "max-width: 220px; white-space: normal;" },
  { name: "output", label: "输出 / 错误", field: "output_preview", align: "left", style: "max-width: 260px; white-space: normal;" }
];

function formatDate(value?: string) {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

function formatDuration(value: number) {
  if (value < 1000) return `${Math.round(value)}ms`;
  return `${(value / 1000).toFixed(1)}s`;
}
</script>

<style scoped lang="sass">
.skill-runs-table
  border-radius: 22px
  overflow: hidden
</style>
