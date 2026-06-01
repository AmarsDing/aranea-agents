<template>
  <AppRegistryTable
    table-class="tools-data-table"
    row-key="id"
    :rows="rows"
    :columns="TOOL_TABLE_COLUMNS"
    :loading="loading"
    :pagination="tablePagination"
    :selected="selected"
    selection="multiple"
    hide-pagination
    @update:selected="$emit('update:selected', $event)"
  >
      <template #body-cell-name="props">
        <q-td :props="props">
          <div class="app-registry-cell-primary">{{ props.row.display_name }}</div>
          <div class="app-registry-cell-sub app-registry-muted-caption">{{ props.row.key }}</div>
        </q-td>
      </template>

      <template #body-cell-category="props">
        <q-td :props="props">
          <q-chip dense color="primary" text-color="white">{{ props.row.category || "custom" }}</q-chip>
          <q-chip dense outline class="q-ml-xs app-registry-source-chip">{{ props.row.source || "external" }}</q-chip>
        </q-td>
      </template>

      <template #body-cell-risk="props">
        <q-td :props="props">
          <q-select
            dense
            outlined
            emit-value
            map-options
            :model-value="props.row.risk_level"
            :options="riskLevelOptions"
            class="tool-risk-inline-select"
            :loading="busyId === props.row.id"
            @update:model-value="$emit('updateRisk', props.row, String($event ?? 'low'))"
          />
          <q-badge v-if="props.row.requires_confirmation" rounded color="warning" class="q-ml-xs">
            需确认
            <q-tooltip>{{ policyChip.requires_confirmation.tooltip }}</q-tooltip>
          </q-badge>
        </q-td>
      </template>

      <template #body-cell-runtime="props">
        <q-td :props="props">
          <q-badge rounded :color="runtimeStatusColor(props.row.runtime_status)">
            {{ runtimeStatusLabel(props.row.runtime_status) }}
          </q-badge>
          <div class="text-caption app-registry-muted-caption q-mt-xs">{{ runtimeKindHint(props.row) }}</div>
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
          <div class="text-caption app-registry-muted-caption">24h {{ props.row.invoke_count_24h }} · 失败 {{ props.row.failure_count }}</div>
        </q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props">
          <div class="app-registry-cell-actions">
          <q-btn flat dense round class="app-registry-icon-btn" color="primary" icon="visibility" @click="$emit('viewDetail', props.row)">
            <q-tooltip>查看</q-tooltip>
          </q-btn>
          <q-btn flat dense round class="app-registry-icon-btn" color="primary" icon="edit" @click="$emit('edit', props.row)">
            <q-tooltip>编辑</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            class="app-registry-icon-btn"
            color="negative"
            icon="delete"
            :loading="busyId === props.row.id"
            @click="$emit('remove', props.row)"
          >
            <q-tooltip>删除</q-tooltip>
          </q-btn>
          </div>
        </q-td>
      </template>
    </AppRegistryTable>
</template>

<script setup lang="ts">
import AppRegistryTable from "../layout/AppRegistryTable.vue";
import type { Tool } from "../../features/tools/types";
import { TOOL_POLICY_CHIP_COPY } from "../../features/tools/toolEditorCopy";
import {
  TOOL_TABLE_COLUMNS,
  riskLevelOptions,
  runtimeKindHint,
  runtimeStatusLabel,
  runtimeStatusColor
} from "./toolUi";

defineProps<{
  rows: Tool[];
  loading: boolean;
  busyId: string;
  selected?: Tool[];
}>();

defineEmits<{
  toggleEnabled: [tool: Tool, value: boolean];
  updateRisk: [tool: Tool, value: string];
  viewDetail: [tool: Tool];
  edit: [tool: Tool];
  remove: [tool: Tool];
  "update:selected": [value: Tool[]];
}>();

const tablePagination = { rowsPerPage: 0 };
const policyChip = TOOL_POLICY_CHIP_COPY;
</script>
