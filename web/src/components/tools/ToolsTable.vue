<template>
  <tool-glass-panel>
    <q-table
      flat
      class="tools-data-table"
      row-key="id"
      :rows="rows"
      :columns="columns"
      :loading="loading"
      :pagination="tablePagination"
      hide-pagination
    >
      <template #body-cell-name="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.display_name }}</div>
          <div class="text-caption muted-caption">{{ props.row.key }}</div>
        </q-td>
      </template>

      <template #body-cell-category="props">
        <q-td :props="props">
          <q-chip dense color="primary" text-color="white">{{ props.row.category || "custom" }}</q-chip>
          <q-chip dense outline class="q-ml-xs source-chip">{{ props.row.source || "external" }}</q-chip>
        </q-td>
      </template>

      <template #body-cell-risk="props">
        <q-td :props="props">
          <q-badge rounded :color="riskQuasarColor(props.row.risk_level)">{{ riskLabel(props.row.risk_level) }}</q-badge>
          <q-badge v-if="props.row.requires_confirmation" rounded color="warning" class="q-ml-xs">需确认</q-badge>
        </q-td>
      </template>

      <template #body-cell-runtime="props">
        <q-td :props="props">
          <q-badge rounded :color="props.row.runtime_status === 'catalog_only' ? 'grey' : 'positive'">
            {{ runtimeStatusLabel(props.row.runtime_status) }}
          </q-badge>
          <div class="text-caption muted-caption q-mt-xs">{{ runtimeKindHint(props.row) }}</div>
        </q-td>
      </template>

      <template #body-cell-enabled="props">
        <q-td :props="props">
          <q-toggle
            dense
            color="primary"
            :model-value="props.row.enabled"
            :disable="busyId === props.row.id"
            @update:model-value="$emit('toggleEnabled', props.row, Boolean($event))"
          />
        </q-td>
      </template>

      <template #body-cell-stats="props">
        <q-td :props="props">
          <div class="text-weight-medium">{{ props.row.invoke_count }} 次</div>
          <div class="text-caption muted-caption">24h {{ props.row.invoke_count_24h }} · 失败 {{ props.row.failure_count }}</div>
        </q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props" class="q-gutter-xs">
          <q-btn flat dense round class="tool-icon-btn" color="primary" icon="visibility" @click="$emit('viewDetail', props.row)">
            <q-tooltip>查看</q-tooltip>
          </q-btn>
          <q-btn flat dense round class="tool-icon-btn" color="primary" icon="edit" @click="$emit('edit', props.row)">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="tool-icon-btn"
            color="negative"
            icon="delete"
            :loading="busyId === props.row.id"
            @click="$emit('remove', props.row)"
          >
            <q-tooltip>删除</q-tooltip>
          </q-btn>
        </q-td>
      </template>
    </q-table>
  </tool-glass-panel>
</template>

<script setup lang="ts">
import type { QTableColumn } from "quasar";
import ToolGlassPanel from "./ToolGlassPanel.vue";
import type { Tool } from "../../features/tools/types";
import {
  riskLabel,
  riskQuasarColor,
  runtimeKindHint,
  runtimeStatusLabel
} from "./toolUi";

defineProps<{
  rows: Tool[];
  loading: boolean;
  busyId: string;
}>();

defineEmits<{
  toggleEnabled: [tool: Tool, value: boolean];
  viewDetail: [tool: Tool];
  edit: [tool: Tool];
  remove: [tool: Tool];
}>();

const tablePagination = { rowsPerPage: 0 };

const columns: QTableColumn<Tool>[] = [
  { name: "name", label: "Tool", field: "display_name", align: "left" },
  { name: "category", label: "分类 / 来源", field: "category", align: "left" },
  { name: "risk", label: "风险", field: "risk_level", align: "left" },
  { name: "runtime", label: "运行时", field: "runtime_status", align: "left" },
  { name: "enabled", label: "启用", field: "enabled", align: "left" },
  { name: "stats", label: "调用", field: "invoke_count", align: "left" },
  { name: "actions", label: "操作", field: "id", align: "right" }
];
</script>

<style scoped lang="sass">
.muted-caption
  color: var(--color-text-secondary)

.tools-data-table :deep(thead tr th)
  color: var(--color-text-secondary)

.source-chip
  border-color: var(--glass-border)

.tool-icon-btn
  color: var(--color-icon-muted)

body:not(.body--dark) .tool-icon-btn:hover
  color: var(--color-accent)
</style>
