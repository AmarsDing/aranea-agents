<template>
  <q-dialog v-model="modelOpen" persistent>
    <q-card class="app-dialog-card app-dialog-card--xl app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between no-wrap">
        <div class="app-glass-dialog__title">执行历史</div>
        <q-btn v-close-popup flat dense round icon="close" />
      </q-card-section>
      <q-separator />

      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-glass-dialog__body">
          <div class="row q-gutter-sm q-mb-md items-center">
            <q-select
              :model-value="taskId"
              class="col"
              dense
              outlined
              clearable
              emit-value
              map-options
              label="定时任务"
              :options="taskOptions"
              @update:model-value="$emit('update:taskId', $event)"
            />
            <q-select
              :model-value="status"
              class="col"
              dense
              outlined
              clearable
              emit-value
              map-options
              label="结果"
              :options="statusOptions"
              @update:model-value="$emit('update:status', $event)"
            />
            <q-btn flat rounded no-caps icon="restart_alt" label="清空" @click="$emit('reset')" />
            <q-btn flat rounded no-caps icon="refresh" label="刷新" :loading="loading" @click="$emit('load')" />
          </div>

          <q-banner v-if="error" rounded class="bg-negative text-white q-mb-md">
            {{ error }}
            <template #action>
              <q-btn flat color="white" label="重试" @click="$emit('load')" />
            </template>
          </q-banner>

          <div v-if="!loading && pagedRuns.length === 0" class="column items-center text-center q-pa-xl">
            <q-avatar size="56px" color="grey-9" text-color="grey-5" icon="history" />
            <div class="text-subtitle1 q-mt-sm text-grey-7">暂无执行记录</div>
          </div>

          <template v-else>
            <AppRegistryTable
              :rows="pagedRuns"
              :columns="columns"
              row-key="id"
              :loading="loading"
              hide-pagination
              :pagination="{ rowsPerPage: 0 }"
            >
              <template #body-cell-task="props">
                <q-td :props="props">
                  <div class="min-width-0">
                    <div class="app-registry-cell-primary">
                      {{ props.row.task_name || taskLabel(props.row.task_id) }}
                    </div>
                    <div class="app-registry-cell-sub">{{ props.row.task_id }}</div>
                  </div>
                </q-td>
              </template>
              <template #body-cell-time="props">
                <q-td :props="props">
                  <div>开始 {{ formatCronDate(props.row.started_at || props.row.created_at) }}</div>
                  <div class="text-caption text-grey-7">结束 {{ formatCronDate(props.row.finished_at) }}</div>
                </q-td>
              </template>
              <template #body-cell-status="props">
                <q-td :props="props">
                  <AppStatusChip :status="props.row.status" />
                  <div
                    v-if="props.row.error_message"
                    class="app-registry-cell-sub ellipsis q-mt-xs"
                    :title="props.row.error_message"
                  >
                    {{ props.row.error_message }}
                  </div>
                </q-td>
              </template>
              <template #body-cell-trigger="props">
                <q-td :props="props">
                  <q-badge outline :color="triggerColor(props.row.trigger)">{{
                    triggerLabel(props.row.trigger)
                  }}</q-badge>
                </q-td>
              </template>
              <template #body-cell-run="props">
                <q-td :props="props">
                  <router-link
                    v-if="props.row.run_id"
                    :to="{ name: 'session-detail', params: { sessionId: props.row.run_id } }"
                    class="app-registry-cell-link"
                  >
                    {{ props.row.run_id.slice(0, 8) }}
                  </router-link>
                  <span v-else class="app-registry-cell-sub">—</span>
                </q-td>
              </template>
            </AppRegistryTable>

            <AppRegistryPagination
              :page="page"
              :page-size="pageSize"
              :page-max="pageMax"
              :total="total"
              :loading="loading"
              label="条记录"
              :page-size-options="[15, 30, 50]"
              @update:page="$emit('update:page', $event)"
              @update:page-size="$emit('update:pageSize', $event)"
            />
          </template>
        </q-card-section>
      </div>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { QTableColumn } from 'quasar';
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import AppRegistryPagination from '../layout/AppRegistryPagination.vue';
import AppStatusChip from '../common/AppStatusChip.vue';
import type { CronTaskRun } from '../../features/cron/types';
import { formatCronDate } from './cronTaskUtils';

defineProps<{
  loading: boolean;
  error: string;
  pagedRuns: CronTaskRun[];
  columns: QTableColumn<CronTaskRun>[];
  taskId: string;
  taskOptions: { label: string; value: string }[];
  status: string;
  statusOptions: { label: string; value: string }[];
  page: number;
  pageSize: number;
  pageMax: number;
  total: number;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
  'update:taskId': [value: string];
  'update:status': [value: string];
  'update:page': [value: number];
  'update:pageSize': [value: number];
  load: [];
  reset: [];
}>();

const modelOpen = defineModel<boolean>({ required: true });

const TRIGGER_LABELS: Record<string, string> = {
  schedule: '定时',
  manual: '手动',
  api: 'API',
};

function taskLabel(id: string) {
  return id || '—';
}

function triggerLabel(value: string) {
  return TRIGGER_LABELS[value] || value || '定时';
}

function triggerColor(value: string) {
  if (value === 'manual') return 'primary';
  if (value === 'api') return 'accent';
  return 'grey-7';
}
</script>
