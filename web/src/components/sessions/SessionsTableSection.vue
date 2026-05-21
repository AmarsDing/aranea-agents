<template>
  <q-table
    flat
    class="sessions-table"
    row-key="id"
    :rows="rows"
    :columns="tableColumns"
    :loading="loading"
    :pagination="{ rowsPerPage: pageSize }"
    hide-pagination
  >
    <template v-if="selectionMode" #header-cell-select="props">
      <q-th :props="props">
        <q-checkbox dense :model-value="pageAllSelected" @update:model-value="$emit('toggle-page', $event)" />
      </q-th>
    </template>

    <template v-if="selectionMode" #body-cell-select="props">
      <q-td :props="props">
        <q-checkbox
          dense
          :model-value="isRowSelected(props.row.id)"
          @update:model-value="$emit('toggle-row', props.row.id, $event)"
        />
      </q-td>
    </template>

    <template #body-cell-session="props">
      <q-td :props="props">
        <div class="text-weight-medium" style="color: var(--color-text-primary)">{{ props.row.title }}</div>
        <div class="sessions-muted text-caption ellipsis">{{ props.row.summary || props.row.id }}</div>
      </q-td>
    </template>

    <template #body-cell-owner="props">
      <q-td :props="props">
        <q-chip dense :color="ownerChipColor(props.row.owner_type)" text-color="white">{{ ownerLabel(props.row.owner_type) }}</q-chip>
        <div class="sessions-muted text-caption">{{ props.row.owner_type === "team" ? props.row.team_id : props.row.agent_id }}</div>
      </q-td>
    </template>

    <template #body-cell-context="props">
      <q-td :props="props" style="min-width: 160px">
        <q-linear-progress
          rounded
          size="10px"
          :value="ratioValue(props.row.context_used_ratio)"
          :color="contextProgressColor(props.row.context_status)"
        />
        <div class="sessions-muted text-caption q-mt-xs">{{ formatPercent(props.row.context_used_ratio) }} · {{ props.row.context_status }}</div>
      </q-td>
    </template>

    <template #body-cell-usage="props">
      <q-td :props="props">
        <div style="color: var(--color-text-primary)">{{ formatNumber(props.row.total_tokens) }} tokens</div>
        <div class="sessions-muted text-caption">
          {{ props.row.model_call_count }} model · {{ props.row.tool_call_count + props.row.skill_call_count + props.row.mcp_call_count }} calls
        </div>
      </q-td>
    </template>

    <template #body-cell-time="props">
      <q-td :props="props">
        <div style="color: var(--color-text-primary)">{{ formatSessionDate(props.row.last_message_at || props.row.updated_at) }}</div>
        <div class="sessions-muted text-caption">创建 {{ formatSessionDate(props.row.created_at) }}</div>
      </q-td>
    </template>

    <template #body-cell-status="props">
      <q-td :props="props">
        <q-badge :color="statusBadgeColor(props.row.status)">{{ props.row.status }}</q-badge>
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <q-btn flat dense round class="sessions-table-action" icon="visibility" :to="{ name: 'session-detail', params: { sessionId: props.row.id } }">
          <q-tooltip>查看详情</q-tooltip>
        </q-btn>
        <q-btn flat dense round icon="archive" class="sessions-table-action-muted" :disable="props.row.status === 'archived' || props.row.status === 'running'" @click="$emit('archive-row', props.row.id)">
          <q-tooltip>{{ props.row.status === 'running' ? '运行中不可归档' : props.row.status === 'archived' ? '已归档' : '归档' }}</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          icon="delete"
          class="sessions-table-action-muted"
          :disable="props.row.status === 'running'"
          @click="$emit('delete-row', props.row.id)"
        >
          <q-tooltip>{{ props.row.status === 'running' ? '运行中不可删除' : '永久删除' }}</q-tooltip>
        </q-btn>
      </q-td>
    </template>
  </q-table>

  <div class="row items-center justify-between q-mt-md sessions-pagination">
    <div class="sessions-muted text-caption">共 {{ total }} 个 Session</div>
    <div class="row items-center q-gutter-sm">
      <q-select
        :model-value="pageSize"
        dense
        outlined
        emit-value
        map-options
        class="sessions-page-size-select"
        :options="pageSizeOptions"
        @update:model-value="$emit('update:pageSize', $event)"
      />
      <q-pagination
        :model-value="page"
        :max="pageMax"
        :max-pages="6"
        direction-links
        boundary-links
        class="sessions-pagination-control"
        @update:model-value="$emit('update:page', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { Session } from "../../features/session/api";
import {
  contextProgressColor,
  formatNumber,
  formatPercent,
  formatSessionDate,
  ownerChipColor,
  ownerLabel,
  ratioValue,
  sessionsTableColumns,
  sessionsTableSelectionColumn,
  statusBadgeColor
} from "./sessionUi";

const props = defineProps<{
  rows: Session[];
  loading?: boolean;
  page: number;
  pageSize: number;
  pageMax: number;
  total: number;
  pageSizeOptions: { label: string; value: number }[];
  selectionMode?: boolean;
  isRowSelected: (id: string) => boolean;
  pageAllSelected: boolean;
}>();

defineEmits<{
  "update:page": [v: number];
  "update:pageSize": [v: number];
  "archive-row": [id: string];
  "delete-row": [id: string];
  "toggle-row": [id: string, checked: boolean];
  "toggle-page": [checked: boolean];
}>();

const tableColumns = computed(() =>
  props.selectionMode ? [sessionsTableSelectionColumn, ...sessionsTableColumns] : sessionsTableColumns
);
</script>

<style scoped>
.sessions-page-size-select {
  width: 110px;
  min-width: 110px;
}
</style>
