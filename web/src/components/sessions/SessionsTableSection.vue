<template>
  <AppRegistryTable
    :key="selectionMode ? 'sessions-table-with-select' : 'sessions-table-without-select'"
    table-class="app-registry-table--sessions"
    row-key="id"
    :rows="rows"
    :columns="tableColumns"
    :loading="loading"
    :pagination="{ rowsPerPage: pageSize }"
    hide-pagination
  >
    <template v-if="selectionMode" #header-cell-select="slotProps">
      <q-th :props="slotProps">
        <q-checkbox dense :model-value="pageAllSelected" @update:model-value="$emit('toggle-page', $event)" />
      </q-th>
    </template>

    <template v-if="selectionMode" #body-cell-select="slotProps">
      <q-td :props="slotProps">
        <q-checkbox
          dense
          :model-value="isRowSelected(slotProps.row.id)"
          @update:model-value="$emit('toggle-row', slotProps.row.id, $event)"
        />
      </q-td>
    </template>

    <template #body-cell-session="slotProps">
      <q-td :props="slotProps">
        <div class="row items-center no-wrap q-gutter-xs">
          <q-icon v-if="isSessionPinned(slotProps.row)" name="push_pin" color="primary" size="16px" />
          <div class="col min-width-0">
            <router-link
              :to="{ name: 'session-detail', params: { sessionId: slotProps.row.id } }"
              class="app-registry-cell-primary session-title-link"
            >
              {{ slotProps.row.title }}
            </router-link>
            <div class="app-registry-cell-sub ellipsis">{{ slotProps.row.summary || slotProps.row.id }}</div>
          </div>
        </div>
      </q-td>
    </template>

    <template #body-cell-owner="slotProps">
      <q-td :props="slotProps">
        <q-chip dense :color="ownerChipColor(slotProps.row.owner_type)" text-color="white">{{
          ownerLabel(slotProps.row.owner_type)
        }}</q-chip>
        <div class="app-registry-cell-sub ellipsis">
          {{ slotProps.row.owner_type === 'team' ? slotProps.row.team_id : slotProps.row.agent_id }}
        </div>
      </q-td>
    </template>

    <template #body-cell-context="slotProps">
      <q-td :props="slotProps">
        <q-linear-progress
          rounded
          size="10px"
          :value="ratioValue(slotProps.row.context_used_ratio)"
          :color="contextProgressColor(slotProps.row.context_status)"
        />
        <div class="app-registry-cell-sub q-mt-xs">
          {{ formatPercent(slotProps.row.context_used_ratio) }} · {{ slotProps.row.context_status }}
        </div>
      </q-td>
    </template>

    <template #body-cell-usage="slotProps">
      <q-td :props="slotProps">
        <div class="app-registry-cell-primary">{{ formatNumber(slotProps.row.total_tokens) }} tokens</div>
        <div class="app-registry-cell-sub">
          {{ slotProps.row.model_call_count }} model ·
          {{ slotProps.row.tool_call_count + slotProps.row.skill_call_count + slotProps.row.mcp_call_count }} calls
        </div>
      </q-td>
    </template>

    <template #body-cell-time="slotProps">
      <q-td :props="slotProps">
        <div class="app-registry-cell-primary">
          {{ formatSessionDate(slotProps.row.last_message_at || slotProps.row.updated_at) }}
        </div>
        <div class="app-registry-cell-sub">
          {{ t('sessionsPage.createdAt') }} {{ formatSessionDate(slotProps.row.created_at) }}
        </div>
      </q-td>
    </template>

    <template #body-cell-status="slotProps">
      <q-td :props="slotProps">
        <SessionStatusBadge
          :status="slotProps.row.status"
          :status-reason="slotProps.row.status_reason"
          :status-changed-at="slotProps.row.status_changed_at"
        />
      </q-td>
    </template>

    <template #no-data>
      <div class="full-width row flex-center q-py-xl text-center">
        <div>
          <q-icon name="search_off" size="48px" color="grey-5" />
          <div class="text-h6 text-weight-medium q-mt-sm">{{ t('sessionsPage.emptyTitle') }}</div>
          <div class="text-caption text-grey-6">{{ t('sessionsPage.emptyHint') }}</div>
        </div>
      </div>
    </template>

    <template #body-cell-actions="slotProps">
      <q-td :props="slotProps">
        <div class="app-registry-cell-actions">
          <q-btn
            flat
            dense
            round
            icon="visibility"
            color="primary"
            :aria-label="t('sessionsPage.actionView')"
            :to="{ name: 'session-detail', params: { sessionId: slotProps.row.id } }"
          >
            <q-tooltip>{{ t('sessionsPage.actionView') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="inventory_2"
            color="primary"
            :aria-label="t('sessionsPage.actionViewArtifacts')"
            :to="{ name: 'artifacts', query: { session: slotProps.row.id } }"
          >
            <q-tooltip>{{ t('sessionsPage.actionViewArtifacts') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="push_pin"
            :color="isSessionPinned(slotProps.row) ? 'primary' : 'grey-6'"
            :aria-label="isSessionPinned(slotProps.row) ? t('sessionsPage.actionUnpin') : t('sessionsPage.actionPin')"
            @click="$emit('toggle-pin', slotProps.row.id, !isSessionPinned(slotProps.row))"
          >
            <q-tooltip>{{
              isSessionPinned(slotProps.row) ? t('sessionsPage.actionUnpin') : t('sessionsPage.actionPin')
            }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="archive"
            color="primary"
            :aria-label="
              slotProps.row.status === 'running' || slotProps.row.status === 'awaiting_confirmation'
                ? t('sessionsPage.actionArchiveRunning')
                : !!slotProps.row.archived_at
                  ? t('sessionsPage.actionArchiveArchived')
                  : t('sessionsPage.actionArchive')
            "
            :disable="
              !!slotProps.row.archived_at ||
              slotProps.row.status === 'running' ||
              slotProps.row.status === 'awaiting_confirmation'
            "
            @click="$emit('archive-row', slotProps.row.id)"
          >
            <q-tooltip>{{
              slotProps.row.status === 'running' || slotProps.row.status === 'awaiting_confirmation'
                ? t('sessionsPage.actionArchiveRunning')
                : !!slotProps.row.archived_at
                  ? t('sessionsPage.actionArchiveArchived')
                  : t('sessionsPage.actionArchive')
            }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="delete"
            color="negative"
            :aria-label="
              slotProps.row.status === 'running' || slotProps.row.status === 'awaiting_confirmation'
                ? t('sessionsPage.actionDeleteRunning')
                : t('sessionsPage.actionDelete')
            "
            :disable="slotProps.row.status === 'running' || slotProps.row.status === 'awaiting_confirmation'"
            @click="$emit('delete-row', slotProps.row.id)"
          >
            <q-tooltip>{{
              slotProps.row.status === 'running' || slotProps.row.status === 'awaiting_confirmation'
                ? t('sessionsPage.actionDeleteRunning')
                : t('sessionsPage.actionDelete')
            }}</q-tooltip>
          </q-btn>
        </div>
      </q-td>
    </template>
  </AppRegistryTable>

  <div class="app-registry-pagination q-mt-md">
    <div class="text-caption">{{ t('sessionsPage.totalSessions', { count: total }) }}</div>
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
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import SessionStatusBadge from './SessionStatusBadge.vue';

const { t } = useI18n();
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
