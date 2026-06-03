<template>
  <AppRegistryTable
    table-class="app-registry-table--sessions"
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
        <div class="row items-center no-wrap q-gutter-xs">
          <q-icon v-if="isSessionPinned(props.row)" name="push_pin" color="primary" size="16px" />
          <div class="col min-width-0">
            <div class="app-registry-cell-primary">{{ props.row.title }}</div>
            <div class="app-registry-cell-sub ellipsis">{{ props.row.summary || props.row.id }}</div>
          </div>
        </div>
      </q-td>
    </template>

    <template #body-cell-owner="props">
      <q-td :props="props">
        <q-chip dense :color="ownerChipColor(props.row.owner_type)" text-color="white">{{
          ownerLabel(props.row.owner_type)
        }}</q-chip>
        <div class="app-registry-cell-sub ellipsis">
          {{ props.row.owner_type === 'team' ? props.row.team_id : props.row.agent_id }}
        </div>
      </q-td>
    </template>

    <template #body-cell-context="props">
      <q-td :props="props">
        <q-linear-progress
          rounded
          size="10px"
          :value="ratioValue(props.row.context_used_ratio)"
          :color="contextProgressColor(props.row.context_status)"
        />
        <div class="app-registry-cell-sub q-mt-xs">
          {{ formatPercent(props.row.context_used_ratio) }} · {{ props.row.context_status }}
        </div>
      </q-td>
    </template>

    <template #body-cell-usage="props">
      <q-td :props="props">
        <div class="app-registry-cell-primary">{{ formatNumber(props.row.total_tokens) }} tokens</div>
        <div class="app-registry-cell-sub">
          {{ props.row.model_call_count }} model ·
          {{ props.row.tool_call_count + props.row.skill_call_count + props.row.mcp_call_count }} calls
        </div>
      </q-td>
    </template>

    <template #body-cell-time="props">
      <q-td :props="props">
        <div class="app-registry-cell-primary">
          {{ formatSessionDate(props.row.last_message_at || props.row.updated_at) }}
        </div>
        <div class="app-registry-cell-sub">创建 {{ formatSessionDate(props.row.created_at) }}</div>
      </q-td>
    </template>

    <template #body-cell-status="props">
      <q-td :props="props">
        <SessionStatusBadge
          :status="props.row.status"
          :status-reason="props.row.status_reason"
          :status-changed-at="props.row.status_changed_at"
        />
      </q-td>
    </template>

    <template #body-cell-actions="props">
      <q-td :props="props">
        <div class="app-registry-cell-actions">
          <q-btn
            flat
            dense
            round
            icon="visibility"
            color="primary"
            :to="{ name: 'session-detail', params: { sessionId: props.row.id } }"
          >
            <q-tooltip>查看详情</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="push_pin"
            :color="isSessionPinned(props.row) ? 'primary' : 'grey-6'"
            @click="$emit('toggle-pin', props.row.id, !isSessionPinned(props.row))"
          >
            <q-tooltip>{{ isSessionPinned(props.row) ? '取消置顶' : '置顶' }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="archive"
            color="primary"
            :disable="
              !!props.row.archived_at || props.row.status === 'running' || props.row.status === 'awaiting_confirmation'
            "
            @click="$emit('archive-row', props.row.id)"
          >
            <q-tooltip>{{
              props.row.status === 'running' || props.row.status === 'awaiting_confirmation'
                ? '执行中不可归档'
                : !!props.row.archived_at
                  ? '已归档'
                  : '归档'
            }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="delete"
            color="negative"
            :disable="props.row.status === 'running' || props.row.status === 'awaiting_confirmation'"
            @click="$emit('delete-row', props.row.id)"
          >
            <q-tooltip>{{
              props.row.status === 'running' || props.row.status === 'awaiting_confirmation'
                ? '执行中或等待确认时不可删除'
                : '永久删除'
            }}</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>

  <div class="app-registry-pagination q-mt-md">
    <div class="text-caption">共 {{ total }} 个 Session</div>
    <div class="row items-center q-gutter-sm">
      <q-select
        :model-value="pageSize"
        dense
        outlined
        emit-value
        map-options
        class="app-field-sm"
        :options="pageSizeOptions"
        @update:model-value="$emit('update:pageSize', $event)"
      />
      <q-pagination
        :model-value="page"
        :max="pageMax"
        :max-pages="6"
        direction-links
        boundary-links
        color="primary"
        @update:model-value="$emit('update:page', $event)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import SessionStatusBadge from './SessionStatusBadge.vue';
import type { Session } from '../../features/session/types';
import {
  contextProgressColor,
  formatNumber,
  formatPercent,
  formatSessionDate,
  isSessionPinned,
  ownerChipColor,
  ownerLabel,
  ratioValue,
  sessionsTableColumns,
  sessionsTableSelectionColumn,
} from './sessionUi';

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
  'update:page': [v: number];
  'update:pageSize': [v: number];
  'archive-row': [id: string];
  'delete-row': [id: string];
  'toggle-pin': [id: string, pinned: boolean];
  'toggle-row': [id: string, checked: boolean];
  'toggle-page': [checked: boolean];
}>();

const tableColumns = computed(() =>
  props.selectionMode ? [sessionsTableSelectionColumn, ...sessionsTableColumns] : sessionsTableColumns,
);
</script>
