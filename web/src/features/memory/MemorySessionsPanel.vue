// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div class="row q-col-gutter-md">
    <div class="col-12 col-lg-5">
      <q-card flat bordered class="memory-card">
        <q-card-section class="row items-center justify-between">
          <div>
            <div class="text-h6">{{ t('memory.sessions.title') }}</div>
            <div class="text-caption text-grey-7">{{ t('memory.sessions.subtitle') }}</div>
          </div>
          <q-btn
            flat
            round
            icon="refresh"
            :aria-label="t('memory.sessions.refreshAria')"
            :loading="loadingSessions"
            @click="$emit('refreshSessions')"
          />
        </q-card-section>
        <q-list separator>
          <q-item
            v-for="session in sessionRows"
            :key="session.id"
            clickable
            :active="selectedSessionId === session.id"
            active-class="memory-active-item"
            @click="$emit('update:selectedSessionId', session.id)"
          >
            <q-item-section>
              <q-item-label>{{ session.title || t('memory.sessions.untitledSession') }}</q-item-label>
              <q-item-label caption
                >{{ session.provider }} / {{ session.model || t('memory.sessions.modelNotRecorded') }}</q-item-label
              >
              <q-linear-progress
                rounded
                size="7px"
                :value="bounded(session.context_used_ratio)"
                :color="contextColor(session.context_status)"
                class="q-mt-sm"
              />
            </q-item-section>
            <q-item-section side>
              <q-chip dense :color="contextColor(session.context_status)" text-color="white">{{
                formatPercent(session.context_used_ratio)
              }}</q-chip>
            </q-item-section>
          </q-item>
        </q-list>
      </q-card>
    </div>
    <div class="col-12 col-lg-7">
      <q-card flat bordered class="memory-card q-mb-md">
        <q-card-section>
          <div class="row items-center justify-between q-gutter-sm">
            <div>
              <div class="text-h6">{{ t('memory.sessions.l0Title') }}</div>
              <div class="text-caption text-grey-7">{{ t('memory.sessions.l0Subtitle') }}</div>
            </div>
            <q-btn
              flat
              rounded
              icon="refresh"
              :label="t('memory.sessions.refresh')"
              :disable="!selectedSessionId"
              :loading="loadingSnapshots"
              @click="$emit('refreshMemory')"
            />
          </div>
        </q-card-section>
        <AppRegistryTable
          :shell="false"
          row-key="id"
          :rows="snapshotRows"
          :columns="snapshotColumns"
          :loading="loadingSnapshots"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-model="props">
            <q-td :props="props">
              <div class="app-registry-cell-primary ellipsis">
                {{ props.row.provider || '-' }} / {{ props.row.model || '-' }}
              </div>
            </q-td>
          </template>
          <template #body-cell-ratio="props">
            <q-td :props="props">
              <q-linear-progress
                rounded
                size="9px"
                :value="bounded(props.row.used_ratio)"
                :color="contextRatioColor(props.row.used_ratio)"
              />
              <div class="text-caption q-mt-xs">{{ formatPercent(props.row.used_ratio) }}</div>
            </q-td>
          </template>
          <template #body-cell-strategy="props">
            <q-td :props="props">
              <span class="app-registry-cell-sub ellipsis">{{ props.row.truncate_strategy || '—' }}</span>
            </q-td>
          </template>
          <template #body-cell-segments="props">
            <q-td :props="props">
              <q-chip dense square color="blue-grey" text-color="white"
                >{{ Object.keys(parseSegments(props.row)).length }} {{ t('memory.sessions.segmentsUnit') }}</q-chip
              >
              <q-chip v-if="parseWarnings(props.row).length" dense square color="warning" text-color="white"
                >{{ parseWarnings(props.row).length }} {{ t('memory.sessions.warningsUnit') }}</q-chip
              >
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
                  :aria-label="t('memory.sessions.viewSnapshotAria')"
                  @click="$emit('openSnapshot', props.row)"
                />
              </div>
            </q-td>
          </template>
          <template #no-data>
            <div class="full-width column items-center q-pa-lg text-grey-7">
              <q-icon name="preview" size="38px" />
              <div class="q-mt-sm">{{ t('memory.sessions.emptySnapshot') }}</div>
            </div>
          </template>
        </AppRegistryTable>
      </q-card>

      <q-card flat bordered class="memory-card">
        <q-card-section>
          <div class="text-h6">{{ t('memory.sessions.l1Title') }}</div>
          <div class="text-caption text-grey-7">{{ t('memory.sessions.l1Subtitle') }}</div>
        </q-card-section>
        <div v-if="loadingTasks" class="row justify-center q-py-md">
          <q-spinner-dots size="28px" color="primary" />
        </div>
        <template v-else>
          <q-list separator>
            <q-item v-for="task in taskRows" :key="task.id">
              <q-item-section>
                <q-item-label>{{ task.task_title || task.task_key || t('memory.sessions.defaultTask') }}</q-item-label>
                <q-item-label caption>{{ task.task_goal || task.id }}</q-item-label>
                <q-linear-progress
                  rounded
                  size="7px"
                  :value="taskBudgetRatio(task)"
                  :color="contextRatioColor(taskBudgetRatio(task))"
                  class="q-mt-sm"
                />
              </q-item-section>
              <q-item-section side>
                <q-chip dense :color="statusColor(task.status)" text-color="white">{{ taskStatusLabel(task.status) }}</q-chip>
              </q-item-section>
            </q-item>
          </q-list>
          <q-card-section v-if="!taskRows.length" class="text-center text-grey-7">
            <q-icon name="assignment" size="38px" />
            <div class="q-mt-sm">{{ t('memory.sessions.emptyTasks') }}</div>
          </q-card-section>
        </template>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { QTableProps } from 'quasar';
import AppRegistryTable from '../../components/layout/AppRegistryTable.vue';
import type { Session } from '../../features/session/types';
import type { L0AssemblySegmentsMap, L0AssemblySnapshot, L1Task } from '../../features/memory/types';
import { memorySessionStatusColor as statusColor } from './memoryTableUi';

const { t, te } = useI18n();

defineProps<{
  sessionRows: Session[];
  selectedSessionId: string | null;
  loadingSessions: boolean;
  snapshotRows: L0AssemblySnapshot[];
  snapshotColumns: QTableProps['columns'];
  loadingSnapshots: boolean;
  taskRows: L1Task[];
  loadingTasks?: boolean;
}>();

defineEmits<{
  'update:selectedSessionId': [value: string];
  refreshSessions: [];
  refreshMemory: [];
  openSnapshot: [snapshot: L0AssemblySnapshot];
}>();

function parseSegments(snapshot: L0AssemblySnapshot): L0AssemblySegmentsMap {
  return parseJSON(snapshot.segments_json, {});
}

function parseWarnings(snapshot: L0AssemblySnapshot): string[] {
  return parseJSON(snapshot.warning_codes_json, []);
}

function parseJSON<T>(raw: string, fallback: T): T {
  try {
    const parsed = JSON.parse(raw || '');
    return parsed ?? fallback;
  } catch {
    return fallback;
  }
}

function bounded(value?: number) {
  const numeric = Number(value) || 0;
  return Math.max(0, Math.min(1, numeric));
}

function taskBudgetRatio(task: L1Task) {
  if (!task.budget_tokens) return 0;
  return bounded(task.used_tokens / task.budget_tokens);
}

function contextColor(status?: string) {
  if (status === 'exceeded' || status === 'critical') return 'negative';
  if (status === 'warning') return 'warning';
  return 'positive';
}

function contextRatioColor(value?: number) {
  const ratio = bounded(value);
  if (ratio >= 0.85) return 'negative';
  if (ratio >= 0.6) return 'warning';
  return 'positive';
}

function formatPercent(value?: number) {
  return `${Math.round((Number(value) || 0) * 100)}%`;
}

function taskStatusLabel(status?: string) {
  if (!status) return '—';
  const key = `memory.sessions.taskStatus.${status}`;
  return te(key) ? t(key) : status;
}
</script>
