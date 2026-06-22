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
            <router-link
              :to="{ name: 'session-detail', params: { sessionId: props.row.id } }"
              class="app-registry-cell-primary session-title-link"
            >
              {{ props.row.title }}
            </router-link>
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
        <div class="app-registry-cell-sub">
          {{ t('sessionsPage.createdAt') }} {{ formatSessionDate(props.row.created_at) }}
        </div>
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

    <template #no-data>
      <div class="full-width row flex-center q-py-xl text-center">
        <div>
          <q-icon name="search_off" size="48px" color="grey-5" />
          <div class="text-h6 text-weight-medium q-mt-sm">{{ t('sessionsPage.emptyTitle') }}</div>
          <div class="text-caption text-grey-6">{{ t('sessionsPage.emptyHint') }}</div>
        </div>
      </div>
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
            :aria-label="t('sessionsPage.actionView')"
            :to="{ name: 'session-detail', params: { sessionId: props.row.id } }"
          >
            <q-tooltip>{{ t('sessionsPage.actionView') }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="push_pin"
            :color="isSessionPinned(props.row) ? 'primary' : 'grey-6'"
            :aria-label="isSessionPinned(props.row) ? t('sessionsPage.actionUnpin') : t('sessionsPage.actionPin')"
            @click="$emit('toggle-pin', props.row.id, !isSessionPinned(props.row))"
          >
            <q-tooltip>{{
              isSessionPinned(props.row) ? t('sessionsPage.actionUnpin') : t('sessionsPage.actionPin')
            }}</q-tooltip>
          </q-btn>
          <q-btn
            flat
            dense
            round
            icon="archive"
            color="primary"
            :aria-label="
              props.row.status === 'running' || props.row.status === 'awaiting_confirmation'
                ? t('sessionsPage.actionArchiveRunning')
                : !!props.row.archived_at
                  ? t('sessionsPage.actionArchiveArchived')
                  : t('sessionsPage.actionArchive')
            "
            :disable="
              !!props.row.archived_at || props.row.status === 'running' || props.row.status === 'awaiting_confirmation'
            "
            @click="$emit('archive-row', props.row.id)"
          >
            <q-tooltip>{{
              props.row.status === 'running' || props.row.status === 'awaiting_confirmation'
                ? t('sessionsPage.actionArchiveRunning')
                : !!props.row.archived_at
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
              props.row.status === 'running' || props.row.status === 'awaiting_confirmation'
                ? t('sessionsPage.actionDeleteRunning')
                : t('sessionsPage.actionDelete')
            "
            :disable="props.row.status === 'running' || props.row.status === 'awaiting_confirmation'"
            @click="$emit('delete-row', props.row.id)"
          >
            <q-tooltip>{{
              props.row.status === 'running' || props.row.status === 'awaiting_confirmation'
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
